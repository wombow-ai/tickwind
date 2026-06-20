package ingest

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/indicators"
)

// signalScanEvery is the background refresh cadence for the signals scan. Signals
// derive from DAILY bars, so they change slowly — a sub-hourly refresh buys nothing.
// signalScanPace is a polite gap between per-ticker computes so the scan never bursts
// the shared Alpaca client (which has no rate limiter) and never starves the live
// price poller; with the bounded tracked-ticker set this spreads the scan over ~tens
// of seconds, well within the 15-min cadence.
const (
	signalScanEvery = 15 * time.Minute
	signalScanPace  = 60 * time.Millisecond
)

// SignalComputeSource computes a ticker's TECHNICAL indicator set (the input to the
// signals layer — NO SEC fundamentals, which signals never read). Satisfied by
// *indicators.Computer; declared here so this package does not depend on the api
// package (which depends on this one).
type SignalComputeSource interface {
	StockIndicatorsTechnical(ctx context.Context, ticker string) indicators.StockIndicatorsResult
}

// SignalScanCache holds a periodically-refreshed map of ticker → deterministic signals
// for the TRACKED ticker set (the bounded ~maxIngestTickers names the poller already
// follows — NOT the whole ~7k price universe, which would storm Alpaca/SEC), so the
// signals SCREENER endpoint can filter instantly without recomputing on the request
// path. Mirrors OptionsCache's background-scan pattern; serves only Go-computed signals
// (anti-hallucination-safe).
type SignalScanCache struct {
	compute SignalComputeSource
	tickers TickerSource // bounded tracked set (e.g. the poller's ingestTickers)
	every   time.Duration
	log     *slog.Logger

	mu       sync.RWMutex
	bySignal map[string][]indicators.Signal
	at       time.Time
}

// NewSignalScanCache builds the cache over a bounded TickerSource (pass the poller's
// ingestTickers, NOT the full universe). A nil logger is tolerated (discarded).
func NewSignalScanCache(compute SignalComputeSource, tickers TickerSource, log *slog.Logger) *SignalScanCache {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &SignalScanCache{
		compute:  compute,
		tickers:  tickers,
		every:    signalScanEvery,
		log:      log,
		bySignal: map[string][]indicators.Signal{},
	}
}

// Run scans the tracked set immediately, then every `every` until ctx is cancelled,
// on a background goroutine (off the request path).
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

// scan recomputes signals for each tracked ticker and atomically swaps in the new map.
// It uses the TECHNICALS-ONLY compute (no SEC) and paces between tickers so it never
// bursts Alpaca. On a total miss (empty set / nothing computed) it keeps the previous
// snapshot rather than blanking the board.
func (c *SignalScanCache) scan(ctx context.Context) {
	tickers := c.tickers(ctx)
	next := make(map[string][]indicators.Signal, len(tickers))
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
			case <-time.After(signalScanPace):
			}
		}
		res := c.compute.StockIndicatorsTechnical(ctx, tk)
		if sigs := indicators.Signals(res); len(sigs) > 0 {
			next[tk] = sigs
		}
		scanned++
	}
	if scanned == 0 {
		return // empty set — keep the previous board
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
