package ingest

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/indicators"
	"github.com/wombow-ai/tickwind/internal/store"
)

// earningsReactionScanEvery / Pace: a stock's earnings-reaction history only changes when a NEW
// earnings is filed, so a 12h refresh is ample; the pace spaces the per-ticker SEC/candle work so a
// scan never bursts.
const (
	earningsReactionScanEvery = 12 * time.Hour
	earningsReactionScanPace  = 80 * time.Millisecond
)

// ERCandleSource yields a ticker's daily candles (satisfied by *BarCache). ERDateSource yields a
// ticker's past earnings-announcement dates (satisfied by *edgar.Client). Declared here so this
// package needn't import api.
type ERCandleSource interface {
	DailyCandles(ctx context.Context, ticker string) ([]store.Candle, error)
}
type ERDateSource interface {
	EarningsDates(ctx context.Context, ticker string) ([]time.Time, error)
}

// EarningsReactionCache precomputes each TRACKED ticker's earnings-reaction AGGREGATE (off the
// request path) so the market-wide earnings calendar can badge how a stock has historically moved
// around its reports without a per-row fetch. Bounded to the tracked set (ingestTickers — the
// recognizable watchlist/popular names, exactly the rows a reader cares about); a ticker with too
// little history simply isn't in the map (insufficient-not-wrong). Every number is Go-computed
// (indicators.ComputeEarningsReaction). On a total miss it keeps the previous map.
type EarningsReactionCache struct {
	candles ERCandleSource
	dates   ERDateSource
	tickers TickerSource
	every   time.Duration
	log     *slog.Logger

	mu sync.RWMutex
	m  map[string]indicators.ReactionSummary
	at time.Time
}

// NewEarningsReactionCache builds the cache over a bounded TickerSource (pass ingestTickers). A nil
// logger is tolerated (discarded).
func NewEarningsReactionCache(candles ERCandleSource, dates ERDateSource, tickers TickerSource, log *slog.Logger) *EarningsReactionCache {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &EarningsReactionCache{candles: candles, dates: dates, tickers: tickers, every: earningsReactionScanEvery, log: log, m: map[string]indicators.ReactionSummary{}}
}

// Run scans immediately, then every `every` until ctx is cancelled, on a background goroutine.
func (c *EarningsReactionCache) Run(ctx context.Context) {
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

// scan recomputes the reaction aggregate for each tracked ticker and atomically swaps the map in. A
// ticker with no candles, no earnings dates, or too little history is omitted (never fabricated).
func (c *EarningsReactionCache) scan(ctx context.Context) {
	tickers := c.tickers(ctx)
	next := make(map[string]indicators.ReactionSummary, len(tickers))
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
			case <-time.After(earningsReactionScanPace):
			}
		}
		candles, err := c.candles.DailyCandles(ctx, tk)
		if err != nil || len(candles) == 0 {
			continue
		}
		dates, err := c.dates.EarningsDates(ctx, tk)
		if err != nil || len(dates) == 0 {
			continue
		}
		er, ok := indicators.ComputeEarningsReaction(dates, candles)
		if !ok {
			continue
		}
		next[tk] = er.Summary()
	}
	if len(next) == 0 {
		return // empty scan — keep the previous map rather than blanking it
	}
	c.mu.Lock()
	c.m, c.at = next, time.Now().UTC()
	c.mu.Unlock()
	c.log.Debug("earnings-reaction scan refreshed", "tickers", len(next))
}

// Reaction returns a ticker's cached earnings-reaction summary (and whether it is present). Safe for
// concurrent use; the scan swaps in a fresh map (never mutates in place).
func (c *EarningsReactionCache) Reaction(ticker string) (indicators.ReactionSummary, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	r, ok := c.m[ticker]
	return r, ok
}

// PopulationRanked ranks the tracked set's cached earnings-reaction aggregates by the chosen VIEW
// (most-volatile | highest-up-rate), high→low, plus when the population was built (zero before the
// first scan). It reads the cache; the only request-path work is the bounded ranking arithmetic in
// indicators.RankEarningsReaction (no compute, no I/O). Empty for an unknown view or a cold map.
// The scan swaps in a fresh map (never mutates in place), so grabbing the map reference under the
// lock then building/ranking outside it is race-safe (the swapped-out map is immutable).
func (c *EarningsReactionCache) PopulationRanked(view string) ([]indicators.ReactionRank, time.Time) {
	c.mu.RLock()
	m, at := c.m, c.at
	c.mu.RUnlock()
	pop := make([]indicators.TickerReaction, 0, len(m))
	for tk, rs := range m {
		pop = append(pop, indicators.TickerReaction{Ticker: tk, ReactionSummary: rs})
	}
	return indicators.RankEarningsReaction(pop, view), at
}
