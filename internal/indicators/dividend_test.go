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
