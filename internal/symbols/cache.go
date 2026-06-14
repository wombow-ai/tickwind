package symbols

import "sync/atomic"

// Cache holds the current Index for lock-free reads, swapped atomically by the
// ingestor. Shared between the ingestor (writer) and the API (reader).
type Cache struct {
	v atomic.Value // holds *Index (may be a typed nil before the first load)
}

// NewCache returns a Cache with no directory loaded yet (searches return empty).
func NewCache() *Cache {
	c := &Cache{}
	c.v.Store((*Index)(nil))
	return c
}

// Set replaces the current Index.
func (c *Cache) Set(idx *Index) { c.v.Store(idx) }

// Get returns the current Index (nil until the first successful load).
func (c *Cache) Get() *Index {
	idx, _ := c.v.Load().(*Index)
	return idx
}

// Search runs against the current snapshot (empty while unloaded).
func (c *Cache) Search(q string, limit int) []Symbol { return c.Get().Search(q, limit) }

// ByCIK resolves a SEC CIK against the current snapshot (none while unloaded).
func (c *Cache) ByCIK(cik int) (Symbol, bool) { return c.Get().ByCIK(cik) }

// AllUSTickers returns every US ticker in the current snapshot (nil while
// unloaded), so the API can enumerate the full universe for the pSEO sitemap.
func (c *Cache) AllUSTickers() []string { return c.Get().USTickers() }
