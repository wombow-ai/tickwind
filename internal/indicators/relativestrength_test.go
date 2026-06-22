package indicators

import (
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

// rsDaily builds `n` consecutive-CALENDAR-day candles from startDate with every close = def.
func rsDaily(startDate string, n int, def float64) []store.Candle {
	start, _ := time.Parse(dateOnly, startDate)
	cs := make([]store.Candle, n)
	for i := range cs {
		d := start.AddDate(0, 0, i)
		cs[i] = store.Candle{Time: d, Open: def, High: def, Low: def, Close: def, Volume: 1}
	}
	return cs
}

// rsSet overrides the close on a specific date (no-op if absent).
func rsSet(cs []store.Candle, date string, c float64) {
	for i := range cs {
		if cs[i].Time.Format(dateOnly) == date {
			cs[i].Open, cs[i].High, cs[i].Low, cs[i].Close = c, c, c, c
			return
		}
	}
}

// rsDrop removes the candle on a specific date (to simulate a calendar gap).
func rsDrop(cs []store.Candle, date string) []store.Candle {
	out := cs[:0:0]
	for _, c := range cs {
		if c.Time.Format(dateOnly) != date {
			out = append(out, c)
		}
	}
	return out
}

func TestComputeRelativeStrength_KnownExcess(t *testing.T) {
	// 60 daily bars: 2024-01-01 .. 2024-02-29. End = 2024-02-29; 1M anchor = 2024-01-29.
	// 3M/6M/1Y anchors fall before the series start → only the 1M window is honest.
	stock := rsDaily("2024-01-01", 60, 110)
	rsSet(stock, "2024-01-29", 100) // 1M start
	rsSet(stock, "2024-02-29", 120) // end → +20%
	bench := rsDaily("2024-01-01", 60, 105)
	rsSet(bench, "2024-01-29", 100)
	rsSet(bench, "2024-02-29", 110) // +10%
	rs, ok := ComputeRelativeStrength(stock, bench)
	if !ok {
		t.Fatal("ok=false, want true")
	}
	if len(rs.Windows) != 1 {
		t.Fatalf("got %d windows, want 1 (only 1M has history)", len(rs.Windows))
	}
	w := rs.Windows[0]
	if w.Label != "1M" || w.StockReturn != 20 || w.BenchmarkReturn != 10 || w.Relative != 10 {
		t.Fatalf("window = %+v, want 1M stock 20 / bench 10 / rel 10", w)
	}
	if rs.AsOf != "2024-02-29" {
		t.Fatalf("as_of = %q, want 2024-02-29", rs.AsOf)
	}
}

func TestComputeRelativeStrength_BenchmarkGapUsesPriorSession(t *testing.T) {
	// Benchmark is MISSING the exact end anchor (2024-02-29); closeAtOrBefore must fall back to
	// the prior session (2024-02-28), not skip the window.
	stock := rsDaily("2024-01-01", 60, 110)
	rsSet(stock, "2024-01-29", 100)
	rsSet(stock, "2024-02-29", 120) // +20%
	bench := rsDaily("2024-01-01", 60, 105)
	rsSet(bench, "2024-01-29", 100)
	rsSet(bench, "2024-02-28", 110) // the prior session the end anchor will fall back to
	bench = rsDrop(bench, "2024-02-29")
	rs, ok := ComputeRelativeStrength(stock, bench)
	if !ok {
		t.Fatal("ok=false, want true (should fall back to the prior benchmark session)")
	}
	w := rs.Windows[0]
	if w.BenchmarkReturn != 10 { // 100 → 110 via the prior-session fallback
		t.Fatalf("benchmark return = %v, want 10 (prior-session fallback)", w.BenchmarkReturn)
	}
	if w.Relative != 10 {
		t.Fatalf("relative = %v, want 10", w.Relative)
	}
}

func TestComputeRelativeStrength_AllWindows(t *testing.T) {
	// ~20 months of daily bars → 1M/3M/6M/1Y all have history.
	stock := rsDaily("2023-01-01", 600, 130)
	bench := rsDaily("2023-01-01", 600, 110)
	rs, ok := ComputeRelativeStrength(stock, bench)
	if !ok {
		t.Fatal("ok=false, want true")
	}
	if len(rs.Windows) != 4 {
		t.Fatalf("got %d windows, want 4", len(rs.Windows))
	}
	for i, label := range []string{"1M", "3M", "6M", "1Y"} {
		if rs.Windows[i].Label != label {
			t.Fatalf("window %d = %q, want %q (shortest→longest)", i, rs.Windows[i].Label, label)
		}
	}
}

func TestComputeRelativeStrength_Insufficient(t *testing.T) {
	tests := []struct {
		name         string
		stock, bench []store.Candle
	}{
		{"history shorter than 1 month", rsDaily("2024-01-01", 15, 110), rsDaily("2024-01-01", 15, 105)},
		{"benchmark too short", rsDaily("2023-01-01", 600, 110), rsDaily("2024-01-01", 1, 105)},
		{"empty stock", nil, rsDaily("2023-01-01", 600, 105)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := ComputeRelativeStrength(tc.stock, tc.bench); ok {
				t.Fatal("ok=true, want false")
			}
		})
	}
}

func TestComputeRelativeStrength_StaleGapSkipped(t *testing.T) {
	// A gappy ticker: one old bar (2024-01-01) then a recent 21-bar cluster (2024-06-01..06-21).
	// Calendar anchoring alone would let the 1M anchor (2024-05-21) fall back to the Jan bar — a
	// months-stale anchor that would mislabel a ~5-month span as "1M". The tolerance must skip it.
	stock := []store.Candle{}
	{
		old := rsDaily("2024-01-01", 1, 100)
		recent := rsDaily("2024-06-01", 21, 130)
		stock = append(append(stock, old...), recent...)
	}
	bench := rsDaily("2024-01-01", 200, 110) // benchmark has continuous coverage
	if _, ok := ComputeRelativeStrength(stock, bench); ok {
		t.Fatal("ok=true, want false — every window's stock anchor is stale-gapped, none honest")
	}
}

func TestCandleAtOrBefore(t *testing.T) {
	series := rsDaily("2024-01-01", 3, 0) // override closes below
	rsSet(series, "2024-01-01", 10)
	rsSet(series, "2024-01-02", 11)
	rsSet(series, "2024-01-03", 12)
	d := func(s string) time.Time { tt, _ := time.Parse(dateOnly, s); return tt }

	if c, ok := candleAtOrBefore(series, d("2024-01-03")); !ok || c.Close != 12 {
		t.Fatalf("exact match: got %v/%v, want 12/true", c.Close, ok)
	}
	if c, ok := candleAtOrBefore(series, d("2024-01-05")); !ok || c.Close != 12 {
		t.Fatalf("after-all: got %v/%v, want 12/true (latest ≤ target)", c.Close, ok)
	}
	if c, ok := candleAtOrBefore(series, d("2024-01-02")); !ok || c.Close != 11 {
		t.Fatalf("mid: got %v/%v, want 11/true", c.Close, ok)
	}
	if _, ok := candleAtOrBefore(series, d("2023-12-31")); ok {
		t.Fatal("before-all: ok=true, want false (no candle ≤ target)")
	}
}

func TestRankRelativeStrength(t *testing.T) {
	pop := []TickerRelStrength{
		{Ticker: "A", RS: RelativeStrength{Windows: []RelStrengthWindow{{Label: "3M", Relative: 5, StockReturn: 10, BenchmarkReturn: 5}}}},
		{Ticker: "B", RS: RelativeStrength{Windows: []RelStrengthWindow{{Label: "3M", Relative: 15}, {Label: "1Y", Relative: 2}}}},
		{Ticker: "C", RS: RelativeStrength{Windows: []RelStrengthWindow{{Label: "1Y", Relative: 20}}}}, // no 3M window
	}

	if !ValidRSWindow("3M") || !ValidRSWindow("1Y") || ValidRSWindow("2W") {
		t.Fatal("ValidRSWindow misclassified a window")
	}

	t.Run("unknown window → empty", func(t *testing.T) {
		if got := RankRelativeStrength(pop, "2W"); len(got) != 0 {
			t.Fatalf("unknown window → %d rows, want 0", len(got))
		}
	})

	t.Run("3M: highest excess first, missing-window omitted", func(t *testing.T) {
		got := RankRelativeStrength(pop, "3M")
		if len(got) != 2 { // C lacks 3M
			t.Fatalf("got %d, want 2 (C omitted)", len(got))
		}
		if got[0].Ticker != "B" || got[0].Relative != 15 || got[1].Ticker != "A" {
			t.Fatalf("3M order = %v, want B(15),A(5)", got)
		}
		if got[1].StockReturn != 10 || got[1].BenchmarkReturn != 5 {
			t.Fatalf("A legs not carried: %+v", got[1])
		}
	})

	t.Run("1Y: C leads, A omitted", func(t *testing.T) {
		got := RankRelativeStrength(pop, "1Y")
		if len(got) != 2 || got[0].Ticker != "C" || got[1].Ticker != "B" {
			t.Fatalf("1Y order = %v, want C(20),B(2)", got)
		}
	})

	t.Run("ties break by ticker asc", func(t *testing.T) {
		tied := []TickerRelStrength{
			{Ticker: "Z", RS: RelativeStrength{Windows: []RelStrengthWindow{{Label: "3M", Relative: 3}}}},
			{Ticker: "A", RS: RelativeStrength{Windows: []RelStrengthWindow{{Label: "3M", Relative: 3}}}},
		}
		got := RankRelativeStrength(tied, "3M")
		if got[0].Ticker != "A" || got[1].Ticker != "Z" {
			t.Fatalf("tie-break = %v, want A,Z", got)
		}
	})
}
