package indicators

import (
	"sort"

	"github.com/wombow-ai/tickwind/internal/store"
)

// SeasonStat is one calendar month's historical return statistics across all available years —
// a disclosed historical pattern, never a forecast.
type SeasonStat struct {
	Month        int     `json:"month"`         // 1..12
	AvgReturn    float64 `json:"avg_return"`    // mean month-over-month % return for this month
	MedianReturn float64 `json:"median_return"` // median % return
	WinRate      float64 `json:"win_rate"`      // fraction of years this month closed positive (0..1)
	Years        int     `json:"years"`         // number of (year, month) samples
}

// Seasonality is a ticker's month-of-year return seasonality, computed deterministically over
// its daily candles: each month's month-over-month close return, aggregated by calendar month
// across the available years. It is a DISCLOSED HISTORICAL STATISTIC (like the backtest) — never
// a prediction, target, or advice. Every number is Go-computed, so it is anti-hallucination-safe.
type Seasonality struct {
	Months   []SeasonStat `json:"months"` // calendar months (1..12) that have ≥1 sample, ascending
	FromYear int          `json:"from_year"`
	ToYear   int          `json:"to_year"`
	Samples  int          `json:"samples"` // total month-over-month samples
}

// ComputeSeasonality groups daily candles (oldest→newest) into calendar months, takes each
// month's last close, forms month-over-month returns, and aggregates them by calendar month.
// ok=false when there are too few months to be meaningful (< 2 distinct months).
func ComputeSeasonality(candles []store.Candle) (Seasonality, bool) {
	if len(candles) < 2 {
		return Seasonality{}, false
	}
	type ym struct{ y, m int }
	lastClose := map[ym]float64{}
	order := make([]ym, 0, 64)
	seen := map[ym]bool{}
	for _, c := range candles {
		y, mo, _ := c.Time.Date()
		key := ym{y, int(mo)}
		if !seen[key] {
			seen[key] = true
			order = append(order, key)
		}
		lastClose[key] = c.Close // ascending candles → the final write is the month's last close
	}
	if len(order) < 2 {
		return Seasonality{}, false
	}
	byMonth := map[int][]float64{}
	total := 0
	for i := 1; i < len(order); i++ {
		pc, cc := lastClose[order[i-1]], lastClose[order[i]]
		if pc <= 0 {
			continue
		}
		ret := (cc/pc - 1) * 100 // the move INTO order[i]'s month → attribute to that calendar month
		byMonth[order[i].m] = append(byMonth[order[i].m], ret)
		total++
	}
	if total == 0 {
		return Seasonality{}, false
	}
	months := make([]SeasonStat, 0, 12)
	for m := 1; m <= 12; m++ {
		rs := byMonth[m]
		if len(rs) == 0 {
			continue
		}
		months = append(months, SeasonStat{
			Month:        m,
			AvgReturn:    round2(meanOf(rs)),
			MedianReturn: round2(medianOf(rs)),
			WinRate:      round2(posRate(rs)),
			Years:        len(rs),
		})
	}
	return Seasonality{
		Months:   months,
		FromYear: order[0].y,
		ToYear:   order[len(order)-1].y,
		Samples:  total,
	}, true
}

func meanOf(xs []float64) float64 {
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

func medianOf(xs []float64) float64 {
	c := append([]float64(nil), xs...)
	sort.Float64s(c)
	n := len(c)
	if n%2 == 1 {
		return c[n/2]
	}
	return (c[n/2-1] + c[n/2]) / 2
}

func posRate(xs []float64) float64 {
	w := 0
	for _, x := range xs {
		if x > 0 {
			w++
		}
	}
	return float64(w) / float64(len(xs))
}
