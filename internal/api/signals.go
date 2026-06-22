package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/wombow-ai/tickwind/internal/auth"
	"github.com/wombow-ai/tickwind/internal/indicators"
)

// freeSignalTeaserLimit is how many signals a non-Pro viewer sees when the signals
// paywall is ON: the first N (deterministic order), with paywall_locked=true and the
// full count so the UI can show an "unlock N more with Pro" CTA. Teaser depth is an
// open owner decision (docs/indicators-monetization-plan.md §7) — change here.
const freeSignalTeaserLimit = 3

// freeScreenTeaserLimit is how many screener matches a non-Pro viewer sees when the
// signals paywall is ON: a teaser (NOT a hard lock) so the pSEO landing pages stay
// crawlable + the free tier funnels into Pro. The full screen is Pro.
const freeScreenTeaserLimit = 5

// SetIndicatorsPaywallEnabled turns the Pro paywall for the signals layer on/off.
// Default off (full signal list for everyone); the owner flips it at go-live.
func (s *Server) SetIndicatorsPaywallEnabled(on bool) { s.indicatorsPaywallEnabled = on }

// stockSignalsResp is the wire shape of GET /v1/stocks/{ticker}/indicator-signals: the ticker,
// the newest underlying data date, the deterministic signals, and — when the paywall
// is on and the viewer is not Pro — paywall_locked=true with total_signals so the UI
// can show how many are gated.
type stockSignalsResp struct {
	Ticker        string              `json:"ticker"`
	AsOf          string              `json:"as_of"`
	Signals       []indicators.Signal `json:"signals"`
	TotalSignals  int                 `json:"total_signals"`
	PaywallLocked bool                `json:"paywall_locked,omitempty"`
}

// getStockSignals serves the deterministic posture-signals for a single ticker. The
// signals are pure Go rules over the already-computed indicators (see
// indicators.Signals) — never LLM-invented — so this is anti-hallucination-safe.
//
// Gating mirrors the deep-report paywall: when indicatorsPaywallEnabled and the viewer
// is not Pro, only the first freeSignalTeaserLimit signals are returned with
// paywall_locked=true (full list for Pro / when the flag is off, exactly as today).
// 404 only when the compute source is unset or there is nothing at all to compute from.
func (s *Server) getStockSignals(w http.ResponseWriter, r *http.Request) {
	if s.indicatorCalc == nil {
		writeJSON(w, http.StatusNotFound, errBody("signals unavailable"))
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	if ticker == "" {
		writeJSON(w, http.StatusNotFound, errBody("no ticker"))
		return
	}
	res := s.indicatorCalc.StockIndicators(r.Context(), ticker)

	// "Nothing at all" → 404 (unknown/non-US ticker), mirroring getStockIndicators. A
	// valid ticker with data but no triggering rule returns 200 with an empty list.
	hasOK := false
	for _, si := range res.Indicators {
		if si.Status == indicators.StatusOK {
			hasOK = true
			break
		}
	}
	if !hasOK && res.AsOf == "" && res.VIX == nil && res.FearGreed == nil {
		writeJSON(w, http.StatusNotFound, errBody("no signals for "+ticker))
		return
	}

	sigs := indicators.Signals(res)
	out := stockSignalsResp{Ticker: res.Ticker, AsOf: res.AsOf, Signals: sigs, TotalSignals: len(sigs)}

	if s.indicatorsPaywallEnabled {
		u, _ := auth.UserFrom(r.Context())
		if s.tierOf(r.Context(), u.ID) != tierPro {
			out.Signals, out.PaywallLocked = teaserSignals(sigs)
		}
	}
	if out.Signals == nil {
		out.Signals = []indicators.Signal{}
	}
	writeJSON(w, http.StatusOK, out)
}

// backtestResp is the wire shape of GET /v1/stocks/{ticker}/backtest.
type backtestResp struct {
	Ticker        string                     `json:"ticker"`
	Result        *indicators.BacktestResult `json:"result,omitempty"`
	PaywallLocked bool                       `json:"paywall_locked,omitempty"`
}

// defaultBacktestHorizon / maxBacktestHorizon bound the forward-return window.
const (
	defaultBacktestHorizon = 20 // ~1 trading month
	maxBacktestHorizon     = 60
)

// freeBacktestRuns is the LIFETIME free backtest allowance for a signed-in non-Pro user — a
// one-time conversion taste of the Pro-locked backtester.
const freeBacktestRuns = 1

// getBacktest replays a signal rule over a ticker's daily candles and returns the
// historical win rate / avg forward return / trade count / buy-and-hold baseline (see
// indicators.BacktestSignal). It is a disclosed historical statistic, never advice.
// Pro-gated: when INDICATORS_PAYWALL_ENABLED and the viewer is not Pro, returns
// paywall_locked (a hard lock — backtesting is Pro-only). Flag off → available to all.
func (s *Server) getBacktest(w http.ResponseWriter, r *http.Request) {
	if s.bars == nil {
		writeJSON(w, http.StatusNotFound, errBody("backtest unavailable"))
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	if ticker == "" {
		writeJSON(w, http.StatusNotFound, errBody("no ticker"))
		return
	}
	rule := strings.TrimSpace(r.URL.Query().Get("rule"))
	if !indicators.BacktestableRule(rule) {
		writeJSON(w, http.StatusBadRequest, errBody("invalid or missing rule"))
		return
	}
	horizon := defaultBacktestHorizon
	if v, ok := parseFloat(r.URL.Query().Get("horizon")); ok && v >= 1 {
		horizon = int(v)
	}
	if horizon > maxBacktestHorizon {
		horizon = maxBacktestHorizon
	}
	u, _ := auth.UserFrom(r.Context())
	freeRun := false // true when THIS run consumes the signed-in non-Pro user's one free backtest
	if s.indicatorsPaywallEnabled && s.tierOf(r.Context(), u.ID) != tierPro {
		// Signed-in non-Pro users get ONE free backtest (lifetime, no reset); anon must sign in.
		if u.ID != "" {
			if used, _ := s.store.GetBacktestFreeUsed(r.Context(), u.ID); used < freeBacktestRuns {
				freeRun = true
			}
		}
		if !freeRun {
			writeJSON(w, http.StatusOK, backtestResp{Ticker: ticker, PaywallLocked: true})
			return
		}
	}
	candles, err := s.bars.DailyCandles(r.Context(), ticker)
	if err != nil || len(candles) == 0 {
		writeJSON(w, http.StatusNotFound, errBody("no price history for "+ticker))
		return
	}
	res, ok := indicators.BacktestSignal(candles, rule, horizon)
	if !ok {
		writeJSON(w, http.StatusUnprocessableEntity, errBody("insufficient history to backtest "+rule))
		return
	}
	// Charge the free allowance only on a SUCCESSFUL run (not on a bad rule / no history).
	if freeRun {
		if err := s.store.IncrBacktestFreeUsed(r.Context(), u.ID); err != nil {
			s.log.Debug("backtest free-use incr failed (non-fatal)", "user", u.ID, "err", err)
		}
	}
	writeJSON(w, http.StatusOK, backtestResp{Ticker: ticker, Result: &res})
}

// indicatorHistoryResp is the wire shape of GET /v1/stocks/{ticker}/indicator-history: the
// ticker + one indicator's date-aligned time series (nil when there is nothing to chart).
type indicatorHistoryResp struct {
	Ticker  string                    `json:"ticker"`
	History *indicators.HistorySeries `json:"history,omitempty"`
}

// getIndicatorHistory serves the TIME SERIES for one technical indicator over a ticker's daily
// candles (see indicators.IndicatorHistory) — the date-aligned line a chart draws (the
// time-series counterpart to the single-point /indicators value). It is pure Go math over the
// public daily candles (reuses the same series the point value is taken from), so it is
// anti-hallucination-safe — never an LLM-invented number. Query: ?id=<catalog id, e.g.
// technical.rsi>&period=<n optional>. 400 on an unsupported/missing id, 404 when there is no
// price history, 422 when history is too short to compute even one point. Currently free
// (mirrors the free single-point indicators); gating is an open owner decision.
func (s *Server) getIndicatorHistory(w http.ResponseWriter, r *http.Request) {
	if s.bars == nil {
		writeJSON(w, http.StatusNotFound, errBody("indicator history unavailable"))
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	if ticker == "" {
		writeJSON(w, http.StatusNotFound, errBody("no ticker"))
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !indicators.HistoryableID(id) {
		writeJSON(w, http.StatusBadRequest, errBody("unsupported or missing indicator id"))
		return
	}
	period := 0 // 0 → the indicator's catalog default
	if v, ok := parseFloat(r.URL.Query().Get("period")); ok && v >= 1 {
		period = int(v)
		if period > 250 {
			period = 250
		}
	}
	candles, err := s.bars.DailyCandles(r.Context(), ticker)
	if err != nil || len(candles) == 0 {
		writeJSON(w, http.StatusNotFound, errBody("no price history for "+ticker))
		return
	}
	hs, ok := indicators.IndicatorHistory(candles, id, period)
	if !ok {
		writeJSON(w, http.StatusUnprocessableEntity, errBody("insufficient history to chart "+id))
		return
	}
	writeJSON(w, http.StatusOK, indicatorHistoryResp{Ticker: ticker, History: &hs})
}

// seasonalityResp is the wire shape of GET /v1/stocks/{ticker}/seasonality: the ticker + its
// month-of-year return seasonality (nil when there is too little history).
type seasonalityResp struct {
	Ticker      string                  `json:"ticker"`
	Seasonality *indicators.Seasonality `json:"seasonality,omitempty"`
}

// getSeasonality serves a ticker's month-of-year return SEASONALITY (indicators.ComputeSeasonality)
// — the historical average/median return + win-rate per calendar month over the available years.
// Pure Go math over the public daily candles (a disclosed HISTORICAL statistic, like the backtest;
// NEVER a forecast, target, or advice), so it is anti-hallucination-safe. 404 when there is no
// price source/history, 422 when history is too short. Currently free.
func (s *Server) getSeasonality(w http.ResponseWriter, r *http.Request) {
	if s.bars == nil {
		writeJSON(w, http.StatusNotFound, errBody("seasonality unavailable"))
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	if ticker == "" {
		writeJSON(w, http.StatusNotFound, errBody("no ticker"))
		return
	}
	candles, err := s.bars.DailyCandles(r.Context(), ticker)
	if err != nil || len(candles) == 0 {
		writeJSON(w, http.StatusNotFound, errBody("no price history for "+ticker))
		return
	}
	se, ok := indicators.ComputeSeasonality(candles)
	if !ok {
		writeJSON(w, http.StatusUnprocessableEntity, errBody("insufficient history for seasonality"))
		return
	}
	writeJSON(w, http.StatusOK, seasonalityResp{Ticker: ticker, Seasonality: &se})
}

// relStrengthBenchmark is the market benchmark a stock's relative strength is measured against —
// the S&P 500 proxy SPY, matching the per-stock beta indicator (indicators.marketBenchmarkTicker).
const relStrengthBenchmark = "SPY"

// relStrengthResp is the wire shape of GET /v1/stocks/{ticker}/relative-strength: the ticker + its
// trailing performance vs the benchmark (nil when there is too little history).
type relStrengthResp struct {
	Ticker           string                       `json:"ticker"`
	RelativeStrength *indicators.RelativeStrength `json:"relative_strength,omitempty"`
}

// getRelativeStrength serves a ticker's trailing RELATIVE STRENGTH vs SPY
// (indicators.ComputeRelativeStrength) — the stock's %-return minus SPY's over the same 1M/3M/6M/1Y
// calendar spans. Pure Go math over the public daily candles (a disclosed HISTORICAL statistic,
// like seasonality/backtest; NEVER a forecast, target, or advice), so it is anti-hallucination-safe.
// 404 when there is no price source/history, 422 when the benchmark is unavailable, history is too
// short, or the ticker IS the benchmark (relative strength vs itself is the degenerate 0). Free.
func (s *Server) getRelativeStrength(w http.ResponseWriter, r *http.Request) {
	if s.bars == nil {
		writeJSON(w, http.StatusNotFound, errBody("relative strength unavailable"))
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	if ticker == "" {
		writeJSON(w, http.StatusNotFound, errBody("no ticker"))
		return
	}
	if ticker == relStrengthBenchmark {
		writeJSON(w, http.StatusUnprocessableEntity, errBody("relative strength vs itself is not meaningful"))
		return
	}
	candles, err := s.bars.DailyCandles(r.Context(), ticker)
	if err != nil || len(candles) == 0 {
		writeJSON(w, http.StatusNotFound, errBody("no price history for "+ticker))
		return
	}
	bench, err := s.bars.DailyCandles(r.Context(), relStrengthBenchmark)
	if err != nil || len(bench) == 0 {
		writeJSON(w, http.StatusUnprocessableEntity, errBody("benchmark price history unavailable"))
		return
	}
	rs, ok := indicators.ComputeRelativeStrength(candles, bench)
	if !ok {
		writeJSON(w, http.StatusUnprocessableEntity, errBody("insufficient history for relative strength"))
		return
	}
	rs.Benchmark = relStrengthBenchmark
	writeJSON(w, http.StatusOK, relStrengthResp{Ticker: ticker, RelativeStrength: &rs})
}

// earningsReactionResp is the wire shape of GET /v1/stocks/{ticker}/earnings-reaction: the ticker
// + how it has historically moved around past earnings (nil when there is too little history).
type earningsReactionResp struct {
	Ticker           string                       `json:"ticker"`
	EarningsReaction *indicators.EarningsReaction `json:"earnings_reaction,omitempty"`
}

// getEarningsReaction serves how a ticker has historically MOVED AROUND its earnings announcements
// (indicators.ComputeEarningsReaction): for each past 8-K item 2.02 (earnings) filing date, the
// close-to-close move spanning the announcement, plus aggregates (avg / avg-magnitude move,
// up-rate, sample size). Dates come from SEC 8-K filings, the move from the public daily candles —
// every number is Go-computed, a disclosed HISTORICAL statistic, NEVER a forecast or advice, so it
// is anti-hallucination-safe. 404 when the price/earnings-date source is unset or there is no price
// history; 422 when earnings dates can't be fetched or there is too little overlap to measure. Free.
func (s *Server) getEarningsReaction(w http.ResponseWriter, r *http.Request) {
	if s.bars == nil || s.earningsDates == nil {
		writeJSON(w, http.StatusNotFound, errBody("earnings reaction unavailable"))
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	if ticker == "" {
		writeJSON(w, http.StatusNotFound, errBody("no ticker"))
		return
	}
	candles, err := s.bars.DailyCandles(r.Context(), ticker)
	if err != nil || len(candles) == 0 {
		writeJSON(w, http.StatusNotFound, errBody("no price history for "+ticker))
		return
	}
	dates, err := s.earningsDates.EarningsDates(r.Context(), ticker)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errBody("earnings dates unavailable for "+ticker))
		return
	}
	er, ok := indicators.ComputeEarningsReaction(dates, candles)
	if !ok {
		writeJSON(w, http.StatusUnprocessableEntity, errBody("insufficient earnings history for "+ticker))
		return
	}
	writeJSON(w, http.StatusOK, earningsReactionResp{Ticker: ticker, EarningsReaction: &er})
}

// scorecardResp is the wire shape of GET /v1/stocks/{ticker}/scorecard: the ticker + its four
// factor PERCENTILES vs the tracked universe (nil when nothing is computable). PopulationAsOf is
// when the ranking distribution was last rebuilt (so a stale ranking is disclosed, not hidden).
type scorecardResp struct {
	Ticker         string                `json:"ticker"`
	AsOf           string                `json:"as_of,omitempty"`
	PopulationAsOf string                `json:"population_as_of,omitempty"`
	Scorecard      *indicators.Scorecard `json:"scorecard,omitempty"`
}

// getScorecard serves a ticker's MULTI-FACTOR SCORECARD (indicators.ComputeScorecard): where it
// ranks — as a PERCENTILE vs Tickwind's tracked universe — on Value / Growth / Quality / Momentum.
// The four factors are independent and DESCRIPTIVE; there is deliberately no blended composite
// "score" and no rating/recommendation (the no-advice line). Every number is Go-computed from the
// public quote + SEC-XBRL fundamentals, so it is anti-hallucination-safe. The target's factor
// metrics are computed on-demand and ranked against the background-cached population. 404 when the
// compute/population source is unset; 422 when neither the population nor the ticker yields a
// computable factor (insufficient — e.g. an ETF with no fundamentals, or before the first scan).
// Currently free.
func (s *Server) getScorecard(w http.ResponseWriter, r *http.Request) {
	if s.indicatorCalc == nil || s.scorecard == nil {
		writeJSON(w, http.StatusNotFound, errBody("scorecard unavailable"))
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	if ticker == "" {
		writeJSON(w, http.StatusNotFound, errBody("no ticker"))
		return
	}
	population, popAt := s.scorecard.Population()
	res := s.indicatorCalc.StockIndicators(r.Context(), ticker)
	target := indicators.ExtractFactorMetrics(res)
	sc := indicators.ComputeScorecard(target, population)
	if !sc.HasAny() {
		writeJSON(w, http.StatusUnprocessableEntity, errBody("insufficient factor data for "+ticker))
		return
	}
	popAsOf := ""
	if !popAt.IsZero() {
		popAsOf = popAt.UTC().Format("2006-01-02")
	}
	writeJSON(w, http.StatusOK, scorecardResp{Ticker: ticker, AsOf: res.AsOf, PopulationAsOf: popAsOf, Scorecard: &sc})
}

// dividendResp is the wire shape of GET /v1/stocks/{ticker}/dividend: the ticker + its dividend
// profile (nil for a non-payer or when nothing is computable).
type dividendResp struct {
	Ticker   string                   `json:"ticker"`
	Dividend *indicators.DividendView `json:"dividend,omitempty"`
}

// getDividend serves a stock's DIVIDEND PROFILE (indicators.ComputeDividend): yield, payout ratio,
// dividends-per-share, free-cash-flow coverage, and YoY dividend growth — the income-investor lens,
// surfacing the SEC-filed dividend figures that otherwise sit buried among the ~160 indicators. Every
// number is Go-computed (descriptive, NEVER a "dividend-safety grade" — the no-advice line), so it is
// anti-hallucination-safe. 404 when the fundamentals source is unset or the ticker has no
// fundamentals; 422 for a NON-PAYER (no dividend profile) or when nothing is computable. Free.
func (s *Server) getDividend(w http.ResponseWriter, r *http.Request) {
	if s.fundamentals == nil {
		writeJSON(w, http.StatusNotFound, errBody("dividend data unavailable"))
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	if ticker == "" {
		writeJSON(w, http.StatusNotFound, errBody("no ticker"))
		return
	}
	f, err := s.fundamentals.Fundamentals(r.Context(), ticker)
	if err != nil || !f.HasData() {
		writeJSON(w, http.StatusNotFound, errBody("no fundamentals for "+ticker))
		return
	}
	// Price (for the yield): the polled quote first, else an on-demand fetch (mirrors getFundamentals).
	price := 0.0
	if q, ok, _ := s.store.GetQuote(r.Context(), ticker); ok && q.Price > 0 {
		price = q.Price
	} else if s.bars != nil {
		if oq, found, qerr := s.bars.LatestQuote(r.Context(), ticker); qerr == nil && found {
			price = oq.Price
		}
	}
	dv, ok := indicators.ComputeDividend(price, f)
	if !ok || !dv.HasAny() {
		writeJSON(w, http.StatusUnprocessableEntity, errBody("no dividend profile for "+ticker))
		return
	}
	writeJSON(w, http.StatusOK, dividendResp{Ticker: ticker, Dividend: &dv})
}

// factorScreenResp is the wire shape of GET /v1/screen/factors. Population is how many tracked
// stocks had a computable percentile for this factor (the leaderboard's denominator, before any
// limit truncation); AsOf is when the ranking population was last rebuilt.
type factorScreenResp struct {
	Factor     string                  `json:"factor"`
	Count      int                     `json:"count"`
	Results    []indicators.FactorRank `json:"results"`
	Population int                     `json:"population"`
	AsOf       string                  `json:"as_of,omitempty"`
}

// defaultFactorScreenLimit / maxFactorScreenLimit bound the factor leaderboard length.
const (
	defaultFactorScreenLimit = 50
	maxFactorScreenLimit     = 200
)

// getFactorScreen serves the market-wide FACTOR LEADERBOARD: every tracked stock ranked by one
// factor's PERCENTILE (`?factor=value|growth|quality|momentum`, `?limit=`) vs the whole tracked
// universe. It reads the background-cached scorecard population (no per-request compute beyond the
// bounded ranking arithmetic) and reuses the SAME ComputeScorecard path as the per-stock scorecard,
// as-of the population's build time (AsOf discloses the vintage). Every number is Go-computed from
// the public quote + SEC-XBRL fundamentals — descriptive percentiles, NO rating/advice — so it is
// anti-hallucination-safe. Free + crawlable (the market-wide view of the
// free per-stock scorecard). 400 on an unknown/missing factor; 404 when the population source is
// unset; a cold or too-small population yields a 200 with an empty list (the scan/ISR refills it).
func (s *Server) getFactorScreen(w http.ResponseWriter, r *http.Request) {
	if s.scorecard == nil {
		writeJSON(w, http.StatusNotFound, errBody("factor screen unavailable"))
		return
	}
	factor := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("factor")))
	if !indicators.ValidFactor(factor) {
		writeJSON(w, http.StatusBadRequest, errBody("factor must be one of value, growth, quality, momentum"))
		return
	}
	ranked, at := s.scorecard.PopulationRanked(factor)
	total := len(ranked) // full ranked count BEFORE truncation (the leaderboard's population)
	lim := queryLimit(r, defaultFactorScreenLimit)
	if lim > maxFactorScreenLimit {
		lim = maxFactorScreenLimit
	}
	if lim > 0 && len(ranked) > lim {
		ranked = ranked[:lim]
	}
	if ranked == nil {
		ranked = []indicators.FactorRank{}
	}
	out := factorScreenResp{Factor: factor, Count: len(ranked), Results: ranked, Population: total}
	if !at.IsZero() {
		out.AsOf = at.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, out)
}

// SignalScanSource is the whole-universe signals SCREENER (a background cache that
// precomputes ticker→signals so the endpoint never recomputes on the request path).
type SignalScanSource interface {
	Screen(q indicators.SignalScreen) ([]indicators.SignalMatch, time.Time)
}

// SetSignalScan injects the signals-screener source after New. nil-safe: the endpoint
// 404s until set.
func (s *Server) SetSignalScan(src SignalScanSource) { s.signalScan = src }

// screenSignalsResp is the wire shape of GET /v1/screen/signals. When paywall_locked,
// Results is the free teaser (first freeScreenTeaserLimit) and TotalMatches is the full
// count so the UI can show "N more with Pro".
type screenSignalsResp struct {
	Count         int                      `json:"count"`
	Results       []indicators.SignalMatch `json:"results"`
	TotalMatches  int                      `json:"total_matches"`
	AsOf          string                   `json:"as_of,omitempty"` // when the scan was built
	PaywallLocked bool                     `json:"paywall_locked,omitempty"`
}

// getScreenSignals screens the whole universe for stocks whose deterministic signals
// match the query (`?direction=bullish|bearish|neutral` & `?signal=<indicator id>` &
// `?limit=`). It reads the background scan cache — no per-request compute. The full
// screen is Pro: when INDICATORS_PAYWALL_ENABLED and the viewer is not Pro, the result
// is truncated to a TEASER (first freeScreenTeaserLimit + paywall_locked + TotalMatches),
// so the pSEO landing pages stay crawlable and the free tier funnels into Pro. Flag off
// → full results for everyone (current-behavior-safe).
func (s *Server) getScreenSignals(w http.ResponseWriter, r *http.Request) {
	if s.signalScan == nil {
		writeJSON(w, http.StatusNotFound, errBody("screener unavailable"))
		return
	}
	q := r.URL.Query()
	matches, at := s.signalScan.Screen(indicators.SignalScreen{
		Direction: strings.ToLower(strings.TrimSpace(q.Get("direction"))),
		SignalID:  strings.TrimSpace(q.Get("signal")),
	})
	total := len(matches) // full match count BEFORE any truncation (drives "N more with Pro")
	lim := queryLimit(r, 100)
	if lim > 200 {
		lim = 200
	}
	if lim > 0 && len(matches) > lim {
		matches = matches[:lim]
	}
	locked := false
	// Pro-gate: a free viewer gets the first freeScreenTeaserLimit matches + a lock flag.
	if s.indicatorsPaywallEnabled {
		u, _ := auth.UserFrom(r.Context())
		if s.tierOf(r.Context(), u.ID) != tierPro && total > freeScreenTeaserLimit {
			matches = matches[:freeScreenTeaserLimit:freeScreenTeaserLimit]
			locked = true
		}
	}
	if matches == nil {
		matches = []indicators.SignalMatch{}
	}
	out := screenSignalsResp{Count: len(matches), Results: matches, TotalMatches: total, PaywallLocked: locked}
	if !at.IsZero() {
		out.AsOf = at.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, out)
}

// teaserSignals returns the free-tier teaser: the first freeSignalTeaserLimit signals
// and locked=true when there are MORE than the limit (so a viewer with few signals
// isn't told something is locked when nothing is). Returns the full slice + false when
// it already fits. Does not mutate the input.
func teaserSignals(sigs []indicators.Signal) (teaser []indicators.Signal, locked bool) {
	if len(sigs) <= freeSignalTeaserLimit {
		return sigs, false
	}
	return sigs[:freeSignalTeaserLimit:freeSignalTeaserLimit], true
}
