package indicators

import (
	"testing"

	"github.com/wombow-ai/tickwind/internal/edgar"
)

func TestComputeDividend(t *testing.T) {
	t.Run("non-payer → not ok (omitted)", func(t *testing.T) {
		_, ok := ComputeDividend(10, edgar.Fundamentals{Shares: 1000, DividendsPaid: 0})
		if ok {
			t.Fatal("a non-payer (DividendsPaid 0) must return ok=false")
		}
	})

	t.Run("full payer → all five metrics", func(t *testing.T) {
		f := edgar.Fundamentals{
			Shares: 1000, DividendsPaid: 100, NetIncome: 500, DividendsPaidPrior: 80,
			OperatingCashFlow: 300, CapEx: 50, Period: "FY2024",
		}
		dv, ok := ComputeDividend(10, f) // market cap = 10 × 1000 = 10,000
		if !ok || !dv.HasAny() {
			t.Fatalf("expected a computable profile, got ok=%v dv=%+v", ok, dv)
		}
		if dv.Period != "FY2024" {
			t.Fatalf("period = %q, want FY2024", dv.Period)
		}
		check := func(name string, got *float64, want float64) {
			if got == nil {
				t.Fatalf("%s is nil, want %.4f", name, want)
			}
			if *got < want-1e-6 || *got > want+1e-6 {
				t.Fatalf("%s = %.4f, want %.4f", name, *got, want)
			}
		}
		check("yield", dv.Yield, 1.0)              // 100 / 10000 = 1%
		check("payout", dv.PayoutRatio, 20.0)      // 100 / 500 = 20%
		check("dps", dv.DPS, 0.1)                  // 100 / 1000
		check("fcf coverage", dv.FCFCoverage, 2.5) // (300-50) / 100 = 2.5x
		check("yoy growth", dv.YoYGrowth, 25.0)    // (100-80)/80 = 25%
	})

	t.Run("a dividend cut shows as negative growth", func(t *testing.T) {
		dv, ok := ComputeDividend(10, edgar.Fundamentals{Shares: 1000, DividendsPaid: 60, DividendsPaidPrior: 100})
		if !ok || dv.YoYGrowth == nil || *dv.YoYGrowth != -40 {
			t.Fatalf("cut should be −40%%, got ok=%v growth=%v", ok, dv.YoYGrowth)
		}
	})

	t.Run("payer with shares only → DPS only, still ok", func(t *testing.T) {
		dv, ok := ComputeDividend(0, edgar.Fundamentals{Shares: 1000, DividendsPaid: 100}) // no price/NI/FCF/prior
		if !ok || !dv.HasAny() {
			t.Fatalf("a payer with shares should still be ok with DPS, got ok=%v dv=%+v", ok, dv)
		}
		if dv.DPS == nil || dv.Yield != nil || dv.PayoutRatio != nil || dv.YoYGrowth != nil {
			t.Fatalf("expected DPS only, got %+v", dv)
		}
	})
}

func divF64(v float64) *float64 { return &v }

func TestRankDividend(t *testing.T) {
	pop := []TickerDividend{
		{Ticker: "T", Dividend: DividendView{Yield: divF64(6.5), PayoutRatio: divF64(60), FCFCoverage: divF64(1.2), YoYGrowth: divF64(2)}},
		{Ticker: "KO", Dividend: DividendView{Yield: divF64(3.0), PayoutRatio: divF64(70), FCFCoverage: divF64(1.5), YoYGrowth: divF64(5)}},
		{Ticker: "AAPL", Dividend: DividendView{Yield: divF64(0.5), PayoutRatio: divF64(15), FCFCoverage: divF64(4.0), YoYGrowth: divF64(4)}},
		{Ticker: "NVDA", Dividend: DividendView{Yield: divF64(0.03), PayoutRatio: divF64(2), FCFCoverage: divF64(50.0), YoYGrowth: divF64(50)}},
		{Ticker: "MO", Dividend: DividendView{Yield: divF64(6.5)}},                              // ties T on yield → ticker tie-break (MO<T)
		{Ticker: "XYZ", Dividend: DividendView{PayoutRatio: divF64(-10), YoYGrowth: divF64(1)}}, // loss-maker: negative payout, no yield
		{Ticker: "NOYLD", Dividend: DividendView{PayoutRatio: divF64(30)}},                      // payout-only
		{Ticker: "", Dividend: DividendView{Yield: divF64(99)}},                                 // empty ticker → dropped
	}
	order := func(rs []DividendRank) []string {
		out := make([]string, len(rs))
		for i, r := range rs {
			out[i] = r.Ticker
		}
		return out
	}
	eq := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	// highest-yield desc; MO & T tie at 6.5 → ticker asc (MO before T); names with no yield omitted.
	if got := order(RankDividend(pop, DividendViewHighestYield)); !eq(got, []string{"MO", "T", "KO", "AAPL", "NVDA"}) {
		t.Fatalf("highest-yield = %v, want [MO T KO AAPL NVDA]", got)
	}
	if got := order(RankDividend(pop, DividendViewFastestGrowing)); !eq(got, []string{"NVDA", "KO", "AAPL", "T", "XYZ"}) {
		t.Fatalf("fastest-growing = %v, want [NVDA KO AAPL T XYZ]", got)
	}
	if got := order(RankDividend(pop, DividendViewBestCovered)); !eq(got, []string{"NVDA", "AAPL", "KO", "T"}) {
		t.Fatalf("best-covered = %v, want [NVDA AAPL KO T]", got)
	}
	// lowest-payout asc, POSITIVE only → XYZ (-10) excluded.
	if got := order(RankDividend(pop, DividendViewLowestPayout)); !eq(got, []string{"NVDA", "AAPL", "NOYLD", "T", "KO"}) {
		t.Fatalf("lowest-payout = %v, want [NVDA AAPL NOYLD T KO]", got)
	}
	if got := RankDividend(pop, "bogus"); got != nil {
		t.Fatalf("unknown view = %v, want nil", got)
	}
	if !ValidDividendView("highest-yield") || !ValidDividendView("lowest-payout") || ValidDividendView("foo") {
		t.Fatal("ValidDividendView mismatch")
	}
	// Each row carries the FULL profile (not just the ranked metric): spot-check KO in highest-yield.
	top := RankDividend(pop, DividendViewHighestYield)
	var ko *DividendRank
	for i := range top {
		if top[i].Ticker == "KO" {
			ko = &top[i]
		}
	}
	if ko == nil || ko.PayoutRatio == nil || *ko.PayoutRatio != 70 || ko.YoYGrowth == nil || *ko.YoYGrowth != 5 {
		t.Fatalf("KO row should carry full profile, got %+v", ko)
	}
}
