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
