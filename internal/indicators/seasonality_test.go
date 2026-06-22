package indicators

import (
	"math"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

// monthlyCandles builds one candle per month starting at Jan of startYear, applying the given
// month-over-month returns (len = months-1; closes start at 100).
func monthlyCandles(startYear int, monthlyReturns []float64) []store.Candle {
	closes := []float64{100}
	for _, r := range monthlyReturns {
		closes = append(closes, closes[len(closes)-1]*(1+r))
	}
	base := time.Date(startYear, 1, 1, 0, 0, 0, 0, time.UTC)
	cs := make([]store.Candle, len(closes))
	for i, c := range closes {
		t := base.AddDate(0, i, 0)
		cs[i] = store.Candle{Time: t, Open: c, High: c, Low: c, Close: c, Volume: 1000}
	}
	return cs
}

func TestComputeSeasonality(t *testing.T) {
	// 36 months (Jan 2021 .. Dec 2023). Every JANUARY return is +5%, every other month +1%.
	returns := make([]float64, 35) // 35 MoM steps for 36 candles
	for i := range returns {
		// returns[i] is the move INTO candle index i+1. Candle index = i+1; its month = (i+1)%12;
		// (i+1)%12 == 0 → January.
		if (i+1)%12 == 0 {
			returns[i] = 0.05
		} else {
			returns[i] = 0.01
		}
	}
	candles := monthlyCandles(2021, returns)

	s, ok := ComputeSeasonality(candles)
	if !ok {
		t.Fatal("seasonality not ok")
	}
	if s.FromYear != 2021 || s.ToYear != 2023 {
		t.Errorf("year range = %d..%d, want 2021..2023", s.FromYear, s.ToYear)
	}
	if s.Samples != 35 {
		t.Errorf("samples = %d, want 35", s.Samples)
	}
	byMonth := map[int]SeasonStat{}
	for _, m := range s.Months {
		byMonth[m.Month] = m
	}
	// January: two samples (2022, 2023), each +5%, always positive.
	jan := byMonth[1]
	if jan.Years != 2 || math.Abs(jan.AvgReturn-5.0) > 1e-6 || math.Abs(jan.WinRate-1.0) > 1e-6 {
		t.Errorf("January = %+v, want Years 2, Avg 5.0, WinRate 1.0", jan)
	}
	// February: three samples (2021/22/23), each +1%.
	feb := byMonth[2]
	if feb.Years != 3 || math.Abs(feb.AvgReturn-1.0) > 1e-6 {
		t.Errorf("February = %+v, want Years 3, Avg 1.0", feb)
	}
	// All 12 calendar months should be present (each occurs ≥1× over 3 years).
	if len(s.Months) != 12 {
		t.Errorf("months covered = %d, want 12", len(s.Months))
	}
}

func TestComputeSeasonality_Guards(t *testing.T) {
	if _, ok := ComputeSeasonality(nil); ok {
		t.Error("nil candles should be not ok")
	}
	if _, ok := ComputeSeasonality([]store.Candle{{Time: time.Now(), Close: 100}}); ok {
		t.Error("single candle should be not ok")
	}
}
