package indicators

import (
	"math"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

const (
	// maxEarningsEvents caps how many individual events the response lists (the aggregates are
	// computed over ALL bracketed events; only the per-event display list is capped).
	maxEarningsEvents = 12
	// minEarningsSamples is the floor below which the stat is withheld (ok=false): fewer than ~a
	// year of quarterly reactions makes an "avg move" / "up-rate" misleading (an up-rate from 1–2
	// samples is a coin-flip dressed as a statistic). Insufficient-not-wrong.
	minEarningsSamples = 4
	// earningsWindowMax bounds the ~2-session announcement window: if a halt / data gap / non-
	// trading-date filing stretches the before→after span beyond this, the event isn't comparable
	// to the others (it would mix in extra sessions), so it is skipped rather than averaged in.
	earningsWindowMax = 8 * 24 * time.Hour
)

// EarningsEvent is one past earnings announcement and the stock's price reaction around it.
type EarningsEvent struct {
	Date string  `json:"date"` // YYYY-MM-DD — the 8-K item 2.02 (results) filing date
	Move float64 `json:"move"` // ~2-session close-to-close % spanning the announcement (see ComputeEarningsReaction)
}

// EarningsReaction summarizes how a stock has historically moved around its earnings
// announcements — a DISCLOSED HISTORICAL STATISTIC (like seasonality/relative-strength), never a
// forecast or advice. Every number is Go-computed from the public daily candles + SEC 8-K (item
// 2.02) filing dates, so it is anti-hallucination-safe.
type EarningsReaction struct {
	Events     []EarningsEvent `json:"events"`       // most recent first, capped at maxEarningsEvents
	AvgMove    float64         `json:"avg_move"`     // mean signed reaction % across all events
	AvgAbsMove float64         `json:"avg_abs_move"` // mean magnitude % — the typical size of the ~2-session move
	UpRate     float64         `json:"up_rate"`      // fraction of events with a positive reaction (0..1)
	Samples    int             `json:"samples"`      // number of events with a measurable reaction (>= minEarningsSamples)
}

// ReactionSummary is the compact, calendar-facing slice of an earnings reaction: the typical move
// magnitude, up-rate, and sample count — WITHOUT the full per-event series (the earnings-calendar
// badge needs only the aggregate, and a slim payload). Every number is Go-computed.
type ReactionSummary struct {
	AvgAbsMove float64 `json:"avg_abs_move"`
	UpRate     float64 `json:"up_rate"`
	Samples    int     `json:"samples"`
}

// Summary reduces a full EarningsReaction to its calendar-facing aggregate.
func (e EarningsReaction) Summary() ReactionSummary {
	return ReactionSummary{AvgAbsMove: e.AvgAbsMove, UpRate: e.UpRate, Samples: e.Samples}
}

// ComputeEarningsReaction measures the price reaction around each earnings date. To be robust to
// the announcement's timing (before-open vs after-close, which a filing date alone can't
// disambiguate), the reaction spans the announcement: the close of the last trading day STRICTLY
// BEFORE the date to the close of the first trading day STRICTLY AFTER it (a ~2-session window
// that captures the move whether it lands on the announcement day or the next; it therefore
// includes a little adjacent-session drift — it is the move AROUND the announcement, not a clean
// single-day gap). A date the candles don't bracket, or whose window a halt/gap stretches beyond
// earningsWindowMax, is skipped — never fabricated. `candles` is ascending (oldest→newest).
// ok=false when fewer than minEarningsSamples events are measurable (insufficient-not-wrong).
func ComputeEarningsReaction(earningsDates []time.Time, candles []store.Candle) (EarningsReaction, bool) {
	if len(candles) < 2 || len(earningsDates) == 0 {
		return EarningsReaction{}, false
	}
	events := make([]EarningsEvent, 0, len(earningsDates))
	var sum, sumAbs float64
	up := 0
	for _, d := range earningsDates {
		bC, okB := candleStrictlyBefore(candles, d)
		aC, okA := candleStrictlyAfter(candles, d)
		if !okB || !okA || bC.Close <= 0 || aC.Close <= 0 {
			continue // candles don't bracket this announcement → skip (insufficient-not-wrong)
		}
		if aC.Time.Sub(bC.Time) > earningsWindowMax {
			continue // a halt/gap stretched the window → not comparable to the other events → skip
		}
		move := (aC.Close/bC.Close - 1) * 100
		events = append(events, EarningsEvent{Date: d.Format(dateOnly), Move: round2(move)})
		sum += move
		sumAbs += math.Abs(move)
		if move > 0 {
			up++
		}
	}
	n := len(events)
	if n < minEarningsSamples {
		return EarningsReaction{}, false
	}
	display := events // earningsDates arrive newest-first, so events already are too
	if len(display) > maxEarningsEvents {
		display = display[:maxEarningsEvents]
	}
	return EarningsReaction{
		Events:     display,
		AvgMove:    round2(sum / float64(n)),
		AvgAbsMove: round2(sumAbs / float64(n)),
		UpRate:     round2(float64(up) / float64(n)),
		Samples:    n,
	}, true
}

// candleStrictlyBefore returns the latest candle (ascending) whose calendar date is strictly
// before d and whose close is positive, and whether one was found.
func candleStrictlyBefore(candles []store.Candle, d time.Time) (store.Candle, bool) {
	ds := d.Format(dateOnly)
	var best store.Candle
	found := false
	for _, c := range candles {
		if c.Time.Format(dateOnly) >= ds {
			break // ascending → reached the date or later
		}
		if c.Close > 0 {
			best = c
			found = true
		}
	}
	return best, found
}

// candleStrictlyAfter returns the earliest candle (ascending) whose calendar date is strictly
// after d and whose close is positive, and whether one was found.
func candleStrictlyAfter(candles []store.Candle, d time.Time) (store.Candle, bool) {
	ds := d.Format(dateOnly)
	for _, c := range candles {
		if c.Time.Format(dateOnly) > ds && c.Close > 0 {
			return c, true
		}
	}
	return store.Candle{}, false
}
