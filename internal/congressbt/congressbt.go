// Package congressbt runs a conservative, transparent follow-trade SIMULATION
// over one congressional member's disclosed Periodic Transaction Report (PTR)
// trades. It is a historical replay, NOT a real trading strategy, NOT advice,
// and NOT a claim of realized returns.
//
// # Methodology (intentionally simple, conservative, fully disclosed)
//
//   - Each disclosed BUY (PTR "purchase") opens an equal-weight virtual position
//     in that ticker at the DISCLOSURE-DATE closing price (TxDate). Real trades
//     happen up to ~45 days before disclosure, so entering at the public
//     disclosure date is the price a follower could actually have paid — a
//     deliberately pessimistic, replicable entry, never the (unknown) execution
//     price.
//   - A position is held until today, OR closed at the disclosure-date close of
//     the member's first SELL ("sale") in the same ticker after the buy. Multiple
//     buys in one ticker average in (still equal-weight per buy); the first
//     subsequent sell flattens the whole ticker (we don't model partial lots —
//     the disclosed amount is only a range, so share counts are unknowable).
//   - Tickers with no usable price history are SKIPPED and counted in coverage
//     (TradesSkipped), never guessed.
//   - The benchmark buys the same dollar, equal-weight, into SPY on each buy date
//     and holds to today (SPY positions are never sold — a pure buy-and-hold
//     baseline), so the comparison is "this member's picks vs. just buying the
//     index on the same days".
//
// All returns are equal-weighted, gross of fees/taxes/slippage, and based on
// daily closes only. The output is a simulation for education, not a performance
// record.
package congressbt

import (
	"sort"
	"strings"
	"time"

	"github.com/wombow-ai/tickwind/internal/congress/ptr"
	"github.com/wombow-ai/tickwind/internal/store"
)

// spyTicker is the benchmark symbol — a plain S&P 500 ETF, fetched through the
// same daily-close source as any other ticker.
const spyTicker = "SPY"

// minCurvePoints is the floor on member buy legs needed to bother producing an
// equity curve / headline number. Below this the result is marked Insufficient.
const minCurvePoints = 1

// Point is one dated sample on the simulated equity curves: the cumulative
// equal-weight return (percent) of the follow-trade portfolio and of the SPY
// benchmark, both indexed to 0% at the window start.
type Point struct {
	Date      string  `json:"date"`       // YYYY-MM-DD
	MemberPct float64 `json:"member_pct"` // cumulative follow-trade return, %
	SpyPct    float64 `json:"spy_pct"`    // cumulative SPY buy-and-hold return, %
}

// Backtest is the result of the follow-trade simulation for one member.
type Backtest struct {
	// Insufficient is true when there isn't enough priced buy history to simulate
	// (no disclosed buys, or none with usable prices). When true the percentages
	// and curve are zero/empty and the frontend shows a "not enough data" notice.
	Insufficient bool `json:"insufficient"`

	MemberReturnPct float64 `json:"member_return_pct"` // final equal-weight follow-trade return, %
	SpyReturnPct    float64 `json:"spy_return_pct"`    // final SPY buy-and-hold return, %

	WindowStart string `json:"window_start"` // YYYY-MM-DD of the earliest simulated buy ("" if none)
	WindowEnd   string `json:"window_end"`   // YYYY-MM-DD of the last data point ("" if none)
	WindowDays  int    `json:"window_days"`  // calendar days from WindowStart to WindowEnd

	TradesUsed    int      `json:"trades_used"`    // buy legs that priced and entered the simulation
	TradesSkipped int      `json:"trades_skipped"` // buy legs dropped for missing price history
	Tickers       []string `json:"tickers"`        // distinct tickers that entered the simulation (sorted)

	Curve []Point `json:"curve"` // dated equity curve (member vs SPY), oldest first
}

// CloseFn returns the daily closing-price candles for a ticker, oldest first
// (the shape produced by store/BarSource DailyCandles). It returns nil/empty
// when no history is available. Injected by the caller so this package is pure
// (no network, no clock beyond `now`).
type CloseFn func(ticker string) []store.Candle

// position tracks one ticker's open follow-trade legs while replaying buys/sells.
type position struct {
	// legs are the entry prices (disclosure-date close) of each still-open buy.
	legs []float64
}

// closedLeg is a sold follow-trade leg: its entry close and its exit close.
type closedLeg struct {
	entry, exit float64
}

// Run replays a member's transactions into the follow-trade simulation as of
// `now`. closes injects price history per ticker; it must be non-nil. The result
// is deterministic for a given (txs, closes, now). It never panics on bad data —
// unparseable or unpriced trades are skipped and counted.
func Run(txs []ptr.Transaction, closes CloseFn, now time.Time) Backtest {
	now = now.UTC()

	// Replay trades oldest-first so buys precede the sells that close them.
	ordered := append([]ptr.Transaction(nil), txs...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].TxDate.Before(ordered[j].TxDate) })

	// priceAt memoizes a ticker's close-on-or-after-a-date lookups.
	cache := map[string][]store.Candle{}
	candlesFor := func(ticker string) []store.Candle {
		tk := strings.ToUpper(strings.TrimSpace(ticker))
		if tk == "" {
			return nil
		}
		if cs, ok := cache[tk]; ok {
			return cs
		}
		cs := closes(tk)
		cache[tk] = cs
		return cs
	}

	positions := map[string]*position{} // open positions by ticker
	var realized []closedLeg            // legs already sold (entry → exit close)
	var entryDates []time.Time          // disclosure dates of every entered buy leg (member + SPY share the dates)
	tickerSet := map[string]struct{}{}  // distinct entered tickers
	var used, skipped int

	for _, tx := range ordered {
		tk := strings.ToUpper(strings.TrimSpace(tx.Ticker))
		if tk == "" || tk == spyTicker {
			continue // assets without a listed symbol can't be priced; SPY is the benchmark
		}
		switch tx.Type {
		case ptr.TxPurchase:
			entry, ok := closeOnOrAfter(candlesFor(tk), tx.TxDate)
			if !ok || entry <= 0 {
				skipped++
				continue
			}
			p := positions[tk]
			if p == nil {
				p = &position{}
				positions[tk] = p
			}
			p.legs = append(p.legs, entry)
			entryDates = append(entryDates, tx.TxDate)
			tickerSet[tk] = struct{}{}
			used++
		case ptr.TxSale:
			p := positions[tk]
			if p == nil || len(p.legs) == 0 {
				continue // a sell with no open simulated lot (bought before the window / unpriced buy)
			}
			exit, ok := closeOnOrAfter(candlesFor(tk), tx.TxDate)
			if !ok || exit <= 0 {
				continue // can't price the exit → leave the position open (held to today)
			}
			for _, entry := range p.legs {
				realized = append(realized, closedLeg{entry: entry, exit: exit})
			}
			p.legs = nil
		default:
			// exchange / unknown: not a directional buy/sell we model.
		}
	}

	if used < minCurvePoints {
		// No priced buys to simulate, but still report coverage so the frontend
		// can say "0 of N trades had usable prices".
		return Backtest{Insufficient: true, TradesSkipped: skipped}
	}

	// Window = earliest entered buy → now.
	sort.Slice(entryDates, func(i, j int) bool { return entryDates[i].Before(entryDates[j]) })
	start := entryDates[0]

	// Each entered buy leg gets one equal-weight unit of capital. Final member
	// return = average per-leg return: open legs marked to today's close, closed
	// legs locked at their sell close. SPY benchmark = one equal-weight unit per
	// SAME entry date, all held to today (buy-and-hold).
	memberRet := finalMemberReturn(realized, positions, candlesFor, now)
	spyRet := finalSpyReturn(candlesFor(spyTicker), entryDates, now)

	tickers := make([]string, 0, len(tickerSet))
	for tk := range tickerSet {
		tickers = append(tickers, tk)
	}
	sort.Strings(tickers)

	bt := Backtest{
		MemberReturnPct: round2(memberRet),
		SpyReturnPct:    round2(spyRet),
		WindowStart:     start.Format("2006-01-02"),
		WindowEnd:       now.Format("2006-01-02"),
		WindowDays:      int(now.Sub(start).Hours()/24 + 0.5),
		TradesUsed:      used,
		TradesSkipped:   skipped,
		Tickers:         tickers,
		Curve:           buildCurve(ordered, candlesFor, start, now),
	}
	return bt
}

// finalMemberReturn averages the per-leg return across all entered buy legs:
// realized legs use their exit close; still-open legs mark to the latest close.
// Returns a percentage (0 if no priced legs remain at the end).
func finalMemberReturn(realized []closedLeg, positions map[string]*position, candlesFor func(string) []store.Candle, now time.Time) float64 {
	var sum float64
	var n int
	for _, leg := range realized {
		if leg.entry > 0 {
			sum += (leg.exit - leg.entry) / leg.entry * 100
			n++
		}
	}
	for tk, p := range positions {
		if len(p.legs) == 0 {
			continue
		}
		mark, ok := lastClose(candlesFor(tk), now)
		if !ok || mark <= 0 {
			continue // an entered-then-delisted name with no recent close: drop from the mark-to-market
		}
		for _, entry := range p.legs {
			if entry > 0 {
				sum += (mark - entry) / entry * 100
				n++
			}
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// finalSpyReturn averages SPY's buy-and-hold return over each entry date (the
// follow-trade benchmark: same days, same equal weight, never sold).
func finalSpyReturn(spy []store.Candle, entryDates []time.Time, now time.Time) float64 {
	mark, ok := lastClose(spy, now)
	if !ok || mark <= 0 {
		return 0
	}
	var sum float64
	var n int
	for _, d := range entryDates {
		entry, ok := closeOnOrAfter(spy, d)
		if !ok || entry <= 0 {
			continue
		}
		sum += (mark - entry) / entry * 100
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// curveStep is the spacing of equity-curve samples (weekly keeps the payload
// small while still showing the shape over a multi-year window).
const curveStep = 7 * 24 * time.Hour

// buildCurve samples both equity curves weekly from start to now. At each sample
// date it replays every buy/sell up to that date and marks open legs to that
// date's close, so the curve is the same equal-weight average return the headline
// uses, evaluated through time. SPY is marked the same way (buy-and-hold per buy
// date). Both curves start at 0%.
func buildCurve(ordered []ptr.Transaction, candlesFor func(string) []store.Candle, start, now time.Time) []Point {
	spy := candlesFor(spyTicker)
	var pts []Point
	for d := start; !d.After(now); d = d.Add(curveStep) {
		pts = append(pts, samplePoint(ordered, candlesFor, spy, d))
	}
	// Always include the final "now" point so the curve ends at today's value.
	if len(pts) == 0 || pts[len(pts)-1].Date != now.Format("2006-01-02") {
		pts = append(pts, samplePoint(ordered, candlesFor, spy, now))
	}
	return pts
}

// samplePoint computes both cumulative returns as of date `asOf` by replaying
// trades up to that date.
func samplePoint(ordered []ptr.Transaction, candlesFor func(string) []store.Candle, spy []store.Candle, asOf time.Time) Point {
	type leg struct {
		entry float64
		open  bool
		exit  float64
	}
	byTicker := map[string][]*leg{}
	var entryDates []time.Time

	for _, tx := range ordered {
		if tx.TxDate.After(asOf) {
			break // ordered oldest-first; nothing past asOf matters yet
		}
		tk := strings.ToUpper(strings.TrimSpace(tx.Ticker))
		if tk == "" || tk == spyTicker {
			continue
		}
		switch tx.Type {
		case ptr.TxPurchase:
			entry, ok := closeOnOrAfter(candlesFor(tk), tx.TxDate)
			if !ok || entry <= 0 {
				continue
			}
			byTicker[tk] = append(byTicker[tk], &leg{entry: entry, open: true})
			entryDates = append(entryDates, tx.TxDate)
		case ptr.TxSale:
			exit, ok := closeOnOrAfter(candlesFor(tk), tx.TxDate)
			if !ok || exit <= 0 {
				continue
			}
			for _, l := range byTicker[tk] {
				if l.open {
					l.open = false
					l.exit = exit
				}
			}
		}
	}

	var sum float64
	var n int
	for tk, legs := range byTicker {
		mark, ok := lastClose(candlesFor(tk), asOf)
		for _, l := range legs {
			if l.entry <= 0 {
				continue
			}
			switch {
			case !l.open:
				sum += (l.exit - l.entry) / l.entry * 100
				n++
			case ok && mark > 0:
				sum += (mark - l.entry) / l.entry * 100
				n++
			}
		}
	}
	memberPct := 0.0
	if n > 0 {
		memberPct = sum / float64(n)
	}

	// SPY benchmark as of asOf.
	spyPct := 0.0
	if mark, ok := lastClose(spy, asOf); ok && mark > 0 {
		var ssum float64
		var sn int
		for _, d := range entryDates {
			entry, ok := closeOnOrAfter(spy, d)
			if !ok || entry <= 0 {
				continue
			}
			ssum += (mark - entry) / entry * 100
			sn++
		}
		if sn > 0 {
			spyPct = ssum / float64(sn)
		}
	}

	return Point{Date: asOf.Format("2006-01-02"), MemberPct: round2(memberPct), SpyPct: round2(spyPct)}
}

// closeOnOrAfter returns the close of the first candle whose date is on or after
// `d` (the first trading day the position could have been opened/exited at the
// disclosure date). ok=false when no candle exists on/after d (e.g. d is in the
// future of the available history). Candles must be oldest-first.
func closeOnOrAfter(candles []store.Candle, d time.Time) (float64, bool) {
	day := d.UTC().Truncate(24 * time.Hour)
	for _, c := range candles {
		if !c.Time.UTC().Truncate(24 * time.Hour).Before(day) {
			return c.Close, true
		}
	}
	return 0, false
}

// lastClose returns the close of the latest candle on or before `asOf` (the
// mark-to-market price as of that date). ok=false when no candle is at/before
// asOf. Candles must be oldest-first.
func lastClose(candles []store.Candle, asOf time.Time) (float64, bool) {
	day := asOf.UTC().Truncate(24 * time.Hour)
	var got float64
	var ok bool
	for _, c := range candles {
		if c.Time.UTC().Truncate(24 * time.Hour).After(day) {
			break
		}
		got, ok = c.Close, true
	}
	return got, ok
}

// round2 rounds to two decimal places (the percentages are display values).
func round2(v float64) float64 {
	if v >= 0 {
		return float64(int64(v*100+0.5)) / 100
	}
	return float64(int64(v*100-0.5)) / 100
}
