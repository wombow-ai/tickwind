package ingest

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/edgar"
	"github.com/wombow-ai/tickwind/internal/indicators"
	"github.com/wombow-ai/tickwind/internal/store"
)

// dividendScanEvery / dividendScanPace: a stock's dividend profile only changes when a new annual
// figure is filed (and the yield drifts with price), so an hourly refresh is ample; fundamentals are
// served from the shared FundamentalsCache (24h TTL) and the price from the polled quote store, so a
// steady-state scan is just arithmetic. The pace spaces the per-ticker work so a scan never bursts.
const (
	dividendScanEvery = 60 * time.Minute
	dividendScanPace  = 60 * time.Millisecond
)

// DividendFundamentalsSource yields a ticker's SEC fundamentals (satisfied by *FundamentalsCache).
// DividendQuoteSource yields a ticker's latest cached quote (satisfied by store.Store) — for the
// yield's price leg. Declared here so this package needn't import api.
type DividendFundamentalsSource interface {
	Fundamentals(ctx context.Context, ticker string) (edgar.Fundamentals, error)
}
type DividendQuoteSource interface {
	GetQuote(ctx context.Context, ticker string) (store.Quote, bool, error)
}

// DividendCache holds a periodically-refreshed dividend-profile POPULATION for the bounded tracked
// universe (analyticTickers — NOT the whole ~7k price universe). The market-wide dividend leaderboard
// ranks this population off the request path; a non-payer (or a name with no computable metric) simply
// isn't in it (insufficient-not-wrong). Every number is Go-computed (indicators.ComputeDividend). On a
// total miss it keeps the previous population rather than blanking it.
type DividendCache struct {
	funds   DividendFundamentalsSource
	quotes  DividendQuoteSource
	tickers TickerSource
	every   time.Duration
	log     *slog.Logger

	mu         sync.RWMutex
	population []indicators.TickerDividend
	at         time.Time
}

// NewDividendCache builds the cache over a bounded TickerSource (pass analyticTickers). A nil logger
// is tolerated (discarded).
func NewDividendCache(funds DividendFundamentalsSource, quotes DividendQuoteSource, tickers TickerSource, log *slog.Logger) *DividendCache {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &DividendCache{funds: funds, quotes: quotes, tickers: tickers, every: dividendScanEvery, log: log}
}

// Run scans immediately, then every `every` until ctx is cancelled, on a background goroutine.
func (c *DividendCache) Run(ctx context.Context) {
	c.scan(ctx)
	t := time.NewTicker(c.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.scan(ctx)
		}
	}
}

// scan recomputes the dividend profile for each tracked ticker and atomically swaps the population in.
// A non-payer, a ticker with no fundamentals, or one with no computable metric is omitted (never
// fabricated). The price leg of the yield is the polled quote (0 when absent → the yield is simply
// omitted for that name until a quote lands; the other metrics still compute). On a total miss it
// keeps the previous population.
func (c *DividendCache) scan(ctx context.Context) {
	tickers := c.tickers(ctx)
	pop := make([]indicators.TickerDividend, 0, len(tickers))
	for i, tk := range tickers {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if i > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(dividendScanPace):
			}
		}
		f, err := c.funds.Fundamentals(ctx, tk)
		if err != nil || !f.HasData() {
			continue
		}
		price := 0.0
		if q, ok, _ := c.quotes.GetQuote(ctx, tk); ok && q.Price > 0 {
			price = q.Price
		}
		dv, ok := indicators.ComputeDividend(price, f)
		if !ok || !dv.HasAny() {
			continue // non-payer or nothing computable → omit
		}
		pop = append(pop, indicators.TickerDividend{Ticker: tk, Dividend: dv})
	}
	if len(pop) == 0 {
		return // empty scan — keep the previous population rather than blanking it
	}
	c.mu.Lock()
	c.population, c.at = pop, time.Now().UTC()
	c.mu.Unlock()
	c.log.Debug("dividend scan refreshed", "tickers", len(pop))
}

// PopulationRanked ranks the tracked universe by the chosen dividend VIEW (highest-yield |
// fastest-growing | best-covered | lowest-payout) and returns the leaderboard + when the population
// was built. Reads the cache; the only request-path work is the bounded ranking arithmetic in
// indicators.RankDividend (no compute, no I/O). Empty for an unknown view or a cold population. The
// scan swaps in a fresh slice (never mutates in place), so reading the header under the lock then
// ranking outside it is race-safe.
func (c *DividendCache) PopulationRanked(view string) ([]indicators.DividendRank, time.Time) {
	c.mu.RLock()
	pop, at := c.population, c.at
	c.mu.RUnlock()
	return indicators.RankDividend(pop, view), at
}
