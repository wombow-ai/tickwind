package indicators

import (
	"fmt"
	"sort"
	"strings"
)

// Signal is a deterministic, rule-based reading over a Go-computed indicator — the
// paid "signals" layer (the LuxAlgo/looknode hook, done WITHOUT an LLM). It is NOT
// advice, a price target, or a rating: it states a disclosed technical condition, and
// Basis cites the source indicator + value + threshold so every signal is traceable
// to a number Go computed. (Anti-hallucination: a signal is a rule, never invented.)
type Signal struct {
	ID        string `json:"id"`        // source indicator id, e.g. "technical.rsi"
	Label     string `json:"label"`     // human label, e.g. "RSI oversold"
	Direction string `json:"direction"` // "bullish" | "bearish" | "neutral"
	Basis     string `json:"basis"`     // traceability, e.g. "RSI 27.4 < 30"
}

// Signal direction constants.
const (
	DirBullish = "bullish"
	DirBearish = "bearish"
	DirNeutral = "neutral"
)

// Signals derives the deterministic POSTURE signals from an already-computed indicator
// set (latest values only — no price context, no series). Pure function over Go-owned
// numbers: no LLM, no new data, no advice. Event signals (crosses) that need the prior
// bar / extra moving averages are a later increment. Only `ok` indicators contribute.
func Signals(res StockIndicatorsResult) []Signal {
	var out []Signal
	// Reference price for price-vs-MA / Bollinger rules (the latest close). When it
	// is absent those rules are simply skipped — never guessed.
	var price float64
	hasPrice := res.Price != nil && *res.Price > 0
	if hasPrice {
		price = *res.Price
	}
	for _, si := range res.Indicators {
		if si.Status != StatusOK || si.Value == nil {
			continue
		}
		v := *si.Value
		switch si.ID {
		case "technical.rsi":
			switch {
			case v < 30:
				out = append(out, Signal{si.ID, "RSI oversold", DirBullish, fmt.Sprintf("RSI %.1f < 30", v)})
			case v > 70:
				out = append(out, Signal{si.ID, "RSI overbought", DirBearish, fmt.Sprintf("RSI %.1f > 70", v)})
			}
		case "technical.stochastic-kdj":
			k := si.Extra["k"]
			switch {
			case k > 80:
				out = append(out, Signal{si.ID, "Stochastic overbought", DirBearish, fmt.Sprintf("KDJ %%K %.1f > 80", k)})
			case k < 20:
				out = append(out, Signal{si.ID, "Stochastic oversold", DirBullish, fmt.Sprintf("KDJ %%K %.1f < 20", k)})
			}
		case "technical.macd":
			dea, hist := si.Extra["signal"], si.Extra["hist"]
			prevHist, hasPrev := si.Extra["prev_hist"]
			switch {
			// Event signals (a histogram sign flip = a MACD cross) take precedence over
			// the standing posture — a cross is the more actionable, less-redundant read.
			case hasPrev && prevHist <= 0 && hist > 0:
				out = append(out, Signal{si.ID, "MACD bullish cross", DirBullish, fmt.Sprintf("MACD histogram %.3f → %.3f (crossed up)", prevHist, hist)})
			case hasPrev && prevHist >= 0 && hist < 0:
				out = append(out, Signal{si.ID, "MACD bearish cross", DirBearish, fmt.Sprintf("MACD histogram %.3f → %.3f (crossed down)", prevHist, hist)})
			case v > dea && hist > 0:
				out = append(out, Signal{si.ID, "MACD above signal", DirBullish, fmt.Sprintf("DIF %.3f > DEA %.3f", v, dea)})
			case v < dea && hist < 0:
				out = append(out, Signal{si.ID, "MACD below signal", DirBearish, fmt.Sprintf("DIF %.3f < DEA %.3f", v, dea)})
			}
		case "technical.sma-ma":
			if hasPrice && v > 0 {
				switch {
				case price > v:
					out = append(out, Signal{si.ID, "Price above SMA", DirBullish, fmt.Sprintf("Price %.2f > SMA %.2f", price, v)})
				case price < v:
					out = append(out, Signal{si.ID, "Price below SMA", DirBearish, fmt.Sprintf("Price %.2f < SMA %.2f", price, v)})
				}
			}
		case "technical.ema":
			if hasPrice && v > 0 {
				switch {
				case price > v:
					out = append(out, Signal{si.ID, "Price above EMA", DirBullish, fmt.Sprintf("Price %.2f > EMA %.2f", price, v)})
				case price < v:
					out = append(out, Signal{si.ID, "Price below EMA", DirBearish, fmt.Sprintf("Price %.2f < EMA %.2f", price, v)})
				}
			}
		case "technical.boll":
			// Band breaches only — the middle band is SMA(20), already covered by the
			// price-vs-SMA rule. Direction is NEUTRAL: a breach is an ambiguous, disclosed
			// "stretched" condition (overbought/oversold vs breakout), not a directional
			// call — we surface the fact, not an interpretation.
			if hasPrice {
				upper, lower := si.Extra["upper"], si.Extra["lower"]
				switch {
				case upper > 0 && price > upper:
					out = append(out, Signal{si.ID, "Price above upper Bollinger band", DirNeutral, fmt.Sprintf("Price %.2f > upper band %.2f", price, upper)})
				case lower > 0 && price < lower:
					out = append(out, Signal{si.ID, "Price below lower Bollinger band", DirNeutral, fmt.Sprintf("Price %.2f < lower band %.2f", price, lower)})
				}
			}
		}
	}
	out = append(out, crossSignals(res)...)
	// Order by salience so the most meaningful signals lead — and, crucially, so the
	// free teaser (first N) surfaces events/extremes rather than the always-on trend
	// posture. Stable, so within a tier the emission order (catalog order) is kept.
	sort.SliceStable(out, func(i, j int) bool { return salienceOf(out[i]) < salienceOf(out[j]) })
	return out
}

// salienceOf ranks a signal so the most actionable lead: 0 = an EVENT (a cross / a
// transition that just happened), 1 = an EXTREME (overbought/oversold, a band breach
// — notable, fires only at the edges), 2 = an always-on TREND posture (price vs a
// moving average, MACD standing above/below its signal — true most of the time, so
// least newsworthy). Lower sorts first.
func salienceOf(s Signal) int {
	switch {
	case strings.Contains(s.Label, "cross"): // MACD bullish/bearish cross, Golden/Death cross
		return 0
	case s.ID == "technical.rsi" || s.ID == "technical.stochastic-kdj" || s.ID == "technical.boll":
		return 1
	default: // price-vs-SMA/EMA, MACD above/below signal
		return 2
	}
}

// maCrossID is the synthetic signal id for the moving-average cross event (it has no
// catalog indicator of its own — it is derived from the close series in this layer).
const maCrossID = "technical.ma-cross"

// crossSignals derives series-based EVENT signals from the close series: the
// golden / death cross (SMA50 × SMA200). Deterministic — it recomputes both moving
// averages for the current and the previous bar (the series minus its last close) and
// emits only on an actual crossover. Needs ≥201 closes (200 for SMA200 + 1 for the
// prior bar); returns nil otherwise (never fabricated).
func crossSignals(res StockIndicatorsResult) []Signal {
	c := res.Closes
	if len(c) < 201 {
		return nil
	}
	prev := c[:len(c)-1]
	cur50, ok1 := sma(c, 50)
	cur200, ok2 := sma(c, 200)
	prev50, ok3 := sma(prev, 50)
	prev200, ok4 := sma(prev, 200)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return nil
	}
	switch {
	case prev50 <= prev200 && cur50 > cur200:
		return []Signal{{maCrossID, "Golden cross (SMA50 × SMA200)", DirBullish, fmt.Sprintf("SMA50 %.2f crossed above SMA200 %.2f", cur50, cur200)}}
	case prev50 >= prev200 && cur50 < cur200:
		return []Signal{{maCrossID, "Death cross (SMA50 × SMA200)", DirBearish, fmt.Sprintf("SMA50 %.2f crossed below SMA200 %.2f", cur50, cur200)}}
	}
	return nil
}
