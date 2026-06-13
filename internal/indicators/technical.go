package indicators

import "math"

// This file holds the PURE technical-indicator math used by the per-stock
// compute layer. Every function is dependency-free and unit-testable, and the
// formulas deliberately mirror web/src/lib/indicators.ts so a server-computed
// headline value matches the value the K-line chart draws client-side:
//   - EMA is SMA-seeded (the StockCharts/MACD standard, not first-price seeding);
//   - RSI uses Wilder smoothing (not a 2/(N+1) EMA, not a plain SMA);
//   - Bollinger uses the POPULATION standard deviation (÷period).
// Phase 1 returns the LATEST value only; the chart still draws the full series
// from its own TS implementation, so these need only agree on the last point.
//
// The catalog formulas note that the Chinese TongDaXin/THS MACD multiplies the
// histogram by 2 and KDJ uses a 2/3·prev + 1/3·new recursion; for chart parity
// (the K-line panes are international StockCharts/TradingView style) we follow
// the international convention here: MACD histogram = DIF − DEA (no ×2), and the
// stochastic uses %K = SMA(RSV, slowK), %D = SMA(%K, slowD), J = 3K − 2D.

// sma returns the simple moving average of the trailing period closes ending at
// the last element, or ok=false when there are fewer than period values.
func sma(values []float64, period int) (float64, bool) {
	if period <= 0 || len(values) < period {
		return 0, false
	}
	sum := 0.0
	for _, v := range values[len(values)-period:] {
		sum += v
	}
	return sum / float64(period), true
}

// emaSeries computes the full SMA-seeded EMA over values, returning a slice the
// same length as values where indices before period-1 are NaN (warmup). This
// mirrors ema() in web/src/lib/indicators.ts so the latest value matches the
// chart. ok=false when there are fewer than period values.
func emaSeries(values []float64, period int) ([]float64, bool) {
	n := len(values)
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	if period <= 0 || n < period {
		return out, false
	}
	k := 2.0 / (float64(period) + 1.0)
	seed := 0.0
	for i := 0; i < period; i++ {
		seed += values[i]
	}
	seed /= float64(period)
	out[period-1] = seed
	prev := seed
	for i := period; i < n; i++ {
		prev = values[i]*k + prev*(1-k)
		out[i] = prev
	}
	return out, true
}

// ema returns the latest SMA-seeded EMA value, or ok=false during warmup.
func ema(values []float64, period int) (float64, bool) {
	s, ok := emaSeries(values, period)
	if !ok {
		return 0, false
	}
	v := s[len(s)-1]
	if math.IsNaN(v) {
		return 0, false
	}
	return v, true
}

// rsiWilder returns the latest Wilder-smoothed RSI over closes, matching rsi()
// in web/src/lib/indicators.ts. It needs at least period+1 closes (one extra for
// the first delta). When there are no losses in-window RSI is 100; no gains → 0.
func rsiWilder(closes []float64, period int) (float64, bool) {
	n := len(closes)
	if period <= 0 || n < period+1 {
		return 0, false
	}
	gain, loss := 0.0, 0.0
	for i := 1; i <= period; i++ {
		d := closes[i] - closes[i-1]
		if d >= 0 {
			gain += d
		} else {
			loss -= d
		}
	}
	avgGain := gain / float64(period)
	avgLoss := loss / float64(period)
	for i := period + 1; i < n; i++ {
		d := closes[i] - closes[i-1]
		g, l := 0.0, 0.0
		if d > 0 {
			g = d
		} else if d < 0 {
			l = -d
		}
		avgGain = (avgGain*float64(period-1) + g) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + l) / float64(period)
	}
	return rsiFrom(avgGain, avgLoss), true
}

// rsiFrom converts the smoothed average gain/loss to an RSI value (0..100),
// matching the TS rsiFrom: no losses → 100, no gains → 0.
func rsiFrom(avgGain, avgLoss float64) float64 {
	if avgLoss == 0 {
		return 100
	}
	if avgGain == 0 {
		return 0
	}
	rs := avgGain / avgLoss
	return 100 - 100/(1+rs)
}

// MACDValue is the latest MACD line, its signal line, and the histogram.
type MACDValue struct {
	Line      float64 // DIF = EMA(fast) − EMA(slow)
	Signal    float64 // DEA = EMA(signal) of the MACD line
	Histogram float64 // Line − Signal (international convention, no ×2)
}

// macd returns the latest MACD line/signal/histogram, matching macd() in
// web/src/lib/indicators.ts: the signal is an SMA-seeded EMA computed over ONLY
// the defined MACD-line points (those where both EMAs exist). ok=false until the
// signal line is defined (needs slow + signal − 1 closes).
func macd(closes []float64, fast, slow, signal int) (MACDValue, bool) {
	if fast <= 0 || slow <= 0 || signal <= 0 {
		return MACDValue{}, false
	}
	emaFast, okF := emaSeries(closes, fast)
	emaSlow, okS := emaSeries(closes, slow)
	if !okF || !okS {
		return MACDValue{}, false
	}
	// Collect the MACD line only where both EMAs are defined.
	defined := make([]float64, 0, len(closes))
	for i := range closes {
		f, s := emaFast[i], emaSlow[i]
		if !math.IsNaN(f) && !math.IsNaN(s) {
			defined = append(defined, f-s)
		}
	}
	if len(defined) == 0 {
		return MACDValue{}, false
	}
	signalSeries, okSig := emaSeries(defined, signal)
	if !okSig {
		return MACDValue{}, false
	}
	line := defined[len(defined)-1]
	dea := signalSeries[len(signalSeries)-1]
	if math.IsNaN(dea) {
		return MACDValue{}, false
	}
	return MACDValue{Line: line, Signal: dea, Histogram: line - dea}, true
}

// BollingerValue is the latest Bollinger Band triple.
type BollingerValue struct {
	Upper  float64
	Middle float64 // SMA(period)
	Lower  float64
}

// bollinger returns the latest Bollinger Bands, matching bollinger() in
// web/src/lib/indicators.ts: middle = SMA(period); bands = middle ± mult·σ where
// σ is the POPULATION standard deviation (÷period) over the same window.
func bollinger(closes []float64, period int, mult float64) (BollingerValue, bool) {
	if period <= 0 || len(closes) < period {
		return BollingerValue{}, false
	}
	window := closes[len(closes)-period:]
	m := 0.0
	for _, v := range window {
		m += v
	}
	m /= float64(period)
	variance := 0.0
	for _, v := range window {
		d := v - m
		variance += d * d
	}
	variance /= float64(period) // population σ²
	sd := math.Sqrt(variance)
	return BollingerValue{Upper: m + mult*sd, Middle: m, Lower: m - mult*sd}, true
}

// atrWilder returns the latest Wilder-smoothed Average True Range over the bars,
// per the catalog formula TR = max(H−L, |H−Cprev|, |L−Cprev|), ATR = Wilder
// smoothing(TR, period). It needs at least period+1 bars (the first TR needs a
// previous close). The seed is the simple average of the first period TRs; each
// subsequent ATR = (prevATR·(period−1) + TR)/period — the same Wilder recursion
// RSI uses.
func atrWilder(highs, lows, closes []float64, period int) (float64, bool) {
	n := len(closes)
	if period <= 0 || n < period+1 || len(highs) != n || len(lows) != n {
		return 0, false
	}
	tr := make([]float64, n) // tr[0] undefined (no prev close); start at index 1
	for i := 1; i < n; i++ {
		hl := highs[i] - lows[i]
		hc := math.Abs(highs[i] - closes[i-1])
		lc := math.Abs(lows[i] - closes[i-1])
		tr[i] = math.Max(hl, math.Max(hc, lc))
	}
	// Seed: average of the first period TRs (indices 1..period).
	seed := 0.0
	for i := 1; i <= period; i++ {
		seed += tr[i]
	}
	atr := seed / float64(period)
	for i := period + 1; i < n; i++ {
		atr = (atr*float64(period-1) + tr[i]) / float64(period)
	}
	return atr, true
}

// KDJValue is the latest Stochastic / KDJ triple.
type KDJValue struct {
	K float64
	D float64
	J float64 // 3K − 2D
}

// stochasticKDJ returns the latest Stochastic %K/%D and the KDJ J line, using the
// international Stochastic convention: RSV = (C − Lₙ)/(Hₙ − Lₙ)·100 over the n-bar
// window, %K = SMA(RSV, slowK), %D = SMA(%K, slowD), J = 3K − 2D (defaults n=9,
// slowK=3, slowD=3). It needs at least n + slowK + slowD − 2 bars so the final
// %D average is defined. A flat window (Hₙ == Lₙ) yields RSV = 50.
func stochasticKDJ(highs, lows, closes []float64, n, slowK, slowD int) (KDJValue, bool) {
	length := len(closes)
	if n <= 0 || slowK <= 0 || slowD <= 0 || len(highs) != length || len(lows) != length {
		return KDJValue{}, false
	}
	if length < n {
		return KDJValue{}, false
	}
	// RSV at every index i >= n-1.
	rsv := make([]float64, 0, length-n+1)
	for i := n - 1; i < length; i++ {
		hi, lo := highs[i], lows[i]
		for j := i - n + 1; j <= i; j++ {
			if highs[j] > hi {
				hi = highs[j]
			}
			if lows[j] < lo {
				lo = lows[j]
			}
		}
		rng := hi - lo
		if rng == 0 {
			rsv = append(rsv, 50)
			continue
		}
		rsv = append(rsv, (closes[i]-lo)/rng*100)
	}
	// %K = SMA(RSV, slowK) series, then %D = SMA(%K, slowD); take the last of each.
	kSeries := smaSeries(rsv, slowK)
	if len(kSeries) == 0 {
		return KDJValue{}, false
	}
	dSeries := smaSeries(kSeries, slowD)
	if len(dSeries) == 0 {
		return KDJValue{}, false
	}
	k := kSeries[len(kSeries)-1]
	d := dSeries[len(dSeries)-1]
	return KDJValue{K: k, D: d, J: 3*k - 2*d}, true
}

// smaSeries returns the rolling SMA(period) of values, one output per full
// window (so its length is len(values) − period + 1, or empty when too short).
func smaSeries(values []float64, period int) []float64 {
	if period <= 0 || len(values) < period {
		return nil
	}
	out := make([]float64, 0, len(values)-period+1)
	sum := 0.0
	for i, v := range values {
		sum += v
		if i >= period {
			sum -= values[i-period]
		}
		if i >= period-1 {
			out = append(out, sum/float64(period))
		}
	}
	return out
}

// vwap returns the volume-weighted average price over the supplied bars, per the
// catalog formula VWAP = Σ(HLC3ᵢ·Vᵢ)/ΣVᵢ where HLC3 = (H+L+C)/3. Phase 1 has only
// the daily-bar cache (no intraday session boundaries), so this is a rolling VWAP
// over the available bars. ok=false when there are no bars or total volume is 0.
func vwap(highs, lows, closes, volumes []float64) (float64, bool) {
	n := len(closes)
	if n == 0 || len(highs) != n || len(lows) != n || len(volumes) != n {
		return 0, false
	}
	pv, totalVol := 0.0, 0.0
	for i := 0; i < n; i++ {
		typical := (highs[i] + lows[i] + closes[i]) / 3
		pv += typical * volumes[i]
		totalVol += volumes[i]
	}
	if totalVol == 0 {
		return 0, false
	}
	return pv / totalVol, true
}

// latestVolume returns the most recent bar's volume, or ok=false when empty.
func latestVolume(volumes []float64) (float64, bool) {
	if len(volumes) == 0 {
		return 0, false
	}
	return volumes[len(volumes)-1], true
}
