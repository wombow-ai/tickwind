package ingest

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/edgar"
)

// ETF-holdings cache TTLs + cap. N-PORT is filed ~monthly, so a day-stale list is fine; a non-fund /
// no-filing result caches briefly so a stock-page view of a non-ETF doesn't re-probe SEC each time.
// The cache holds the top etfHoldingsCacheN positions so a request larger than the first is still
// served from cache (the FE panel shows 20, the chat tool 15).
const (
	etfHoldingsTTL    = 24 * time.Hour
	etfHoldingsNegTTL = 1 * time.Hour
	etfHoldingsCacheN = 50
	// etfEnrichN bounds how many top positions get an OpenFIGI CUSIP→ticker lookup — the panel shows
	// 20, so enrich 20. With an OpenFIGI key that is 1 request (keyed cap 100, ~0.5s); keyless it is 2
	// batches (~2.8s, still acceptable — far under the ~12s enriching all 50 keyless would cost). The
	// big holdings are the ones worth a cross-link anyway.
	etfEnrichN = 20
)

// ETFHoldingsFetcher fetches a fund/ETF's raw holdings from its N-PORT filing (satisfied by *edgar.Client).
type ETFHoldingsFetcher interface {
	ETFHoldings(ctx context.Context, ticker string, max int) ([]edgar.ETFHolding, time.Time, error)
}

// CUSIPMapper resolves CUSIPs to US tickers, caching results (satisfied by *openfigi.Client). nil → no
// enrichment (holdings keep only the tickers the filing itself carried).
type CUSIPMapper interface {
	Map(ctx context.Context, cusips []string) (map[string]string, error)
}

// ETFHoldingsCache serves an ETF/fund's top holdings on-demand with a 24h TTL, so a stock-page view or
// a chat query doesn't re-hit the SEC N-PORT filing each time. It also ENRICHES holdings the filing
// left ticker-less with an OpenFIGI CUSIP→ticker lookup, so the frontend can cross-link a position to
// its stock page and the chat can name it. Enrichment is best-effort and never fabricates: an unmapped
// CUSIP (or a mapper error) simply leaves the ticker empty. Satisfies api.ETFHoldingsSource.
type ETFHoldingsCache struct {
	fetch  ETFHoldingsFetcher
	mapper CUSIPMapper // optional; nil → no ticker enrichment

	mu    sync.Mutex
	cache map[string]etfHoldingsEntry
}

type etfHoldingsEntry struct {
	holdings []edgar.ETFHolding
	asOf     time.Time
	err      error
	at       time.Time
}

// NewETFHoldingsCache builds the cache over a holdings fetcher + an optional CUSIP→ticker mapper.
func NewETFHoldingsCache(fetch ETFHoldingsFetcher, mapper CUSIPMapper) *ETFHoldingsCache {
	return &ETFHoldingsCache{fetch: fetch, mapper: mapper, cache: make(map[string]etfHoldingsEntry)}
}

// ETFHoldings returns the cached top holdings for an ETF/fund (fetching + enriching on a miss or when
// stale), then returns the requested top `max`. Positive results are held etfHoldingsTTL, errors
// etfHoldingsNegTTL.
func (c *ETFHoldingsCache) ETFHoldings(ctx context.Context, ticker string, max int) ([]edgar.ETFHolding, time.Time, error) {
	key := strings.ToUpper(strings.TrimSpace(ticker))

	c.mu.Lock()
	e, ok := c.cache[key]
	c.mu.Unlock()
	if ok {
		ttl := etfHoldingsTTL
		if e.err != nil {
			ttl = etfHoldingsNegTTL
		}
		if time.Since(e.at) < ttl {
			return capHoldings(e.holdings, max), e.asOf, e.err
		}
	}

	holdings, asOf, err := c.fetch.ETFHoldings(ctx, key, etfHoldingsCacheN)
	if err == nil {
		c.enrichTickers(ctx, holdings) // mutate before caching/sharing → race-free
	}

	c.mu.Lock()
	c.cache[key] = etfHoldingsEntry{holdings: holdings, asOf: asOf, err: err, at: time.Now()}
	c.mu.Unlock()
	return capHoldings(holdings, max), asOf, err
}

// enrichTickers fills in a ticker for holdings the filing left ticker-less, via the CUSIP→ticker
// mapper. Best-effort: a mapper error or an unmapped CUSIP leaves the ticker empty (never fabricated).
func (c *ETFHoldingsCache) enrichTickers(ctx context.Context, holdings []edgar.ETFHolding) {
	if c.mapper == nil {
		return
	}
	// Only the top etfEnrichN positions (= 1 keyless OpenFIGI batch) are resolved, so the cold
	// fetch stays fast — see etfEnrichN.
	n := len(holdings)
	if n > etfEnrichN {
		n = etfEnrichN
	}
	var cusips []string
	for i := 0; i < n; i++ {
		if holdings[i].Ticker == "" && holdings[i].CUSIP != "" {
			cusips = append(cusips, holdings[i].CUSIP)
		}
	}
	if len(cusips) == 0 {
		return
	}
	m, err := c.mapper.Map(ctx, cusips)
	if err != nil {
		return
	}
	for i := 0; i < n; i++ {
		if holdings[i].Ticker == "" {
			if tk := m[strings.ToUpper(strings.TrimSpace(holdings[i].CUSIP))]; tk != "" {
				holdings[i].Ticker = tk
			}
		}
	}
}

// capHoldings returns the top n (the fetcher already weight-sorted), or all when n<=0 or n>=len. It
// shares the underlying array (the elements are immutable once cached — callers only read).
func capHoldings(h []edgar.ETFHolding, n int) []edgar.ETFHolding {
	if n <= 0 || n >= len(h) {
		return h
	}
	return h[:n]
}
