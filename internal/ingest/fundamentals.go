package ingest

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/edgar"
)

// Fundamentals TTLs: XBRL figures change only on a new filing, so a long
// positive TTL is fine; failures (non-US tickers absent from EDGAR) cache for a
// shorter window so we don't re-hit the SEC companyfacts API on every view.
const (
	fundamentalsTTL    = 24 * time.Hour
	fundamentalsNegTTL = 1 * time.Hour
)

// FundamentalsCache serves XBRL-derived fundamentals per ticker, caching each
// result (and failures) so repeated stock-page views don't re-hit the SEC
// companyfacts API. Satisfies api.FundamentalsSource.
type FundamentalsCache struct {
	client *edgar.Client

	mu    sync.Mutex
	cache map[string]fundEntry
}

type fundEntry struct {
	f   edgar.Fundamentals
	err error
	at  time.Time
}

// NewFundamentalsCache builds a cache over an EDGAR client.
func NewFundamentalsCache(client *edgar.Client) *FundamentalsCache {
	return &FundamentalsCache{client: client, cache: make(map[string]fundEntry)}
}

// Fundamentals returns the cached figures for ticker, fetching from EDGAR when
// missing or stale (positive results held fundamentalsTTL, errors negTTL).
func (c *FundamentalsCache) Fundamentals(ctx context.Context, ticker string) (edgar.Fundamentals, error) {
	key := strings.ToUpper(strings.TrimSpace(ticker))

	c.mu.Lock()
	e, ok := c.cache[key]
	c.mu.Unlock()
	if ok {
		ttl := fundamentalsTTL
		if e.err != nil {
			ttl = fundamentalsNegTTL
		}
		if time.Since(e.at) < ttl {
			return e.f, e.err
		}
	}

	f, err := c.client.Fundamentals(ctx, key)

	c.mu.Lock()
	c.cache[key] = fundEntry{f: f, err: err, at: time.Now()}
	c.mu.Unlock()
	return f, err
}
