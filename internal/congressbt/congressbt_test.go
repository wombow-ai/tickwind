package congressbt

import (
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/congress/ptr"
	"github.com/wombow-ai/tickwind/internal/store"
)

// day builds a UTC date.
func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// candles builds an oldest-first daily series: one candle per element, starting
// at `start` and stepping one day each (close = the given value).
func candles(start time.Time, closes ...float64) []store.Candle {
	cs := make([]store.Candle, len(closes))
	for i, c := range closes {
		cs[i] = store.Candle{Time: start.AddDate(0, 0, i), Close: c}
	}
	return cs
}

// fakeCloses returns a CloseFn backed by a per-ticker map (nil for unknown).
func fakeCloses(m map[string][]store.Candle) CloseFn {
	return func(ticker string) []store.Candle { return m[ticker] }
}

func buy(ticker string, d time.Time) ptr.Transaction {
	return ptr.Transaction{Ticker: ticker, Type: ptr.TxPurchase, TxDate: d}
}

func sell(ticker string, d time.Time) ptr.Transaction {
	return ptr.Transaction{Ticker: ticker, Type: ptr.TxSale, TxDate: d}
}

func TestRun(t *testing.T) {
	now := day(2024, time.January, 10)

	tests := []struct {
		name        string
		txs         []ptr.Transaction
		prices      map[string][]store.Candle
		wantInsuf   bool
		wantMember  float64 // ignored when wantInsuf
		wantSpy     float64
		wantUsed    int
		wantSkipped int
		wantTickers []string
	}{
		{
			name: "outperform: AAPL doubles, SPY flat",
			txs:  []ptr.Transaction{buy("AAPL", day(2024, time.January, 1))},
			prices: map[string][]store.Candle{
				// entry 100 on Jan 1, mark 200 by Jan 10 → +100%
				"AAPL": candles(day(2024, time.January, 1), 100, 110, 120, 130, 140, 150, 160, 170, 180, 200),
				"SPY":  candles(day(2024, time.January, 1), 400, 400, 400, 400, 400, 400, 400, 400, 400, 400),
			},
			wantInsuf:   false,
			wantMember:  100,
			wantSpy:     0,
			wantUsed:    1,
			wantSkipped: 0,
			wantTickers: []string{"AAPL"},
		},
		{
			name: "underperform: pick falls, SPY rises",
			txs:  []ptr.Transaction{buy("XYZ", day(2024, time.January, 1))},
			prices: map[string][]store.Candle{
				// entry 100 → mark 50 → -50%
				"XYZ": candles(day(2024, time.January, 1), 100, 95, 90, 85, 80, 75, 70, 65, 60, 50),
				// SPY entry 100 → mark 120 → +20%
				"SPY": candles(day(2024, time.January, 1), 100, 102, 104, 106, 108, 110, 112, 114, 116, 120),
			},
			wantInsuf:   false,
			wantMember:  -50,
			wantSpy:     20,
			wantUsed:    1,
			wantSkipped: 0,
			wantTickers: []string{"XYZ"},
		},
		{
			name:      "no buys: only sales/exchanges → insufficient",
			txs:       []ptr.Transaction{sell("AAPL", day(2024, time.January, 1)), {Ticker: "MSFT", Type: ptr.TxExchange, TxDate: day(2024, time.January, 2)}},
			prices:    map[string][]store.Candle{"AAPL": candles(day(2024, time.January, 1), 100, 110)},
			wantInsuf: true,
		},
		{
			name: "no prices: buy a ticker with no history → skipped → insufficient",
			txs:  []ptr.Transaction{buy("NOPE", day(2024, time.January, 1))},
			prices: map[string][]store.Candle{
				"SPY": candles(day(2024, time.January, 1), 100, 120),
			},
			wantInsuf:   true,
			wantSkipped: 1,
		},
		{
			name: "sell closes the position: locks the gain at the sell close, not today",
			txs: []ptr.Transaction{
				buy("NVDA", day(2024, time.January, 1)),
				sell("NVDA", day(2024, time.January, 5)),
			},
			prices: map[string][]store.Candle{
				// entry 100 (Jan 1), sell close 150 (Jan 5) → +50% locked.
				// Price later crashes to 10, which must NOT affect the result.
				"NVDA": candles(day(2024, time.January, 1), 100, 110, 120, 130, 150, 40, 30, 20, 15, 10),
				"SPY":  candles(day(2024, time.January, 1), 100, 100, 100, 100, 100, 100, 100, 100, 100, 100),
			},
			wantInsuf:   false,
			wantMember:  50, // realized at the sell close, immune to the later crash
			wantSpy:     0,
			wantUsed:    1,
			wantSkipped: 0,
			wantTickers: []string{"NVDA"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Run(tc.txs, fakeCloses(tc.prices), now)
			if got.Insufficient != tc.wantInsuf {
				t.Fatalf("Insufficient = %v, want %v", got.Insufficient, tc.wantInsuf)
			}
			if tc.wantInsuf {
				if tc.wantSkipped != 0 && got.TradesSkipped != tc.wantSkipped {
					t.Errorf("TradesSkipped = %d, want %d", got.TradesSkipped, tc.wantSkipped)
				}
				return
			}
			if got.MemberReturnPct != tc.wantMember {
				t.Errorf("MemberReturnPct = %v, want %v", got.MemberReturnPct, tc.wantMember)
			}
			if got.SpyReturnPct != tc.wantSpy {
				t.Errorf("SpyReturnPct = %v, want %v", got.SpyReturnPct, tc.wantSpy)
			}
			if got.TradesUsed != tc.wantUsed {
				t.Errorf("TradesUsed = %d, want %d", got.TradesUsed, tc.wantUsed)
			}
			if got.TradesSkipped != tc.wantSkipped {
				t.Errorf("TradesSkipped = %d, want %d", got.TradesSkipped, tc.wantSkipped)
			}
			if len(got.Tickers) != len(tc.wantTickers) {
				t.Errorf("Tickers = %v, want %v", got.Tickers, tc.wantTickers)
			}
			if len(got.Curve) == 0 {
				t.Errorf("Curve is empty; want at least one point")
			}
			// Curve must be monotonic in date and end at WindowEnd.
			if last := got.Curve[len(got.Curve)-1]; last.Date != got.WindowEnd {
				t.Errorf("curve last date = %s, WindowEnd = %s", last.Date, got.WindowEnd)
			}
		})
	}
}

// TestRunEqualWeightAcrossTickers verifies two buys in different tickers are
// equal-weighted (averaged), not dollar-weighted.
func TestRunEqualWeightAcrossTickers(t *testing.T) {
	now := day(2024, time.January, 10)
	txs := []ptr.Transaction{
		buy("AAA", day(2024, time.January, 1)), // +100%
		buy("BBB", day(2024, time.January, 1)), // 0%
	}
	prices := map[string][]store.Candle{
		"AAA": candles(day(2024, time.January, 1), 10, 11, 12, 13, 14, 15, 16, 17, 18, 20),
		"BBB": candles(day(2024, time.January, 1), 50, 50, 50, 50, 50, 50, 50, 50, 50, 50),
		"SPY": candles(day(2024, time.January, 1), 100, 100, 100, 100, 100, 100, 100, 100, 100, 100),
	}
	got := Run(txs, fakeCloses(prices), now)
	if got.MemberReturnPct != 50 { // (100 + 0) / 2
		t.Errorf("equal-weight member return = %v, want 50", got.MemberReturnPct)
	}
	if got.TradesUsed != 2 {
		t.Errorf("TradesUsed = %d, want 2", got.TradesUsed)
	}
}

// TestRunSkipsUntickeredAndSpy verifies bond/fund rows (no ticker) and SPY rows
// themselves are excluded from the member legs.
func TestRunSkipsUntickeredAndSpy(t *testing.T) {
	now := day(2024, time.January, 10)
	txs := []ptr.Transaction{
		buy("", day(2024, time.January, 1)),    // a bond/fund: no ticker
		buy("SPY", day(2024, time.January, 1)), // the benchmark itself: excluded from member legs
		buy("AAA", day(2024, time.January, 1)),
	}
	prices := map[string][]store.Candle{
		"AAA": candles(day(2024, time.January, 1), 10, 11, 12, 13, 14, 15, 16, 17, 18, 20),
		"SPY": candles(day(2024, time.January, 1), 100, 100, 100, 100, 100, 100, 100, 100, 100, 110),
	}
	got := Run(txs, fakeCloses(prices), now)
	if got.TradesUsed != 1 || len(got.Tickers) != 1 || got.Tickers[0] != "AAA" {
		t.Errorf("expected only AAA used; got used=%d tickers=%v", got.TradesUsed, got.Tickers)
	}
}
