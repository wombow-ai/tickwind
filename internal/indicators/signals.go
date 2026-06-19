package indicators

import "fmt"

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
			switch {
			case v > dea && hist > 0:
				out = append(out, Signal{si.ID, "MACD above signal", DirBullish, fmt.Sprintf("DIF %.3f > DEA %.3f", v, dea)})
			case v < dea && hist < 0:
				out = append(out, Signal{si.ID, "MACD below signal", DirBearish, fmt.Sprintf("DIF %.3f < DEA %.3f", v, dea)})
			}
		}
	}
	return out
}
