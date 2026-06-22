package indicators

import (
	"math"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

// histCandles builds daily candles with the given closes on consecutive dates.
func histCandles(closes []float64) []store.Candle {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	cs := make([]store.Candle, len(closes))
	for i, c := range closes {
		cs[i] = store.Candle{Time: base.AddDate(0, 0, i), Open: c, High: c + 1, Low: c - 1, Close: c, Volume: 1000}
	}
	return cs
}

func eq4(a, b float64) bool { return math.Abs(a-b) <= 1e-4 }

// The headline point of every history series must equal the single-point computeFn value —
// the chart's latest reading must match what the stock page shows (one source of truth).
func TestIndicatorHistory_LatestMatchesPointValue(t *testing.T) {
	closes := make([]float64, 80)
	for i := range closes {
		closes[i] = 100 + 10*math.Sin(float64(i)/5) + float64(i)*0.2
	}
	candles := histCandles(closes)

	latest := func(id string) float64 {
		hs, ok := IndicatorHistory(candles, id, 0)
		if !ok || len(hs.Points) == 0 {
			t.Fatalf("%s: history not ok / empty", id)
		}
		// dates strictly increasing, no NaN/Inf leaked
		for i, p := range hs.Points {
			if math.IsNaN(p.Value) || math.IsInf(p.Value, 0) {
				t.Fatalf("%s: bad value at %d", id, i)
			}
			if i > 0 && hs.Points[i-1].Date >= p.Date {
				t.Fatalf("%s: dates not strictly increasing at %d", id, i)
			}
		}
		return hs.Points[len(hs.Points)-1].Value
	}

	if smaV, _ := sma(closes, defaultSMAPeriod); !eq4(latest("technical.sma-ma"), math.Round(smaV*1e4)/1e4) {
		t.Errorf("SMA latest mismatch")
	}
	if emaV, _ := ema(closes, defaultEMAPeriod); !eq4(latest("technical.ema"), math.Round(emaV*1e4)/1e4) {
		t.Errorf("EMA latest mismatch")
	}
	if rsiV, _ := rsiWilder(closes, defaultRSIPeriod); !eq4(latest("technical.rsi"), math.Round(rsiV*1e4)/1e4) {
		t.Errorf("RSI latest mismatch")
	}
	if mv, _ := macd(closes, defaultMACDFast, defaultMACDSlow, defaultMACDSignal); !eq4(latest("technical.macd"), math.Round(mv.Line*1e4)/1e4) {
		t.Errorf("MACD line latest mismatch")
	}
	if bv, _ := bollinger(closes, defaultBollPeriod, defaultBollMult); !eq4(latest("technical.boll"), math.Round(bv.Middle*1e4)/1e4) {
		t.Errorf("BOLL middle latest mismatch")
	}

	highs := make([]float64, len(candles))
	lows := make([]float64, len(candles))
	for i, c := range candles {
		highs[i] = c.High
		lows[i] = c.Low
	}
	if av, _ := atrWilder(highs, lows, closes, defaultATRPeriod); !eq4(latest("technical.atr"), math.Round(av*1e4)/1e4) {
		t.Errorf("ATR latest mismatch")
	}
	if kv, _ := stochasticKDJ(highs, lows, closes, defaultStochN, defaultStochSlowK, defaultStochSlowD); !eq4(latest("technical.stochastic-kdj"), math.Round(kv.K*1e4)/1e4) {
		t.Errorf("KDJ %%K latest mismatch")
	}
}

// KDJ's extra D and J lines must match the point triple too.
func TestIndicatorHistory_KDJLines(t *testing.T) {
	closes := make([]float64, 90)
	for i := range closes {
		closes[i] = 40 + float64(i)*0.5 + 8*math.Sin(float64(i)/3)
	}
	candles := histCandles(closes)
	highs := make([]float64, len(candles))
	lows := make([]float64, len(candles))
	for i, c := range candles {
		highs[i] = c.High
		lows[i] = c.Low
	}
	hs, ok := IndicatorHistory(candles, "technical.stochastic-kdj", 0)
	if !ok {
		t.Fatal("kdj history not ok")
	}
	kv, _ := stochasticKDJ(highs, lows, closes, defaultStochN, defaultStochSlowK, defaultStochSlowD)
	d := hs.Lines["d"]
	j := hs.Lines["j"]
	if len(d) == 0 || len(j) == 0 {
		t.Fatal("kdj missing d/j lines")
	}
	if !eq4(hs.Points[len(hs.Points)-1].Value, math.Round(kv.K*1e4)/1e4) {
		t.Errorf("K latest mismatch")
	}
	if !eq4(d[len(d)-1].Value, math.Round(kv.D*1e4)/1e4) {
		t.Errorf("D latest mismatch")
	}
	if !eq4(j[len(j)-1].Value, math.Round(kv.J*1e4)/1e4) {
		t.Errorf("J latest mismatch")
	}
}

// The extra aligned lines (MACD signal/histogram, BOLL bands) must also match the point triple.
func TestIndicatorHistory_ExtraLinesMatch(t *testing.T) {
	closes := make([]float64, 80)
	for i := range closes {
		closes[i] = 50 + float64(i) + 5*math.Cos(float64(i)/4)
	}
	candles := histCandles(closes)

	macdHS, ok := IndicatorHistory(candles, "technical.macd", 0)
	if !ok {
		t.Fatal("macd history not ok")
	}
	mv, _ := macd(closes, defaultMACDFast, defaultMACDSlow, defaultMACDSignal)
	sig := macdHS.Lines["signal"]
	hist := macdHS.Lines["histogram"]
	if len(sig) == 0 || len(hist) == 0 {
		t.Fatal("macd missing signal/histogram lines")
	}
	if !eq4(sig[len(sig)-1].Value, math.Round(mv.Signal*1e4)/1e4) {
		t.Errorf("MACD signal latest mismatch")
	}
	if !eq4(hist[len(hist)-1].Value, math.Round(mv.Histogram*1e4)/1e4) {
		t.Errorf("MACD histogram latest mismatch")
	}

	bollHS, ok := IndicatorHistory(candles, "technical.boll", 0)
	if !ok {
		t.Fatal("boll history not ok")
	}
	bv, _ := bollinger(closes, defaultBollPeriod, defaultBollMult)
	up := bollHS.Lines["upper"]
	low := bollHS.Lines["lower"]
	if !eq4(up[len(up)-1].Value, math.Round(bv.Upper*1e4)/1e4) {
		t.Errorf("BOLL upper latest mismatch")
	}
	if !eq4(low[len(low)-1].Value, math.Round(bv.Lower*1e4)/1e4) {
		t.Errorf("BOLL lower latest mismatch")
	}
}

func TestIndicatorHistory_Guards(t *testing.T) {
	// Unsupported id (vwap is intentionally not history-charted; fundamentals never are).
	if _, ok := IndicatorHistory(histCandles([]float64{1, 2, 3}), "technical.vwap", 0); ok {
		t.Error("expected unsupported id to be not ok")
	}
	// Insufficient history (fewer closes than the period).
	if _, ok := IndicatorHistory(histCandles([]float64{1, 2, 3}), "technical.sma-ma", 0); ok {
		t.Error("expected insufficient history to be not ok")
	}
	// Empty candles.
	if _, ok := IndicatorHistory(nil, "technical.rsi", 0); ok {
		t.Error("expected empty candles to be not ok")
	}
	if !HistoryableID("technical.rsi") || !HistoryableID("technical.atr") || HistoryableID("technical.vwap") {
		t.Error("HistoryableID wrong")
	}
}
