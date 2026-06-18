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
}

// NewPricePoller builds a poller. publish may be nil to disable broadcasting.
func NewPricePoller(st store.Store, client *alpaca.Client, tickers TickerSource, every time.Duration, publish func(store.Quote), log *slog.Logger) *PricePoller {
	return &PricePoller{store: st, client: client, tickers: tickers, every: every, publish: publish, log: log}
}

// SetAdapters registers per-market price adapters keyed by Market; bare (US)
// tickers keep using Alpaca.
func (p *PricePoller) SetAdapters(a map[market.Market]MarketAdapter) { p.adapters = a }

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
	// Split the tracked set: non-US markets dispatch to their per-market adapter
	// (e.g. .TW EOD); US tickers are priced together in a few BULK snapshot requests
	// rather than one serial REST call each — so a ~200-ticker cycle takes ~1-2s
	// instead of tens of seconds and live prices refresh at the configured cadence.
	var usSyms []string
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
		usSyms = append(usSyms, ticker)
	}
	if len(usSyms) == 0 {
		return
	}
	quotes, err := p.client.SnapshotQuotesLive(ctx, usSyms)
	if err != nil && len(quotes) == 0 {
		p.log.Warn("price poll (bulk) failed", "tickers", len(usSyms), "err", err)
		return
	}
	for _, q := range quotes {
		p.save(ctx, q)
	}
	p.log.Debug("price poll", "us", len(usSyms), "priced", len(quotes))
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
