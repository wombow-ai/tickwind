package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/symbols"
	"github.com/wombow-ai/tickwind/internal/universe"
)

// QuoteSnapshotter returns full quotes (price + change reference) per symbol in
// bulk (satisfied by *alpaca.Client).
type QuoteSnapshotter interface {
	SnapshotQuotes(ctx context.Context, symbols []string) (map[string]store.Quote, error)
}

// UniverseIngestor sweeps the whole US symbol universe for its latest quote into
// an in-memory Cache on a slow cadence, so any stock — even one nobody has
// visited — has an instant price without an on-demand Alpaca call (and the
// screener has broad data). Runs in its own goroutine, off the request path.
// Memory-only + rebuildable (one sweep ≈ a few hundred KB), like the Opportunity
// and Hot caches — deliberately NOT a per-sweep DB write storm.
type UniverseIngestor struct {
	prices  QuoteSnapshotter
	symbols *symbols.Cache
	cache   *universe.Cache
	every   time.Duration
	log     *slog.Logger
}

// NewUniverseIngestor builds the ingestor; every is the sweep cadence.
func NewUniverseIngestor(prices QuoteSnapshotter, syms *symbols.Cache, cache *universe.Cache, every time.Duration, log *slog.Logger) *UniverseIngestor {
	return &UniverseIngestor{prices: prices, symbols: syms, cache: cache, every: every, log: log}
}

// Run blocks until ctx is cancelled.
func (u *UniverseIngestor) Run(ctx context.Context) {
	// The symbol directory ingests asynchronously on startup; wait briefly for it
	// so the first sweep has tickers (then settle into the cadence).
	for i := 0; i < 15 && u.symbols.Get().Len() == 0; i++ {
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
	u.sweep(ctx)

	t := time.NewTicker(u.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			u.sweep(ctx)
		}
	}
}

func (u *UniverseIngestor) sweep(ctx context.Context) {
	tickers := u.symbols.Get().USTickers()
	if len(tickers) == 0 {
		return // directory not loaded yet
	}
	quotes, err := u.prices.SnapshotQuotes(ctx, tickers)
	if err != nil {
		u.log.Warn("universe: sweep failed", "err", err)
		return
	}
	u.cache.Set(quotes)
	u.log.Info("universe: swept", "symbols", len(tickers), "priced", len(quotes))
}
