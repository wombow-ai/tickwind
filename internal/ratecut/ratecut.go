// Package ratecut aggregates Federal Reserve interest-rate (rate-cut) odds from
// two keyless public prediction-market APIs — Kalshi and Polymarket — into a
// single normalized shape for the macro "rate decision" view.
//
// Each source exposes a Fetch(ctx) (Market, error) client. A Market is one
// prediction question (e.g. "Fed funds rate after the next FOMC meeting?" or
// "How many Fed rate cuts in 2026?") with a set of mutually-exclusive Outcomes,
// each carrying a 0–1 implied probability. The two sources price slightly
// different questions (Kalshi: the post-meeting rate level; Polymarket: the
// count of 25bp cuts on the year), so they are surfaced side by side rather than
// merged.
//
// An Aggregator drives both clients on a schedule and stores the latest result
// per source in an atomic Cache; one source failing never clears the other's
// last-good snapshot. All parsing is defensive: a shape change yields an error
// (never a panic) and individual malformed/closed outcomes are skipped.
//
// Scope: macro interest-rate markets only. The clients target the Fed funds /
// rate-cut series specifically and never touch election or other politically
// contentious markets.
package ratecut

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Outcome is a single mutually-exclusive result within a Market with its
// market-implied probability.
type Outcome struct {
	// Label is the human-readable bucket, e.g. "3.50%–3.75%", "1 (25 bps)" or
	// "25 bps decrease".
	Label string `json:"label"`
	// Probability is the implied chance of this outcome, normalized to 0–1.
	Probability float64 `json:"probability"`
}

// Market is one normalized prediction-market question with its outcomes.
type Market struct {
	// Source identifies the provider: "kalshi" or "polymarket".
	Source string `json:"source"`
	// Question is the market's headline question.
	Question string `json:"question"`
	// AsOf is the time the snapshot was fetched, RFC3339 UTC.
	AsOf string `json:"as_of"`
	// Outcomes are the mutually-exclusive results, highest probability first.
	Outcomes []Outcome `json:"outcomes"`
	// URL links to the source market page (best effort; may be empty).
	URL string `json:"url,omitempty"`
}

// Fetcher is the common contract implemented by both source clients.
type Fetcher interface {
	// Fetch returns the latest normalized Market for this source.
	Fetch(ctx context.Context) (Market, error)
	// Source is the provider identifier ("kalshi" / "polymarket").
	Source() string
}

// sortOutcomes orders outcomes by descending probability, breaking ties by label
// for deterministic output. It sorts in place and returns the slice.
func sortOutcomes(o []Outcome) []Outcome {
	sort.SliceStable(o, func(i, j int) bool {
		if o[i].Probability != o[j].Probability {
			return o[i].Probability > o[j].Probability
		}
		return o[i].Label < o[j].Label
	})
	return o
}

// Cache holds the most recent Market per source, updated atomically. Reads are
// lock-free-ish via a small RWMutex over a tiny map; a fetch error for one
// source leaves that source's previous snapshot untouched.
type Cache struct {
	mu      sync.RWMutex
	markets map[string]Market // keyed by Source
	updated time.Time
}

// NewCache returns an empty Cache (Get returns nothing until the first Set).
func NewCache() *Cache {
	return &Cache{markets: make(map[string]Market)}
}

// Set stores (replacing) the snapshot for m.Source and bumps the update time.
func (c *Cache) Set(m Market) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.markets == nil {
		c.markets = make(map[string]Market)
	}
	c.markets[m.Source] = m
	c.updated = time.Now().UTC()
}

// Get returns all cached markets sorted by source name for stable output. The
// returned slice is freshly allocated and safe for the caller to mutate.
func (c *Cache) Get() []Market {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Market, 0, len(c.markets))
	for _, m := range c.markets {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Source < out[j].Source })
	return out
}

// Source returns the cached Market for a single provider and whether it exists.
func (c *Cache) Source(source string) (Market, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m, ok := c.markets[source]
	return m, ok
}

// Len is the number of sources currently cached.
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.markets)
}

// UpdatedAt is when any source was last Set (zero before the first Set).
func (c *Cache) UpdatedAt() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.updated
}

// Aggregator drives a set of Fetchers and writes their results into a Cache. It
// is fault-tolerant: one source erroring neither aborts the others nor clears
// the failing source's last-good snapshot.
type Aggregator struct {
	cache    *Cache
	fetchers []Fetcher
}

// NewAggregator builds an Aggregator over the given fetchers (typically a Kalshi
// and a Polymarket client) backed by a fresh Cache.
func NewAggregator(fetchers ...Fetcher) *Aggregator {
	return &Aggregator{cache: NewCache(), fetchers: fetchers}
}

// Cache exposes the underlying snapshot store for read access (e.g. by an API
// handler).
func (a *Aggregator) Cache() *Cache { return a.cache }

// Refresh fetches every source once, storing each success in the cache and
// collecting per-source errors. It returns a map of source→error for the
// sources that failed (nil/empty when all succeeded); successful sources are
// always cached regardless of whether siblings failed.
func (a *Aggregator) Refresh(ctx context.Context) map[string]error {
	type result struct {
		source string
		market Market
		err    error
	}
	results := make(chan result, len(a.fetchers))
	var wg sync.WaitGroup
	for _, f := range a.fetchers {
		wg.Add(1)
		go func(f Fetcher) {
			defer wg.Done()
			m, err := f.Fetch(ctx)
			results <- result{source: f.Source(), market: m, err: err}
		}(f)
	}
	wg.Wait()
	close(results)

	var errs map[string]error
	for r := range results {
		if r.err != nil {
			if errs == nil {
				errs = make(map[string]error)
			}
			errs[r.source] = r.err
			continue
		}
		a.cache.Set(r.market)
	}
	return errs
}
