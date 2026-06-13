package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/wombow-ai/tickwind/internal/cboe"
	"github.com/wombow-ai/tickwind/internal/finrashvol"
	"github.com/wombow-ai/tickwind/internal/sentiment"
	"github.com/wombow-ai/tickwind/internal/yahoo"
)

// vixSymbol is the CBOE Volatility Index quoted via Yahoo's chart endpoint (the
// same owner-authorized gray source as the homepage indices strip).
const vixSymbol = "^VIX"

// putCallProxyTicker is the broad-market option chain used as the equity
// put/call-ratio proxy. SPY is the most-liquid US-market ETF, so its
// whole-chain put/call volume reads as a market-wide hedging gauge — a stand-in
// for the official CBOE equity put/call ratio, which has no keyless feed.
const putCallProxyTicker = "SPY"

// sentimentMinShortVolume mirrors the leaderboard floor: only symbols with at
// least this much reported volume feed the daily-average short-pressure
// component, so thin-name noise doesn't skew the market-wide mean.
const sentimentMinShortVolume = 1_000_000

// VIXQuoter fetches a single Yahoo quote (used for ^VIX). Implemented by
// *yahoo.Client.
type VIXQuoter interface {
	Quote(ctx context.Context, symbol string) (yahoo.Quote, bool, error)
}

// OptionChainSource fetches a delayed option chain for a ticker (used for the
// SPY put/call proxy). Implemented by *cboe.Client. ok=false when the symbol has
// no listed options.
type OptionChainSource interface {
	Options(ctx context.Context, ticker string) (cboe.Chain, bool, error)
}

// ShortPctAverager reports the latest day's mean short percentage across the
// liquid names on the leaderboard. Implemented by *finrashvol.Cache via its Top
// ranking (no new raw-data API — reuses the display-only Top).
type ShortPctAverager interface {
	Top(n int, minTotalVolume int64) []finrashvol.ShortVol
	AsOf() string
}

// SentimentIngestor computes a Fear & Greed index once per cycle from whatever
// market-mood inputs are wired up and stores the Result + a daily history point
// in a *sentiment.Cache.
//
// Components are best-effort and independent: a source that's nil or fails is
// simply omitted, and sentiment.Compute re-weights the remaining components
// (equal-weighted), so the index degrades gracefully as inputs come and go. The
// initial wiring uses VIX (Yahoo ^VIX), the SPY put/call proxy (Cboe) and the
// FINRA daily short-pressure average; breadth / new-highs-lows / social-heat are
// TODO once those whole-market inputs are easy to source.
type SentimentIngestor struct {
	vix      VIXQuoter
	options  OptionChainSource
	shortAvg ShortPctAverager
	cache    *sentiment.Cache
	every    time.Duration
	now      func() time.Time // injectable clock for deterministic tests
	log      *slog.Logger
}

// NewSentimentIngestor builds the ingestor. Any source may be nil, which drops
// the corresponding component; the index uses whatever remains. Call Run to start.
func NewSentimentIngestor(vix VIXQuoter, options OptionChainSource, shortAvg ShortPctAverager, cache *sentiment.Cache, every time.Duration, log *slog.Logger) *SentimentIngestor {
	if log == nil {
		log = slog.Default()
	}
	return &SentimentIngestor{
		vix:      vix,
		options:  options,
		shortAvg: shortAvg,
		cache:    cache,
		every:    every,
		now:      func() time.Time { return time.Now().UTC() },
		log:      log,
	}
}

// Run computes immediately (a startup warm) and then on every tick until ctx is
// cancelled. The index is a daily reading, so a ~24h cadence is expected.
func (i *SentimentIngestor) Run(ctx context.Context) {
	i.compute(ctx)
	t := time.NewTicker(i.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			i.compute(ctx)
		}
	}
}

// compute gathers the available component inputs, runs sentiment.Compute and
// stores the Result with today's date as the history point. A run with no usable
// components yields the neutral default; the cache's same-day collapse means
// repeated runs in one day overwrite that day's point rather than piling up.
func (i *SentimentIngestor) compute(ctx context.Context) {
	in := i.gather(ctx)
	res := sentiment.Compute(in)
	date := i.now().Format("2006-01-02")
	i.cache.Set(res, date)
	i.log.Info("sentiment computed", "score", res.Score, "label", res.Label, "components", res.Available)
}

// gather builds the Inputs from the wired sources, omitting any that are nil or
// fail (so Compute re-weights). Each source is queried independently and logged
// at debug on failure — a missing mood input shouldn't be noisy.
func (i *SentimentIngestor) gather(ctx context.Context) sentiment.Inputs {
	var in sentiment.Inputs

	if i.vix != nil {
		if q, ok, err := i.vix.Quote(ctx, vixSymbol); err != nil {
			i.log.Debug("sentiment: VIX fetch failed", "err", err)
		} else if ok && q.Price > 0 {
			v := q.Price
			in.VIX = &v
		}
	}

	if i.options != nil {
		if chain, ok, err := i.options.Options(ctx, putCallProxyTicker); err != nil {
			i.log.Debug("sentiment: put/call fetch failed", "err", err)
		} else if ok {
			if byVol, _ := cboe.PutCallRatio(chain.Contracts); byVol > 0 {
				pc := byVol
				in.PutCallRatio = &pc
			}
		}
	}

	if i.shortAvg != nil {
		if avg, ok := averageShortPct(i.shortAvg.Top(0, sentimentMinShortVolume)); ok {
			in.ShortPct = &avg
		}
	}

	// TODO: wire market breadth (advancers/decliners), new-highs/new-lows
	// momentum and a social-heat component once whole-market inputs for those are
	// easy to source. sentiment.Compute already re-weights to whatever is present,
	// so these can be added incrementally without touching the weighting logic.

	return in
}

// averageShortPct returns the mean short percentage over the rows, or ok=false
// when there are none (so the component is skipped rather than fed a zero).
func averageShortPct(rows []finrashvol.ShortVol) (float64, bool) {
	if len(rows) == 0 {
		return 0, false
	}
	var sum float64
	for _, r := range rows {
		sum += r.ShortPct
	}
	return sum / float64(len(rows)), true
}
