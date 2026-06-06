package ingest

import (
	"context"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/alpaca"
)

// BarCache serves recent daily closing prices for sparklines, caching each
// ticker's series for a TTL so repeated page views don't re-hit Alpaca (daily
// bars only change once per day). It satisfies api.BarSource.
type BarCache struct {
	client *alpaca.Client
	limit  int
	ttl    time.Duration

	mu      sync.Mutex
	entries map[string]barEntry
}

type barEntry struct {
	closes []float64
	at     time.Time
}

// NewBarCache builds a cache fetching `limit` daily closes per ticker, holding
// each series for ttl.
func NewBarCache(client *alpaca.Client, limit int, ttl time.Duration) *BarCache {
	return &BarCache{
		client:  client,
		limit:   limit,
		ttl:     ttl,
		entries: make(map[string]barEntry),
	}
}

// DailyBars returns the cached series for ticker, fetching and caching it when
// missing or stale.
func (b *BarCache) DailyBars(ctx context.Context, ticker string) ([]float64, error) {
	b.mu.Lock()
	e, ok := b.entries[ticker]
	b.mu.Unlock()
	if ok && time.Since(e.at) < b.ttl {
		return e.closes, nil
	}

	closes, err := b.client.DailyBars(ctx, ticker, b.limit)
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	b.entries[ticker] = barEntry{closes: closes, at: time.Now()}
	b.mu.Unlock()
	return closes, nil
}
