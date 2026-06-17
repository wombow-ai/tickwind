// Package sentiment computes a tide-style Fear & Greed index from a handful of
// optional market-mood inputs, modelled on CNN's Fear & Greed methodology
// (https://www.cnn.com/markets/fear-and-greed). Every component maps onto a
// common 0–100 greed scale where a higher score means more greed and a lower
// score means more fear; the headline score is the equal-weighted average of
// whichever components were supplied.
//
// The package is pure computation: it never fetches data. Callers feed in the
// raw component values from whatever upstream sources they already have (see the
// Inputs fields for the intended source per component) and receive a Result. A
// concurrency-safe atomic-snapshot Cache (see cache.go) holds the latest Result
// plus a history of daily points for charting and cold-start backfill.
package sentiment

import (
	"fmt"
	"math"
)

// Inputs holds the raw value of each Fear & Greed component. Every field is
// optional: a nil pointer (or, for the breadth/momentum integer pairs, a zero
// denominator) means the component is skipped and the headline score is the
// equal-weighted average of the remaining components. This lets the caller wire
// up only the sources it currently has and add more over time without changing
// the weighting logic.
type Inputs struct {
	// VIX is the CBOE Volatility Index level. Lower volatility reads as greed.
	// (Currently always nil — no keyless redistribution-safe ^VIX feed is wired
	// since the gray Yahoo source was removed — so Compute re-weights around it.)
	VIX *float64
	// PutCallRatio is the equity put/call ratio (e.g. from internal/cboe). A
	// lower ratio (more calls than puts) reads as greed.
	PutCallRatio *float64
	// Advancers and Decliners are the count of rising versus falling issues,
	// used together for the market-breadth component. Both are needed (a zero
	// sum skips the component).
	Advancers *int
	// Decliners — see Advancers.
	Decliners *int
	// NewHighs and NewLows are the count of issues at 52-week highs versus lows,
	// used together for the momentum / high-low component. Both are needed (a
	// zero sum skips the component).
	NewHighs *int
	// NewLows — see NewHighs.
	NewLows *int
	// Heat is a social-media buzz intensity already normalised to 0–100 by the
	// caller (e.g. derived from ApeWisdom mention momentum). It is treated
	// directly as a greed score.
	Heat *float64
	// ShortPct is the average short interest as a percent of volume (e.g. from
	// internal/finrashvol). Higher short interest reads as fear.
	ShortPct *float64
}

// Component is one scored contributor to the headline index. Score is on the
// same 0–100 greed scale as the headline. Note is a short human-readable
// description of the raw value and how it mapped.
type Component struct {
	// Name identifies the component (e.g. "VIX", "Put/Call Ratio").
	Name string
	// Score is the component's contribution on the 0–100 greed scale.
	Score int
	// Note is a short description of the raw value behind the score.
	Note string
}

// Result is the computed Fear & Greed index for one set of Inputs.
type Result struct {
	// Score is the headline index, 0 (extreme fear) to 100 (extreme greed).
	Score int
	// Label is the English sentiment band for Score (see classify).
	Label string
	// LabelZh is the Chinese sentiment band for Score.
	LabelZh string
	// Components lists each component that was scored, in a stable order.
	Components []Component
	// Available is the number of components that participated (len(Components)).
	Available int
}

// Compute scores every supplied component and returns the equal-weighted Result.
// When no component is available it returns a Neutral result with Score 50 and
// Available 0, so callers always get a well-formed value.
//
// Per-component mappings (all clamped to [0,100]):
//   - VIX: lower is greedier. Linearly maps [12,40] reversed to [90,10].
//   - Put/Call Ratio: lower is greedier. Linearly maps [0.7,1.2] reversed to [85,15].
//   - Breadth: Advancers/(Advancers+Decliners)*100, already a greed share.
//   - Momentum (high/low): NewHighs/(NewHighs+NewLows)*100, already a greed share.
//   - Heat: used directly as the greed score (caller pre-normalises to 0–100).
//   - Short %: daily short VOLUME is structurally ~48%, so the range is centred
//     on that baseline — [40,56] reversed to [65,35] (≈48%→50) — and reads as a
//     deviation from the norm rather than treating all shorting as fear.
func Compute(in Inputs) Result {
	comps := make([]Component, 0, 6)

	if in.VIX != nil {
		comps = append(comps, Component{
			Name:  "VIX",
			Score: scoreVIX(*in.VIX),
			Note:  formatNote("VIX %.1f", *in.VIX),
		})
	}
	if in.PutCallRatio != nil {
		comps = append(comps, Component{
			Name:  "Put/Call Ratio",
			Score: scorePutCall(*in.PutCallRatio),
			Note:  formatNote("put/call %.2f", *in.PutCallRatio),
		})
	}
	if in.Advancers != nil && in.Decliners != nil {
		if total := *in.Advancers + *in.Decliners; total > 0 {
			comps = append(comps, Component{
				Name:  "Market Breadth",
				Score: clampInt(roundHalfUp(float64(*in.Advancers) / float64(total) * 100)),
				Note:  formatNote("%d advancing vs %d declining", *in.Advancers, *in.Decliners),
			})
		}
	}
	if in.NewHighs != nil && in.NewLows != nil {
		if total := *in.NewHighs + *in.NewLows; total > 0 {
			comps = append(comps, Component{
				Name:  "Momentum",
				Score: clampInt(roundHalfUp(float64(*in.NewHighs) / float64(total) * 100)),
				Note:  formatNote("%d new highs vs %d new lows", *in.NewHighs, *in.NewLows),
			})
		}
	}
	if in.Heat != nil {
		comps = append(comps, Component{
			Name:  "Social Heat",
			Score: clampInt(roundHalfUp(*in.Heat)),
			Note:  formatNote("buzz intensity %.0f/100", *in.Heat),
		})
	}
	if in.ShortPct != nil {
		comps = append(comps, Component{
			Name:  "Short Interest",
			Score: scoreShortPct(*in.ShortPct),
			Note:  formatNote("short %.1f%% of volume", *in.ShortPct),
		})
	}

	if len(comps) == 0 {
		return Result{
			Score:      50,
			Label:      "Neutral",
			LabelZh:    "中性",
			Components: []Component{},
			Available:  0,
		}
	}

	sum := 0
	for _, c := range comps {
		sum += c.Score
	}
	score := roundHalfUp(float64(sum) / float64(len(comps)))
	label, labelZh := classify(score)
	return Result{
		Score:      score,
		Label:      label,
		LabelZh:    labelZh,
		Components: comps,
		Available:  len(comps),
	}
}

// scoreVIX maps the VIX level onto the greed scale: lower volatility is greedier.
// [12,40] is reversed onto [90,10] and clamped to [0,100].
func scoreVIX(vix float64) int {
	return clampInt(roundHalfUp(linMap(vix, 12, 40, 90, 10)))
}

// scorePutCall maps the equity put/call ratio onto the greed scale: a lower
// ratio is greedier. [0.7,1.2] is reversed onto [85,15] and clamped to [0,100].
func scorePutCall(r float64) int {
	return clampInt(roundHalfUp(linMap(r, 0.7, 1.2, 85, 15)))
}

// scoreShortPct maps FINRA daily short-VOLUME (% of the day's volume) onto the
// greed scale: elevated short selling is more fearful. The key calibration fact
// is that daily short volume is STRUCTURALLY high — market-wide it sits around
// ~45–50% (it counts every short-marked print, including market-maker hedging,
// not directional short positioning), so the neutral baseline is ~48%, NOT 0.
// The range is therefore centred on the baseline: [40,56] is reversed onto
// [65,35] (≈48%→50 neutral) and clamped to [0,100], so the component reads as a
// gentle DEVIATION from the structural norm rather than treating all shorting as
// fear. (Earlier it mapped [10,50]→[80,10], which scored a normal ~48% as ~13 —
// extreme fear — and persistently biased the headline index toward Fear.)
// TODO: a trailing self-baseline (deviation vs the symbol's/market's own recent
// short-volume average) would be more robust than a fixed centre; revisit when
// short-volume history is retained.
func scoreShortPct(pct float64) int {
	return clampInt(roundHalfUp(linMap(pct, 40, 56, 65, 35)))
}

// linMap linearly maps x from the input range [inLo,inHi] to the output range
// [outLo,outHi]. The output range may be inverted (outLo > outHi) to express a
// reversed relationship. The result is not clamped; callers clamp as needed.
func linMap(x, inLo, inHi, outLo, outHi float64) float64 {
	if inHi == inLo {
		return outLo
	}
	t := (x - inLo) / (inHi - inLo)
	return outLo + t*(outHi-outLo)
}

// classify returns the English and Chinese sentiment band for a 0–100 score.
// Bands (lower bound inclusive, upper bound exclusive except the top band):
// 0–25 Extreme Fear, 25–45 Fear, 45–55 Neutral, 55–75 Greed, 75–100 Extreme Greed.
func classify(score int) (label, labelZh string) {
	switch {
	case score < 25:
		return "Extreme Fear", "极度恐惧"
	case score < 45:
		return "Fear", "恐惧"
	case score < 55:
		return "Neutral", "中性"
	case score < 75:
		return "Greed", "贪婪"
	default:
		return "Extreme Greed", "极度贪婪"
	}
}

// clampInt constrains v to the [0,100] greed scale.
func clampInt(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// roundHalfUp rounds x to the nearest integer, rounding halves away from zero.
func roundHalfUp(x float64) int {
	return int(math.Round(x))
}

// formatNote is a thin wrapper over fmt.Sprintf for building Component.Note,
// kept as a single point so note formatting stays consistent.
func formatNote(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
