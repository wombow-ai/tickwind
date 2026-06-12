package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/wombow-ai/tickwind/internal/alpaca"
	"github.com/wombow-ai/tickwind/internal/market"
	"github.com/wombow-ai/tickwind/internal/store"
)

// PricePoller periodically fetches the latest all-session price for each ticker
// (from the TickerSource), writes it to the store, and publishes it to
// subscribers. It runs only when Alpaca credentials are configured.
type PricePoller struct {
	store    store.Store
	client   *alpaca.Client
	tickers  TickerSource
	every    time.Duration
	publish  func(store.Quote) // optional; broadcasts to live subscribers
	log      *slog.Logger
	adapters map[market.Market]MarketAdapter // per-market dispatch; US = Alpaca
	fb       ConsolidatedQuoter              // optional pre/post-aware freshness fallback
}

// NewPricePoller builds a poller. publish may be nil to disable broadcasting.
func NewPricePoller(st store.Store, client *alpaca.Client, tickers TickerSource, every time.Duration, publish func(store.Quote), log *slog.Logger) *PricePoller {
	return &PricePoller{store: st, client: client, tickers: tickers, every: every, publish: publish, log: log}
}

// SetAdapters registers per-market price adapters keyed by Market; bare (US)
// tickers keep using Alpaca.
func (p *PricePoller) SetAdapters(a map[market.Market]MarketAdapter) { p.adapters = a }

// SetConsolidatedFallback registers a pre/post-aware fallback (e.g.
// yahoo.Consolidated). When a polled US ticker's IEX trade is stale — common
// for thin names after hours — its extended-hours price is overlaid, mirroring
// the on-demand BarCache path so watchlisted thin names don't freeze at the
// regular close in pre/post-market. nil disables it.
func (p *PricePoller) SetConsolidatedFallback(fb ConsolidatedQuoter) { p.fb = fb }

// Run blocks until ctx is cancelled, polling every p.every.
func (p *PricePoller) Run(ctx context.Context) {
	p.poll(ctx)
	t := time.NewTicker(p.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.poll(ctx)
		}
	}
}

func (p *PricePoller) poll(ctx context.Context) {
	for _, ticker := range p.tickers(ctx) {
		if a := p.adapters[market.Of(ticker)]; a != nil { // non-US (e.g. .TW EOD)
			q, ok, err := a.Quote(ctx, ticker)
			if err != nil {
				p.log.Warn("intl price poll failed", "ticker", ticker, "market", a.Market(), "err", err)
				continue
			}
			if ok {
				p.save(ctx, q)
			}
			continue
		}
		q, err := p.client.LatestQuote(ctx, ticker)
		if err != nil {
			p.log.Warn("price poll failed", "ticker", ticker, "err", err)
			continue
		}
		// Freshness fallback: a stale/absent IEX trade (thin names go minutes–
		// hours between IEX prints, esp. in extended hours) is overlaid with the
		// consolidated pre/post-market last trade — same logic as BarCache.
		if p.fb != nil && (q.Price == 0 || time.Since(q.At) > staleQuoteAfter) {
			if pr, pc, at, ok, ferr := p.fb.Quote(ctx, ticker); ferr == nil && ok && at.After(q.At) {
				q.Ticker = ticker
				q = overlayConsolidated(q, pr, pc, at, p.client.SessionAt(at))
			}
		}
		p.save(ctx, q)
		p.log.Debug("price", "ticker", ticker, "price", q.Price, "session", q.Session)
	}
}

// save upserts a quote and broadcasts it to live subscribers.
func (p *PricePoller) save(ctx context.Context, q store.Quote) {
	if err := p.store.UpsertQuote(ctx, q); err != nil {
		p.log.Warn("price upsert failed", "ticker", q.Ticker, "err", err)
		return
	}
	if p.publish != nil {
		p.publish(q)
	}
}
