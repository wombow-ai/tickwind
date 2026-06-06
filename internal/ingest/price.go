package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/wombow-ai/tickwind/internal/alpaca"
	"github.com/wombow-ai/tickwind/internal/store"
)

// PricePoller periodically fetches the latest all-session price for each ticker
// (from the TickerSource), writes it to the store, and publishes it to
// subscribers. It runs only when Alpaca credentials are configured.
type PricePoller struct {
	store   store.Store
	client  *alpaca.Client
	tickers TickerSource
	every   time.Duration
	publish func(store.Quote) // optional; broadcasts to live subscribers
	log     *slog.Logger
}

// NewPricePoller builds a poller. publish may be nil to disable broadcasting.
func NewPricePoller(st store.Store, client *alpaca.Client, tickers TickerSource, every time.Duration, publish func(store.Quote), log *slog.Logger) *PricePoller {
	return &PricePoller{store: st, client: client, tickers: tickers, every: every, publish: publish, log: log}
}

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
		q, err := p.client.LatestQuote(ctx, ticker)
		if err != nil {
			p.log.Warn("price poll failed", "ticker", ticker, "err", err)
			continue
		}
		if err := p.store.UpsertQuote(ctx, q); err != nil {
			p.log.Warn("price upsert failed", "ticker", ticker, "err", err)
			continue
		}
		if p.publish != nil {
			p.publish(q)
		}
		p.log.Debug("price", "ticker", ticker, "price", q.Price, "session", q.Session)
	}
}
