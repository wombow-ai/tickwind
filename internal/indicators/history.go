package indicators

import (
	"math"

	"github.com/wombow-ai/tickwind/internal/store"
)

// HistorySeries is one technical indicator computed across a ticker's FULL daily candle
// history — the date-aligned line (a TradingView-style indicator chart over time), the
// time-series counterpart to the single-point StockIndicator. Every value is deterministic
// Go math over public candles, so it carries the same anti-hallucination guarantee as the
// point values (the LLM never invents a number; it can only describe what Go computed).
// Points are oldest→newest; warmup bars where the indicator is undefined are OMITTED, never
// fabricated. Lines carries the extra aligned bands (MACD signal/histogram, Bollinger
// upper/lower) so a multi-line indicator renders as one chart.
type HistorySeries struct {
	Indicator string             `json:"indicator"`       // catalog id, e.g. technical.rsi
	Period    int                `json:"period,omitempty"`
	Unit      string             `json:"unit"`            // % | price | ratio | x | usd | ""
	Points    []Point            `json:"points"`          // the primary line, oldest→newest
	Lines     map[string][]Point `json:"lines,omitempty"` // extra aligned lines (signal/histogram, upper/lower)
}

// historyDefaults maps each history-capable indicator id to its default period (0 when the
// period field is not meaningful, e.g. MACD which is parameterised by fast/slow/signal).
var historyDefaults = map[string]int{
	"technical.sma-ma": defaultSMAPeriod,
	"technical.ema":    defaultEMAPeriod,
	"technical.rsi":    defaultRSIPeriod,
	"technical.macd":   0,
	"technical.boll":   defaultBollPeriod,
}

// HistoryableID reports whether an indicator id has a time-series history implementation.
func HistoryableID(id string) bool {
	_, ok := historyDefaults[id]
	return ok
}

// HistoryableIDs returns the supported history indicator ids (unordered).
func HistoryableIDs() []string {
	ids := make([]string, 0, len(historyDefaults))
	for id := range historyDefaults {
		ids = append(ids, id)
	}
	return ids
}

// IndicatorHistory computes the time series for a supported technical indicator over the
// ticker's daily candles (oldest→newest, as DailyCandles returns them). ok=false when the id
// is unsupported or there is insufficient history to compute even one point. period<=0 uses
// the catalog default. Each series uses the SAME math as the single-point computeFn, so the
// chart's latest point equals the value shown on the stock page.
func IndicatorHistory(candles []store.Candle, id string, period int) (HistorySeries, bool) {
	if len(candles) == 0 {
		return HistorySeries{}, false
	}
	closes := make([]float64, len(candles))
	for i, c := range candles {
		closes[i] = c.Close
	}

	switch id {
	case "technical.sma-ma":
		if period <= 0 {
			period = defaultSMAPeriod
		}
		compact := smaSeries(closes, period)
		if compact == nil {
			return HistorySeries{}, false
		}
		// smaSeries is COMPACT (length n-period+1); element j aligns to closes[j+period-1].
		full := nanPadded(len(closes))
		for j, v := range compact {
			full[j+period-1] = v
		}
		return finishSingle(candles, id, period, unitPrice, full)

	case "technical.ema":
		if period <= 0 {
			period = defaultEMAPeriod
		}
		s, ok := emaSeries(closes, period)
		if !ok {
			return HistorySeries{}, false
		}
		return finishSingle(candles, id, period, unitPrice, s)

	case "technical.rsi":
		if period <= 0 {
			period = defaultRSIPeriod
		}
		s, ok := rsiSeries(closes, period)
		if !ok {
			return HistorySeries{}, false
		}
		return finishSingle(candles, id, period, unitNone, s)

	case "technical.macd":
		line, sig, hist, ok := macdSeriesPadded(closes, defaultMACDFast, defaultMACDSlow, defaultMACDSignal)
		if !ok {
			return HistorySeries{}, false
		}
		pts := pointsFromPadded(candles, line)
		if len(pts) == 0 {
			return HistorySeries{}, false
		}
		return HistorySeries{
			Indicator: id,
			Unit:      unitNone, // matches the macd computeFn headline unit
			Points:    pts,
			Lines: map[string][]Point{
				"signal":    pointsFromPadded(candles, sig),
				"histogram": pointsFromPadded(candles, hist),
			},
		}, true

	case "technical.boll":
		if period <= 0 {
			period = defaultBollPeriod
		}
		mid, up, low, ok := bollSeriesPadded(closes, period, defaultBollMult)
		if !ok {
			return HistorySeries{}, false
		}
		pts := pointsFromPadded(candles, mid)
		if len(pts) == 0 {
			return HistorySeries{}, false
		}
		return HistorySeries{
			Indicator: id,
			Period:    period,
			Unit:      unitPrice,
			Points:    pts,
			Lines: map[string][]Point{
				"upper": pointsFromPadded(candles, up),
				"lower": pointsFromPadded(candles, low),
			},
		}, true
	}
	return HistorySeries{}, false
}

// finishSingle builds a single-line HistorySeries from a full-length padded series, failing
// (ok=false) when no point is defined.
func finishSingle(candles []store.Candle, id string, period int, unit string, full []float64) (HistorySeries, bool) {
	pts := pointsFromPadded(candles, full)
	if len(pts) == 0 {
		return HistorySeries{}, false
	}
	return HistorySeries{Indicator: id, Period: period, Unit: unit, Points: pts}, true
}

// nanPadded returns a length-n slice pre-filled with NaN (the "undefined / warmup" marker).
func nanPadded(n int) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = math.NaN()
	}
	return s
}

// pointsFromPadded emits dated points for a full-length (== len(candles)) series, skipping
// any warmup / undefined bar (NaN or Inf). Values are rounded to 4 decimals to bound payload
// size without visible precision loss.
func pointsFromPadded(candles []store.Candle, series []float64) []Point {
	pts := make([]Point, 0, len(series))
	for i, v := range series {
		if i >= len(candles) || math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		pts = append(pts, Point{Date: candles[i].Time.Format("2006-01-02"), Value: math.Round(v*1e4) / 1e4})
	}
	return pts
}

// macdSeriesPadded returns full-length (NaN-padded, aligned to closes) MACD line / signal /
// histogram series, using the SAME convention as macd(): the signal EMA is taken over the
// COMPACTED MACD line (the bars where both EMAs are defined), then mapped back to dates.
func macdSeriesPadded(closes []float64, fast, slow, signal int) (line, sig, hist []float64, ok bool) {
	emaFast, okF := emaSeries(closes, fast)
	emaSlow, okS := emaSeries(closes, slow)
	if !okF || !okS {
		return nil, nil, nil, false
	}
	n := len(closes)
	line = nanPadded(n)
	sig = nanPadded(n)
	hist = nanPadded(n)
	idx := make([]int, 0, n)    // closes indices where the MACD line is defined, in order
	defined := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		f, s := emaFast[i], emaSlow[i]
		if !math.IsNaN(f) && !math.IsNaN(s) {
			d := f - s
			line[i] = d
			idx = append(idx, i)
			defined = append(defined, d)
		}
	}
	if len(defined) == 0 {
		return nil, nil, nil, false
	}
	signalSeries, okSig := emaSeries(defined, signal)
	if !okSig {
		return nil, nil, nil, false
	}
	for j, v := range signalSeries {
		if math.IsNaN(v) {
			continue
		}
		i := idx[j]
		sig[i] = v
		hist[i] = line[i] - v
	}
	return line, sig, hist, true
}

// bollSeriesPadded returns full-length (NaN-padded) Bollinger middle / upper / lower bands,
// matching bollinger(): middle = SMA(period); bands = middle ± mult·σ where σ is the
// POPULATION standard deviation (÷period) over the same window.
func bollSeriesPadded(closes []float64, period int, mult float64) (mid, up, low []float64, ok bool) {
	n := len(closes)
	if period <= 0 || n < period {
		return nil, nil, nil, false
	}
	mid = nanPadded(n)
	up = nanPadded(n)
	low = nanPadded(n)
	for i := period - 1; i < n; i++ {
		window := closes[i-period+1 : i+1]
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
		variance /= float64(period)
		sd := math.Sqrt(variance)
		mid[i] = m
		up[i] = m + mult*sd
		low[i] = m - mult*sd
	}
	return mid, up, low, true
}
