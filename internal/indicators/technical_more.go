package indicators

import "math"

// This file registers the EXPANDED technical-indicator set (design §1.1): ~50
// ids computed from daily OHLCV via the pure helpers in technical.go. The
// closures here follow the SAME no-fabrication contract as technicalRegistry():
// each calls setOK(si, value, unit) ONLY when every input is present and any
// denominator is valid, otherwise setInsufficient(si, reason) with no Value.
// Multi-line indicators populate si.Extra (e.g. dmi-adx → plusDI/minusDI/adx,
// donchian → upper/mid/lower). The Wire agent merges this registry into the
// Computer's registry; technicalRegistryMore() is the only exported surface.
//
// DAILY-VARIANT LABELS. A few indicators are canonically computed on MONTHLY
// bars (Coppock Curve, KST, Mass Index). With only the daily OHLCV cache we
// compute the DAILY variant — the formula is faithful but the period units are
// days, not months. These are labeled here and stay faithful (no relabeling of
// the served number); the interpretation simply reads on a daily cadence.

// Default windows for the expanded technical set. Where the catalog carries a
// default_params {"period": n}, paramPeriod honors it; otherwise these documented
// defaults apply (the canonical period for each indicator).
const (
	defaultWMAPeriod        = 20
	defaultDEMAPeriod       = 20
	defaultZLEMAPeriod      = 20
	defaultHMAPeriod        = 16
	defaultSMMAPeriod       = 14
	defaultKAMAPeriod       = 10
	defaultALMAWindow       = 9
	defaultVIDYAPeriod      = 14
	defaultEnvPeriod        = 20
	defaultEnvPercent       = 2.5
	defaultDonchianPeriod   = 20
	defaultKeltnerEMA       = 20
	defaultKeltnerATR       = 10
	defaultSupertrendATR    = 10
	defaultSupertrendMult   = 3.0
	defaultDMIPeriod        = 14
	defaultVortexPeriod     = 14
	defaultAroonPeriod      = 25
	defaultCCIPeriod        = 20
	defaultWilliamsRPeriod  = 14
	defaultMomentumPeriod   = 10
	defaultROCPeriod        = 12
	defaultCMOPeriod        = 14
	defaultRMIPeriod        = 14
	defaultRMIMomentum      = 5
	defaultTRIXPeriod       = 15
	defaultDPOPeriod        = 20
	defaultStochRSIPeriod   = 14
	defaultUOFast           = 7
	defaultUOMid            = 14
	defaultUOSlow           = 28
	defaultHVPeriod         = 20
	defaultChaikinVolEMA    = 10
	defaultChaikinVolChange = 10
	defaultChopPeriod       = 14
	defaultRWIPeriod        = 14
	defaultCMFPeriod        = 20
	defaultMFIPeriod        = 14
	defaultForceIndexEMA    = 13
	defaultEMVPeriod        = 14
	defaultVOFast           = 12
	defaultVOSlow           = 26
	defaultVROCPeriod       = 12
	defaultTSILong          = 25
	defaultTSIShort         = 13
	defaultPPOFast          = 12
	defaultPPOSlow          = 26
	defaultPPOSignal        = 9
	defaultKVOFast          = 34
	defaultKVOSlow          = 55
	defaultTIIPeriod        = 30
	defaultCFOPeriod        = 14
	defaultDynamicMIShort   = 5
	defaultDynamicMILong    = 20
	defaultDynamicMIBase    = 14
	defaultBullBearEMA      = 13
	defaultFisherPeriod     = 9
	defaultElderForce       = 13
	defaultSMIPeriod        = 10
	defaultSMISmooth        = 3
)

// technicalRegistryMore returns the expanded technical closures keyed by the exact
// catalog id. Each entry computes a deterministic latest scalar from daily OHLCV,
// honoring si.DefaultParams where present (via paramPeriod) and the documented
// defaults above otherwise.
func technicalRegistryMore() map[string]computeFn {
	return map[string]computeFn{
		// --- Moving averages & bands (closes only unless noted) ---
		"technical.wma": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultWMAPeriod)
			if v, ok := wma(in.closes, p); ok {
				setOK(si, v, unitPrice)
			} else {
				setInsufficient(si, "not enough daily closes for the WMA window")
			}
		},
		"technical.dema-tema": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultDEMAPeriod)
			dema, tema, ok := demaTema(in.closes, p)
			if !ok {
				setInsufficient(si, "not enough daily closes for DEMA/TEMA (needs ~3× the period)")
				return
			}
			setOK(si, dema, unitPrice)
			si.Extra = map[string]float64{"dema": dema, "tema": tema}
		},
		"technical.zlema": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultZLEMAPeriod)
			if v, ok := zlema(in.closes, p); ok {
				setOK(si, v, unitPrice)
			} else {
				setInsufficient(si, "not enough daily closes for ZLEMA")
			}
		},
		"technical.hma": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultHMAPeriod)
			if v, ok := hma(in.closes, p); ok {
				setOK(si, v, unitPrice)
			} else {
				setInsufficient(si, "not enough daily closes for HMA")
			}
		},
		"technical.smma-rma-wilder": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultSMMAPeriod)
			if v, ok := smma(in.closes, p); ok {
				setOK(si, v, unitPrice)
			} else {
				setInsufficient(si, "not enough daily closes for SMMA")
			}
		},
		"technical.kama": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultKAMAPeriod)
			if v, ok := kama(in.closes, p, 2, 30); ok {
				setOK(si, v, unitPrice)
			} else {
				setInsufficient(si, "not enough daily closes for KAMA")
			}
		},
		"technical.alma": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultALMAWindow)
			if v, ok := alma(in.closes, p, 0.85, 6); ok {
				setOK(si, v, unitPrice)
			} else {
				setInsufficient(si, "not enough daily closes for ALMA")
			}
		},
		"technical.vidya": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultVIDYAPeriod)
			if v, ok := vidya(in.closes, p); ok {
				setOK(si, v, unitPrice)
			} else {
				setInsufficient(si, "not enough daily closes for VIDYA")
			}
		},
		"technical.env": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultEnvPeriod)
			mid, ok := sma(in.closes, p)
			if !ok {
				setInsufficient(si, "not enough daily closes for the envelope MA")
				return
			}
			frac := defaultEnvPercent / 100
			setOK(si, mid, unitPrice)
			si.Extra = map[string]float64{"upper": mid * (1 + frac), "mid": mid, "lower": mid * (1 - frac)}
		},
		"technical.gmma": func(in computeInput, si *StockIndicator) {
			short := []int{3, 5, 8, 10, 12, 15}
			long := []int{30, 35, 40, 45, 50, 60}
			extra := make(map[string]float64, len(short)+len(long))
			ok := true
			for _, n := range append(append([]int{}, short...), long...) {
				v, e := ema(in.closes, n)
				if !e {
					ok = false
					break
				}
				extra[emaKey(n)] = v
			}
			if !ok {
				setInsufficient(si, "not enough daily closes for the GMMA long group (needs 60 bars)")
				return
			}
			// Headline = the fastest short-group EMA(3).
			setOK(si, extra[emaKey(3)], unitPrice)
			si.Extra = extra
		},
		"technical.bbw": func(in computeInput, si *StockIndicator) {
			period, mult := paramsBoll(si.DefaultParams)
			b, ok := bollinger(in.closes, period, mult)
			if !ok || b.Middle == 0 {
				setInsufficient(si, "not enough daily closes (or zero mid) for Bollinger width")
				return
			}
			setOK(si, (b.Upper-b.Lower)/b.Middle, unitRatio)
		},
		"technical.b": func(in computeInput, si *StockIndicator) {
			period, mult := paramsBoll(si.DefaultParams)
			b, ok := bollinger(in.closes, period, mult)
			if !ok {
				setInsufficient(si, "not enough daily closes for %B")
				return
			}
			rng := b.Upper - b.Lower
			if rng == 0 {
				setInsufficient(si, "flat Bollinger band (zero width) — %B undefined")
				return
			}
			setOK(si, (in.closes[len(in.closes)-1]-b.Lower)/rng, unitRatio)
		},
		"technical.sd": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultBollPeriod)
			if v, ok := rollingStd(in.closes, p); ok {
				setOK(si, v, unitPrice)
			} else {
				setInsufficient(si, "not enough daily closes for standard deviation")
			}
		},

		// --- Channels / trend (H,L,C) ---
		"technical.dc": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultDonchianPeriod)
			up, okU := highestHigh(in.highs, p)
			lo, okL := lowestLow(in.lows, p)
			if !okU || !okL {
				setInsufficient(si, "not enough daily bars for the Donchian channel")
				return
			}
			mid := (up + lo) / 2
			setOK(si, mid, unitPrice)
			si.Extra = map[string]float64{"upper": up, "mid": mid, "lower": lo}
		},
		"technical.kc": func(in computeInput, si *StockIndicator) {
			mid, okM := ema(in.closes, defaultKeltnerEMA)
			atr, okA := atrWilder(in.highs, in.lows, in.closes, defaultKeltnerATR)
			if !okM || !okA {
				setInsufficient(si, "not enough daily bars for Keltner Channels")
				return
			}
			setOK(si, mid, unitPrice)
			si.Extra = map[string]float64{"upper": mid + 2*atr, "mid": mid, "lower": mid - 2*atr}
		},
		"technical.st": func(in computeInput, si *StockIndicator) {
			v, trend, ok := supertrend(in.highs, in.lows, in.closes, defaultSupertrendATR, defaultSupertrendMult)
			if !ok {
				setInsufficient(si, "not enough daily bars for Supertrend")
				return
			}
			setOK(si, v, unitPrice)
			si.Extra = map[string]float64{"supertrend": v, "trend": trend} // trend: +1 up, −1 down
		},
		"technical.sar": func(in computeInput, si *StockIndicator) {
			v, trend, ok := parabolicSAR(in.highs, in.lows, 0.02, 0.20)
			if !ok {
				setInsufficient(si, "not enough daily bars for Parabolic SAR")
				return
			}
			setOK(si, v, unitPrice)
			si.Extra = map[string]float64{"sar": v, "trend": trend}
		},
		"technical.dmi-adx": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultDMIPeriod)
			d, ok := dmiADX(in.highs, in.lows, in.closes, p)
			if !ok {
				setInsufficient(si, "not enough daily bars for DMI/ADX (needs ~2× the period)")
				return
			}
			setOK(si, d.ADX, unitNone)
			si.Extra = map[string]float64{"plusDI": d.PlusDI, "minusDI": d.MinusDI, "adx": d.ADX}
		},
		"technical.vi": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultVortexPeriod)
			plus, minus, ok := vortex(in.highs, in.lows, in.closes, p)
			if !ok {
				setInsufficient(si, "not enough daily bars for the Vortex indicator")
				return
			}
			setOK(si, plus, unitNone)
			si.Extra = map[string]float64{"plusVI": plus, "minusVI": minus}
		},
		"technical.aroon": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultAroonPeriod)
			sinceHigh, okH := barsSinceHighest(in.highs, p)
			sinceLow, okL := barsSinceLowest(in.lows, p)
			if !okH || !okL {
				setInsufficient(si, "not enough daily bars for Aroon")
				return
			}
			up := float64(p-sinceHigh) / float64(p) * 100
			down := float64(p-sinceLow) / float64(p) * 100
			setOK(si, up-down, unitNone)
			si.Extra = map[string]float64{"up": up, "down": down, "osc": up - down}
		},
		"technical.pp": func(in computeInput, si *StockIndicator) {
			n := len(in.closes)
			if n < 2 || len(in.highs) != n || len(in.lows) != n {
				setInsufficient(si, "need a prior daily bar for pivot points")
				return
			}
			h, l, c := in.highs[n-2], in.lows[n-2], in.closes[n-2] // prior bar
			pp := (h + l + c) / 3
			setOK(si, pp, unitPrice)
			si.Extra = map[string]float64{
				"pp": pp,
				"r1": 2*pp - l, "s1": 2*pp - h,
				"r2": pp + (h - l), "s2": pp - (h - l),
			}
		},
		"technical.gaps": func(in computeInput, si *StockIndicator) {
			n := len(in.closes)
			if n < 2 || len(in.highs) != n || len(in.lows) != n {
				setInsufficient(si, "need a prior daily bar to detect a gap")
				return
			}
			var gap float64 // +1 gap up, −1 gap down, 0 none
			switch {
			case in.lows[n-1] > in.highs[n-2]:
				gap = 1
			case in.highs[n-1] < in.lows[n-2]:
				gap = -1
			}
			setOK(si, gap, unitNone)
		},
		"technical.fractals-bill-williams": func(in computeInput, si *StockIndicator) {
			up, down, ok := williamsFractal(in.highs, in.lows)
			if !ok {
				setInsufficient(si, "need at least 5 daily bars for a Bill Williams fractal")
				return
			}
			// Headline: +1 an up-fractal formed on the center bar, −1 down, 0 none.
			var v float64
			if up {
				v += 1
			}
			if down {
				v -= 1
			}
			setOK(si, v, unitNone)
		},
		"technical.alligator": func(in computeInput, si *StockIndicator) {
			med := hl2(in.highs, in.lows)
			jaw, okJ := smma(med, 13)
			teeth, okT := smma(med, 8)
			lips, okL := smma(med, 5)
			if med == nil || !okJ || !okT || !okL {
				setInsufficient(si, "not enough daily bars for the Alligator lines")
				return
			}
			setOK(si, jaw, unitPrice)
			si.Extra = map[string]float64{"jaw": jaw, "teeth": teeth, "lips": lips}
		},

		// --- Momentum / oscillators ---
		"technical.cci": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultCCIPeriod)
			if v, ok := cci(in.highs, in.lows, in.closes, p); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "not enough daily bars (or zero deviation) for CCI")
			}
		},
		"technical.williams-r": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultWilliamsRPeriod)
			hi, okH := highestHigh(in.highs, p)
			lo, okL := lowestLow(in.lows, p)
			if !okH || !okL {
				setInsufficient(si, "not enough daily bars for Williams %R")
				return
			}
			rng := hi - lo
			if rng == 0 {
				setInsufficient(si, "flat range (zero) — Williams %R undefined")
				return
			}
			setOK(si, (hi-in.closes[len(in.closes)-1])/rng*-100, unitNone)
		},
		"technical.mtm": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultMomentumPeriod)
			n := len(in.closes)
			if n < p+1 {
				setInsufficient(si, "not enough daily closes for momentum")
				return
			}
			setOK(si, in.closes[n-1]-in.closes[n-1-p], unitPrice)
		},
		"technical.roc": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultROCPeriod)
			if v, ok := roc(in.closes, p); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "not enough daily closes (or zero reference) for ROC")
			}
		},
		"technical.cmo": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultCMOPeriod)
			if v, ok := cmo(in.closes, p); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "not enough daily closes for CMO")
			}
		},
		"technical.rmi": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultRMIPeriod)
			if v, ok := rmi(in.closes, p, defaultRMIMomentum); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "not enough daily closes for RMI")
			}
		},
		"technical.tsi": func(in computeInput, si *StockIndicator) {
			if v, ok := tsi(in.closes, defaultTSILong, defaultTSIShort); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "not enough daily closes for TSI")
			}
		},
		"technical.trix": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultTRIXPeriod)
			if v, ok := trix(in.closes, p); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "not enough daily closes for TRIX (needs ~3× the period)")
			}
		},
		"technical.ppo": func(in computeInput, si *StockIndicator) {
			line, signal, hist, ok := ppo(in.closes, defaultPPOFast, defaultPPOSlow, defaultPPOSignal)
			if !ok {
				setInsufficient(si, "not enough daily closes for PPO")
				return
			}
			setOK(si, line, unitPercent)
			si.Extra = map[string]float64{"signal": signal, "hist": hist}
		},
		"technical.dpo": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultDPOPeriod)
			if v, ok := dpo(in.closes, p); ok {
				setOK(si, v, unitPrice)
			} else {
				setInsufficient(si, "not enough daily closes for DPO")
			}
		},
		"technical.kst": func(in computeInput, si *StockIndicator) {
			// Daily-variant KST (canonically monthly — see file header).
			if v, ok := kst(in.closes); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "not enough daily closes for KST")
			}
		},
		"technical.stochrsi": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultStochRSIPeriod)
			if v, ok := stochRSI(in.closes, p); ok {
				setOK(si, v, unitRatio)
			} else {
				setInsufficient(si, "not enough daily closes for StochRSI (needs ~2× the period)")
			}
		},
		"technical.uo": func(in computeInput, si *StockIndicator) {
			if v, ok := ultimateOscillator(in.highs, in.lows, in.closes, defaultUOFast, defaultUOMid, defaultUOSlow); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "not enough daily bars for the Ultimate Oscillator")
			}
		},
		"technical.smi": func(in computeInput, si *StockIndicator) {
			if v, ok := stochasticMomentumIndex(in.highs, in.lows, in.closes, defaultSMIPeriod, defaultSMISmooth); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "not enough daily bars for SMI")
			}
		},
		"technical.ao": func(in computeInput, si *StockIndicator) {
			med := hl2(in.highs, in.lows)
			fast, okF := sma(med, 5)
			slow, okS := sma(med, 34)
			if med == nil || !okF || !okS {
				setInsufficient(si, "not enough daily bars for the Awesome Oscillator")
				return
			}
			setOK(si, fast-slow, unitPrice)
		},
		"technical.bull-bear-power": func(in computeInput, si *StockIndicator) {
			e, ok := ema(in.closes, defaultBullBearEMA)
			n := len(in.closes)
			if !ok || len(in.highs) != n || len(in.lows) != n {
				setInsufficient(si, "not enough daily bars for Bull/Bear Power")
				return
			}
			bull := in.highs[n-1] - e
			bear := in.lows[n-1] - e
			setOK(si, bull, unitPrice)
			si.Extra = map[string]float64{"bull": bull, "bear": bear}
		},
		"technical.fisher-transform": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultFisherPeriod)
			if v, ok := fisherTransform(in.highs, in.lows, p); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "not enough daily bars for the Fisher Transform")
			}
		},
		"technical.coppock-curve": func(in computeInput, si *StockIndicator) {
			// Daily-variant Coppock (canonically monthly — see file header).
			if v, ok := coppock(in.closes, 14, 11, 10); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "not enough daily closes for the Coppock curve")
			}
		},
		"technical.tii": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultTIIPeriod)
			if v, ok := trendIntensityIndex(in.closes, p); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "not enough daily closes for the Trend Intensity Index")
			}
		},
		"technical.cfo": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultCFOPeriod)
			fc, ok := linregForecast(in.closes, p)
			if !ok {
				setInsufficient(si, "not enough daily closes for the Forecast Oscillator")
				return
			}
			c := in.closes[len(in.closes)-1] // ok guarantees a full window, so non-empty
			if c == 0 {
				setInsufficient(si, "zero latest price — Forecast Oscillator undefined")
				return
			}
			setOK(si, (c-fc)/c*100, unitPercent)
		},
		"technical.crsi": func(in computeInput, si *StockIndicator) {
			if v, ok := connorsRSI(in.closes); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "not enough daily closes for Connors RSI (needs ~100 bars)")
			}
		},
		"technical.dynamic-mi": func(in computeInput, si *StockIndicator) {
			if v, ok := dynamicMomentumIndex(in.closes, defaultDynamicMIBase, defaultDynamicMIShort, defaultDynamicMILong); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "not enough daily closes for the Dynamic Momentum Index")
			}
		},

		// --- Volatility ---
		"technical.hv": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultHVPeriod)
			s, ok := stdLogReturns(in.closes, p)
			if !ok {
				setInsufficient(si, "not enough daily closes for historical volatility")
				return
			}
			setOK(si, s*math.Sqrt(252)*100, unitPercent)
		},
		"technical.chv": func(in computeInput, si *StockIndicator) {
			if v, ok := chaikinVolatility(in.highs, in.lows, defaultChaikinVolEMA, defaultChaikinVolChange); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "not enough daily bars for Chaikin Volatility")
			}
		},
		"technical.chop": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultChopPeriod)
			if v, ok := choppinessIndex(in.highs, in.lows, in.closes, p); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "not enough daily bars for the Choppiness Index")
			}
		},
		"technical.mi": func(in computeInput, si *StockIndicator) {
			// Daily-variant Mass Index (canonically monthly — see file header).
			if v, ok := massIndex(in.highs, in.lows, 9, 25); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "not enough daily bars for the Mass Index")
			}
		},
		"technical.rwi": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultRWIPeriod)
			high, low, ok := randomWalkIndex(in.highs, in.lows, in.closes, p)
			if !ok {
				setInsufficient(si, "not enough daily bars for the Random Walk Index")
				return
			}
			setOK(si, high, unitNone)
			si.Extra = map[string]float64{"rwiHigh": high, "rwiLow": low}
		},
		"technical.rvi": func(in computeInput, si *StockIndicator) {
			line, signal, ok := relativeVigorIndex(in.opens, in.highs, in.lows, in.closes)
			if !ok {
				setInsufficient(si, "not enough daily bars (or missing opens) for the Relative Vigor Index")
				return
			}
			setOK(si, line, unitNone)
			si.Extra = map[string]float64{"signal": signal}
		},

		// --- Volume ---
		"technical.obv": func(in computeInput, si *StockIndicator) {
			s := obvSeries(in.closes, in.volumes)
			if s == nil || len(s) < 2 {
				setInsufficient(si, "not enough daily bars for OBV")
				return
			}
			setOK(si, s[len(s)-1], unitNone)
		},
		"technical.adl": func(in computeInput, si *StockIndicator) {
			s := adlSeries(in.highs, in.lows, in.closes, in.volumes)
			if s == nil {
				setInsufficient(si, "missing daily OHLCV for the A/D line")
				return
			}
			setOK(si, s[len(s)-1], unitNone)
		},
		"technical.cmf": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultCMFPeriod)
			if v, ok := chaikinMoneyFlow(in.highs, in.lows, in.closes, in.volumes, p); ok {
				setOK(si, v, unitRatio)
			} else {
				setInsufficient(si, "not enough daily bars (or zero volume) for CMF")
			}
		},
		"technical.cho": func(in computeInput, si *StockIndicator) {
			adl := adlSeries(in.highs, in.lows, in.closes, in.volumes)
			fast, okF := ema(adl, 3)
			slow, okS := ema(adl, 10)
			if adl == nil || !okF || !okS {
				setInsufficient(si, "not enough daily bars for the Chaikin Oscillator")
				return
			}
			setOK(si, fast-slow, unitNone)
		},
		"technical.mfi": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultMFIPeriod)
			if v, ok := moneyFlowIndex(in.highs, in.lows, in.closes, in.volumes, p); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "not enough daily bars for the Money Flow Index")
			}
		},
		"technical.fi": func(in computeInput, si *StockIndicator) {
			if v, ok := forceIndex(in.closes, in.volumes, defaultForceIndexEMA); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "not enough daily bars for the Force Index")
			}
		},
		"technical.pvt": func(in computeInput, si *StockIndicator) {
			s, ok := pvtSeries(in.closes, in.volumes)
			if !ok {
				setInsufficient(si, "not enough daily bars (or a zero prior close) for PVT")
				return
			}
			setOK(si, s[len(s)-1], unitNone)
		},
		"technical.pvi-nvi": func(in computeInput, si *StockIndicator) {
			pvi, nvi, ok := pviNvi(in.closes, in.volumes)
			if !ok {
				setInsufficient(si, "not enough daily bars for PVI/NVI")
				return
			}
			setOK(si, pvi, unitNone)
			si.Extra = map[string]float64{"pvi": pvi, "nvi": nvi}
		},
		"technical.kvo": func(in computeInput, si *StockIndicator) {
			if v, ok := klingerVolumeOscillator(in.highs, in.lows, in.closes, in.volumes, defaultKVOFast, defaultKVOSlow); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "not enough daily bars for the Klinger Volume Oscillator")
			}
		},
		"technical.eom-emv": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultEMVPeriod)
			if v, ok := easeOfMovement(in.highs, in.lows, in.volumes, p); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "not enough daily bars (or zero volume/range) for Ease of Movement")
			}
		},
		"technical.vo-pvo": func(in computeInput, si *StockIndicator) {
			fast, okF := ema(in.volumes, defaultVOFast)
			slow, okS := ema(in.volumes, defaultVOSlow)
			if !okF || !okS || slow == 0 {
				setInsufficient(si, "not enough daily volume bars for the Volume Oscillator")
				return
			}
			setOK(si, (fast-slow)/slow*100, unitPercent)
		},
		"technical.vroc": func(in computeInput, si *StockIndicator) {
			p := paramPeriod(si.DefaultParams, defaultVROCPeriod)
			if v, ok := roc(in.volumes, p); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "not enough daily volume bars (or zero reference) for Volume ROC")
			}
		},
	}
}

// emaKey is the Extra-map key for a GMMA EMA line of the given period.
func emaKey(period int) string {
	switch period {
	case 3:
		return "ema3"
	case 5:
		return "ema5"
	case 8:
		return "ema8"
	case 10:
		return "ema10"
	case 12:
		return "ema12"
	case 15:
		return "ema15"
	case 30:
		return "ema30"
	case 35:
		return "ema35"
	case 40:
		return "ema40"
	case 45:
		return "ema45"
	case 50:
		return "ema50"
	case 60:
		return "ema60"
	}
	return "ema"
}

// --- composite math used by the closures above (pure, unit-tested) ---

// demaTema returns the latest Double and Triple EMA: DEMA = 2·EMA1 − EMA2 (EMA of
// EMA), TEMA = 3·EMA1 − 3·EMA2 + EMA3. It needs roughly 3× period defined points;
// ok=false during warmup.
func demaTema(closes []float64, period int) (dema, tema float64, ok bool) {
	e1, ok1 := emaSeries(closes, period)
	if !ok1 {
		return 0, 0, false
	}
	e2full, ok2 := emaSeries(compact(e1), period)
	if !ok2 {
		return 0, 0, false
	}
	e3full, ok3 := emaSeries(compact(e2full), period)
	if !ok3 {
		return 0, 0, false
	}
	v1 := e1[len(e1)-1]
	v2 := e2full[len(e2full)-1]
	v3 := e3full[len(e3full)-1]
	if math.IsNaN(v1) || math.IsNaN(v2) || math.IsNaN(v3) {
		return 0, 0, false
	}
	return 2*v1 - v2, 3*v1 - 3*v2 + v3, true
}

// zlema returns the latest Zero-Lag EMA: with lag = (period−1)/2, build the
// de-lagged series C + (C − C[t−lag]) then EMA it. ok=false when too short.
func zlema(closes []float64, period int) (float64, bool) {
	lag := (period - 1) / 2
	n := len(closes)
	if period <= 0 || n < period+lag {
		return 0, false
	}
	delagged := make([]float64, 0, n-lag)
	for i := lag; i < n; i++ {
		delagged = append(delagged, closes[i]+(closes[i]-closes[i-lag]))
	}
	return ema(delagged, period)
}

// hma returns the latest Hull Moving Average: WMA(2·WMA(C,n/2) − WMA(C,n), √n),
// which reduces lag while staying smooth. ok=false when too short.
func hma(closes []float64, period int) (float64, bool) {
	if period < 2 {
		return 0, false
	}
	half := period / 2
	sqrtN := int(math.Round(math.Sqrt(float64(period))))
	if sqrtN < 1 {
		sqrtN = 1
	}
	wHalf := wmaSeries(closes, half)
	wFull := wmaSeries(closes, period)
	if wHalf == nil || wFull == nil {
		return 0, false
	}
	// Align the two WMA series on their common (newest) tail.
	m := len(wFull)
	if m > len(wHalf) {
		m = len(wHalf)
	}
	raw := make([]float64, m)
	for i := 0; i < m; i++ {
		raw[i] = 2*wHalf[len(wHalf)-m+i] - wFull[len(wFull)-m+i]
	}
	return wma(raw, sqrtN)
}

// kama returns the latest Kaufman Adaptive Moving Average over period with the
// fast/slow EMA bounds (canonically 2 and 30): the efficiency ratio scales the
// smoothing constant between the fast and slow limits. ok=false when too short.
//
// KAMA is a recursive IIR filter: it is SEEDED ONCE at the first computable bar
// (index period-1, the first bar with a full period-window of changes behind it)
// and iterated forward over the ENTIRE history, so the latest value depends on
// every prior bar (like emaSeries / vidya). The seed is closes[period-1] (the
// raw close at the first computable bar — the simplest seed; the SC floor pulls
// it toward the price within a few bars, and over ~250 bars the choice of seed
// is immaterial).
func kama(closes []float64, period, fast, slow int) (float64, bool) {
	n := len(closes)
	if period <= 0 || n < period+1 {
		return 0, false
	}
	fastSC := 2.0 / (float64(fast) + 1)
	slowSC := 2.0 / (float64(slow) + 1)
	kama := closes[period-1] // seed once at the first computable bar
	for i := period; i < n; i++ {
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
		kama += sc * (closes[i] - kama)
	}
	return kama, true
}

// alma returns the latest Arnaud Legoux Moving Average over window with the given
// offset (0..1) and sigma: a Gaussian-weighted sum that trades smoothness against
// responsiveness. ok=false when too short.
func alma(closes []float64, window int, offset, sigma float64) (float64, bool) {
	n := len(closes)
	if window <= 0 || n < window || sigma == 0 {
		return 0, false
	}
	m := offset * float64(window-1)
	s := float64(window) / sigma
	w := closes[n-window:]
	num, den := 0.0, 0.0
	for i := 0; i < window; i++ {
		wt := math.Exp(-((float64(i) - m) * (float64(i) - m)) / (2 * s * s))
		num += wt * w[i]
		den += wt
	}
	if den == 0 {
		return 0, false
	}
	return num / den, true
}

// vidya returns the latest Variable Index Dynamic Average: an EMA whose smoothing
// constant is scaled by the absolute CMO (volatility), so it adapts to trend
// strength. ok=false when too short.
func vidya(closes []float64, period int) (float64, bool) {
	cmoS, ok := cmoSeries(closes, period)
	if !ok {
		return 0, false
	}
	n := len(closes)
	alpha := 2.0 / (float64(period) + 1)
	// Seed at the first index where CMO is defined.
	start := -1
	for i := 0; i < n; i++ {
		if !math.IsNaN(cmoS[i]) {
			start = i
			break
		}
	}
	if start < 0 {
		return 0, false
	}
	v := closes[start]
	for i := start + 1; i < n; i++ {
		k := alpha * math.Abs(cmoS[i]) / 100
		v = closes[i]*k + v*(1-k)
	}
	return v, true
}

// supertrend returns the latest Supertrend value and trend side (+1 up, −1 down)
// using HL2 ± mult·ATR with the standard flip recursion. ok=false when too short.
func supertrend(highs, lows, closes []float64, atrPeriod int, mult float64) (value, trend float64, ok bool) {
	n := len(closes)
	if n < atrPeriod+2 || len(highs) != n || len(lows) != n {
		return 0, 0, false
	}
	tr := trueRange(highs, lows, closes)
	// Wilder ATR series aligned to closes (NaN before atrPeriod).
	atr := make([]float64, n)
	for i := range atr {
		atr[i] = math.NaN()
	}
	seed := 0.0
	for i := 1; i <= atrPeriod; i++ {
		seed += tr[i]
	}
	a := seed / float64(atrPeriod)
	atr[atrPeriod] = a
	for i := atrPeriod + 1; i < n; i++ {
		a = (a*float64(atrPeriod-1) + tr[i]) / float64(atrPeriod)
		atr[i] = a
	}
	var finalUpper, finalLower float64
	dir := 1.0 // start long
	first := true
	for i := atrPeriod; i < n; i++ {
		mid := (highs[i] + lows[i]) / 2
		bUpper := mid + mult*atr[i]
		bLower := mid - mult*atr[i]
		if first {
			finalUpper, finalLower = bUpper, bLower
			first = false
			continue
		}
		if bUpper < finalUpper || closes[i-1] > finalUpper {
			finalUpper = bUpper
		}
		if bLower > finalLower || closes[i-1] < finalLower {
			finalLower = bLower
		}
		if closes[i] > finalUpper {
			dir = 1
		} else if closes[i] < finalLower {
			dir = -1
		}
	}
	if dir > 0 {
		return finalLower, 1, true
	}
	return finalUpper, -1, true
}

// parabolicSAR returns the latest Parabolic SAR value and trend side (+1 up, −1
// down) using the standard AF recursion (step accel, max cap). ok=false when too
// short.
//
// Per Wilder, the projected SAR is first BOUNDED so it cannot penetrate the price
// range of the prior TWO bars (uptrend: the lower of lows[i-1], lows[i-2];
// downtrend: the higher of highs[i-1], highs[i-2]) — and only THEN is the
// penetration/reversal test applied to that bounded SAR.
func parabolicSAR(highs, lows []float64, step, maxAF float64) (sar, trend float64, ok bool) {
	n := len(highs)
	if n < 3 || len(lows) != n {
		return 0, 0, false
	}
	up := highs[1] > highs[0]
	af := step
	var ep float64
	if up {
		sar = lows[0]
		ep = highs[1]
	} else {
		sar = highs[0]
		ep = lows[1]
	}
	for i := 1; i < n; i++ {
		sar = sar + af*(ep-sar)
		if up {
			// Bound the SAR by the lower of the prior two lows BEFORE the
			// penetration test.
			if i >= 2 {
				sar = math.Min(sar, math.Min(lows[i-1], lows[i-2]))
			} else {
				sar = math.Min(sar, lows[i-1])
			}
			if lows[i] < sar { // flip to down
				up = false
				sar = ep
				ep = lows[i]
				af = step
				continue
			}
			if highs[i] > ep {
				ep = highs[i]
				af = math.Min(af+step, maxAF)
			}
		} else {
			// Bound the SAR by the higher of the prior two highs BEFORE the
			// penetration test.
			if i >= 2 {
				sar = math.Max(sar, math.Max(highs[i-1], highs[i-2]))
			} else {
				sar = math.Max(sar, highs[i-1])
			}
			if highs[i] > sar { // flip to up
				up = true
				sar = ep
				ep = highs[i]
				af = step
				continue
			}
			if lows[i] < ep {
				ep = lows[i]
				af = math.Min(af+step, maxAF)
			}
		}
	}
	if up {
		return sar, 1, true
	}
	return sar, -1, true
}

// vortex returns the latest +VI / −VI: +VI = Σ|H−Lprev|/ΣTR, −VI = Σ|L−Hprev|/ΣTR
// over the trailing period. ok=false when too short or ΣTR is zero.
func vortex(highs, lows, closes []float64, period int) (plus, minus float64, ok bool) {
	n := len(closes)
	if period <= 0 || n < period+1 || len(highs) != n || len(lows) != n {
		return 0, 0, false
	}
	tr := trueRange(highs, lows, closes)
	sumVMPlus, sumVMMinus, sumTR := 0.0, 0.0, 0.0
	for i := n - period; i < n; i++ {
		sumVMPlus += math.Abs(highs[i] - lows[i-1])
		sumVMMinus += math.Abs(lows[i] - highs[i-1])
		sumTR += tr[i]
	}
	if sumTR == 0 {
		return 0, 0, false
	}
	return sumVMPlus / sumTR, sumVMMinus / sumTR, true
}

// cci returns the latest Commodity Channel Index: (HLC3 − SMA(HLC3,period)) /
// (0.015·mean-abs-deviation). ok=false when too short or the deviation is zero.
func cci(highs, lows, closes []float64, period int) (float64, bool) {
	tp := hlc3(highs, lows, closes)
	if tp == nil || len(tp) < period {
		return 0, false
	}
	window := tp[len(tp)-period:]
	mean := 0.0
	for _, v := range window {
		mean += v
	}
	mean /= float64(period)
	mad := 0.0
	for _, v := range window {
		mad += math.Abs(v - mean)
	}
	mad /= float64(period)
	if mad == 0 {
		return 0, false
	}
	return (window[period-1] - mean) / (0.015 * mad), true
}

// rmi returns the latest Relative Momentum Index: an RSI computed over the
// momentum series C − C[t−momentum] rather than the 1-bar delta. ok=false when
// too short.
func rmi(closes []float64, period, momentum int) (float64, bool) {
	n := len(closes)
	if period <= 0 || momentum <= 0 || n < period+momentum+1 {
		return 0, false
	}
	mom := make([]float64, 0, n-momentum)
	for i := momentum; i < n; i++ {
		mom = append(mom, closes[i]-closes[i-momentum])
	}
	// Wilder-smooth gains/losses of the momentum series.
	gain, loss := 0.0, 0.0
	for i := 1; i <= period; i++ {
		d := mom[i] - mom[i-1]
		if d >= 0 {
			gain += d
		} else {
			loss -= d
		}
	}
	avgGain := gain / float64(period)
	avgLoss := loss / float64(period)
	for i := period + 1; i < len(mom); i++ {
		d := mom[i] - mom[i-1]
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

// tsi returns the latest True Strength Index: 100·EMA(EMA(ΔC,long),short) /
// EMA(EMA(|ΔC|,long),short). ok=false when too short or the denominator is zero.
func tsi(closes []float64, long, short int) (float64, bool) {
	n := len(closes)
	if n < long+short+1 {
		return 0, false
	}
	delta := make([]float64, n-1)
	absDelta := make([]float64, n-1)
	for i := 1; i < n; i++ {
		delta[i-1] = closes[i] - closes[i-1]
		absDelta[i-1] = math.Abs(delta[i-1])
	}
	num, okN := doubleSmoothEMA(delta, long, short)
	den, okD := doubleSmoothEMA(absDelta, long, short)
	if !okN || !okD || den == 0 {
		return 0, false
	}
	return 100 * num / den, true
}

// doubleSmoothEMA returns the latest EMA(EMA(values,first),second). ok=false
// during warmup.
func doubleSmoothEMA(values []float64, first, second int) (float64, bool) {
	e1, ok := emaSeries(values, first)
	if !ok {
		return 0, false
	}
	return ema(compact(e1), second)
}

// trix returns the latest TRIX: the 1-bar percent rate-of-change of a triple-EMA
// of closes. ok=false during warmup or on a zero prior value.
func trix(closes []float64, period int) (float64, bool) {
	e1, ok1 := emaSeries(closes, period)
	if !ok1 {
		return 0, false
	}
	e2, ok2 := emaSeries(compact(e1), period)
	if !ok2 {
		return 0, false
	}
	e3, ok3 := emaSeries(compact(e2), period)
	if !ok3 {
		return 0, false
	}
	t := compact(e3)
	if len(t) < 2 || t[len(t)-2] == 0 {
		return 0, false
	}
	return (t[len(t)-1] - t[len(t)-2]) / t[len(t)-2] * 100, true
}

// ppo returns the latest Percentage Price Oscillator line, signal and histogram:
// PPO = (EMAfast − EMAslow)/EMAslow·100, signal = EMA(PPO,signal). ok=false during
// warmup or on a zero slow EMA.
func ppo(closes []float64, fast, slow, signal int) (line, sig, hist float64, ok bool) {
	ef, okF := emaSeries(closes, fast)
	es, okS := emaSeries(closes, slow)
	if !okF || !okS {
		return 0, 0, 0, false
	}
	ppoSeries := make([]float64, 0, len(closes))
	for i := range closes {
		f, s := ef[i], es[i]
		if math.IsNaN(f) || math.IsNaN(s) || s == 0 {
			continue
		}
		ppoSeries = append(ppoSeries, (f-s)/s*100)
	}
	if len(ppoSeries) == 0 {
		return 0, 0, 0, false
	}
	sigVal, okSig := ema(ppoSeries, signal)
	if !okSig {
		return 0, 0, 0, false
	}
	line = ppoSeries[len(ppoSeries)-1]
	return line, sigVal, line - sigVal, true
}

// dpo returns the latest Detrended Price Oscillator: C[t−(period/2+1)] −
// SMA(C,period), removing the trend to expose cycles. ok=false when too short.
func dpo(closes []float64, period int) (float64, bool) {
	n := len(closes)
	shift := period/2 + 1
	if period <= 0 || n < period+shift {
		return 0, false
	}
	ma, ok := sma(closes, period)
	if !ok {
		return 0, false
	}
	return closes[n-1-shift] - ma, true
}

// kst returns the latest Know Sure Thing (daily-variant): the weighted sum of four
// SMA-smoothed ROCs (10/15/20/30, smoothed 10/10/10/15, weights 1/2/3/4).
// ok=false when too short.
func kst(closes []float64) (float64, bool) {
	rocPeriods := []int{10, 15, 20, 30}
	smaPeriods := []int{10, 10, 10, 15}
	weights := []float64{1, 2, 3, 4}
	sum := 0.0
	for i := range rocPeriods {
		rs := rocSeries(closes, rocPeriods[i])
		if rs == nil {
			return 0, false
		}
		smoothed := smaSeries(compact(rs), smaPeriods[i])
		if len(smoothed) == 0 {
			return 0, false
		}
		sum += smoothed[len(smoothed)-1] * weights[i]
	}
	return sum, true
}

// stochRSI returns the latest Stochastic RSI: (RSI − minRSI)/(maxRSI − minRSI)
// over the trailing period of an RSI series (0..1). ok=false when too short or the
// RSI window is flat.
func stochRSI(closes []float64, period int) (float64, bool) {
	rs, ok := rsiSeries(closes, period)
	if !ok {
		return 0, false
	}
	defined := compact(rs)
	if len(defined) < period {
		return 0, false
	}
	window := defined[len(defined)-period:]
	lo, hi := window[0], window[0]
	for _, v := range window {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	if hi == lo {
		return 0, false
	}
	return (window[period-1] - lo) / (hi - lo), true
}

// ultimateOscillator returns the latest Ultimate Oscillator: a weighted blend of
// buying-pressure/true-range ratios over three windows (fast/mid/slow). ok=false
// when too short or a true-range sum is zero.
func ultimateOscillator(highs, lows, closes []float64, fast, mid, slow int) (float64, bool) {
	n := len(closes)
	if n < slow+1 || len(highs) != n || len(lows) != n {
		return 0, false
	}
	bp := make([]float64, n) // buying pressure
	tr := make([]float64, n)
	for i := 1; i < n; i++ {
		minLC := math.Min(lows[i], closes[i-1])
		maxHC := math.Max(highs[i], closes[i-1])
		bp[i] = closes[i] - minLC
		tr[i] = maxHC - minLC
	}
	avg := func(p int) (float64, bool) {
		sbp, str := 0.0, 0.0
		for i := n - p; i < n; i++ {
			sbp += bp[i]
			str += tr[i]
		}
		if str == 0 {
			return 0, false
		}
		return sbp / str, true
	}
	a1, ok1 := avg(fast)
	a2, ok2 := avg(mid)
	a3, ok3 := avg(slow)
	if !ok1 || !ok2 || !ok3 {
		return 0, false
	}
	return 100 * (4*a1 + 2*a2 + a3) / 7, true
}

// stochasticMomentumIndex returns the latest SMI: a double-EMA-smoothed
// (C − midpoint)/(½·range) over the period window, scaled to ±100. ok=false when
// too short or the smoothed range is zero.
func stochasticMomentumIndex(highs, lows, closes []float64, period, smooth int) (float64, bool) {
	n := len(closes)
	if period <= 0 || n < period+2*smooth || len(highs) != n || len(lows) != n {
		return 0, false
	}
	rel := make([]float64, 0, n-period+1) // C − midpoint
	rng := make([]float64, 0, n-period+1) // high−low range
	for i := period - 1; i < n; i++ {
		hi, lo := highs[i], lows[i]
		for j := i - period + 1; j <= i; j++ {
			if highs[j] > hi {
				hi = highs[j]
			}
			if lows[j] < lo {
				lo = lows[j]
			}
		}
		mid := (hi + lo) / 2
		rel = append(rel, closes[i]-mid)
		rng = append(rng, hi-lo)
	}
	num, okN := doubleSmoothEMA(rel, smooth, smooth)
	den, okD := doubleSmoothEMA(rng, smooth, smooth)
	if !okN || !okD || den == 0 {
		return 0, false
	}
	return 100 * num / (den / 2), true
}

// fisherTransform returns the latest Fisher Transform: ½·ln((1+x)/(1−x)) on the
// price position x normalized to (−1,1) over the period window. ok=false when too
// short or the window is flat.
func fisherTransform(highs, lows []float64, period int) (float64, bool) {
	n := len(highs)
	if period <= 0 || n < period || len(lows) != n {
		return 0, false
	}
	med := hl2(highs, lows)
	value := 0.0
	fish := 0.0
	start := period - 1
	for i := start; i < n; i++ {
		hi, lo := med[i], med[i]
		for j := i - period + 1; j <= i; j++ {
			if med[j] > hi {
				hi = med[j]
			}
			if med[j] < lo {
				lo = med[j]
			}
		}
		rng := hi - lo
		x := 0.0
		if rng != 0 {
			x = 2*((med[i]-lo)/rng) - 1
		}
		value = 0.66*x + 0.67*value
		value = math.Max(-0.999, math.Min(0.999, value))
		fish = 0.5*math.Log((1+value)/(1-value)) + 0.5*fish
	}
	return fish, true
}

// coppock returns the latest Coppock Curve (daily-variant): WMA(ROClong + ROCshort,
// wmaPeriod). ok=false when too short.
func coppock(closes []float64, longROC, shortROC, wmaPeriod int) (float64, bool) {
	rLong := rocSeries(closes, longROC)
	rShort := rocSeries(closes, shortROC)
	if rLong == nil || rShort == nil {
		return 0, false
	}
	sum := make([]float64, 0, len(closes))
	for i := range closes {
		if math.IsNaN(rLong[i]) || math.IsNaN(rShort[i]) {
			continue
		}
		sum = append(sum, rLong[i]+rShort[i])
	}
	if len(sum) < wmaPeriod {
		return 0, false
	}
	return wma(sum, wmaPeriod)
}

// trendIntensityIndex returns the latest Trend Intensity Index: 100·Σpos-dev /
// (Σpos-dev + Σneg-dev) of close-minus-SMA over the trailing half-period. ok=false
// when too short or all deviations are zero.
func trendIntensityIndex(closes []float64, period int) (float64, bool) {
	n := len(closes)
	half := period / 2
	if half <= 0 || n < period+half {
		return 0, false
	}
	maSeries := smaSeries(closes, period)
	if len(maSeries) < half {
		return 0, false
	}
	posSum, negSum := 0.0, 0.0
	for k := 0; k < half; k++ {
		c := closes[n-1-k]
		m := maSeries[len(maSeries)-1-k]
		dev := c - m
		if dev > 0 {
			posSum += dev
		} else {
			negSum += -dev
		}
	}
	tot := posSum + negSum
	if tot == 0 {
		return 0, false
	}
	return 100 * posSum / tot, true
}

// connorsRSI returns the latest Connors RSI: the mean of RSI(C,3), RSI(streak,2),
// and percentRank(1-bar return, 100). ok=false when too short (needs ~100 bars).
func connorsRSI(closes []float64) (float64, bool) {
	n := len(closes)
	if n < 102 {
		return 0, false
	}
	rsiPrice, ok1 := rsiWilder(closes, 3)
	if !ok1 {
		return 0, false
	}
	// Up/down streak series (consecutive same-direction closes; sign carries).
	streak := make([]float64, n)
	for i := 1; i < n; i++ {
		switch {
		case closes[i] > closes[i-1]:
			if streak[i-1] > 0 {
				streak[i] = streak[i-1] + 1
			} else {
				streak[i] = 1
			}
		case closes[i] < closes[i-1]:
			if streak[i-1] < 0 {
				streak[i] = streak[i-1] - 1
			} else {
				streak[i] = -1
			}
		default:
			streak[i] = 0
		}
	}
	rsiStreak, ok2 := rsiWilder(streak, 2)
	if !ok2 {
		return 0, false
	}
	// 1-bar percent returns, then percent-rank the latest over the prior 100.
	rets := make([]float64, 0, n-1)
	for i := 1; i < n; i++ {
		if closes[i-1] == 0 {
			return 0, false
		}
		rets = append(rets, (closes[i]-closes[i-1])/closes[i-1]*100)
	}
	pr, ok3 := percentRank(rets, 100)
	if !ok3 {
		return 0, false
	}
	return (rsiPrice + rsiStreak + pr) / 3, true
}

// dynamicMomentumIndex returns the latest Dynamic Momentum Index: an RSI whose
// period adapts to the volatility ratio (recent σ / long σ) around a base period.
// ok=false when too short.
func dynamicMomentumIndex(closes []float64, base, shortVol, longVol int) (float64, bool) {
	n := len(closes)
	if n < longVol+base+1 {
		return 0, false
	}
	sdShort, ok1 := rollingStd(closes, shortVol)
	sdLong, ok2 := rollingStd(closes, longVol)
	if !ok1 || !ok2 || sdLong == 0 || sdShort == 0 {
		return 0, false
	}
	period := int(math.Round(float64(base) / (sdShort / sdLong)))
	if period < 3 {
		period = 3
	}
	if period > 30 {
		period = 30
	}
	return rsiWilder(closes, period)
}

// chaikinVolatility returns the latest Chaikin Volatility: the percent change of
// EMA(H−L,emaPeriod) over the change window. ok=false when too short or the
// reference EMA is zero.
func chaikinVolatility(highs, lows []float64, emaPeriod, change int) (float64, bool) {
	n := len(highs)
	if n == 0 || len(lows) != n {
		return 0, false
	}
	spread := make([]float64, n)
	for i := 0; i < n; i++ {
		spread[i] = highs[i] - lows[i]
	}
	es, ok := emaSeries(spread, emaPeriod)
	if !ok {
		return 0, false
	}
	defined := compact(es)
	if len(defined) < change+1 {
		return 0, false
	}
	cur := defined[len(defined)-1]
	ref := defined[len(defined)-1-change]
	if ref == 0 {
		return 0, false
	}
	return (cur - ref) / ref * 100, true
}

// choppinessIndex returns the latest Choppiness Index: 100·log10(ΣTR/(n-high −
// n-low))/log10(period), where ~100 = choppy, ~0 = trending. ok=false when too
// short or the range is zero.
func choppinessIndex(highs, lows, closes []float64, period int) (float64, bool) {
	n := len(closes)
	if period <= 1 || n < period+1 || len(highs) != n || len(lows) != n {
		return 0, false
	}
	tr := trueRange(highs, lows, closes)
	sumTR := 0.0
	for i := n - period; i < n; i++ {
		sumTR += tr[i]
	}
	hi, _ := highestHigh(highs, period)
	lo, _ := lowestLow(lows, period)
	rng := hi - lo
	if rng <= 0 || sumTR <= 0 {
		return 0, false
	}
	return 100 * math.Log10(sumTR/rng) / math.Log10(float64(period)), true
}

// massIndex returns the latest Mass Index (daily-variant): the sum over sumPeriod
// of EMA(H−L,emaPeriod)/EMA(EMA(H−L,emaPeriod),emaPeriod). ok=false when too short.
func massIndex(highs, lows []float64, emaPeriod, sumPeriod int) (float64, bool) {
	n := len(highs)
	if n == 0 || len(lows) != n {
		return 0, false
	}
	spread := make([]float64, n)
	for i := 0; i < n; i++ {
		spread[i] = highs[i] - lows[i]
	}
	e1, ok1 := emaSeries(spread, emaPeriod)
	if !ok1 {
		return 0, false
	}
	e2, ok2 := emaSeries(compact(e1), emaPeriod)
	if !ok2 {
		return 0, false
	}
	// Align the EMA1 and EMA2 series on their common tail (ratio per bar).
	d1 := compact(e1)
	d2 := compact(e2)
	m := len(d2)
	if len(d1) < m {
		m = len(d1)
	}
	if m < sumPeriod {
		return 0, false
	}
	ratios := make([]float64, m)
	for i := 0; i < m; i++ {
		den := d2[len(d2)-m+i]
		if den == 0 {
			return 0, false
		}
		ratios[i] = d1[len(d1)-m+i] / den
	}
	sum := 0.0
	for i := m - sumPeriod; i < m; i++ {
		sum += ratios[i]
	}
	return sum, true
}

// randomWalkIndex returns the latest RWI-high and RWI-low: (H − L[t−k])/(ATR·√k)
// and (H[t−k] − L)/(ATR·√k) maximized over k = 2..period. ok=false when too short.
func randomWalkIndex(highs, lows, closes []float64, period int) (high, low float64, ok bool) {
	n := len(closes)
	if period < 2 || n < period+1 || len(highs) != n || len(lows) != n {
		return 0, 0, false
	}
	atr, okA := atrWilder(highs, lows, closes, period)
	if !okA || atr == 0 {
		return 0, 0, false
	}
	maxHigh, maxLow := math.Inf(-1), math.Inf(-1)
	for k := 2; k <= period; k++ {
		denom := atr * math.Sqrt(float64(k))
		if denom == 0 {
			continue
		}
		h := (highs[n-1] - lows[n-1-k]) / denom
		l := (highs[n-1-k] - lows[n-1]) / denom
		if h > maxHigh {
			maxHigh = h
		}
		if l > maxLow {
			maxLow = l
		}
	}
	if math.IsInf(maxHigh, -1) || math.IsInf(maxLow, -1) {
		return 0, 0, false
	}
	return maxHigh, maxLow, true
}

// relativeVigorIndex returns the latest RVI line and its signal: a 4-bar-weighted
// (C−O) over a 4-bar-weighted (H−L), then a 4-bar symmetric signal. Needs Open.
// ok=false when too short or the weighted range is zero.
func relativeVigorIndex(opens, highs, lows, closes []float64) (line, signal float64, ok bool) {
	n := len(closes)
	if n < 8 || len(opens) != n || len(highs) != n || len(lows) != n {
		return 0, 0, false
	}
	co := make([]float64, n)
	hl := make([]float64, n)
	for i := 0; i < n; i++ {
		co[i] = closes[i] - opens[i]
		hl[i] = highs[i] - lows[i]
	}
	// 4-bar weighted (1,2,2,1)/6 numerator and denominator series.
	weighted := func(s []float64, i int) float64 {
		return (s[i] + 2*s[i-1] + 2*s[i-2] + s[i-3]) / 6
	}
	rvi := make([]float64, 0, n-3)
	for i := 3; i < n; i++ {
		den := weighted(hl, i)
		if den == 0 {
			return 0, 0, false
		}
		rvi = append(rvi, weighted(co, i)/den)
	}
	if len(rvi) < 4 {
		return 0, 0, false
	}
	last := len(rvi) - 1
	line = rvi[last]
	signal = (rvi[last] + 2*rvi[last-1] + 2*rvi[last-2] + rvi[last-3]) / 6
	return line, signal, true
}

// williamsFractal reports whether the most recently completed center bar (2 bars
// from the end) is an up-fractal (its high exceeds the two bars each side) and/or
// a down-fractal (its low is below the two bars each side). ok=false with fewer
// than 5 bars.
func williamsFractal(highs, lows []float64) (up, down, ok bool) {
	n := len(highs)
	if n < 5 || len(lows) != n {
		return false, false, false
	}
	c := n - 3 // center of the trailing 5-bar window
	up = highs[c] > highs[c-1] && highs[c] > highs[c-2] && highs[c] > highs[c+1] && highs[c] > highs[c+2]
	down = lows[c] < lows[c-1] && lows[c] < lows[c-2] && lows[c] < lows[c+1] && lows[c] < lows[c+2]
	return up, down, true
}

// chaikinMoneyFlow returns the latest Chaikin Money Flow: Σ(MFM·V)/ΣV over the
// trailing period. ok=false when too short or volume sums to zero.
func chaikinMoneyFlow(highs, lows, closes, volumes []float64, period int) (float64, bool) {
	n := len(closes)
	if period <= 0 || n < period || len(highs) != n || len(lows) != n || len(volumes) != n {
		return 0, false
	}
	mfvSum, volSum := 0.0, 0.0
	for i := n - period; i < n; i++ {
		mfvSum += moneyFlowMultiplier(highs[i], lows[i], closes[i]) * volumes[i]
		volSum += volumes[i]
	}
	if volSum == 0 {
		return 0, false
	}
	return mfvSum / volSum, true
}

// moneyFlowIndex returns the latest Money Flow Index: 100 − 100/(1 + positive
// money flow / negative money flow) over the trailing period. ok=false when too
// short.
func moneyFlowIndex(highs, lows, closes, volumes []float64, period int) (float64, bool) {
	n := len(closes)
	if period <= 0 || n < period+1 || len(highs) != n || len(lows) != n || len(volumes) != n {
		return 0, false
	}
	tp := hlc3(highs, lows, closes)
	pos, neg := 0.0, 0.0
	for i := n - period; i < n; i++ {
		rmf := tp[i] * volumes[i]
		switch {
		case tp[i] > tp[i-1]:
			pos += rmf
		case tp[i] < tp[i-1]:
			neg += rmf
		}
	}
	if neg == 0 {
		if pos == 0 {
			return 50, true // no flow either way
		}
		return 100, true
	}
	ratio := pos / neg
	return 100 - 100/(1+ratio), true
}

// forceIndex returns the latest Force Index: EMA((C − Cprev)·V, period). ok=false
// when too short.
func forceIndex(closes, volumes []float64, period int) (float64, bool) {
	n := len(closes)
	if n < 2 || len(volumes) != n {
		return 0, false
	}
	raw := make([]float64, n-1)
	for i := 1; i < n; i++ {
		raw[i-1] = (closes[i] - closes[i-1]) * volumes[i]
	}
	return ema(raw, period)
}

// pvtSeries returns the Price Volume Trend series: prev + V·(C − Cprev)/Cprev.
// ok=false when too short or a prior close is zero (the ratio is undefined).
func pvtSeries(closes, volumes []float64) ([]float64, bool) {
	n := len(closes)
	if n < 2 || len(volumes) != n {
		return nil, false
	}
	out := make([]float64, n)
	for i := 1; i < n; i++ {
		if closes[i-1] == 0 {
			return nil, false
		}
		out[i] = out[i-1] + volumes[i]*(closes[i]-closes[i-1])/closes[i-1]
	}
	return out, true
}

// pviNvi returns the latest Positive and Negative Volume Index: PVI updates on
// rising-volume days, NVI on falling-volume days, each by the close return; both
// start at 1000. ok=false when too short.
func pviNvi(closes, volumes []float64) (pvi, nvi float64, ok bool) {
	n := len(closes)
	if n < 2 || len(volumes) != n {
		return 0, 0, false
	}
	pvi, nvi = 1000, 1000
	for i := 1; i < n; i++ {
		if closes[i-1] == 0 {
			return 0, 0, false
		}
		ret := (closes[i] - closes[i-1]) / closes[i-1]
		if volumes[i] > volumes[i-1] {
			pvi += pvi * ret
		} else if volumes[i] < volumes[i-1] {
			nvi += nvi * ret
		}
	}
	return pvi, nvi, true
}

// klingerVolumeOscillator returns the latest Klinger Volume Oscillator:
// EMA(VF,fast) − EMA(VF,slow), where VF is the canonical signed Volume Force.
//
// Volume Force carries BOTH the HLC3 trend direction AND the trend's magnitude
// (Klinger, via TradingView/Investopedia): with dm = High−Low and cm a running
// cumulative measurement that accumulates dm while the trend persists and resets
// to dm + dm_prev on a trend flip,
//
//	trend = +1 if HLC3 > prior HLC3, −1 if lower, and CARRIES the prior bar's
//	         trend forward when HLC3 is unchanged (never defaults to +1);
//	VF    = volume · |2·((dm/cm) − 1)| · trend · 100   (cm==0 → VF contributes 0).
//
// ok=false when too short.
func klingerVolumeOscillator(highs, lows, closes, volumes []float64, fast, slow int) (float64, bool) {
	n := len(closes)
	if n < slow+2 || len(highs) != n || len(lows) != n || len(volumes) != n {
		return 0, false
	}
	tp := hlc3(highs, lows, closes)
	vf := make([]float64, 0, n-1)
	trend := 1.0  // prior-bar trend, carried forward on an unchanged HLC3
	cm := 0.0     // cumulative measurement
	dmPrev := 0.0 // prior bar's daily measurement (High−Low)
	for i := 1; i < n; i++ {
		dm := highs[i] - lows[i]
		newTrend := trend // carry prior trend when HLC3 is unchanged
		switch {
		case tp[i] > tp[i-1]:
			newTrend = 1.0
		case tp[i] < tp[i-1]:
			newTrend = -1.0
		}
		if newTrend != trend {
			cm = dm + dmPrev // reset on a trend flip
		} else {
			cm += dm // accumulate while the trend persists
		}
		force := 0.0
		if cm != 0 {
			force = volumes[i] * math.Abs(2*((dm/cm)-1)) * newTrend * 100
		}
		vf = append(vf, force)
		trend = newTrend
		dmPrev = dm
	}
	ef, okF := ema(vf, fast)
	es, okS := ema(vf, slow)
	if !okF || !okS {
		return 0, false
	}
	return ef - es, true
}

// easeOfMovement returns the latest SMA-smoothed Ease of Movement: the period
// average of (HL2 − HL2prev) / (V/scale / (H−L)). ok=false when too short or a
// per-bar denominator is zero.
func easeOfMovement(highs, lows, volumes []float64, period int) (float64, bool) {
	n := len(highs)
	if period <= 0 || n < period+1 || len(lows) != n || len(volumes) != n {
		return 0, false
	}
	const scale = 1e8 // box-ratio scale (standard EMV convention)
	emv := make([]float64, n)
	for i := 1; i < n; i++ {
		mid := (highs[i] + lows[i]) / 2
		midPrev := (highs[i-1] + lows[i-1]) / 2
		rng := highs[i] - lows[i]
		if rng == 0 || volumes[i] == 0 {
			return 0, false
		}
		boxRatio := (volumes[i] / scale) / rng
		if boxRatio == 0 {
			return 0, false
		}
		emv[i] = (mid - midPrev) / boxRatio
	}
	return sma(emv[1:], period)
}
