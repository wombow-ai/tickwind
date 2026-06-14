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

// --- additional pure helpers for the expanded technical set (technical_more.go) ---
//
// Every helper below is dependency-free and unit-tested. Series helpers return a
// slice the same length as the input with NaN during warmup (mirroring emaSeries),
// so a caller can take the last element and check IsNaN. Scalar helpers return
// (value, ok) and report ok=false on too-few bars or a zero denominator, so the
// closures in technical_more.go can map !ok → setInsufficient (never fabricate).

// wma returns the latest linear Weighted Moving Average over the trailing period
// values: WMA = (n·Cₙ + … + 1·C₁) / (n(n+1)/2), with the largest weight on the
// most recent value. ok=false when there are fewer than period values.
func wma(values []float64, period int) (float64, bool) {
	if period <= 0 || len(values) < period {
		return 0, false
	}
	window := values[len(values)-period:]
	num := 0.0
	for i, v := range window {
		num += v * float64(i+1) // weight 1..period, newest highest
	}
	den := float64(period*(period+1)) / 2
	return num / den, true
}

// wmaSeries returns the rolling WMA(period) over values, one output per full
// window (length len(values) − period + 1, or nil when too short). Used by HMA.
func wmaSeries(values []float64, period int) []float64 {
	if period <= 0 || len(values) < period {
		return nil
	}
	out := make([]float64, 0, len(values)-period+1)
	den := float64(period*(period+1)) / 2
	for end := period; end <= len(values); end++ {
		window := values[end-period : end]
		num := 0.0
		for i, v := range window {
			num += v * float64(i+1)
		}
		out = append(out, num/den)
	}
	return out
}

// smmaSeries returns the full Wilder SMMA / RMA over values: the seed is the
// SMA of the first period values, then SMMA = (prev·(period−1) + value)/period —
// the same recursion atrWilder/rsiWilder smooth with, exposed as a reusable
// series. Warmup indices (< period−1) are NaN. ok=false when too short.
func smmaSeries(values []float64, period int) ([]float64, bool) {
	n := len(values)
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	if period <= 0 || n < period {
		return out, false
	}
	seed := 0.0
	for i := 0; i < period; i++ {
		seed += values[i]
	}
	seed /= float64(period)
	out[period-1] = seed
	prev := seed
	for i := period; i < n; i++ {
		prev = (prev*float64(period-1) + values[i]) / float64(period)
		out[i] = prev
	}
	return out, true
}

// smma returns the latest Wilder SMMA / RMA value, or ok=false during warmup.
func smma(values []float64, period int) (float64, bool) {
	s, ok := smmaSeries(values, period)
	if !ok {
		return 0, false
	}
	v := s[len(s)-1]
	if math.IsNaN(v) {
		return 0, false
	}
	return v, true
}

// rollingStd returns the population standard deviation (÷period) of the trailing
// period values — the same σ bollinger() computes internally, exposed for the
// standalone SD/BBW indicators. ok=false when there are fewer than period values.
func rollingStd(values []float64, period int) (float64, bool) {
	if period <= 0 || len(values) < period {
		return 0, false
	}
	window := values[len(values)-period:]
	mean := 0.0
	for _, v := range window {
		mean += v
	}
	mean /= float64(period)
	variance := 0.0
	for _, v := range window {
		d := v - mean
		variance += d * d
	}
	variance /= float64(period)
	return math.Sqrt(variance), true
}

// logReturns returns the natural-log returns ln(Cᵢ/Cᵢ₋₁) of the close series
// (length len(closes)−1), or nil when fewer than two closes or a non-positive
// price would make the log undefined.
func logReturns(closes []float64) []float64 {
	if len(closes) < 2 {
		return nil
	}
	out := make([]float64, 0, len(closes)-1)
	for i := 1; i < len(closes); i++ {
		if closes[i] <= 0 || closes[i-1] <= 0 {
			return nil
		}
		out = append(out, math.Log(closes[i]/closes[i-1]))
	}
	return out
}

// stdLogReturns returns the population standard deviation of the trailing period
// log returns of closes (the per-period volatility before annualizing). It needs
// at least period+1 closes (period returns). ok=false otherwise or on a
// non-positive price.
func stdLogReturns(closes []float64, period int) (float64, bool) {
	r := logReturns(closes)
	if r == nil || len(r) < period {
		return 0, false
	}
	return rollingStd(r, period)
}

// linregForecast fits an ordinary-least-squares line over the trailing period
// values (x = 0..period−1) and returns the forecast at the last point x =
// period−1 (i.e. the regression value at the most recent bar). ok=false when
// there are fewer than period values or the window is degenerate.
func linregForecast(values []float64, period int) (float64, bool) {
	if period <= 1 || len(values) < period {
		return 0, false
	}
	window := values[len(values)-period:]
	n := float64(period)
	var sx, sy, sxx, sxy float64
	for i, v := range window {
		x := float64(i)
		sx += x
		sy += v
		sxx += x * x
		sxy += x * v
	}
	den := n*sxx - sx*sx
	if den == 0 {
		return 0, false
	}
	slope := (n*sxy - sx*sy) / den
	intercept := (sy - slope*sx) / n
	return intercept + slope*float64(period-1), true
}

// percentRank returns the percent (0..100) of the trailing window (period prior
// values) that the latest value strictly exceeds — the ConnorsRSI percent-rank
// term. It compares values[last] against the period values before it, so it needs
// at least period+1 values. ok=false otherwise.
func percentRank(values []float64, period int) (float64, bool) {
	n := len(values)
	if period <= 0 || n < period+1 {
		return 0, false
	}
	cur := values[n-1]
	window := values[n-1-period : n-1]
	below := 0
	for _, v := range window {
		if v < cur {
			below++
		}
	}
	return float64(below) / float64(period) * 100, true
}

// rsiSeries returns the Wilder RSI at every index where it is defined (the same
// recursion rsiWilder uses), with NaN at warmup indices (< period). It needs at
// least period+1 closes; ok=false otherwise. Used by StochRSI / ConnorsRSI.
func rsiSeries(closes []float64, period int) ([]float64, bool) {
	n := len(closes)
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	if period <= 0 || n < period+1 {
		return out, false
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
	out[period] = rsiFrom(avgGain, avgLoss)
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
		out[i] = rsiFrom(avgGain, avgLoss)
	}
	return out, true
}

// cmoSeries returns the Chande Momentum Oscillator at every index where it is
// defined: CMO = 100·(ΣUp − ΣDown)/(ΣUp + ΣDown) over the trailing period close
// deltas. Warmup indices (< period) are NaN. It needs at least period+1 closes;
// ok=false otherwise. Used by VIDYA (adaptive smoothing) and the standalone CMO.
func cmoSeries(closes []float64, period int) ([]float64, bool) {
	n := len(closes)
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	if period <= 0 || n < period+1 {
		return out, false
	}
	for i := period; i < n; i++ {
		up, down := 0.0, 0.0
		for j := i - period + 1; j <= i; j++ {
			d := closes[j] - closes[j-1]
			if d > 0 {
				up += d
			} else {
				down -= d
			}
		}
		sum := up + down
		if sum == 0 {
			out[i] = 0
			continue
		}
		out[i] = 100 * (up - down) / sum
	}
	return out, true
}

// cmo returns the latest Chande Momentum Oscillator, or ok=false during warmup.
func cmo(closes []float64, period int) (float64, bool) {
	s, ok := cmoSeries(closes, period)
	if !ok {
		return 0, false
	}
	v := s[len(s)-1]
	if math.IsNaN(v) {
		return 0, false
	}
	return v, true
}

// trueRange returns the True Range series TR = max(H−L, |H−Cprev|, |L−Cprev|),
// length n with tr[0] = H−L (no prior close). nil when the inputs are mismatched
// or empty.
func trueRange(highs, lows, closes []float64) []float64 {
	n := len(closes)
	if n == 0 || len(highs) != n || len(lows) != n {
		return nil
	}
	tr := make([]float64, n)
	tr[0] = highs[0] - lows[0]
	for i := 1; i < n; i++ {
		hl := highs[i] - lows[i]
		hc := math.Abs(highs[i] - closes[i-1])
		lc := math.Abs(lows[i] - closes[i-1])
		tr[i] = math.Max(hl, math.Max(hc, lc))
	}
	return tr
}

// DMIValue is the latest Directional Movement triple: +DI, −DI and ADX.
type DMIValue struct {
	PlusDI  float64
	MinusDI float64
	ADX     float64
}

// dmiADX returns the latest +DI / −DI / ADX using Wilder smoothing over period
// bars: +DM/−DM and TR are Wilder-smoothed, DI = 100·smoothedDM/smoothedTR, DX =
// 100·|+DI − −DI|/(+DI + −DI), ADX = Wilder-smoothed DX. It needs at least
// 2·period+1 bars so the ADX (a smoothing of period DX values) is defined.
// ok=false otherwise.
func dmiADX(highs, lows, closes []float64, period int) (DMIValue, bool) {
	n := len(closes)
	if period <= 0 || n < 2*period+1 || len(highs) != n || len(lows) != n {
		return DMIValue{}, false
	}
	plusDM := make([]float64, n)
	minusDM := make([]float64, n)
	tr := make([]float64, n)
	for i := 1; i < n; i++ {
		up := highs[i] - highs[i-1]
		down := lows[i-1] - lows[i]
		if up > down && up > 0 {
			plusDM[i] = up
		}
		if down > up && down > 0 {
			minusDM[i] = down
		}
		hl := highs[i] - lows[i]
		hc := math.Abs(highs[i] - closes[i-1])
		lc := math.Abs(lows[i] - closes[i-1])
		tr[i] = math.Max(hl, math.Max(hc, lc))
	}
	// Wilder-smooth +DM/−DM/TR seeded with the sum over the first period (1..period).
	sPlus, sMinus, sTR := 0.0, 0.0, 0.0
	for i := 1; i <= period; i++ {
		sPlus += plusDM[i]
		sMinus += minusDM[i]
		sTR += tr[i]
	}
	dx := make([]float64, 0, n)
	diStep := func() (float64, float64, bool) {
		if sTR == 0 {
			return 0, 0, false
		}
		return 100 * sPlus / sTR, 100 * sMinus / sTR, true
	}
	if pdi, mdi, ok := diStep(); ok {
		if d := pdi + mdi; d != 0 {
			dx = append(dx, 100*math.Abs(pdi-mdi)/d)
		} else {
			dx = append(dx, 0)
		}
	} else {
		dx = append(dx, 0)
	}
	for i := period + 1; i < n; i++ {
		sPlus = sPlus - sPlus/float64(period) + plusDM[i]
		sMinus = sMinus - sMinus/float64(period) + minusDM[i]
		sTR = sTR - sTR/float64(period) + tr[i]
		pdi, mdi, ok := diStep()
		if !ok {
			dx = append(dx, 0)
			continue
		}
		if d := pdi + mdi; d != 0 {
			dx = append(dx, 100*math.Abs(pdi-mdi)/d)
		} else {
			dx = append(dx, 0)
		}
	}
	// ADX = Wilder smoothing of the DX series (seed = SMA of the first period DX).
	if len(dx) < period {
		return DMIValue{}, false
	}
	adx := 0.0
	for i := 0; i < period; i++ {
		adx += dx[i]
	}
	adx /= float64(period)
	for i := period; i < len(dx); i++ {
		adx = (adx*float64(period-1) + dx[i]) / float64(period)
	}
	// Latest +DI/−DI from the final smoothed sums.
	pdi, mdi, ok := diStep()
	if !ok {
		return DMIValue{}, false
	}
	return DMIValue{PlusDI: pdi, MinusDI: mdi, ADX: adx}, true
}

// obvSeries returns the cumulative On-Balance Volume series: starts at 0, adds
// the bar volume on an up close, subtracts it on a down close, unchanged on a
// flat close. nil when the inputs are mismatched or empty.
func obvSeries(closes, volumes []float64) []float64 {
	n := len(closes)
	if n == 0 || len(volumes) != n {
		return nil
	}
	out := make([]float64, n)
	for i := 1; i < n; i++ {
		switch {
		case closes[i] > closes[i-1]:
			out[i] = out[i-1] + volumes[i]
		case closes[i] < closes[i-1]:
			out[i] = out[i-1] - volumes[i]
		default:
			out[i] = out[i-1]
		}
	}
	return out
}

// moneyFlowMultiplier returns ((C−L) − (H−C))/(H−L) for one bar, or 0 when the
// range is zero (a flat bar contributes no flow — the standard ADL convention).
func moneyFlowMultiplier(high, low, close float64) float64 {
	rng := high - low
	if rng == 0 {
		return 0
	}
	return ((close - low) - (high - close)) / rng
}

// adlSeries returns the Accumulation/Distribution Line: a running sum of the
// money-flow multiplier × volume per bar. nil when the inputs are mismatched or
// empty.
func adlSeries(highs, lows, closes, volumes []float64) []float64 {
	n := len(closes)
	if n == 0 || len(highs) != n || len(lows) != n || len(volumes) != n {
		return nil
	}
	out := make([]float64, n)
	run := 0.0
	for i := 0; i < n; i++ {
		run += moneyFlowMultiplier(highs[i], lows[i], closes[i]) * volumes[i]
		out[i] = run
	}
	return out
}

// hlc3 returns the typical price (H+L+C)/3 series, or nil on mismatched inputs.
func hlc3(highs, lows, closes []float64) []float64 {
	n := len(closes)
	if n == 0 || len(highs) != n || len(lows) != n {
		return nil
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		out[i] = (highs[i] + lows[i] + closes[i]) / 3
	}
	return out
}

// hl2 returns the median price (H+L)/2 series, or nil on mismatched inputs.
func hl2(highs, lows []float64) []float64 {
	n := len(highs)
	if n == 0 || len(lows) != n {
		return nil
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		out[i] = (highs[i] + lows[i]) / 2
	}
	return out
}

// highestHigh returns the maximum of the trailing period highs, ok=false when
// there are fewer than period values.
func highestHigh(highs []float64, period int) (float64, bool) {
	if period <= 0 || len(highs) < period {
		return 0, false
	}
	hi := math.Inf(-1)
	for _, v := range highs[len(highs)-period:] {
		if v > hi {
			hi = v
		}
	}
	return hi, true
}

// lowestLow returns the minimum of the trailing period lows, ok=false when there
// are fewer than period values.
func lowestLow(lows []float64, period int) (float64, bool) {
	if period <= 0 || len(lows) < period {
		return 0, false
	}
	lo := math.Inf(1)
	for _, v := range lows[len(lows)-period:] {
		if v < lo {
			lo = v
		}
	}
	return lo, true
}

// barsSinceHighest returns the number of bars since the highest high within the
// trailing window of period+1 bars (0 = the latest bar is the extreme), for
// Aroon. ok=false when there are fewer than period+1 values.
func barsSinceHighest(highs []float64, period int) (int, bool) {
	n := len(highs)
	if period <= 0 || n < period+1 {
		return 0, false
	}
	window := highs[n-period-1:]
	maxIdx := 0
	for i, v := range window {
		if v >= window[maxIdx] {
			maxIdx = i
		}
	}
	return len(window) - 1 - maxIdx, true
}

// barsSinceLowest returns the number of bars since the lowest low within the
// trailing window of period+1 bars (0 = the latest bar is the extreme), for
// Aroon. ok=false when there are fewer than period+1 values.
func barsSinceLowest(lows []float64, period int) (int, bool) {
	n := len(lows)
	if period <= 0 || n < period+1 {
		return 0, false
	}
	window := lows[n-period-1:]
	minIdx := 0
	for i, v := range window {
		if v <= window[minIdx] {
			minIdx = i
		}
	}
	return len(window) - 1 - minIdx, true
}

// roc returns the latest Rate of Change percent: (C − C[t−period])/C[t−period]·100.
// It needs at least period+1 closes and a non-zero reference; ok=false otherwise.
func roc(values []float64, period int) (float64, bool) {
	n := len(values)
	if period <= 0 || n < period+1 {
		return 0, false
	}
	ref := values[n-1-period]
	if ref == 0 {
		return 0, false
	}
	return (values[n-1] - ref) / ref * 100, true
}

// rocSeries returns the Rate of Change percent at every index where it is defined
// (>= period), NaN during warmup, nil when too short. Used by the Coppock curve.
func rocSeries(values []float64, period int) []float64 {
	n := len(values)
	if period <= 0 || n < period+1 {
		return nil
	}
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	for i := period; i < n; i++ {
		ref := values[i-period]
		if ref == 0 {
			continue
		}
		out[i] = (values[i] - ref) / ref * 100
	}
	return out
}

// compact drops the leading NaN warmup entries of a series, returning the defined
// tail (which a series helper like emaSeries can then consume). Empty when all NaN.
func compact(series []float64) []float64 {
	out := make([]float64, 0, len(series))
	for _, v := range series {
		if !math.IsNaN(v) {
			out = append(out, v)
		}
	}
	return out
}
