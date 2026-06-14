package sentiment

import (
	"sync"
	"time"
)

// Point is one dated sample of the headline score, used for the history curve
// and cold-start backfill. Date is a calendar day in "2006-01-02" form.
type Point struct {
	// Date is the calendar day of the sample, formatted "2006-01-02".
	Date string
	// Score is the headline Fear & Greed score for that day, 0–100.
	Score int
}

// Cache holds the latest computed Result and a history of daily Points behind a
// single mutex, following the atomic-snapshot pattern used by other tickwind
// caches: readers always get a self-consistent copy and never block writers for
// long. The zero value is ready to use.
type Cache struct {
	mu        sync.RWMutex
	latest    Result
	hasLatest bool
	history   []Point
	updatedAt time.Time
}

// NewCache returns an empty, ready-to-use Cache. The zero Cache is equally
// valid; NewCache exists for call-site clarity.
func NewCache() *Cache {
	return &Cache{}
}

// Set records r as the latest Result and appends (or replaces) the history
// Point for date, stamping the update time. If the most recent history Point is
// already for date, it is overwritten so repeated same-day updates collapse to
// one point per day; otherwise a new point is appended. An empty date updates
// the latest Result without touching history.
func (c *Cache) Set(r Result, date string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.latest = r
	c.hasLatest = true
	c.updatedAt = time.Now()
	if date == "" {
		return
	}
	if n := len(c.history); n > 0 && c.history[n-1].Date == date {
		c.history[n-1].Score = r.Score
		return
	}
	c.history = append(c.history, Point{Date: date, Score: r.Score})
}

// Seed initializes the in-memory history from a chronological (oldest→newest)
// slice of Points, replacing whatever history was there. It is meant to be called
// once at startup — after loading the durable history from the store and before
// the ingestor's first Set — so History() returns the accumulated series
// immediately. Points is copied, so the caller may retain and mutate it. A nil or
// empty slice clears the history.
func (c *Cache) Seed(points []Point) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(points) == 0 {
		c.history = nil
		return
	}
	c.history = make([]Point, len(points))
	copy(c.history, points)
}

// Latest returns the most recent Result and true, or the zero Result and false
// before any Set.
func (c *Cache) Latest() (Result, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latest, c.hasLatest
}

// History returns a copy of the recorded daily Points in chronological order,
// or nil if none have been recorded. The slice is safe to retain and mutate.
func (c *Cache) History() []Point {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.history) == 0 {
		return nil
	}
	out := make([]Point, len(c.history))
	copy(out, c.history)
	return out
}

// UpdatedAt returns the time of the last Set, or the zero time before any Set.
func (c *Cache) UpdatedAt() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.updatedAt
}
