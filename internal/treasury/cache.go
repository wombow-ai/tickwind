package treasury

import (
	"sync"
	"time"
)

// Cache holds the latest fetched yield Curve plus the time it was last updated,
// behind a single RWMutex (the atomic-snapshot pattern other tickwind caches
// use): readers always get a self-consistent copy and never block a writer for
// long. A failed refresh simply never calls Set, so the last good curve stands.
// The zero value is ready to use; Latest reports ok=false until the first Set.
type Cache struct {
	mu        sync.RWMutex
	curve     Curve
	has       bool
	updatedAt time.Time
}

// NewCache returns an empty, ready-to-use Cache.
func NewCache() *Cache {
	return &Cache{}
}

// Set records curve as the latest snapshot and stamps the update time.
func (c *Cache) Set(curve Curve) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.curve = curve
	c.has = true
	c.updatedAt = time.Now().UTC()
}

// Latest returns the most recent Curve and true, or the zero Curve and false
// before any Set.
func (c *Cache) Latest() (Curve, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.curve, c.has
}

// UpdatedAt returns the time of the last Set, or the zero time before any Set.
func (c *Cache) UpdatedAt() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.updatedAt
}
