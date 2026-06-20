package indicators

import "sort"

// SignalScreen is a deterministic screener query over the signals layer: find the
// stocks whose computed signals match. Both filters are optional and AND-ed — an empty
// query matches every stock that has any signal at all. This is the pure core the
// signals-screen endpoint (and its background cache) build on; it invents nothing,
// it only filters already-computed deterministic signals.
type SignalScreen struct {
	// Direction limits to bullish | bearish | neutral (empty = any).
	Direction string
	// SignalID limits to a source indicator id, e.g. "technical.ma-cross" (golden/death
	// cross), "technical.rsi" (RSI extremes), "technical.macd" (empty = any). Combine
	// with Direction to narrow further (e.g. ma-cross + bullish = golden crosses only).
	SignalID string
}

// Matches reports whether a single signal satisfies the (AND-ed) query filters.
func (q SignalScreen) Matches(s Signal) bool {
	if q.Direction != "" && s.Direction != q.Direction {
		return false
	}
	if q.SignalID != "" && s.ID != q.SignalID {
		return false
	}
	return true
}

// SignalMatch is one screened stock: its ticker plus the signals that matched the
// query (so the UI can show *why* it matched — each with its traceable basis).
type SignalMatch struct {
	Ticker  string   `json:"ticker"`
	Signals []Signal `json:"signals"`
}

// ScreenSignals filters a precomputed ticker→signals map by the query and returns the
// stocks with at least one matching signal (carrying only the matching signals),
// sorted by ticker for stable, deterministic output. Pure — no compute, no I/O — so
// the caller (a background cache) owns the expensive per-ticker signal computation and
// this stays trivially testable.
func ScreenSignals(bySignal map[string][]Signal, q SignalScreen) []SignalMatch {
	out := make([]SignalMatch, 0)
	for ticker, sigs := range bySignal {
		var matched []Signal
		for _, s := range sigs {
			if q.Matches(s) {
				matched = append(matched, s)
			}
		}
		if len(matched) > 0 {
			out = append(out, SignalMatch{Ticker: ticker, Signals: matched})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ticker < out[j].Ticker })
	return out
}
