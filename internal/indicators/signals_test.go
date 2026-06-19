package indicators

import "testing"

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
		{"kdj overbought", ind("technical.stochastic-kdj", StatusOK, f(60), map[string]float64{"k": 85}), "technical.stochastic-kdj", DirBearish, "KDJ %K 85.0 > 80"},
		{"kdj oversold", ind("technical.stochastic-kdj", StatusOK, f(60), map[string]float64{"k": 12}), "technical.stochastic-kdj", DirBullish, "KDJ %K 12.0 < 20"},
		{"kdj neutral -> none", ind("technical.stochastic-kdj", StatusOK, f(60), map[string]float64{"k": 50}), "", "", ""},
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

// TestSignalsMultiple verifies a full result emits one signal per triggering indicator.
func TestSignalsMultiple(t *testing.T) {
	res := StockIndicatorsResult{Indicators: []StockIndicator{
		ind("technical.rsi", StatusOK, f(25), nil),
		ind("technical.stochastic-kdj", StatusOK, f(60), map[string]float64{"k": 90}),
		ind("technical.macd", StatusOK, f(2.0), map[string]float64{"signal": 1.0, "hist": 1.0}),
		ind("technical.atr", StatusOK, f(3.2), nil), // no rule -> ignored
	}}
	got := Signals(res)
	if len(got) != 3 {
		t.Fatalf("expected 3 signals, got %d: %+v", len(got), got)
	}
}
