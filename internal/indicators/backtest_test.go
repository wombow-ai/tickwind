package indicators

import (
	"testing"

	"github.com/wombow-ai/tickwind/internal/store"
)

// candlesFromCloses builds candles carrying only Close (all the backtest uses).
func candlesFromCloses(closes []float64) []store.Candle {
	out := make([]store.Candle, len(closes))
	for i, c := range closes {
		out[i] = store.Candle{Close: c}
	}
	return out
}

// platformThenTrend builds `flat` bars at `base`, then `trend` bars stepping by `step`
// — a regime change that forces an SMA50×SMA200 cross during the trend leg.
func platformThenTrend(flat int, base float64, trend int, step float64) []float64 {
	out := make([]float64, 0, flat+trend)
	for i := 0; i < flat; i++ {
		out = append(out, base)
	}
	v := base
	for i := 0; i < trend; i++ {
		v += step
		out = append(out, v)
	}
	return out
}

func TestBacktestSignalGuards(t *testing.T) {
	long := candlesFromCloses(platformThenTrend(200, 100, 120, 1)) // 320 bars
	if _, ok := BacktestSignal(long, "not_a_rule", 10); ok {
		t.Error("unknown rule should be ok=false")
	}
	if _, ok := BacktestSignal(long, "golden_cross", 0); ok {
		t.Error("horizon 0 should be ok=false")
	}
	if _, ok := BacktestSignal(candlesFromCloses(platformThenTrend(50, 100, 10, 1)), "golden_cross", 10); ok {
		t.Error("too-short history should be ok=false")
	}
	if !BacktestableRule("golden_cross") || BacktestableRule("price_above") {
		t.Error("BacktestableRule misclassified")
	}
}

func TestBacktestGoldenCross(t *testing.T) {
	// Flat then a steady uptrend → SMA50 crosses above SMA200 once, and price keeps
	// rising, so every post-cross forward window is a win.
	candles := candlesFromCloses(platformThenTrend(200, 100, 120, 1)) // ends at 220
	res, ok := BacktestSignal(candles, "golden_cross", 10)
	if !ok {
		t.Fatal("expected ok on a 320-bar series")
	}
	if res.Trades < 1 {
		t.Fatalf("golden cross should fire at least once on this series, got %+v", res)
	}
	if res.WinRate != 1 {
		t.Errorf("uptrend golden crosses should all win, win_rate = %v", res.WinRate)
	}
	if res.AvgReturn <= 0 {
		t.Errorf("avg forward return should be positive, got %v", res.AvgReturn)
	}
	if res.Baseline <= 0 {
		t.Errorf("buy-and-hold baseline should be positive on an uptrend, got %v", res.Baseline)
	}
}

func TestBacktestDeathCross(t *testing.T) {
	// Flat then a steady downtrend → SMA50 crosses below SMA200, price keeps falling.
	candles := candlesFromCloses(platformThenTrend(200, 200, 120, -1)) // ends at 80
	res, ok := BacktestSignal(candles, "death_cross", 10)
	if !ok {
		t.Fatal("expected ok")
	}
	if res.Trades < 1 {
		t.Fatalf("death cross should fire at least once, got %+v", res)
	}
	if res.AvgReturn >= 0 {
		t.Errorf("avg forward return should be negative in a downtrend, got %v", res.AvgReturn)
	}
	if res.Baseline >= 0 {
		t.Errorf("baseline should be negative on a downtrend, got %v", res.Baseline)
	}
}

func TestBacktestBaseline(t *testing.T) {
	// A controlled span: first close 100, last 150 → buy-and-hold baseline = +50%.
	closes := make([]float64, 300)
	for i := range closes {
		closes[i] = 100 + float64(i)*(50.0/299.0)
	}
	res, ok := BacktestSignal(candlesFromCloses(closes), "golden_cross", 10)
	if !ok {
		t.Fatal("expected ok")
	}
	if res.Baseline != 50 {
		t.Errorf("baseline = %v, want 50.00 (100 → 150)", res.Baseline)
	}
}
