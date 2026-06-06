package guru

import "sync/atomic"

// Cache holds the latest Guru-watch rail for lock-free reads (atomic pointer
// swap), shared between the ingestor (writer) and the API (reader).
type Cache struct {
	v atomic.Value
}

// NewCache returns a Cache seeded with an empty rail.
func NewCache() *Cache {
	c := &Cache{}
	c.v.Store([]Item{})
	return c
}

// Set replaces the current rail.
func (c *Cache) Set(items []Item) { c.v.Store(items) }

// Get returns the current rail (newest first).
func (c *Cache) Get() []Item { return c.v.Load().([]Item) }
