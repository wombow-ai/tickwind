// Package universe holds a periodically-refreshed snapshot of the whole US stock
// universe's latest quotes (price + change reference), so any stock — even one
// nobody has visited — has an instant price without an on-demand fetch, and the
// screener has broad data to filter. Lock-free reads via an atomic snapshot,
// mirroring internal/opportunity's in-memory cache (rebuildable, off the DB).
package universe

import (
	"sort"
	"sync/atomic"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

// Snapshot is an immutable universe snapshot (upper ticker → latest quote).
type Snapshot struct {
	Quotes    map[string]store.Quote
	UpdatedAt time.Time
}

// Cache holds the latest Snapshot, swapped atomically by the ingestor.
type Cache struct {
	v atomic.Value // *Snapshot
}

// NewCache returns an empty cache (Len 0 until the first sweep).
func NewCache() *Cache { return &Cache{} }

// Set replaces the snapshot with a fresh quote map.
func (c *Cache) Set(quotes map[string]store.Quote) {
	c.v.Store(&Snapshot{Quotes: quotes, UpdatedAt: time.Now().UTC()})
}

func (c *Cache) snap() *Snapshot {
	s, _ := c.v.Load().(*Snapshot)
	return s
}

// Get returns the cached quote for an (already upper-cased) ticker.
func (c *Cache) Get(ticker string) (store.Quote, bool) {
	s := c.snap()
	if s == nil {
		return store.Quote{}, false
	}
	q, ok := s.Quotes[ticker]
	return q, ok
}

// Snapshot returns the full ticker→quote map (the screener iterates it). The map
// is swapped wholesale by the ingestor and never mutated in place, so the
// returned reference is safe for concurrent reads. Returns an empty (non-nil) map
// when never swept.
func (c *Cache) Snapshot() map[string]store.Quote {
	if s := c.snap(); s != nil && s.Quotes != nil {
		return s.Quotes
	}
	return map[string]store.Quote{}
}

// Tickers returns the sorted list of quote-bearing symbols in the snapshot —
// every ticker the sweep got a usable price for (the ingestor only stores
// price>0 quotes, so this is exactly the screener/Len universe). Returns a
// non-nil (possibly empty) slice; safe for concurrent reads (the underlying map
// is swapped wholesale, never mutated). Used by the pSEO sitemap to emit a
// /stock/{t} page per quote-bearing name.
func (c *Cache) Tickers() []string {
	s := c.snap()
	if s == nil || len(s.Quotes) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(s.Quotes))
	for tk := range s.Quotes {
		out = append(out, tk)
	}
	sort.Strings(out)
	return out
}

// Len is the number of pre-cached tickers (0 for an empty cache).
func (c *Cache) Len() int {
	if s := c.snap(); s != nil {
		return len(s.Quotes)
	}
	return 0
}

// UpdatedAt is when the snapshot was last refreshed (zero if never).
func (c *Cache) UpdatedAt() time.Time {
	if s := c.snap(); s != nil {
		return s.UpdatedAt
	}
	return time.Time{}
}
