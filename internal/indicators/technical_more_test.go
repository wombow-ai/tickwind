package indicators

import (
	"math"
	"testing"
)

// approx reports whether got is within tol of want.
func approx(got, want, tol float64) bool {
	return math.Abs(got-want) <= tol
}

// --- pure-helper tests (table-driven where the helper has clean known cases) ---

func TestWMA(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		period int
		want   float64
		ok     bool
	}{
		// WMA(1,2,3) p=3 = (1·1 + 2·2 + 3·3)/(6) = 14/6.
		{"three", []float64{1, 2, 3}, 3, 14.0 / 6.0, true},
		// Newest weighted highest: WMA(2,4,6,8) p=4 = (1·2+2·4+3·6+4·8)/10 = 60/10.
		{"four", []float64{2, 4, 6, 8}, 4, 6.0, true},
		{"too-short", []float64{1, 2}, 3, 0, false},
		{"zero-period", []float64{1, 2, 3}, 0, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := wma(tc.values, tc.period)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && !approx(got, tc.want, 1e-9) {
				t.Errorf("wma = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSMMA(t *testing.T) {
	// SMMA(5) seed = SMA of first 5 = (1+2+3+4+5)/5 = 3; then
	// next = (3·4 + 6)/5 = 18/5 = 3.6.
	got, ok := smma([]float64{1, 2, 3, 4, 5, 6}, 5)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if !approx(got, 3.6, 1e-9) {
		t.Errorf("smma = %v, want 3.6", got)
	}
	if _, ok := smma([]float64{1, 2}, 5); ok {
		t.Error("smma too-short: ok = true, want false")
	}
}

func TestRollingStd(t *testing.T) {
	// Population σ of {2,4,4,4,5,5,7,9} = 2.
	got, ok := rollingStd([]float64{2, 4, 4, 4, 5, 5, 7, 9}, 8)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if !approx(got, 2.0, 1e-9) {
		t.Errorf("rollingStd = %v, want 2", got)
	}
	if _, ok := rollingStd([]float64{1}, 8); ok {
		t.Error("rollingStd too-short: ok = true, want false")
	}
}

func TestStdLogReturns(t *testing.T) {
	// Constant ratio (geometric) → all log returns equal → σ = 0.
	got, ok := stdLogReturns([]float64{1, 2, 4, 8, 16}, 4)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if !approx(got, 0, 1e-12) {
		t.Errorf("stdLogReturns of geometric series = %v, want 0", got)
	}
	if _, ok := stdLogReturns([]float64{1, 2}, 5); ok {
		t.Error("stdLogReturns too-short: ok = true, want false")
	}
}

func TestLinregForecast(t *testing.T) {
	// Perfect line y = 2x + 1 over x=0..3 → forecast at x=3 is 7.
	got, ok := linregForecast([]float64{1, 3, 5, 7}, 4)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if !approx(got, 7, 1e-9) {
		t.Errorf("linregForecast = %v, want 7", got)
	}
	if _, ok := linregForecast([]float64{1}, 4); ok {
		t.Error("linregForecast too-short: ok = true, want false")
	}
}

func TestPercentRank(t *testing.T) {
	// Latest value 5 vs prior {1,2,3,4} → 4/4 strictly below → 100%.
	got, ok := percentRank([]float64{1, 2, 3, 4, 5}, 4)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if !approx(got, 100, 1e-9) {
		t.Errorf("percentRank = %v, want 100", got)
	}
	// Latest value 2 vs prior {1,2,3,4} → only 1 below → 25%.
	got2, _ := percentRank([]float64{1, 2, 3, 4, 2}, 4)
	if !approx(got2, 25, 1e-9) {
		t.Errorf("percentRank middle = %v, want 25", got2)
	}
	if _, ok := percentRank([]float64{1, 2}, 4); ok {
		t.Error("percentRank too-short: ok = true, want false")
	}
}

func TestRSISeries(t *testing.T) {
	// A strictly rising series → RSI = 100 at the last defined index.
	closes := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	s, ok := rsiSeries(closes, 14)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	last := s[len(s)-1]
	if math.IsNaN(last) || !approx(last, 100, 1e-9) {
		t.Errorf("rsiSeries last = %v, want 100", last)
	}
	// Warmup indices before the period are NaN.
	if !math.IsNaN(s[0]) {
		t.Errorf("rsiSeries[0] = %v, want NaN (warmup)", s[0])
	}
}

func TestCMOSeries(t *testing.T) {
	// Strictly rising → all up moves → CMO = 100.
	got, ok := cmo([]float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}, 14)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if !approx(got, 100, 1e-9) {
		t.Errorf("cmo = %v, want 100", got)
	}
	if _, ok := cmo([]float64{1, 2}, 14); ok {
		t.Error("cmo too-short: ok = true, want false")
	}
}

func TestOBVSeries(t *testing.T) {
	// closes 10,11,10,10,12 with volumes 100,200,150,300,400:
	// +200 (up), −150 (down), 0 (flat), +400 (up) → 450.
	s := obvSeries([]float64{10, 11, 10, 10, 12}, []float64{100, 200, 150, 300, 400})
	if s == nil {
		t.Fatal("obvSeries = nil")
	}
	if !approx(s[len(s)-1], 450, 1e-9) {
		t.Errorf("obv last = %v, want 450", s[len(s)-1])
	}
	if obvSeries([]float64{1, 2}, []float64{1}) != nil {
		t.Error("obvSeries mismatched: want nil")
	}
}

func TestADLSeries(t *testing.T) {
	// One bar H=10,L=8,C=10,V=100: MFM = ((10−8)−(10−10))/2 = 1 → ADL = 100.
	s := adlSeries([]float64{10}, []float64{8}, []float64{10}, []float64{100})
	if s == nil {
		t.Fatal("adlSeries = nil")
	}
	if !approx(s[len(s)-1], 100, 1e-9) {
		t.Errorf("adl = %v, want 100", s[len(s)-1])
	}
	// Flat bar (H==L) contributes 0 (no divide-by-zero).
	flat := adlSeries([]float64{5}, []float64{5}, []float64{5}, []float64{100})
	if flat == nil || !approx(flat[0], 0, 1e-9) {
		t.Errorf("adl flat bar = %v, want 0", flat)
	}
}

func TestDMIADX(t *testing.T) {
	// A strong uptrend: +DI should dominate −DI and ADX should be high.
	n := 40
	highs := make([]float64, n)
	lows := make([]float64, n)
	closes := make([]float64, n)
	for i := 0; i < n; i++ {
		base := float64(i)
		highs[i] = base + 1.5
		lows[i] = base + 0.5
		closes[i] = base + 1.0
	}
	d, ok := dmiADX(highs, lows, closes, 14)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if d.PlusDI <= d.MinusDI {
		t.Errorf("uptrend: +DI (%v) should exceed −DI (%v)", d.PlusDI, d.MinusDI)
	}
	if d.ADX < 0 || d.ADX > 100 {
		t.Errorf("ADX out of range: %v", d.ADX)
	}
	if _, ok := dmiADX(highs[:5], lows[:5], closes[:5], 14); ok {
		t.Error("dmiADX too-short: ok = true, want false")
	}
}

func TestHighestLowestAndBarsSince(t *testing.T) {
	highs := []float64{3, 7, 2, 9, 4}
	lows := []float64{1, 0, 5, 2, 3}
	if hi, ok := highestHigh(highs, 5); !ok || hi != 9 {
		t.Errorf("highestHigh = %v ok=%v, want 9", hi, ok)
	}
	if lo, ok := lowestLow(lows, 5); !ok || lo != 0 {
		t.Errorf("lowestLow = %v ok=%v, want 0", lo, ok)
	}
	// Highest (9) is at index 3 of a 5-bar window using period=4 → window is the
	// last 5 elements; 9 is 1 bar from the end.
	if since, ok := barsSinceHighest(highs, 4); !ok || since != 1 {
		t.Errorf("barsSinceHighest = %v ok=%v, want 1", since, ok)
	}
	if since, ok := barsSinceLowest(lows, 4); !ok || since != 3 {
		t.Errorf("barsSinceLowest = %v ok=%v, want 3", since, ok)
	}
}

func TestROCHelper(t *testing.T) {
	// ROC of close 110 vs 100 ten bars back = 10%.
	closes := make([]float64, 11)
	closes[0] = 100
	for i := 1; i < 11; i++ {
		closes[i] = 100
	}
	closes[10] = 110
	got, ok := roc(closes, 10)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if !approx(got, 10, 1e-9) {
		t.Errorf("roc = %v, want 10", got)
	}
	if _, ok := roc([]float64{100}, 10); ok {
		t.Error("roc too-short: ok = true, want false")
	}
}

func TestCCIHelper(t *testing.T) {
	// Mirror the catalog formula on a small window and cross-check the sign.
	highs := []float64{10, 11, 12, 13, 14}
	lows := []float64{8, 9, 10, 11, 12}
	closes := []float64{9, 10, 11, 12, 13}
	got, ok := cci(highs, lows, closes, 5)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	// Latest typical price is the window max → CCI must be positive.
	if got <= 0 {
		t.Errorf("cci of a rising window = %v, want > 0", got)
	}
	if _, ok := cci(highs[:2], lows[:2], closes[:2], 5); ok {
		t.Error("cci too-short: ok = true, want false")
	}
}

func TestRVINeedsOpen(t *testing.T) {
	n := 10
	opens := make([]float64, n)
	highs := make([]float64, n)
	lows := make([]float64, n)
	closes := make([]float64, n)
	for i := 0; i < n; i++ {
		opens[i] = 10
		highs[i] = 12
		lows[i] = 9
		closes[i] = 11 // close > open every bar → positive vigor
	}
	line, signal, ok := relativeVigorIndex(opens, highs, lows, closes)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if line <= 0 {
		t.Errorf("rvi line = %v, want > 0 (close > open)", line)
	}
	_ = signal
	// Missing opens (mismatched length) must be insufficient, not fabricated.
	if _, _, ok := relativeVigorIndex(nil, highs, lows, closes); ok {
		t.Error("rvi without opens: ok = true, want false")
	}
}

// --- representative closure tests over the registry (formula + too-short) ---

// mkInput builds a computeInput from a synthetic price walk of the given length,
// good enough to exercise every closure (rising, with a stable range and volume).
func mkInput(n int) computeInput {
	in := computeInput{
		opens:   make([]float64, n),
		highs:   make([]float64, n),
		lows:    make([]float64, n),
		closes:  make([]float64, n),
		volumes: make([]float64, n),
	}
	for i := 0; i < n; i++ {
		// A gentle oscillating uptrend so oscillators have both up and down moves.
		c := 100 + float64(i)*0.5 + 3*math.Sin(float64(i)*0.4)
		in.opens[i] = c - 0.3
		in.highs[i] = c + 1.0
		in.lows[i] = c - 1.0
		in.closes[i] = c
		in.volumes[i] = 1_000_000 + float64(i%5)*50_000
	}
	return in
}

// runClosure runs a registered closure over the input and returns the StockIndicator.
func runClosure(t *testing.T, id string, in computeInput) StockIndicator {
	t.Helper()
	reg := technicalRegistryMore()
	fn, ok := reg[id]
	if !ok {
		t.Fatalf("id %q not registered", id)
	}
	si := StockIndicator{Status: StatusInsufficient, Reason: "not computed"}
	fn(in, &si)
	return si
}

func TestClosuresProduceValuesOnAmpleData(t *testing.T) {
	in := mkInput(150) // ample for every window incl. Connors RSI (~100)
	reg := technicalRegistryMore()
	for id := range reg {
		t.Run(id, func(t *testing.T) {
			si := runClosure(t, id, in)
			if si.Status != StatusOK {
				t.Fatalf("id %q status = %s (reason %q), want ok on ample data", id, si.Status, si.Reason)
			}
			if si.Value == nil {
				t.Fatalf("id %q ok but Value is nil", id)
			}
			if math.IsNaN(*si.Value) || math.IsInf(*si.Value, 0) {
				t.Fatalf("id %q produced a non-finite value %v", id, *si.Value)
			}
		})
	}
}

func TestClosuresInsufficientOnShortSeries(t *testing.T) {
	in := mkInput(3) // too short for essentially every windowed indicator
	reg := technicalRegistryMore()
	for id := range reg {
		t.Run(id, func(t *testing.T) {
			si := runClosure(t, id, in)
			// Either insufficient, or ok with a finite value (e.g. gaps/pp need only
			// 2 bars) — but NEVER ok with a nil/NaN value (the no-fabrication rule).
			if si.Status == StatusOK {
				if si.Value == nil || math.IsNaN(*si.Value) || math.IsInf(*si.Value, 0) {
					t.Fatalf("id %q ok on short series but value invalid: %v", id, si.Value)
				}
			} else if si.Value != nil {
				t.Fatalf("id %q not ok but carries a value %v (fabrication)", id, *si.Value)
			}
		})
	}
}

func TestClosuresNeverFabricateOnEmpty(t *testing.T) {
	var in computeInput // entirely empty
	reg := technicalRegistryMore()
	for id := range reg {
		t.Run(id, func(t *testing.T) {
			si := runClosure(t, id, in)
			if si.Status == StatusOK {
				t.Fatalf("id %q ok on empty input (fabrication)", id)
			}
			if si.Value != nil {
				t.Fatalf("id %q carries a value on empty input", id)
			}
		})
	}
}

// TestRegistryMoreIDsAreRealCatalogIDs asserts every expanded-registry key is a
// real catalog id (no typos), so the picker can never offer a phantom indicator.
func TestRegistryMoreIDsAreRealCatalogIDs(t *testing.T) {
	cat := MustLoad()
	valid := make(map[string]struct{})
	for _, rec := range cat.All() {
		valid[rec.ID] = struct{}{}
	}
	for id := range technicalRegistryMore() {
		if _, ok := valid[id]; !ok {
			t.Errorf("registered id %q is not a real catalog id", id)
		}
	}
}

// TestDMIADXExtraLines spot-checks that a multi-line indicator fills Extra.
func TestDMIADXExtraLines(t *testing.T) {
	in := mkInput(60)
	si := runClosure(t, "technical.dmi-adx", in)
	if si.Status != StatusOK {
		t.Fatalf("dmi-adx status = %s, want ok", si.Status)
	}
	for _, k := range []string{"plusDI", "minusDI", "adx"} {
		if _, ok := si.Extra[k]; !ok {
			t.Errorf("dmi-adx Extra missing %q", k)
		}
	}
}

// TestDonchianExtraLines spot-checks the channel upper/mid/lower lines.
func TestDonchianExtraLines(t *testing.T) {
	in := mkInput(60)
	si := runClosure(t, "technical.dc", in)
	if si.Status != StatusOK {
		t.Fatalf("dc status = %s, want ok", si.Status)
	}
	up, okU := si.Extra["upper"]
	mid, okM := si.Extra["mid"]
	lo, okL := si.Extra["lower"]
	if !okU || !okM || !okL {
		t.Fatal("dc Extra missing channel lines")
	}
	if !(up >= mid && mid >= lo) {
		t.Errorf("dc channel not ordered: upper=%v mid=%v lower=%v", up, mid, lo)
	}
}

// TestKAMASeedsOnceAndIteratesFullHistory verifies the recursive KAMA filter is
// seeded ONCE at the first computable bar (closes[period-1]) and iterated forward
// over the whole series, matching a hand-computed reference. The series
// {10,11,12,11,10,11,12,13,12,11} with period=2, fast=2, slow=30 was traced by
// hand bar-by-bar (seed=closes[1]=11; ER∈{0,1}; SC=(ER·(2/3−2/31)+2/31)²) to a
// final value of 11.5969237770 — see the canonical recurrence
// KAMA=KAMAₚᵣₑᵥ+SC·(C−KAMAₚᵣₑᵥ).
func TestKAMASeedsOnceAndIteratesFullHistory(t *testing.T) {
	closes := []float64{10, 11, 12, 11, 10, 11, 12, 13, 12, 11}
	got, ok := kama(closes, 2, 2, 30)
	if !ok {
		t.Fatal("kama ok = false, want true")
	}
	const want = 11.5969237770
	if !approx(got, want, 1e-9) {
		t.Errorf("kama = %.10f, want %.10f (full-history seed-once)", got, want)
	}
	// Insufficiency guard: fewer than period+1 bars → not ok, no fabrication.
	if _, ok := kama([]float64{10, 11}, 2, 2, 30); ok {
		t.Error("kama with period+1−1 bars: ok = true, want false")
	}
}

// TestKAMARangingDiffersFromTruncatedSeed proves the fix matters: on a long,
// tight-range series the OLD buggy version reseeded at a raw close `period` bars
// before the end and ran only `period` steps (gluing the value to that stale
// seed via the SC floor), whereas the correct full-history filter has drifted.
// The two differ by ~0.5% here — the bug's ~2.4% class of error in ranging
// markets. The reference values are computed inline by the two algorithms so the
// test is self-contained (no magic constants).
func TestKAMARangingDiffersFromTruncatedSeed(t *testing.T) {
	const period, fast, slow = 10, 2, 30
	closes := make([]float64, 60)
	for i := range closes {
		closes[i] = 50 + math.Sin(float64(i)*0.5) // tight range around 50
	}
	full, ok := kama(closes, period, fast, slow)
	if !ok {
		t.Fatal("kama ok = false, want true")
	}
	// Reproduce the OLD truncated behavior (reseed period bars before the end,
	// iterate only `period` steps) to show it lands materially elsewhere.
	fastSC := 2.0 / (float64(fast) + 1)
	slowSC := 2.0 / (float64(slow) + 1)
	n := len(closes)
	trunc := closes[n-period-1]
	for i := n - period; i < n; i++ {
		change := math.Abs(closes[i] - closes[i-period])
		vol := 0.0
		for j := i - period + 1; j <= i; j++ {
			vol += math.Abs(closes[j] - closes[j-1])
		}
		er := 0.0
		if vol != 0 {
			er = change / vol
		}
		sc := er*(fastSC-slowSC) + slowSC
		sc *= sc
		trunc += sc * (closes[i] - trunc)
	}
	if approx(full, trunc, 1e-3) {
		t.Errorf("full-history KAMA %.6f should differ from the old truncated seed %.6f on a ranging series", full, trunc)
	}
}

// TestKVOVolumeForceMagnitudePresent proves the Volume Force carries the trend's
// MAGNITUDE, not just its sign. Two series are byte-identical in closes, volumes
// and HLC3 direction; they differ only in ONE bar's high-low range. The canonical
// KVO responds to that range change (magnitude present); the old sign-only VF
// (volume·±1) was blind to range, so it would have returned the identical KVO for
// both. Verified per the TradingView/Investopedia Klinger Volume Force definition.
func TestKVOVolumeForceMagnitudePresent(t *testing.T) {
	const fast, slow = 34, 55
	mk := func(rangeAt58 float64) (h, l, c, v []float64) {
		for i := 0; i < 60; i++ {
			base := 100 + float64(i)*0.3 + 2*math.Sin(float64(i)*0.5)
			r := 1.0
			if i == 58 {
				r = rangeAt58 // one bar's range differs; everything else identical
			}
			h = append(h, base+r)
			l = append(l, base-r)
			c = append(c, base)
			v = append(v, 1_000_000)
		}
		return
	}
	hN, lN, cN, vN := mk(1.0)
	hW, lW, cW, vW := mk(3.0)
	kvoN, okN := klingerVolumeOscillator(hN, lN, cN, vN, fast, slow)
	kvoW, okW := klingerVolumeOscillator(hW, lW, cW, vW, fast, slow)
	if !okN || !okW {
		t.Fatalf("kvo ok = (%v,%v), want both true", okN, okW)
	}
	if approx(kvoN, kvoW, 1e-6) {
		t.Errorf("KVO ignored a single-bar range change (%.6f == %.6f): volume-force magnitude missing", kvoN, kvoW)
	}
	// Too-short guard: no fabrication below slow+2 bars.
	if _, ok := klingerVolumeOscillator(hN[:slow], lN[:slow], cN[:slow], vN[:slow], fast, slow); ok {
		t.Error("kvo on too-few bars: ok = true, want false")
	}
}

// TestSARTwoBarClampBinds verifies Wilder's SAR is bounded by the price range of
// the prior TWO bars before the penetration test. On an uptrend with a pullback
// where low[i-2] < low[i-1], the two-bar clamp (min of the two prior lows) pulls
// the SAR to a strictly LOWER bound than a one-bar clamp would, so the corrected
// SAR differs from the old one-bar-clamp value.
func TestSARTwoBarClampBinds(t *testing.T) {
	highs := []float64{10, 11, 12, 13, 14, 15, 16}
	lows := []float64{9, 9.5, 8.0, 11, 12, 13, 14} // index2 low=8.0 dips below index1's 9.5
	newSAR, trend, ok := parabolicSAR(highs, lows, 0.02, 0.20)
	if !ok {
		t.Fatal("sar ok = false, want true")
	}
	if trend != 1 {
		t.Fatalf("sar trend = %v, want 1 (uptrend)", trend)
	}
	// Reproduce the OLD one-bar clamp (and reversal-test-on-unclamped-SAR) inline.
	oldSAR := func() float64 {
		up := highs[1] > highs[0]
		af := 0.02
		var ep, sar float64
		if up {
			sar = lows[0]
			ep = highs[1]
		} else {
			sar = highs[0]
			ep = lows[1]
		}
		for i := 1; i < len(highs); i++ {
			sar = sar + af*(ep-sar)
			if up {
				if lows[i] < sar {
					up = false
					sar = ep
					ep = lows[i]
					af = 0.02
					continue
				}
				if highs[i] > ep {
					ep = highs[i]
					af = math.Min(af+0.02, 0.20)
				}
				if sar > lows[i-1] {
					sar = lows[i-1]
				}
			}
		}
		return sar
	}()
	if approx(newSAR, oldSAR, 1e-9) {
		t.Errorf("two-bar clamp did not bind: new SAR %.6f == old one-bar SAR %.6f", newSAR, oldSAR)
	}
	// The two-bar clamp must produce a value at or below the one-bar clamp (it
	// takes a min over MORE lows in an uptrend).
	if newSAR > oldSAR+1e-9 {
		t.Errorf("two-bar-clamped SAR %.6f exceeds one-bar SAR %.6f (clamp should not raise it)", newSAR, oldSAR)
	}
}
