package opportunity

import "sync/atomic"

// Cache holds the latest Opportunity board for lock-free reads (atomic pointer
// swap), shared between the scheduler (writer) and the API (reader).
type Cache struct {
	v atomic.Value
}

// NewCache returns a Cache seeded with an empty board.
func NewCache() *Cache {
	c := &Cache{}
	c.v.Store([]Stock{})
	return c
}

// Set replaces the current board.
func (c *Cache) Set(board []Stock) { c.v.Store(board) }

// Get returns the current board (top first).
func (c *Cache) Get() []Stock { return c.v.Load().([]Stock) }
