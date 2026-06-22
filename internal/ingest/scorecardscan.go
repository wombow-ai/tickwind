package ingest

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/indicators"
)

// scorecardScanEvery / scorecardScanPace: the factor population (the percentile distribution) is
// built from fundamentals + 1y returns, which barely move intraday, so an hourly refresh is plenty.
// The pace spaces per-ticker computes so the scan never bursts; fundamentals are served from the
// shared FundamentalsCache (24h TTL), so steady-state compute is just arithmetic over cached data.
const (
	scorecardScanEvery = 60 * time.Minute
	scorecardScanPace  = 60 * time.Millisecond
)

// ScorecardComputeSource computes a ticker's FULL indicator set (incl. SEC fundamentals — the
// factor scorecard reads pe/pb/ps, growth, roe/roic/margins/piotroski, tsr). Satisfied by
// *indicators.Computer; declared here so this package doesn't depend on api.
type ScorecardComputeSource interface {
	StockIndicators(ctx context.Context, ticker string) indicators.StockIndicatorsResult
}

// ScorecardCache holds a periodically-refreshed factor-metric POPULATION for the bounded tracked
// universe (ingestTickers, ~maxIngestTickers names — NOT the whole ~7k price universe). The
// scorecard endpoint ranks a single stock's factors against this population to get descriptive
// percentiles, off the request path. Anti-hallucination-safe (every metric is Go-computed).
type ScorecardCache struct {
	compute ScorecardComputeSource
	tickers TickerSource
	every   time.Duration
	log     *slog.Logger

	mu         sync.RWMutex
	population []indicators.FactorMetrics
	at         time.Time
}

// NewScorecardCache builds the cache over a bounded TickerSource (pass ingestTickers, NOT the full
// universe). A nil logger is tolerated (discarded).
func NewScorecardCache(compute ScorecardComputeSource, tickers TickerSource, log *slog.Logger) *ScorecardCache {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &ScorecardCache{compute: compute, tickers: tickers, every: scorecardScanEvery, log: log}
}

// Run scans the tracked set immediately, then every `every` until ctx is cancelled, on a background
// goroutine (off the request path).
func (c *ScorecardCache) Run(ctx context.Context) {
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

// scan recomputes the factor-metric population for the tracked set and atomically swaps it in. It
// computes the FULL indicator set per ticker (fundamentals cached), paced between tickers. A ticker
// with no usable factor metric still contributes nothing (its NaN fields are skipped when ranking).
// On a total miss it keeps the previous population rather than blanking it.
func (c *ScorecardCache) scan(ctx context.Context) {
	tickers := c.tickers(ctx)
	pop := make([]indicators.FactorMetrics, 0, len(tickers))
	scanned := 0
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
			case <-time.After(scorecardScanPace):
			}
		}
		res := c.compute.StockIndicators(ctx, tk)
		pop = append(pop, indicators.ExtractFactorMetrics(res))
		scanned++
	}
	if scanned == 0 {
		return // empty set — keep the previous population
	}
	c.mu.Lock()
	c.population, c.at = pop, time.Now().UTC()
	c.mu.Unlock()
	c.log.Debug("scorecard scan refreshed", "tickers", scanned)
}

// Population returns the latest factor-metric population (the ranking distribution) + when it was
// built (zero time before the first scan). Cache-read only — the slice is shared read-only (the
// scan swaps in a fresh slice; callers must not mutate it).
func (c *ScorecardCache) Population() ([]indicators.FactorMetrics, time.Time) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.population, c.at
}
