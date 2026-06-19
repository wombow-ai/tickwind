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
const freeSignalTeaserLimit = 2

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

// SignalScanSource is the whole-universe signals SCREENER (a background cache that
// precomputes ticker→signals so the endpoint never recomputes on the request path).
type SignalScanSource interface {
	Screen(q indicators.SignalScreen) ([]indicators.SignalMatch, time.Time)
}

// SetSignalScan injects the signals-screener source after New. nil-safe: the endpoint
// 404s until set.
func (s *Server) SetSignalScan(src SignalScanSource) { s.signalScan = src }

// screenSignalsResp is the wire shape of GET /v1/screen/signals.
type screenSignalsResp struct {
	Count         int                      `json:"count"`
	Results       []indicators.SignalMatch `json:"results"`
	AsOf          string                   `json:"as_of,omitempty"` // when the scan was built
	PaywallLocked bool                     `json:"paywall_locked,omitempty"`
}

// getScreenSignals screens the whole universe for stocks whose deterministic signals
// match the query (`?direction=bullish|bearish|neutral` & `?signal=<indicator id>` &
// `?limit=`). It reads the background scan cache — no per-request compute. The screener
// is a Pro feature: when INDICATORS_PAYWALL_ENABLED and the viewer is not Pro it returns
// an empty, paywall_locked result (a HARD lock, not a teaser — screening is Pro-only per
// the plan). Flag off → available to everyone (current-behavior-safe).
func (s *Server) getScreenSignals(w http.ResponseWriter, r *http.Request) {
	if s.signalScan == nil {
		writeJSON(w, http.StatusNotFound, errBody("screener unavailable"))
		return
	}
	if s.indicatorsPaywallEnabled {
		u, _ := auth.UserFrom(r.Context())
		if s.tierOf(r.Context(), u.ID) != tierPro {
			writeJSON(w, http.StatusOK, screenSignalsResp{Results: []indicators.SignalMatch{}, PaywallLocked: true})
			return
		}
	}
	q := r.URL.Query()
	matches, at := s.signalScan.Screen(indicators.SignalScreen{
		Direction: strings.ToLower(strings.TrimSpace(q.Get("direction"))),
		SignalID:  strings.TrimSpace(q.Get("signal")),
	})
	lim := queryLimit(r, 100)
	if lim > 200 {
		lim = 200
	}
	if lim > 0 && len(matches) > lim {
		matches = matches[:lim]
	}
	if matches == nil {
		matches = []indicators.SignalMatch{}
	}
	out := screenSignalsResp{Count: len(matches), Results: matches}
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
