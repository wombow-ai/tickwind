// Package institutional holds a periodically-refreshed snapshot of recent
// Schedule 13D/13G beneficial-ownership filings (institutional / activist stakes)
// from SEC EDGAR. Lock-free reads via an atomic snapshot, mirroring
// internal/congress — memory-only + rebuildable, off the request path.
package institutional

import (
	"sync/atomic"
	"time"

	"github.com/wombow-ai/tickwind/internal/sec"
)

// Cache holds the latest 13D/13G snapshot, swapped atomically by the ingestor.
type Cache struct {
	v atomic.Value // *snapshot
}

type snapshot struct {
	filings   []sec.OwnershipRef
	updatedAt time.Time
}

// NewCache returns an empty cache (Len 0 until the first refresh).
func NewCache() *Cache { return &Cache{} }

// Set replaces the snapshot with a fresh filings slice (expected newest first).
func (c *Cache) Set(filings []sec.OwnershipRef) {
	c.v.Store(&snapshot{filings: filings, updatedAt: time.Now().UTC()})
}

func (c *Cache) snap() *snapshot {
	s, _ := c.v.Load().(*snapshot)
	return s
}

// Get returns the cached filings (newest first), or nil when never refreshed.
func (c *Cache) Get() []sec.OwnershipRef {
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
