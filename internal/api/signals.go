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
