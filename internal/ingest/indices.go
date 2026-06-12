package ingest

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/yahoo"
)

// indexSymbols are the homepage majors, in display order. These are the real
// indices (Alpaca's free feed has no index symbols and Finnhub gates them
// behind a paid plan, so they come from Yahoo's chart endpoint — same
// owner-authorized gray source as the HK quotes, display-only).
var indexSymbols = []struct{ symbol, name string }{
	{"^GSPC", "S&P 500"},
	{"^DJI", "Dow Jones"},
	{"^IXIC", "Nasdaq"},
	{"^HSI", "Hang Seng"}, // Hong Kong (HKD, HK hours) — trades while the US is closed
}

// IndexQuoter is the slice of *yahoo.Client the indices cache uses.
type IndexQuoter interface {
	Quote(ctx context.Context, symbol string) (yahoo.Quote, bool, error)
}

// IndicesCache holds the latest major-index levels, swapped atomically by Run's
// periodic sweeps. A failed fetch keeps the symbol's previous level (better a
// minute-stale index than a hole in the strip).
type IndicesCache struct {
	src   IndexQuoter
	every time.Duration
	log   *slog.Logger

	mu     sync.RWMutex
	quotes []store.IndexQuote
}

// NewIndicesCache builds the cache; call Run to start refreshing.
func NewIndicesCache(src IndexQuoter, every time.Duration, log *slog.Logger) *IndicesCache {
	if log == nil {
		log = slog.Default()
	}
	return &IndicesCache{src: src, every: every, log: log}
}

// Indices returns the latest levels (display order), nil before the first sweep.
func (c *IndicesCache) Indices() []store.IndexQuote {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.quotes == nil {
		return nil
	}
	out := make([]store.IndexQuote, len(c.quotes))
	copy(out, c.quotes)
	return out
}

// Run sweeps immediately and then on every tick until ctx is cancelled.
func (c *IndicesCache) Run(ctx context.Context) {
	c.sweep(ctx)
	t := time.NewTicker(c.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.sweep(ctx)
		}
	}
}

// sweep refreshes every index, falling back to the cached level per symbol on
// fetch errors so transient Yahoo hiccups never blank the strip.
func (c *IndicesCache) sweep(ctx context.Context) {
	prev := make(map[string]store.IndexQuote, len(indexSymbols))
	for _, q := range c.Indices() {
		prev[q.Symbol] = q
	}
	out := make([]store.IndexQuote, 0, len(indexSymbols))
	for _, s := range indexSymbols {
		yq, ok, err := c.src.Quote(ctx, s.symbol)
		if err != nil || !ok || yq.Price <= 0 {
			if old, has := prev[s.symbol]; has {
				out = append(out, old)
			}
			if err != nil {
				c.log.Warn("index quote failed", "symbol", s.symbol, "err", err)
			}
			continue
		}
		out = append(out, store.IndexQuote{
			Symbol:    s.symbol,
			Name:      s.name,
			Price:     yq.Price,
			PrevClose: yq.PrevClose,
			Source:    "yahoo",
			At:        yq.At,
		})
	}
	if len(out) == 0 {
		return // total outage: keep whatever we had
	}
	c.mu.Lock()
	c.quotes = out
	c.mu.Unlock()
}
