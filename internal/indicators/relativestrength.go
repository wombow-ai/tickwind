package indicators

import (
	"sort"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

// RelStrengthWindow is a ticker's price performance vs a benchmark over one trailing CALENDAR
// window, measured over the same span for both legs. Relative = StockReturn - BenchmarkReturn
// (excess return, in percentage points): positive = the stock outperformed the benchmark.
type RelStrengthWindow struct {
	Label           string  `json:"label"`            // "1M","3M","6M","1Y"
	StockReturn     float64 `json:"stock_return"`     // stock % return over the window
	BenchmarkReturn float64 `json:"benchmark_return"` // benchmark % return over the SAME span
	Relative        float64 `json:"relative"`         // stock_return - benchmark_return (excess, pp)
}

// RelativeStrength is a ticker's trailing relative performance against a market benchmark (SPY),
// computed deterministically over the daily candles. Each window is anchored by CALENDAR DATE
// (e.g. "1M" = exactly one calendar month back from the latest bar), and both legs read the
// close at-or-before the SAME two dates — so the excess return is a fair comparison and the label
// always matches the real span (a thin/gappy ticker can't get a "1M" window that secretly spans
// months; it just omits the windows its history can't honestly fill). It is a DISCLOSED
// HISTORICAL STATISTIC (like seasonality/backtest) — never a forecast, target, or advice. Every
// number is Go-computed, so it is anti-hallucination-safe.
type RelativeStrength struct {
	Benchmark string              `json:"benchmark"` // benchmark ticker the returns are measured against
	AsOf      string              `json:"as_of"`     // YYYY-MM-DD of the latest stock candle
	Windows   []RelStrengthWindow `json:"windows"`   // computable trailing windows, shortest→longest
}

// relStrengthWindows are the trailing CALENDAR windows, shortest→longest. A window is emitted only
// when the stock has a bar at-or-before its start date (real history reaching that far back) AND
// the benchmark has a close at-or-before both anchor dates — otherwise it is skipped, never
// fabricated nor mislabeled.
var relStrengthWindows = []struct {
	label    string
	addYears int
	addMonth int
}{
	{"1M", 0, -1},
	{"3M", 0, -3},
	{"6M", 0, -6},
	{"1Y", -1, 0},
}

// relStrengthAnchorTolerance is how far an anchor bar may sit before its target date. It absorbs
// normal weekend/holiday gaps (a target landing on a closed session resolves to the prior trading
// day, ≤ ~4 days) plus the month-end AddDate normalization (a day or two), while REJECTING a
// stale-gap fallback — e.g. a thinly-traded ticker whose nearest bar before "1 month ago" is
// actually months old. Without this, closeAtOrBefore would silently anchor on that stale bar and
// the "1M" label would lie about the real span; rejecting keeps it insufficient-not-wrong.
const relStrengthAnchorTolerance = 10 * 24 * time.Hour

// TickerRelStrength pairs a ticker with its computed relative strength — the RS-leaderboard cache
// retains these so it can rank named stocks by any window's excess return.
type TickerRelStrength struct {
	Ticker string
	RS     RelativeStrength
}

// RSRank is one stock's standing on the relative-strength leaderboard for a given window: its excess
// return (stock − benchmark, percentage points) plus the two legs for context. DESCRIPTIVE — a
// disclosed historical statistic, never a forecast or advice.
type RSRank struct {
	Ticker          string  `json:"ticker"`
	Relative        float64 `json:"relative"`
	StockReturn     float64 `json:"stock_return"`
	BenchmarkReturn float64 `json:"benchmark_return"`
}

// ValidRSWindow reports whether label is one of the ranked trailing windows ("1M","3M","6M","1Y").
func ValidRSWindow(label string) bool {
	for _, w := range relStrengthWindows {
		if w.label == label {
			return true
		}
	}
	return false
}

// RankRelativeStrength ranks every population member by its excess return over `window` (e.g. "3M"),
// highest→lowest (ticker tie-break, for deterministic output). A ticker that lacks the window (its
// history doesn't reach back that far) is omitted — never fabricated. Empty for an unknown window.
func RankRelativeStrength(pop []TickerRelStrength, window string) []RSRank {
	out := make([]RSRank, 0, len(pop))
	if !ValidRSWindow(window) {
		return out
	}
	for _, m := range pop {
		for _, w := range m.RS.Windows {
			if w.Label == window {
				out = append(out, RSRank{Ticker: m.Ticker, Relative: w.Relative, StockReturn: w.StockReturn, BenchmarkReturn: w.BenchmarkReturn})
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Relative != out[j].Relative {
			return out[i].Relative > out[j].Relative
		}
		return out[i].Ticker < out[j].Ticker
	})
	return out
}

// ComputeRelativeStrength measures `stock`'s trailing return against `benchmark`'s over each
// calendar window. Both series are ascending (oldest→newest). The end anchor is the latest stock
// bar; the start anchor for each window is that date shifted back by the window's calendar length,
// and BOTH legs use their bar at-or-before the start/end anchor dates (so a calendar gap in either
// series can't skew the comparison). Every anchor bar must sit within relStrengthAnchorTolerance
// of its target date, else the window is skipped (no stale-gap mislabel). ok=false when no window
// is computable (too little stock history, or the benchmark can't freshly cover the end anchor).
func ComputeRelativeStrength(stock, benchmark []store.Candle) (RelativeStrength, bool) {
	if len(stock) < 2 || len(benchmark) < 2 {
		return RelativeStrength{}, false
	}
	endDate := stock[len(stock)-1].Time
	sEnd := stock[len(stock)-1].Close
	if sEnd <= 0 {
		return RelativeStrength{}, false
	}
	bEndC, okEnd := candleAtOrBefore(benchmark, endDate)
	if !okEnd || endDate.Sub(bEndC.Time) > relStrengthAnchorTolerance {
		return RelativeStrength{}, false // benchmark can't cover the current date freshly
	}
	bEnd := bEndC.Close

	windows := make([]RelStrengthWindow, 0, len(relStrengthWindows))
	for _, w := range relStrengthWindows {
		target := endDate.AddDate(w.addYears, w.addMonth, 0)
		sC, okS := candleAtOrBefore(stock, target)
		bC, okB := candleAtOrBefore(benchmark, target)
		if !okS || !okB {
			continue // no stock history back to `target`, or benchmark can't cover it
		}
		// Both anchors must be near `target` — a stale fallback would mislabel the span.
		if target.Sub(sC.Time) > relStrengthAnchorTolerance || target.Sub(bC.Time) > relStrengthAnchorTolerance {
			continue
		}
		stockRet := (sEnd/sC.Close - 1) * 100
		benchRet := (bEnd/bC.Close - 1) * 100
		windows = append(windows, RelStrengthWindow{
			Label:           w.label,
			StockReturn:     round2(stockRet),
			BenchmarkReturn: round2(benchRet),
			Relative:        round2(stockRet - benchRet),
		})
	}
	if len(windows) == 0 {
		return RelativeStrength{}, false
	}
	return RelativeStrength{AsOf: endDate.Format(dateOnly), Windows: windows}, true
}

const dateOnly = "2006-01-02"

// candleAtOrBefore returns the latest candle in `series` (ascending) whose calendar date is on or
// before `target` and whose close is positive, plus whether one was found. ISO date strings sort
// chronologically so a lexical compare is exact. Both the stock and benchmark candles come from
// the same daily source (alpaca.DailyOHLC), so they share a Location and the date-only key aligns
// the legs — keep that invariant if either leg's source ever changes.
func candleAtOrBefore(series []store.Candle, target time.Time) (store.Candle, bool) {
	tgt := target.Format(dateOnly)
	var best store.Candle
	found := false
	for _, c := range series {
		if c.Time.Format(dateOnly) > tgt {
			break // ascending → no later candle can be ≤ target
		}
		if c.Close > 0 {
			best = c
			found = true
		}
	}
	return best, found
}
