package indicators

import (
	"math"

	"github.com/wombow-ai/tickwind/internal/store"
)

// BacktestResult summarizes how a signal RULE performed historically on one ticker:
// every time the signal fired, what was the forward return over `Horizon` trading
// days? It is a deterministic, disclosed statistic over Go-computed signals — NOT a
// prediction, advice, or a guarantee. Past behavior of a rule, nothing more.
type BacktestResult struct {
	Rule      string  `json:"rule"`
	Horizon   int     `json:"horizon"`    // forward window, trading days
	Trades    int     `json:"trades"`     // times the signal fired with a full forward window
	Wins      int     `json:"wins"`       // trades whose forward return was positive
	WinRate   float64 `json:"win_rate"`   // wins / trades (0 when no trades), 0..1
	AvgReturn float64 `json:"avg_return"` // mean forward return, %
	Baseline  float64 `json:"baseline"`   // buy-and-hold over the tested span, %
}

// backtestMinBars is the shortest history the longest rule (the 200-period golden
// cross) needs before any signal can be detected and still leave a forward window.
const backtestMinBars = 220

// barSignalDetector reports whether `rule` fired AT bar i — a transition detected by
// comparing the indicator state at bar i (closes[:i+1]) with bar i-1 (closes[:i]).
// Returns false when there isn't enough history to compute either side (never guesses).
type barSignalDetector func(closes []float64, i int) bool

// crossedUp/crossedDown report a strict cross of a over b between the prior and current
// bar, given the (value, ok) pairs. A side that couldn't compute suppresses the signal.
func crossedUp(prevA, prevB float64, okPrev bool, curA, curB float64, okCur bool) bool {
	return okPrev && okCur && prevA <= prevB && curA > curB
}
func crossedDown(prevA, prevB float64, okPrev bool, curA, curB float64, okCur bool) bool {
	return okPrev && okCur && prevA >= prevB && curA < curB
}

// backtestDetectors maps each backtestable rule to its per-bar transition detector.
// All reuse the same Go indicator math the live signals use, so a backtest can never
// disagree with what the signals layer would have shown on that bar.
var backtestDetectors = map[string]barSignalDetector{
	"golden_cross": func(c []float64, i int) bool {
		p50, okp50 := sma(c[:i], 50)
		p200, okp200 := sma(c[:i], 200)
		n50, okn50 := sma(c[:i+1], 50)
		n200, okn200 := sma(c[:i+1], 200)
		return crossedUp(p50, p200, okp50 && okp200, n50, n200, okn50 && okn200)
	},
	"death_cross": func(c []float64, i int) bool {
		p50, okp50 := sma(c[:i], 50)
		p200, okp200 := sma(c[:i], 200)
		n50, okn50 := sma(c[:i+1], 50)
		n200, okn200 := sma(c[:i+1], 200)
		return crossedDown(p50, p200, okp50 && okp200, n50, n200, okn50 && okn200)
	},
	"macd_bullish_cross": func(c []float64, i int) bool {
		prev, okp := macd(c[:i], defaultMACDFast, defaultMACDSlow, defaultMACDSignal)
		cur, okc := macd(c[:i+1], defaultMACDFast, defaultMACDSlow, defaultMACDSignal)
		return okp && okc && prev.Histogram <= 0 && cur.Histogram > 0
	},
	"macd_bearish_cross": func(c []float64, i int) bool {
		prev, okp := macd(c[:i], defaultMACDFast, defaultMACDSlow, defaultMACDSignal)
		cur, okc := macd(c[:i+1], defaultMACDFast, defaultMACDSlow, defaultMACDSignal)
		return okp && okc && prev.Histogram >= 0 && cur.Histogram < 0
	},
	"rsi_oversold": func(c []float64, i int) bool {
		prev, okp := rsiWilder(c[:i], defaultRSIPeriod)
		cur, okc := rsiWilder(c[:i+1], defaultRSIPeriod)
		return okp && okc && prev >= 30 && cur < 30 // crossed DOWN into oversold (entry)
	},
	"rsi_overbought": func(c []float64, i int) bool {
		prev, okp := rsiWilder(c[:i], defaultRSIPeriod)
		cur, okc := rsiWilder(c[:i+1], defaultRSIPeriod)
		return okp && okc && prev <= 70 && cur > 70 // crossed UP into overbought (entry)
	},
}

// BacktestableRule reports whether a rule can be backtested by BacktestSignal.
func BacktestableRule(rule string) bool {
	_, ok := backtestDetectors[rule]
	return ok
}

// BacktestSignal replays a signal rule over a ticker's daily candles (oldest→newest):
// at each bar where the rule fires, it records the forward return over `horizon`
// trading days, then reports the win rate, average forward return, trade count, and a
// buy-and-hold baseline over the same span. Pure — no network, no clock — so it is
// fully deterministic and unit-testable. ok=false for an unknown rule, a non-positive
// horizon, or too little history.
func BacktestSignal(candles []store.Candle, rule string, horizon int) (BacktestResult, bool) {
	detect, known := backtestDetectors[rule]
	if !known || horizon <= 0 {
		return BacktestResult{}, false
	}
	closes := make([]float64, len(candles))
	for i, c := range candles {
		closes[i] = c.Close
	}
	n := len(closes)
	if n < backtestMinBars+horizon {
		return BacktestResult{}, false
	}

	res := BacktestResult{Rule: rule, Horizon: horizon}
	var sumRet float64
	// Stop at n-horizon so every counted trade has a full forward window.
	for i := 1; i < n-horizon; i++ {
		if !detect(closes, i) {
			continue
		}
		entry := closes[i]
		if entry <= 0 {
			continue
		}
		ret := (closes[i+horizon] - entry) / entry * 100
		res.Trades++
		if ret > 0 {
			res.Wins++
		}
		sumRet += ret
	}
	if res.Trades > 0 {
		res.AvgReturn = round2(sumRet / float64(res.Trades))
		res.WinRate = round2(float64(res.Wins) / float64(res.Trades))
	}
	if closes[0] > 0 {
		res.Baseline = round2((closes[n-1] - closes[0]) / closes[0] * 100)
	}
	return res, true
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
