package ingest

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/wombow-ai/tickwind/internal/cboe"
	"github.com/wombow-ai/tickwind/internal/finrashvol"
	"github.com/wombow-ai/tickwind/internal/sentiment"
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/universe"
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

// FearGreedStore persists each day's headline Fear & Greed score so the history
// curve survives redeploys (the live index lives only in the in-memory cache).
// Implemented by *store.Split / any store.Store. The ingestor is nil-safe: when
// no store is wired, it just skips the durable write.
type FearGreedStore interface {
	SaveFearGreed(ctx context.Context, date string, score int) error
}

// BreadthSource reports market breadth — the count of advancing versus declining
// issues across the whole universe — for the Fear & Greed breadth component.
// ok=false (or advancers+decliners==0) means there's no usable data yet, so the
// component is skipped rather than fed a fake neutral. Satisfied by an adapter
// over the universe price cache (see universeBreadth in main.go's wiring).
type BreadthSource interface {
	Breadth() (advancers, decliners int, ok bool)
}

// HeatSource reports a market-wide social-attention "heat" already normalised to
// 0–100 (higher = hotter = greedier) for the Fear & Greed social-heat component.
// ok=false means there's no usable hot-list data yet, so the component is skipped.
// Satisfied by an adapter over the trending hot-list (see hotListHeat).
type HeatSource interface {
	Heat() (float64, bool)
}

// SentimentIngestor computes a Fear & Greed index once per cycle from whatever
// market-mood inputs are wired up and stores the Result + a daily history point
// in a *sentiment.Cache.
//
// Components are best-effort and independent: a source that's nil or fails is
// simply omitted, and sentiment.Compute re-weights the remaining components
// (equal-weighted), so the index degrades gracefully as inputs come and go. The
// wiring uses VIX (Yahoo ^VIX), the SPY put/call proxy (Cboe), the FINRA daily
// short-pressure average, market breadth (advancers/decliners over the universe
// price cache) and social heat (the trending hot-list). New-highs/new-lows is
// still TODO — the universe cache holds only price + day-change, no 52-week range,
// so that momentum component is deferred until a whole-market high/low feed exists.
type SentimentIngestor struct {
	vix      VIXQuoter
	options  OptionChainSource
	shortAvg ShortPctAverager
	breadth  BreadthSource // optional; nil drops the breadth component
	heat     HeatSource    // optional; nil drops the social-heat component
	cache    *sentiment.Cache
	fgStore  FearGreedStore // durable history; nil → skip the persist
	every    time.Duration
	now      func() time.Time // injectable clock for deterministic tests
	log      *slog.Logger
}

// NewSentimentIngestor builds the ingestor. Any source may be nil, which drops
// the corresponding component; the index uses whatever remains. fgStore persists
// each day's score for the durable history and may be nil (the persist is then
// skipped). Call Run to start.
func NewSentimentIngestor(vix VIXQuoter, options OptionChainSource, shortAvg ShortPctAverager, cache *sentiment.Cache, fgStore FearGreedStore, every time.Duration, log *slog.Logger) *SentimentIngestor {
	if log == nil {
		log = slog.Default()
	}
	return &SentimentIngestor{
		vix:      vix,
		options:  options,
		shortAvg: shortAvg,
		cache:    cache,
		fgStore:  fgStore,
		every:    every,
		now:      func() time.Time { return time.Now().UTC() },
		log:      log,
	}
}

// SetBreadthSource wires the optional market-breadth source (advancers vs
// decliners over the universe price cache). Call it before Run; a nil source
// leaves the breadth component off, which the index re-weights around.
func (i *SentimentIngestor) SetBreadthSource(b BreadthSource) { i.breadth = b }

// SetHeatSource wires the optional social-heat source (a 0–100 attention proxy
// from the trending hot-list). Call it before Run; a nil source leaves the
// social-heat component off, which the index re-weights around.
func (i *SentimentIngestor) SetHeatSource(h HeatSource) { i.heat = h }

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
	// Persist today's headline score to the durable Market store so the history
	// survives redeploys (the cache is in-memory). Best-effort: a write failure is
	// logged, not fatal — the in-memory point is already recorded.
	if i.fgStore != nil {
		if err := i.fgStore.SaveFearGreed(ctx, date, res.Score); err != nil {
			i.log.Warn("sentiment: persist fear&greed failed", "err", err)
		}
	}
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

	if i.breadth != nil {
		if adv, dec, ok := i.breadth.Breadth(); ok && adv+dec > 0 {
			a, d := adv, dec
			in.Advancers = &a
			in.Decliners = &d
		}
	}

	if i.heat != nil {
		if h, ok := i.heat.Heat(); ok {
			heat := h
			in.Heat = &heat
		}
	}

	// NewHighs/NewLows (the high-low momentum component) is intentionally left
	// nil: the universe price cache carries only the latest price + day-change
	// reference, not a 52-week range, so counting issues at new highs vs lows
	// would require a per-stock fetch the platform deliberately avoids. Compute
	// skips the component, so this is a true 5-of-6 reading, not a fabricated one.

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

// breadthMaxSanePct / breadthMinSanePct bound a usable day-change so a delayed-
// data reverse-split artifact (a price that looks like +1000% or −99%) doesn't
// count as an advancer/decliner. They mirror api.maxSaneChangePct/minSaneChangePct
// (duplicated rather than imported to avoid an api→ingest import cycle).
const (
	breadthMaxSanePct = 300.0
	breadthMinSanePct = -95.0
)

// universeBreadth adapts the universe price cache into a BreadthSource: it counts
// how many tickers closed the day up versus down, the market-breadth input to the
// Fear & Greed index. A broad advance reads as greed, a broad decline as fear.
//
// Derivation: for each cached quote with a positive previous close, the day change
// is (price−prevClose)/prevClose. A change > 0 is an advancer, < 0 a decliner;
// exactly-flat and uncomputable quotes (no/zero price or prev close, or an
// implausible split-artifact move outside [−95%, +300%]) are ignored. The result
// is a market-wide count, fed straight into sentiment.Inputs.Advancers/Decliners,
// which scores breadth as advancers/(advancers+decliners)·100.
type universeBreadth struct{ cache *universe.Cache }

// NewUniverseBreadth builds a BreadthSource over the universe price cache. A nil
// cache (or one that hasn't swept yet) reports ok=false, so the breadth component
// is skipped rather than fed a fabricated value.
func NewUniverseBreadth(cache *universe.Cache) BreadthSource { return &universeBreadth{cache: cache} }

// Breadth counts advancers vs decliners across the cached universe. ok=false when
// the cache is nil/empty or no quote yields a usable day change.
func (u *universeBreadth) Breadth() (advancers, decliners int, ok bool) {
	if u == nil || u.cache == nil {
		return 0, 0, false
	}
	for _, q := range u.cache.Snapshot() {
		if q.Price <= 0 || q.PrevClose <= 0 {
			continue // no usable price or change reference
		}
		pct := (q.Price - q.PrevClose) / q.PrevClose * 100
		if pct > breadthMaxSanePct || pct < breadthMinSanePct {
			continue // delayed-data reverse-split artifact
		}
		switch {
		case pct > 0:
			advancers++
		case pct < 0:
			decliners++
		}
	}
	return advancers, decliners, advancers+decliners > 0
}

// hotLister is the minimal store capability hotListHeat needs: read one ranked
// board of the trending hot-list. Satisfied by store.Store (memory/postgres/Split).
type hotLister interface {
	HotList(ctx context.Context, board string, limit int) ([]store.HotStock, error)
}

// heatBoard is the hot-list board hotListHeat reads — the volume×momentum "most
// discussed" leaderboard built by buildBoards.
const heatBoard = "hot"

// heatTopN caps how many leaderboard rows feed the heat proxy (the whole "hot"
// board is only ~40 rows, so this just bounds the read).
const heatTopN = 40

// hotListHeat adapts the trending hot-list into a HeatSource: a market-wide
// social-attention level on the 0–100 greed scale, the Fear & Greed social-heat
// input. Surging chatter reads as greed.
//
// Derivation (documented + bounded): each "hot" row carries its 24h mention
// growth in Change (the fraction (mentions−mentionsPrev)/mentionsPrev, floored at
// 0 in buildBoards). The proxy is the MENTION-WEIGHTED average 24h growth across
// the board — volume × momentum, so a megacap's chatter counts more than a thin
// name's — mapped onto [0,100] with a flat board (0% growth) at the 50 neutral
// midpoint and a doubling of aggregate chatter (+100% growth) saturating to 100:
//
//	heat = clamp(50 + 50·min(weightedGrowth, 1), 0, 100)
//
// Because buildBoards floors growth at 0, heat never drops below 50 — a cooling
// market reads as neutral attention, not fear (fear is already carried by the VIX,
// put/call and breadth components). ok=false when the board is empty or has no
// usable mention base, so the component is skipped rather than fed a fake 50.
type hotListHeat struct {
	store hotLister
	now   func() time.Time
}

// NewHotListHeat builds a HeatSource over the trending hot-list store. A nil store
// reports ok=false (the social-heat component is then skipped).
func NewHotListHeat(st hotLister) HeatSource {
	return &hotListHeat{store: st, now: func() time.Time { return time.Now().UTC() }}
}

// Heat returns the 0–100 attention proxy and true, or 0/false when the hot-list is
// empty or carries no usable mention base.
func (h *hotListHeat) Heat() (float64, bool) {
	if h == nil || h.store == nil {
		return 0, false
	}
	// The hot-list refresh is independent of this read; a short timeout keeps the
	// daily sentiment cycle from blocking on a slow store.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := h.store.HotList(ctx, heatBoard, heatTopN)
	if err != nil || len(rows) == 0 {
		return 0, false
	}
	var weightedGrowth, totalMentions float64
	for _, r := range rows {
		if r.Mentions <= 0 {
			continue
		}
		// Change is the 24h mention-growth fraction (floored at 0 in buildBoards);
		// weight it by mention volume so loud names dominate the market reading.
		weightedGrowth += r.Change * float64(r.Mentions)
		totalMentions += float64(r.Mentions)
	}
	if totalMentions == 0 {
		return 0, false // no usable mention base
	}
	growth := weightedGrowth / totalMentions
	heat := 50 + 50*math.Min(growth, 1)
	return math.Max(0, math.Min(100, heat)), true
}
