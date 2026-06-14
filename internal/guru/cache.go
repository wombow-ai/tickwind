package guru

import (
	"sync/atomic"
	"time"
)

// Cache holds the latest Guru-watch rail for lock-free reads (atomic pointer
// swap), shared between the ingestor (writer) and the API (reader). It also
// stamps the time of the last replace so the API can report rail freshness.
type Cache struct {
	v  atomic.Value // []Item
	at atomic.Value // time.Time of the last Set
}

// NewCache returns a Cache seeded with an empty rail.
func NewCache() *Cache {
	c := &Cache{}
	c.v.Store([]Item{})
	c.at.Store(time.Time{})
	return c
}

// Set replaces the current rail and records the update time.
func (c *Cache) Set(items []Item) {
	c.v.Store(items)
	c.at.Store(time.Now())
}

// Get returns the current rail (newest first).
func (c *Cache) Get() []Item { return c.v.Load().([]Item) }

// UpdatedAt returns the time of the last Set, or the zero time before any Set.
func (c *Cache) UpdatedAt() time.Time { return c.at.Load().(time.Time) }
