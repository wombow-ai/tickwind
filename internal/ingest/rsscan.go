package ingest

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/indicators"
)

// rsScanEvery / Pace / Benchmark: relative strength is daily-candle-derived (barely moves intraday),
// so a 45-min refresh is plenty; the pace spaces per-ticker work; SPY is the market benchmark (matches
// the per-stock relative-strength endpoint + the beta indicator).
const (
	rsScanEvery = 45 * time.Minute
	rsScanPace  = 60 * time.Millisecond
	rsBenchmark = "SPY"
)

// RelativeStrengthCache periodically computes each tracked stock's trailing relative strength vs SPY
// (off the request path) so the market-wide RS leaderboard (GET /v1/screen/relative-strength) can
// rank by any window without per-request compute. Bounded to analyticTickers. Anti-hallucination-safe:
// every number is Go-computed; a ticker whose history can't honestly fill a window is simply absent
// from that window's ranking, never fabricated. On a total miss it keeps the previous population.
type RelativeStrengthCache struct {
	candles ERCandleSource // satisfied by *BarCache (DailyCandles)
	tickers TickerSource
	every   time.Duration
	log     *slog.Logger

	mu  sync.RWMutex
	pop []indicators.TickerRelStrength
	at  time.Time
}

// NewRelativeStrengthCache builds the cache over a bounded TickerSource (pass analyticTickers). A nil
// logger is tolerated (discarded).
func NewRelativeStrengthCache(candles ERCandleSource, tickers TickerSource, log *slog.Logger) *RelativeStrengthCache {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &RelativeStrengthCache{candles: candles, tickers: tickers, every: rsScanEvery, log: log}
}

// Run scans immediately, then every `every` until ctx is cancelled, on a background goroutine.
func (c *RelativeStrengthCache) Run(ctx context.Context) {
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

// scan recomputes each tracked ticker's relative strength vs SPY and atomically swaps the population
// in. SPY candles are fetched ONCE per scan (not per ticker). A ticker with no candles or too little
// history is omitted (never fabricated).
func (c *RelativeStrengthCache) scan(ctx context.Context) {
	bench, err := c.candles.DailyCandles(ctx, rsBenchmark)
	if err != nil || len(bench) == 0 {
		c.log.Warn("rs scan: benchmark candles unavailable", "err", err)
		return // can't measure relative strength without the benchmark; keep last-good
	}
	tickers := c.tickers(ctx)
	pop := make([]indicators.TickerRelStrength, 0, len(tickers))
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
			case <-time.After(rsScanPace):
			}
		}
		if tk == rsBenchmark {
			continue // relative strength vs itself is the degenerate 0
		}
		cs, err := c.candles.DailyCandles(ctx, tk)
		if err != nil || len(cs) == 0 {
			continue
		}
		rs, ok := indicators.ComputeRelativeStrength(cs, bench)
		if !ok || len(rs.Windows) == 0 {
			continue
		}
		rs.Benchmark = rsBenchmark
		pop = append(pop, indicators.TickerRelStrength{Ticker: tk, RS: rs})
	}
	if len(pop) == 0 {
		return // empty scan — keep the previous population
	}
	c.mu.Lock()
	c.pop, c.at = pop, time.Now().UTC()
	c.mu.Unlock()
	c.log.Debug("rs scan refreshed", "tickers", len(pop))
}

// RankByWindow ranks the tracked universe by relative strength over one window ("1M".."1Y"),
// highest→lowest, + when the population was built. Reads the cache (the only request-path work is the
// bounded ranking arithmetic in indicators.RankRelativeStrength). Empty for an unknown window or a
// cold cache. The scan swaps in a fresh slice (never mutates), so reading the header under the lock
// then ranking outside it is safe.
func (c *RelativeStrengthCache) RankByWindow(window string) ([]indicators.RSRank, time.Time) {
	c.mu.RLock()
	pop, at := c.pop, c.at
	c.mu.RUnlock()
	return indicators.RankRelativeStrength(pop, window), at
}

// minRSPopulation is the floor below which an RS percentile is withheld — ranking a stock against
// too few peers isn't meaningful (mirrors indicators.minScorecardPopulation).
const minRSPopulation = 8

// RSPercentile returns `ticker`'s relative-strength PERCENTILE for `window` ("1M".."1Y") — how its
// excess return vs SPY ranks within the tracked universe (0–100, higher = stronger). It powers the
// research report's "relative to market" section. The population distribution is read from the cache;
// the target's own excess return is reused from the population when present, else computed on-demand
// from its daily candles (so ANY ticker gets a percentile, not only the cached universe). ok=false
// when the population is too small (< minRSPopulation) or the target's window can't be computed
// (insufficient history) — never fabricated. Also returns the population size + as-of for attribution.
func (c *RelativeStrengthCache) RSPercentile(ctx context.Context, ticker, window string) (float64, int, time.Time, bool) {
	ranks, at := c.RankByWindow(window)
	if len(ranks) < minRSPopulation {
		return 0, 0, at, false
	}
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	rel, ok := relForTicker(ranks, ticker)
	if !ok {
		// Not in the tracked universe → compute its excess return vs SPY on-demand.
		rel, ok = c.computeRel(ctx, ticker, window)
		if !ok {
			return 0, 0, at, false
		}
	}
	below := 0
	for _, r := range ranks {
		if r.Relative < rel {
			below++
		}
	}
	return 100 * float64(below) / float64(len(ranks)), len(ranks), at, true
}

// relForTicker returns the ticker's excess return from a ranked window, if it is in the population.
func relForTicker(ranks []indicators.RSRank, ticker string) (float64, bool) {
	for _, r := range ranks {
		if r.Ticker == ticker {
			return r.Relative, true
		}
	}
	return 0, false
}

// computeRel computes `ticker`'s excess return vs SPY for `window` on-demand (a target outside the
// cached universe), reusing the SAME candle source + ComputeRelativeStrength the scan uses.
func (c *RelativeStrengthCache) computeRel(ctx context.Context, ticker, window string) (float64, bool) {
	if ticker == "" || ticker == rsBenchmark {
		return 0, false
	}
	bench, err := c.candles.DailyCandles(ctx, rsBenchmark)
	if err != nil || len(bench) == 0 {
		return 0, false
	}
	cs, err := c.candles.DailyCandles(ctx, ticker)
	if err != nil || len(cs) == 0 {
		return 0, false
	}
	rs, ok := indicators.ComputeRelativeStrength(cs, bench)
	if !ok {
		return 0, false
	}
	for _, w := range rs.Windows {
		if w.Label == window {
			return w.Relative, true
		}
	}
	return 0, false
}
