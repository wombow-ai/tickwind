package congress

import (
	"sync/atomic"
	"time"
)

// Cache holds the latest snapshot of recent Periodic Transaction Reports,
// swapped atomically by the ingestor. Memory-only + rebuildable (the House Clerk
// index is cheap to re-fetch), mirroring internal/universe's atomic cache —
// lock-free reads, no per-refresh DB writes.
type Cache struct {
	v atomic.Value // *snapshot
}

type snapshot struct {
	filings   []Filing
	updatedAt time.Time
}

// NewCache returns an empty cache (Len 0 until the first refresh).
func NewCache() *Cache { return &Cache{} }

// Set replaces the snapshot with a fresh filings slice (expected newest first).
func (c *Cache) Set(filings []Filing) {
	c.v.Store(&snapshot{filings: filings, updatedAt: time.Now().UTC()})
}

func (c *Cache) snap() *snapshot {
	s, _ := c.v.Load().(*snapshot)
	return s
}

// Get returns the cached filings (newest first), or nil when never refreshed.
func (c *Cache) Get() []Filing {
	if s := c.snap(); s != nil {
		return s.filings
	}
	return nil
}

// Len is the number of cached filings (0 for an empty cache).
func (c *Cache) Len() int {
	if s := c.snap(); s != nil {
		return len(s.filings)
	}
	return 0
}

// UpdatedAt is when the snapshot was last refreshed (zero if never).
func (c *Cache) UpdatedAt() time.Time {
	if s := c.snap(); s != nil {
		return s.updatedAt
	}
	return time.Time{}
}
