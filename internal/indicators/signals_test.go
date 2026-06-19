package indicators

import (
	"strings"
	"testing"
)

func f(v float64) *float64 { return &v }

func ind(id, status string, value *float64, extra map[string]float64) StockIndicator {
	return StockIndicator{
		Indicator: Indicator{ID: id},
		Status:    status,
		Value:     value,
		Extra:     extra,
	}
}

// findSignal returns the first signal with the given source indicator id (or false).
func findSignal(sigs []Signal, id string) (Signal, bool) {
	for _, s := range sigs {
		if s.ID == id {
			return s, true
		}
	}
	return Signal{}, false
}

func TestSignals(t *testing.T) {
	cases := []struct {
		name      string
		in        StockIndicator
		wantID    string // "" means expect NO signal from this indicator
		wantDir   string
		wantBasis string
	}{
		{"rsi oversold", ind("technical.rsi", StatusOK, f(27.4), nil), "technical.rsi", DirBullish, "RSI 27.4 < 30"},
		{"rsi overbought", ind("technical.rsi", StatusOK, f(72.1), nil), "technical.rsi", DirBearish, "RSI 72.1 > 70"},
		{"rsi neutral -> none", ind("technical.rsi", StatusOK, f(50), nil), "", "", ""},
		// KDJ reads %K from the validated headline Value (= v.K), not Extra — so Value carries %K here.
		{"kdj overbought", ind("technical.stochastic-kdj", StatusOK, f(85), map[string]float64{"k": 85}), "technical.stochastic-kdj", DirBearish, "KDJ %K 85.0 > 80"},
		{"kdj oversold", ind("technical.stochastic-kdj", StatusOK, f(12), map[string]float64{"k": 12}), "technical.stochastic-kdj", DirBullish, "KDJ %K 12.0 < 20"},
		{"kdj neutral -> none", ind("technical.stochastic-kdj", StatusOK, f(50), map[string]float64{"k": 50}), "", "", ""},
		// Regression (M1): %K comes from Value even when Extra has no "k" key — never fabricates a 0.
		{"kdj from value, no extra", ind("technical.stochastic-kdj", StatusOK, f(15), nil), "technical.stochastic-kdj", DirBullish, "KDJ %K 15.0 < 20"},
		{"macd above signal", ind("technical.macd", StatusOK, f(1.250), map[string]float64{"signal": 0.800, "hist": 0.450}), "technical.macd", DirBullish, "DIF 1.250 > DEA 0.800"},
		{"macd below signal", ind("technical.macd", StatusOK, f(-0.500), map[string]float64{"signal": -0.200, "hist": -0.300}), "technical.macd", DirBearish, "DIF -0.500 < DEA -0.200"},
		{"macd mixed -> none", ind("technical.macd", StatusOK, f(1.0), map[string]float64{"signal": 0.5, "hist": -0.1}), "", "", ""},
		// guards
		{"insufficient skipped", ind("technical.rsi", StatusInsufficient, nil, nil), "", "", ""},
		{"nil value skipped", ind("technical.rsi", StatusOK, nil, nil), "", "", ""},
		{"unknown id ignored", ind("technical.atr", StatusOK, f(3.2), nil), "", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Signals(StockIndicatorsResult{Indicators: []StockIndicator{tc.in}})
			if tc.wantID == "" {
				if len(got) != 0 {
					t.Fatalf("expected no signal, got %+v", got)
				}
				return
			}
			s, ok := findSignal(got, tc.wantID)
			if !ok {
				t.Fatalf("expected a %s signal, got %+v", tc.wantID, got)
			}
			if s.Direction != tc.wantDir {
				t.Errorf("direction = %q, want %q", s.Direction, tc.wantDir)
			}
			if s.Basis != tc.wantBasis {
				t.Errorf("basis = %q, want %q", s.Basis, tc.wantBasis)
			}
		})
	}
}

func TestSignalsPriceBased(t *testing.T) {
	boll := map[string]float64{"upper": 205, "mid": 190, "lower": 175}
	cases := []struct {
		name      string
		resPrice  *float64
		in        StockIndicator
		wantID    string // "" => expect NO signal
		wantDir   string
		wantBasis string
	}{
		{"price above SMA", f(190), ind("technical.sma-ma", StatusOK, f(182), nil), "technical.sma-ma", DirBullish, "Price 190.00 > SMA 182.00"},
		{"price below SMA", f(170), ind("technical.sma-ma", StatusOK, f(182), nil), "technical.sma-ma", DirBearish, "Price 170.00 < SMA 182.00"},
		{"price above EMA", f(190), ind("technical.ema", StatusOK, f(185), nil), "technical.ema", DirBullish, "Price 190.00 > EMA 185.00"},
		{"price below EMA", f(180), ind("technical.ema", StatusOK, f(185), nil), "technical.ema", DirBearish, "Price 180.00 < EMA 185.00"},
		{"above upper band", f(210), ind("technical.boll", StatusOK, f(190), boll), "technical.boll", DirNeutral, "Price 210.00 > upper band 205.00"},
		{"below lower band", f(170), ind("technical.boll", StatusOK, f(190), boll), "technical.boll", DirNeutral, "Price 170.00 < lower band 175.00"},
		{"within bands -> none", f(190), ind("technical.boll", StatusOK, f(190), boll), "", "", ""},
		{"no price -> SMA skipped", nil, ind("technical.sma-ma", StatusOK, f(182), nil), "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Signals(StockIndicatorsResult{Price: tc.resPrice, Indicators: []StockIndicator{tc.in}})
			if tc.wantID == "" {
				if len(got) != 0 {
					t.Fatalf("expected no signal, got %+v", got)
				}
				return
			}
			s, ok := findSignal(got, tc.wantID)
			if !ok {
				t.Fatalf("expected a %s signal, got %+v", tc.wantID, got)
			}
			if s.Direction != tc.wantDir {
				t.Errorf("direction = %q, want %q", s.Direction, tc.wantDir)
			}
			if s.Basis != tc.wantBasis {
				t.Errorf("basis = %q, want %q", s.Basis, tc.wantBasis)
			}
		})
	}
}

func TestSignalsMacdCross(t *testing.T) {
	macdInd := func(line, signal, hist float64, extra map[string]float64) StockIndicator {
		m := map[string]float64{"signal": signal, "hist": hist}
		for k, v := range extra {
			m[k] = v
		}
		return ind("technical.macd", StatusOK, f(line), m)
	}
	cases := []struct {
		name      string
		in        StockIndicator
		wantLabel string
		wantDir   string
		wantBasis string
	}{
		{
			// hist flips - to + → bullish cross, and it WINS over the posture even though
			// DIF > DEA would also fire "MACD above signal".
			"bullish cross beats posture",
			macdInd(1.0, 0.5, 0.030, map[string]float64{"prev_hist": -0.010}),
			"MACD bullish cross", DirBullish, "MACD histogram -0.010 → 0.030 (crossed up)",
		},
		{
			"bearish cross",
			macdInd(-0.4, -0.2, -0.020, map[string]float64{"prev_hist": 0.040}),
			"MACD bearish cross", DirBearish, "MACD histogram 0.040 → -0.020 (crossed down)",
		},
		{
			// prev_hist present but no sign flip → falls through to the posture signal.
			"no flip → posture",
			macdInd(1.25, 0.80, 0.450, map[string]float64{"prev_hist": 0.200}),
			"MACD above signal", DirBullish, "DIF 1.250 > DEA 0.800",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Signals(StockIndicatorsResult{Indicators: []StockIndicator{tc.in}})
			s, ok := findSignal(got, "technical.macd")
			if !ok {
				t.Fatalf("expected a macd signal, got %+v", got)
			}
			if s.Label != tc.wantLabel {
				t.Errorf("label = %q, want %q", s.Label, tc.wantLabel)
			}
			if s.Direction != tc.wantDir {
				t.Errorf("direction = %q, want %q", s.Direction, tc.wantDir)
			}
			if s.Basis != tc.wantBasis {
				t.Errorf("basis = %q, want %q", s.Basis, tc.wantBasis)
			}
		})
	}
}

func TestSignalSalienceOrder(t *testing.T) {
	// A result that yields all three tiers: a trend posture (price > SMA), an extreme
	// (RSI oversold) and an event (golden cross from the close series). They are emitted
	// posture→extreme→event, so a correct salience sort must REVERSE them.
	golden := func() []float64 {
		s := make([]float64, 200)
		for i := range s {
			s[i] = 100
		}
		return append(s, 200) // 200×100 then a jump → golden cross
	}()
	res := StockIndicatorsResult{
		Price:  f(110),
		Closes: golden,
		Indicators: []StockIndicator{
			ind("technical.sma-ma", StatusOK, f(100), nil), // posture: Price above SMA (tier 2)
			ind("technical.rsi", StatusOK, f(25), nil),     // extreme: RSI oversold (tier 1)
		},
	}
	got := Signals(res)
	if len(got) != 3 {
		t.Fatalf("want 3 signals, got %d: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Label, "cross") {
		t.Errorf("lead signal should be the event/cross, got %q", got[0].Label)
	}
	if got[1].ID != "technical.rsi" {
		t.Errorf("second should be the RSI extreme, got %q (%s)", got[1].Label, got[1].ID)
	}
	if got[2].ID != "technical.sma-ma" {
		t.Errorf("last should be the trend posture, got %q (%s)", got[2].Label, got[2].ID)
	}
	// The teaser (first 2) must therefore surface the event + the extreme, not posture.
	for _, s := range got[:2] {
		if s.ID == "technical.sma-ma" {
			t.Error("teaser surfaced the always-on trend posture over a more salient signal")
		}
	}
}

func TestCrossSignals(t *testing.T) {
	flat := func(n int, v float64) []float64 {
		s := make([]float64, n)
		for i := range s {
			s[i] = v
		}
		return s
	}
	// 200 flat closes (so prev SMA50 == prev SMA200 == 100), then a final bar that
	// pushes the recent (SMA50) mean above/below the long (SMA200) mean → a clean cross.
	golden := append(flat(200, 100), 200) // jump up: cur SMA50 102.00 > SMA200 100.50
	death := append(flat(200, 100), 0)    // drop:    cur SMA50  98.00 < SMA200  99.50

	t.Run("golden cross", func(t *testing.T) {
		got := crossSignals(StockIndicatorsResult{Closes: golden})
		if len(got) != 1 {
			t.Fatalf("want 1 signal, got %+v", got)
		}
		if got[0].ID != maCrossID || got[0].Direction != DirBullish {
			t.Errorf("got %+v, want bullish %s", got[0], maCrossID)
		}
		if got[0].Basis != "SMA50 102.00 crossed above SMA200 100.50" {
			t.Errorf("basis = %q", got[0].Basis)
		}
	})
	t.Run("death cross", func(t *testing.T) {
		got := crossSignals(StockIndicatorsResult{Closes: death})
		if len(got) != 1 || got[0].Direction != DirBearish {
			t.Fatalf("want 1 bearish, got %+v", got)
		}
		if got[0].Basis != "SMA50 98.00 crossed below SMA200 99.50" {
			t.Errorf("basis = %q", got[0].Basis)
		}
	})
	t.Run("no flip → none", func(t *testing.T) {
		if got := crossSignals(StockIndicatorsResult{Closes: flat(201, 100)}); len(got) != 0 {
			t.Errorf("flat series should not cross, got %+v", got)
		}
	})
	t.Run("too short → none", func(t *testing.T) {
		if got := crossSignals(StockIndicatorsResult{Closes: flat(200, 100)}); len(got) != 0 {
			t.Errorf("<201 closes should yield no cross, got %+v", got)
		}
	})
	// End-to-end: Signals() appends the cross to the per-indicator signals.
	t.Run("Signals appends the cross", func(t *testing.T) {
		got := Signals(StockIndicatorsResult{Closes: golden})
		if _, ok := findSignal(got, maCrossID); !ok {
			t.Fatalf("Signals did not surface the golden cross: %+v", got)
		}
	})
}

// TestSignalsMultiple verifies a full result emits one signal per triggering indicator.
func TestSignalsMultiple(t *testing.T) {
	res := StockIndicatorsResult{Indicators: []StockIndicator{
		ind("technical.rsi", StatusOK, f(25), nil),
		ind("technical.stochastic-kdj", StatusOK, f(90), map[string]float64{"k": 90}),
		ind("technical.macd", StatusOK, f(2.0), map[string]float64{"signal": 1.0, "hist": 1.0}),
		ind("technical.atr", StatusOK, f(3.2), nil), // no rule -> ignored
	}}
	got := Signals(res)
	if len(got) != 3 {
		t.Fatalf("expected 3 signals, got %d: %+v", len(got), got)
	}
}
