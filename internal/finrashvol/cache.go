package finrashvol

import (
	"sort"
	"sync/atomic"
	"time"
)

// DefaultHistoryDays is the number of trailing trading days kept per symbol for
// the 7/30-day short-pressure curve on the stock page.
const DefaultHistoryDays = 40

// Cache is a concurrency-safe in-memory snapshot of the latest trading day's
// short volume plus a short rolling history per symbol, shared between the
// ingest writer (Set) and API readers (Latest/History/AsOf/Len). Reads are
// lock-free via an atomic snapshot pointer swap; each Set builds a fresh
// snapshot, so readers never observe a half-applied update.
type Cache struct {
	v          atomic.Value // *snapshot
	historyDay int          // trailing trading days to retain per symbol
}

// snapshot is one immutable view: the latest day's per-symbol map, the latest
// date, and a per-symbol rolling history (oldest first).
type snapshot struct {
	latest    map[string]ShortVol
	history   map[string][]ShortVol
	asOf      string    // latest day's report date, YYYY-MM-DD
	updatedAt time.Time // wall-clock of the last Set
}

// NewCache returns an empty cache retaining DefaultHistoryDays of per-symbol
// history. Reads return zero values until the first Set.
func NewCache() *Cache { return NewCacheWithHistory(DefaultHistoryDays) }

// NewCacheWithHistory returns an empty cache retaining the given number of
// trailing trading days per symbol (values < 1 fall back to DefaultHistoryDays).
func NewCacheWithHistory(days int) *Cache {
	if days < 1 {
		days = DefaultHistoryDays
	}
	c := &Cache{historyDay: days}
	c.v.Store(&snapshot{
		latest:  map[string]ShortVol{},
		history: map[string][]ShortVol{},
	})
	return c
}

func (c *Cache) snap() *snapshot { return c.v.Load().(*snapshot) }

// Set installs one trading day's rows as the new latest day and appends them to
// the per-symbol rolling history (trimmed to the retention window). Rows for a
// date already present as the latest day replace it rather than duplicating, so
// re-ingesting the same day is idempotent. The newest distinct date by string
// order becomes AsOf. Symbols absent from this day keep their prior history but
// drop out of Latest.
func (c *Cache) Set(day []ShortVol) {
	prev := c.snap()
	asOf := prev.asOf
	for _, r := range day {
		if r.Date > asOf {
			asOf = r.Date
		}
	}

	latest := make(map[string]ShortVol, len(day))
	history := make(map[string][]ShortVol, len(prev.history))
	for sym, h := range prev.history {
		// Drop any prior entry for the incoming date (idempotent re-ingest),
		// then keep the rest; the new point is appended below.
		kept := h[:0:0]
		for _, e := range h {
			if e.Date != asOf {
				kept = append(kept, e)
			}
		}
		history[sym] = kept
	}
	for _, r := range day {
		latest[r.Symbol] = r
		history[r.Symbol] = append(history[r.Symbol], r)
	}
	// Sort each symbol's history oldest-first and trim to the window.
	for sym, h := range history {
		sort.Slice(h, func(i, j int) bool { return h[i].Date < h[j].Date })
		if len(h) > c.historyDay {
			h = h[len(h)-c.historyDay:]
		}
		history[sym] = h
	}

	c.v.Store(&snapshot{
		latest:    latest,
		history:   history,
		asOf:      asOf,
		updatedAt: time.Now().UTC(),
	})
}

// Latest returns the latest day's short volume for sym and whether it was
// present in the most recent file.
func (c *Cache) Latest(sym string) (ShortVol, bool) {
	v, ok := c.snap().latest[sym]
	return v, ok
}

// History returns sym's retained short-volume history, oldest first (a copy the
// caller may freely use). It is nil when the symbol has never been seen.
func (c *Cache) History(sym string) []ShortVol {
	h := c.snap().history[sym]
	if len(h) == 0 {
		return nil
	}
	out := make([]ShortVol, len(h))
	copy(out, h)
	return out
}

// AsOf returns the report date of the latest day held (YYYY-MM-DD), or "" when
// the cache has never been set.
func (c *Cache) AsOf() string { return c.snap().asOf }

// UpdatedAt returns the wall-clock time of the last Set (zero when never set).
func (c *Cache) UpdatedAt() time.Time { return c.snap().updatedAt }

// Len returns the number of symbols in the latest day.
func (c *Cache) Len() int { return len(c.snap().latest) }

// Top returns the latest day's symbols ranked by ShortPct descending, capped at
// n (n <= 0 returns all). Only rows with TotalVolume >= minTotalVolume are
// considered, which filters out thin names whose short percentage is noisy.
// This backs a site-side "most-shorted today" ranking; do not expose the raw
// per-symbol rows in bulk (FINRA display-only terms).
func (c *Cache) Top(n int, minTotalVolume int64) []ShortVol {
	s := c.snap()
	out := make([]ShortVol, 0, len(s.latest))
	for _, v := range s.latest {
		if v.TotalVolume >= minTotalVolume {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ShortPct != out[j].ShortPct {
			return out[i].ShortPct > out[j].ShortPct
		}
		return out[i].Symbol < out[j].Symbol // stable tiebreak
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}
