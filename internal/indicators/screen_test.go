package indicators

import "testing"

func TestScreenSignals(t *testing.T) {
	// A small precomputed universe: each ticker → its signals.
	bySignal := map[string][]Signal{
		"AAPL": {
			{ID: "technical.rsi", Label: "RSI oversold", Direction: DirBullish, Basis: "RSI 27.4 < 30"},
			{ID: "technical.ma-cross", Label: "Golden cross (SMA50 × SMA200)", Direction: DirBullish, Basis: "SMA50 > SMA200"},
		},
		"MSFT": {
			{ID: "technical.rsi", Label: "RSI overbought", Direction: DirBearish, Basis: "RSI 72.1 > 70"},
		},
		"NVDA": {
			{ID: "technical.ma-cross", Label: "Death cross (SMA50 × SMA200)", Direction: DirBearish, Basis: "SMA50 < SMA200"},
		},
		"TSLA": {}, // no signals → never matches
	}

	tickers := func(ms []SignalMatch) []string {
		out := make([]string, len(ms))
		for i, m := range ms {
			out[i] = m.Ticker
		}
		return out
	}
	eq := func(got, want []string) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}

	t.Run("empty query matches any stock with a signal, sorted", func(t *testing.T) {
		got := tickers(ScreenSignals(bySignal, SignalScreen{}))
		if want := []string{"AAPL", "MSFT", "NVDA"}; !eq(got, want) {
			t.Fatalf("got %v, want %v (TSLA has no signals)", got, want)
		}
	})

	t.Run("direction filter", func(t *testing.T) {
		got := tickers(ScreenSignals(bySignal, SignalScreen{Direction: DirBullish}))
		if want := []string{"AAPL"}; !eq(got, want) {
			t.Fatalf("bullish: got %v, want %v", got, want)
		}
		got = tickers(ScreenSignals(bySignal, SignalScreen{Direction: DirBearish}))
		if want := []string{"MSFT", "NVDA"}; !eq(got, want) {
			t.Fatalf("bearish: got %v, want %v", got, want)
		}
	})

	t.Run("golden crosses = ma-cross + bullish", func(t *testing.T) {
		ms := ScreenSignals(bySignal, SignalScreen{SignalID: "technical.ma-cross", Direction: DirBullish})
		if want := []string{"AAPL"}; !eq(tickers(ms), want) {
			t.Fatalf("got %v, want %v", tickers(ms), want)
		}
		// Only the matching signal is carried (AAPL also has an RSI signal that must NOT leak).
		if len(ms) != 1 || len(ms[0].Signals) != 1 || ms[0].Signals[0].ID != "technical.ma-cross" {
			t.Fatalf("match should carry only the matching signal, got %+v", ms)
		}
	})

	t.Run("signal id filter alone", func(t *testing.T) {
		got := tickers(ScreenSignals(bySignal, SignalScreen{SignalID: "technical.rsi"}))
		if want := []string{"AAPL", "MSFT"}; !eq(got, want) {
			t.Fatalf("rsi: got %v, want %v", got, want)
		}
	})

	t.Run("no match → empty (non-nil) slice", func(t *testing.T) {
		ms := ScreenSignals(bySignal, SignalScreen{SignalID: "technical.does-not-exist"})
		if ms == nil || len(ms) != 0 {
			t.Fatalf("want empty non-nil, got %+v", ms)
		}
	})
}
