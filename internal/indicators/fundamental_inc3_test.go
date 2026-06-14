package indicators

import (
	"testing"

	"github.com/wombow-ai/tickwind/internal/edgar"
)

// TestAltmanZ verifies the Z-score math on a hand-computed fixture and that any
// missing core input yields ok=false (insufficient, never a partial Z).
func TestAltmanZ(t *testing.T) {
	// X1=(400−200)/1000=0.2 · X2=300/1000=0.3 · X3=150/1000=0.15
	// X4=(50×40)/500=4.0 · X5=800/1000=0.8
	// Z = 1.2·0.2 + 1.4·0.3 + 3.3·0.15 + 0.6·4.0 + 1.0·0.8 = 4.355
	f := edgar.Fundamentals{
		TotalAssets:         1000,
		AssetsCurrent:       400,
		LiabilitiesCurrent:  200,
		RetainedEarnings:    300,
		OperatingIncomeLoss: 150, // EBIT (reported)
		TotalLiabilities:    500,
		Shares:              40,
		Revenue:             800,
	}
	z, ok := altmanZ(50, f) // price 50 × 40 shares = 2000 market cap
	if !ok {
		t.Fatal("altmanZ ok=false on a complete fixture")
	}
	if !floatEq(z, 4.355, 1e-9) {
		t.Fatalf("altmanZ = %v, want 4.355", z)
	}

	// Negative retained earnings (heavy-buyback issuer) is valid: X2 goes 0.3→−1.0,
	// so Z falls by 1.4·1.3 = 1.82 → 2.535.
	g := f
	g.RetainedEarnings = -1000 // X2 = -1.0
	if z2, ok := altmanZ(50, g); !ok || !floatEq(z2, 2.535, 1e-9) {
		t.Fatalf("altmanZ with negative retained earnings = %v,%v, want 2.535", z2, ok)
	}

	// Any missing core input → insufficient (no partial Z fabricated).
	for _, bad := range []struct {
		name string
		mut  func(*edgar.Fundamentals)
		px   float64
	}{
		{"no total assets", func(x *edgar.Fundamentals) { x.TotalAssets = 0 }, 50},
		{"no total liabilities", func(x *edgar.Fundamentals) { x.TotalLiabilities = 0 }, 50},
		{"no revenue", func(x *edgar.Fundamentals) { x.Revenue = 0 }, 50},
		{"no current assets (unclassified BS)", func(x *edgar.Fundamentals) { x.AssetsCurrent = 0 }, 50},
		{"no current liabilities", func(x *edgar.Fundamentals) { x.LiabilitiesCurrent = 0 }, 50},
		{"no EBIT (no op-income, no int/tax)", func(x *edgar.Fundamentals) { x.OperatingIncomeLoss = 0 }, 50},
		{"no price", func(x *edgar.Fundamentals) {}, 0},
		{"no shares", func(x *edgar.Fundamentals) { x.Shares = 0 }, 50},
	} {
		t.Run(bad.name, func(t *testing.T) {
			h := f
			bad.mut(&h)
			if _, ok := altmanZ(bad.px, h); ok {
				t.Errorf("altmanZ ok=true with %q, want insufficient", bad.name)
			}
		})
	}
}

// piotroskiFixture6 is a hand-computed fixture scoring exactly 6/9. Each point:
//
//	ROA = NetIncome/TotalAssets = 100/1000 = 0.10 ; ROAprior = 40/800 = 0.05
//	current ratio = 400/200 = 2.0 ; prior = 300/200 = 1.5
//	gross margin = 500/1000 = 0.50 ; prior = 360/900 = 0.40
//	asset turnover = 1000/1000 = 1.0 ; prior = 900/800 = 1.125
//
//	(1) ROA>0:          0.10 > 0            → 1
//	(2) OCF>0:          90 > 0              → 1
//	(3) ΔROA>0:         0.10 > 0.05         → 1
//	(4) accrual:        OCF 90 > NI 100     → 0  (cash trails earnings)
//	(5) ΔLeverage:      LTD 200 ≤ 250       → 1
//	(6) ΔCurrentRatio:  2.0 > 1.5           → 1
//	(7) no dilution:    Shares 1100 ≤ 1000  → 0  (shares grew)
//	(8) ΔGrossMargin:   0.50 > 0.40         → 1
//	(9) ΔAssetTurnover: 1.0 > 1.125         → 0  (turnover fell)
//	                                          = 6
var piotroskiFixture6 = edgar.Fundamentals{
	TotalAssets: 1000, TotalAssetsPrior: 800,
	NetIncome: 100, NetIncomePrior: 40,
	OperatingCashFlow: 90,
	LongTermDebt:      200, LongTermDebtPrior: 250,
	AssetsCurrent: 400, LiabilitiesCurrent: 200,
	AssetsCurrentPrior: 300, LiabilitiesCurrentPrior: 200,
	Shares: 1100, SharesPrior: 1000,
	GrossProfit: 500, Revenue: 1000,
	GrossProfitPrior: 360, RevenuePrior: 900,
}

// TestPiotroskiF verifies the 9-point sum on a hand-computed 6/9 fixture, a tweaked
// 9/9 (every test flipped to pass), and the all-or-nothing gating: any missing prior
// field or zero denominator yields ok=false (insufficient), never a partial score.
func TestPiotroskiF(t *testing.T) {
	if s, ok := piotroskiF(piotroskiFixture6); !ok || s != 6 {
		t.Fatalf("piotroskiF(fixture6) = %d,%v, want 6,true", s, ok)
	}

	// 9/9: flip the three failing points — OCF above NI (accrual), no new shares,
	// and rising asset turnover (shrink prior revenue so prior turnover < current).
	perfect := piotroskiFixture6
	perfect.OperatingCashFlow = 150 // > NI 100 → point 4
	perfect.Shares = 1000           // ≤ 1000 → point 7
	perfect.RevenuePrior = 700      // prior turnover 700/800=0.875 < 1.0 → point 9; gmPrior 360/700≈0.514 > 0.50 → point 8 now FAILS
	// Re-tune gross profit so point 8 still passes with the smaller prior revenue.
	perfect.GrossProfitPrior = 300 // gmPrior 300/700≈0.4286 < 0.50 → point 8 passes
	if s, ok := piotroskiF(perfect); !ok || s != 9 {
		t.Fatalf("piotroskiF(perfect) = %d,%v, want 9,true", s, ok)
	}

	// A debt-free-vs-prior firm: current LongTermDebt 0, prior 250 (paid it all down)
	// → point 5 awarded (0 ≤ 250). Prior debt present, so still sufficient.
	paidDown := piotroskiFixture6
	paidDown.LongTermDebt = 0
	if s, ok := piotroskiF(paidDown); !ok || s != 6 {
		t.Fatalf("piotroskiF(paidDown debt-free) = %d,%v, want 6,true (point 5 still awarded)", s, ok)
	}

	// All-or-nothing: any required denominator ≤ 0 OR any prior field absent → ok=false.
	for _, bad := range []struct {
		name string
		mut  func(*edgar.Fundamentals)
	}{
		{"no total assets", func(x *edgar.Fundamentals) { x.TotalAssets = 0 }},
		{"no prior total assets", func(x *edgar.Fundamentals) { x.TotalAssetsPrior = 0 }},
		{"no current liabilities", func(x *edgar.Fundamentals) { x.LiabilitiesCurrent = 0 }},
		{"no prior current liabilities", func(x *edgar.Fundamentals) { x.LiabilitiesCurrentPrior = 0 }},
		{"no revenue", func(x *edgar.Fundamentals) { x.Revenue = 0 }},
		{"no prior revenue", func(x *edgar.Fundamentals) { x.RevenuePrior = 0 }},
		{"no prior net income", func(x *edgar.Fundamentals) { x.NetIncomePrior = 0 }},
		{"no prior gross profit", func(x *edgar.Fundamentals) { x.GrossProfitPrior = 0 }},
		{"no prior current assets", func(x *edgar.Fundamentals) { x.AssetsCurrentPrior = 0 }},
		{"no prior long-term debt (no prior balance sheet)", func(x *edgar.Fundamentals) { x.LongTermDebtPrior = 0 }},
		{"no prior shares", func(x *edgar.Fundamentals) { x.SharesPrior = 0 }},
	} {
		t.Run(bad.name, func(t *testing.T) {
			h := piotroskiFixture6
			bad.mut(&h)
			if _, ok := piotroskiF(h); ok {
				t.Errorf("piotroskiF ok=true with %q, want insufficient (all-or-nothing)", bad.name)
			}
		})
	}
}

// TestPiotroskiFRegistry asserts the closure is registered, gates on hasFund, emits a
// dimensionless (Unit "") INTEGER score on the OK path, and reports insufficient (no
// value) when a prior field is missing.
func TestPiotroskiFRegistry(t *testing.T) {
	fn, ok := fundamentalRegistryInc3()["fundamental.piotroski-f-score"]
	if !ok {
		t.Fatal("fundamental.piotroski-f-score not registered")
	}
	// No fundamentals → insufficient, no value.
	si := StockIndicator{}
	fn(computeInput{hasFund: false}, &si)
	if si.Status != StatusInsufficient || si.Value != nil {
		t.Fatalf("no-fund path = %+v, want insufficient/no value", si)
	}
	// Complete fixture → ok, dimensionless, integer value.
	si = StockIndicator{}
	fn(computeInput{hasFund: true, fund: piotroskiFixture6}, &si)
	if si.Status != StatusOK || si.Value == nil || si.Unit != unitNone {
		t.Fatalf("ok path = %+v, want ok / unit '' / a value", si)
	}
	if *si.Value != 6 || *si.Value != float64(int(*si.Value)) {
		t.Fatalf("value = %v, want the integer 6", *si.Value)
	}
	// Missing prior field → insufficient, no value emitted (never a partial score).
	si = StockIndicator{}
	missing := piotroskiFixture6
	missing.SharesPrior = 0
	fn(computeInput{hasFund: true, fund: missing}, &si)
	if si.Status != StatusInsufficient || si.Value != nil {
		t.Fatalf("missing-prior path = %+v, want insufficient/no value", si)
	}
}

// TestAltmanZRegistry asserts the closure is registered, gates on hasFund, and emits
// a dimensionless (Unit "") score on the OK path.
func TestAltmanZRegistry(t *testing.T) {
	fn, ok := fundamentalRegistryInc3()["fundamental.altman-z-score"]
	if !ok {
		t.Fatal("fundamental.altman-z-score not registered")
	}
	// No fundamentals → insufficient, no value.
	si := StockIndicator{}
	fn(computeInput{hasFund: false}, &si)
	if si.Status != StatusInsufficient || si.Value != nil {
		t.Fatalf("no-fund path = %+v, want insufficient/no value", si)
	}
	// Complete fixture → ok, dimensionless.
	si = StockIndicator{}
	fn(computeInput{hasFund: true, price: 50, fund: edgar.Fundamentals{
		TotalAssets: 1000, AssetsCurrent: 400, LiabilitiesCurrent: 200, RetainedEarnings: 300,
		OperatingIncomeLoss: 150, TotalLiabilities: 500, Shares: 40, Revenue: 800,
	}}, &si)
	if si.Status != StatusOK || si.Value == nil || si.Unit != unitNone {
		t.Fatalf("ok path = %+v, want ok / unit '' / a value", si)
	}
}
