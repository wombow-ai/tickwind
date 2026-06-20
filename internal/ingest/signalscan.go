package ingest

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/indicators"
)

// signalScanEvery is the background refresh cadence for the whole-universe signals
// scan. Signals derive from DAILY bars (and the deterministic rules over them), so
// they change slowly — a sub-hourly refresh buys nothing. The scan runs off the
// request path, so this is purely a freshness/cost tradeoff.
const signalScanEvery = 15 * time.Minute

// SignalComputeSource computes a ticker's indicator set (the input to the signals
// layer). Satisfied by *indicators.Computer — declared here so this package does not
// depend on the api package (which depends on this one).
type SignalComputeSource interface {
	StockIndicators(ctx context.Context, ticker string) indicators.StockIndicatorsResult
}

// UniverseTickers yields the universe of tickers to scan (satisfied by *universe.Cache).
type UniverseTickers interface {
	Tickers() []string
}

// SignalScanCache holds a periodically-refreshed map of ticker → deterministic
// signals for the WHOLE universe, so the signals SCREENER endpoint can filter
// instantly without recomputing 200 tickers on the request path. It mirrors
// OptionsCache's whole-market background-scan pattern. Everything it serves is
// Go-computed (anti-hallucination-safe) — it only caches what indicators.Signals
// produced.
type SignalScanCache struct {
	compute  SignalComputeSource
	universe UniverseTickers
	every    time.Duration
	log      *slog.Logger

	mu       sync.RWMutex
	bySignal map[string][]indicators.Signal
	at       time.Time
}

// NewSignalScanCache builds the cache. A nil logger is tolerated (discarded).
func NewSignalScanCache(compute SignalComputeSource, universe UniverseTickers, log *slog.Logger) *SignalScanCache {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &SignalScanCache{
		compute:  compute,
		universe: universe,
		every:    signalScanEvery,
		log:      log,
		bySignal: map[string][]indicators.Signal{},
	}
}

// Run scans the universe immediately, then every `every` until ctx is cancelled.
// It runs on a background goroutine — the per-ticker indicator compute reads cached
// data (warm for the ingested universe) but a cold ticker may trigger a candle
// fetch, which is fine off the request path.
func (c *SignalScanCache) Run(ctx context.Context) {
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

// scan recomputes signals for every universe ticker and atomically swaps in the new
// map. On a total miss (empty universe / nothing computed) it keeps the previous
// snapshot rather than blanking the board.
func (c *SignalScanCache) scan(ctx context.Context) {
	tickers := c.universe.Tickers()
	next := make(map[string][]indicators.Signal, len(tickers))
	scanned := 0
	for _, tk := range tickers {
		select {
		case <-ctx.Done():
			return
		default:
		}
		res := c.compute.StockIndicators(ctx, tk)
		if sigs := indicators.Signals(res); len(sigs) > 0 {
			next[tk] = sigs
		}
		scanned++
	}
	if scanned == 0 {
		return // empty universe — keep the previous board
	}
	c.mu.Lock()
	c.bySignal, c.at = next, time.Now().UTC()
	c.mu.Unlock()
	c.log.Debug("signal scan refreshed", "tickers", scanned, "with_signals", len(next))
}

// SignalsFor returns a ticker's cached signals (nil if the ticker is not in the latest
// scan — e.g. not in the universe, or before the first scan). Cache-read only, no
// compute — used by the alert evaluator to check signal-condition alerts cheaply.
func (c *SignalScanCache) SignalsFor(ticker string) []indicators.Signal {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.bySignal[ticker]
}

// Screen filters the latest cached scan by the query and returns the matches plus
// when the scan was built (zero time before the first scan completes). Instant — no
// compute, no I/O.
func (c *SignalScanCache) Screen(q indicators.SignalScreen) ([]indicators.SignalMatch, time.Time) {
	c.mu.RLock()
	snap, at := c.bySignal, c.at
	c.mu.RUnlock()
	return indicators.ScreenSignals(snap, q), at
}
