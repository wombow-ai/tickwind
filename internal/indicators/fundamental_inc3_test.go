package indicators

import (
	"context"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/edgar"
	"github.com/wombow-ai/tickwind/internal/store"
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

// --- RISK / RETURN: beta + TSR ---

// constReturns returns a slice of n copies of v (a flat return series).
func constReturns(n int, v float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = v
	}
	return out
}

// TestBeta verifies the beta math on known aligned series and the anti-fabrication
// gating: a perfectly-correlated 2× stock → β=2.0; zero market variance → ok=false;
// fewer than betaMinPairs pairs or a length mismatch → ok=false.
func TestBeta(t *testing.T) {
	const n = 80 // >= betaMinPairs (60)

	// Perfectly correlated, stock moves 2× the market each day → β = 2.0 exactly.
	market := make([]float64, n)
	stock := make([]float64, n)
	for i := 0; i < n; i++ {
		// A non-constant market (so var(market) > 0): a small alternating wiggle.
		m := 0.01
		if i%2 == 1 {
			m = -0.005
		}
		market[i] = m
		stock[i] = 2 * m
	}
	if b, ok := beta(stock, market); !ok || !floatEq(b, 2.0, 1e-9) {
		t.Fatalf("beta(2× market) = %v,%v, want 2.0,true", b, ok)
	}

	// Inverse 1× market → β = −1.0.
	inverse := make([]float64, n)
	for i := range market {
		inverse[i] = -market[i]
	}
	if b, ok := beta(inverse, market); !ok || !floatEq(b, -1.0, 1e-9) {
		t.Fatalf("beta(inverse) = %v,%v, want -1.0,true", b, ok)
	}

	// Zero market variance (a flat market) → ok=false (cannot divide by 0 variance).
	// A constant 0 market sums/means to exactly 0, so var(market) is exactly 0.
	if _, ok := beta(constReturns(n, 0.01), constReturns(n, 0)); ok {
		t.Error("beta ok with zero market variance; want insufficient")
	}

	// Too few pairs (< betaMinPairs) → ok=false even with positive variance.
	short := betaMinPairs - 1
	sm := make([]float64, short)
	ss := make([]float64, short)
	for i := 0; i < short; i++ {
		m := 0.01
		if i%2 == 1 {
			m = -0.005
		}
		sm[i] = m
		ss[i] = 2 * m
	}
	if _, ok := beta(ss, sm); ok {
		t.Errorf("beta ok with only %d pairs; want insufficient (need %d)", short, betaMinPairs)
	}

	// Length mismatch → ok=false (the series must be aligned/equal-length).
	if _, ok := beta(make([]float64, n), make([]float64, n-1)); ok {
		t.Error("beta ok with mismatched lengths; want insufficient")
	}

	// Empty input → ok=false (no panic).
	if _, ok := beta(nil, nil); ok {
		t.Error("beta ok on empty input; want insufficient")
	}
}

// TestTSR verifies the ~1-year total-shareholder-return formula + gating: a non-payer
// reads the pure price return; a dividend payer adds the per-share dividend; below
// tsrMinCloses closes → insufficient; a non-positive start price → insufficient; and
// price<=0 falls back to the last close.
func TestTSR(t *testing.T) {
	// A flat ramp where the close tsrTradingDays back is 100 and the latest is 100; we
	// override end via the price argument so the math is exact and independent of ramp
	// shape. start = closes[len-252].
	closes := make([]float64, 260)
	for i := range closes {
		closes[i] = 100 // start = closes[260-252] = 100
	}

	// Non-payer, price 120 → pure price return (120-100)/100*100 = 20%.
	nonPayer := edgar.Fundamentals{Shares: 1000} // no DividendsPaid
	if v, ok := tsr(closes, 120, nonPayer); !ok || !floatEq(v, 20, 1e-9) {
		t.Fatalf("tsr(non-payer) = %v,%v, want 20,true", v, ok)
	}

	// Dividend payer: DividendsPaid 2000 over 1000 shares → $2/share. TSR =
	// ((120-100)+2)/100*100 = 22%.
	payer := edgar.Fundamentals{Shares: 1000, DividendsPaid: 2000}
	if v, ok := tsr(closes, 120, payer); !ok || !floatEq(v, 22, 1e-9) {
		t.Fatalf("tsr(payer) = %v,%v, want 22,true", v, ok)
	}

	// price <= 0 falls back to the last close (here 100) → (100-100+2)/100*100 = 2%.
	if v, ok := tsr(closes, 0, payer); !ok || !floatEq(v, 2, 1e-9) {
		t.Fatalf("tsr(price 0 → last close) = %v,%v, want 2,true", v, ok)
	}

	// < tsrMinCloses closes → insufficient (not a 1-year window).
	if _, ok := tsr(closes[:tsrMinCloses-1], 120, payer); ok {
		t.Errorf("tsr ok with only %d closes; want insufficient (need %d)", tsrMinCloses-1, tsrMinCloses)
	}

	// Exactly tsrMinCloses closes (fewer than tsrTradingDays) → start = closes[0],
	// still ok. All 100s here → pure price return to price 110 = 10%.
	short := make([]float64, tsrMinCloses)
	for i := range short {
		short[i] = 100
	}
	if v, ok := tsr(short, 110, nonPayer); !ok || !floatEq(v, 10, 1e-9) {
		t.Fatalf("tsr(exactly min closes) = %v,%v, want 10,true", v, ok)
	}

	// Non-positive start price → insufficient (cannot divide by a 0 start).
	badStart := make([]float64, 260)
	for i := range badStart {
		badStart[i] = 100
	}
	badStart[260-tsrTradingDays] = 0 // start = 0
	if _, ok := tsr(badStart, 120, nonPayer); ok {
		t.Error("tsr ok with a non-positive start price; want insufficient")
	}

	// Non-positive end (price 0 AND last close 0) → insufficient.
	zeroEnd := make([]float64, 260)
	zeroEnd[260-tsrTradingDays] = 100
	zeroEnd[len(zeroEnd)-1] = 0
	if _, ok := tsr(zeroEnd, 0, nonPayer); ok {
		t.Error("tsr ok with a non-positive end price; want insufficient")
	}
}

// mkCandles builds n daily candles starting at base, one calendar day apart, each with
// the given close (Open/High/Low set around it). Used to exercise the date-alignment.
func mkCandles(base time.Time, closes []float64) []store.Candle {
	out := make([]store.Candle, len(closes))
	for i, c := range closes {
		out[i] = store.Candle{
			Time:  base.AddDate(0, 0, i),
			Open:  c,
			High:  c + 0.5,
			Low:   c - 0.5,
			Close: c,
		}
	}
	return out
}

// TestAlignedReturns verifies the DATE-alignment: only consecutive stock days whose
// BOTH dates also have a SPY close form a pair, the two output series are equal-length,
// and a SPY gap drops exactly the affected pair(s).
func TestAlignedReturns(t *testing.T) {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// Full overlap: 4 consecutive days → 3 paired returns, equal length.
	stock := mkCandles(base, []float64{100, 110, 121, 133.1}) // +10% each day
	market := mkCandles(base, []float64{200, 202, 204.02, 206.0602})
	sr, mr := alignedReturns(stock, market)
	if len(sr) != 3 || len(mr) != 3 {
		t.Fatalf("full overlap lengths = %d,%d, want 3,3", len(sr), len(mr))
	}
	for _, r := range sr {
		if !floatEq(r, 0.1, 1e-9) {
			t.Errorf("stock return = %v, want 0.1", r)
		}
	}

	// SPY missing the middle day (day index 1): the pairs (day0→day1) and (day1→day2)
	// both reference the missing day and are dropped; only (day2→day3) survives → 1 pair.
	marketGap := []store.Candle{
		{Time: base, Close: 200},
		// day index 1 absent
		{Time: base.AddDate(0, 0, 2), Close: 204},
		{Time: base.AddDate(0, 0, 3), Close: 206},
	}
	sr2, mr2 := alignedReturns(stock, marketGap)
	if len(sr2) != 1 || len(mr2) != 1 {
		t.Fatalf("gap lengths = %d,%d, want 1,1 (the two pairs touching the missing day dropped)", len(sr2), len(mr2))
	}

	// Degenerate inputs → empty (no panic).
	if sr3, mr3 := alignedReturns(stock[:1], market); sr3 != nil || mr3 != nil {
		t.Error("alignedReturns with a single stock candle should be empty")
	}
}

// fakeMultiOHLCV returns a different candle series per ticker (so a stock and its SPY
// benchmark can be distinguished), plus an optional per-ticker error.
type fakeMultiOHLCV struct {
	byTicker map[string][]store.Candle
}

func (f fakeMultiOHLCV) DailyCandles(_ context.Context, ticker string) ([]store.Candle, error) {
	return f.byTicker[ticker], nil
}

// TestTSRRegistry asserts the closure is registered, gates on hasFund, emits a percent
// value on the OK path, and reports insufficient (no value) without enough closes.
func TestTSRRegistry(t *testing.T) {
	fn, ok := fundamentalRegistryInc3()["fundamental.tsr"]
	if !ok {
		t.Fatal("fundamental.tsr not registered")
	}
	closes := make([]float64, 260)
	for i := range closes {
		closes[i] = 100
	}
	// No fundamentals → insufficient (the dividend term needs Fundamentals).
	si := StockIndicator{}
	fn(computeInput{hasFund: false, closes: closes, price: 120}, &si)
	if si.Status != StatusInsufficient || si.Value != nil {
		t.Fatalf("no-fund path = %+v, want insufficient/no value", si)
	}
	// Full input → ok, percent unit, value 20.
	si = StockIndicator{}
	fn(computeInput{hasFund: true, closes: closes, price: 120, fund: edgar.Fundamentals{Shares: 1000}}, &si)
	if si.Status != StatusOK || si.Unit != unitPercent || si.Value == nil || !floatEq(*si.Value, 20, 1e-9) {
		t.Fatalf("ok path = %+v, want ok / unit '%%' / value 20", si)
	}
	// Too few closes → insufficient, no value.
	si = StockIndicator{}
	fn(computeInput{hasFund: true, closes: closes[:10], price: 120, fund: edgar.Fundamentals{Shares: 1000}}, &si)
	if si.Status != StatusInsufficient || si.Value != nil {
		t.Fatalf("short-closes path = %+v, want insufficient/no value", si)
	}
}

// TestBetaRegistry asserts the closure is registered, emits an "x" multiple on the OK
// path (over prepared aligned returns), and reports insufficient (no value) when the
// aligned series are empty (the SPY-unavailable / ticker==SPY case).
func TestBetaRegistry(t *testing.T) {
	fn, ok := fundamentalRegistryInc3()["fundamental.beta"]
	if !ok {
		t.Fatal("fundamental.beta not registered")
	}
	const n = 80
	market := make([]float64, n)
	stock := make([]float64, n)
	for i := 0; i < n; i++ {
		m := 0.01
		if i%2 == 1 {
			m = -0.005
		}
		market[i] = m
		stock[i] = 2 * m
	}
	// Prepared aligned returns → ok, "x" multiple, value 2.0.
	si := StockIndicator{}
	fn(computeInput{stockReturns: stock, marketReturns: market}, &si)
	if si.Status != StatusOK || si.Unit != unitMult || si.Value == nil || !floatEq(*si.Value, 2.0, 1e-9) {
		t.Fatalf("ok path = %+v, want ok / unit 'x' / value 2.0", si)
	}
	// No aligned returns (SPY unavailable / ticker==SPY) → insufficient, no value.
	si = StockIndicator{}
	fn(computeInput{}, &si)
	if si.Status != StatusInsufficient || si.Value != nil {
		t.Fatalf("empty-returns path = %+v, want insufficient/no value", si)
	}
}

// TestComputeBeta_AlignsAgainstSPY drives the Computer end-to-end: with both the stock
// and SPY candles available, beta is computed over the date-aligned series and emitted
// "x". A flat-stock-vs-moving-SPY fixture yields β = 0 (the stock has zero covariance
// with the market).
func TestComputeBeta_AlignsAgainstSPY(t *testing.T) {
	cat := MustLoad()
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// 90 aligned days. SPY wiggles (positive variance); the stock is constant → β = 0.
	spyCloses := make([]float64, 90)
	stkCloses := make([]float64, 90)
	for i := range spyCloses {
		v := 400.0
		if i%2 == 1 {
			v = 404.0
		}
		spyCloses[i] = v
		stkCloses[i] = 50 // flat → zero covariance with the market
	}
	stock := mkCandles(base, stkCloses)
	spy := mkCandles(base, spyCloses)

	c := NewComputer(cat,
		fakeMultiOHLCV{byTicker: map[string][]store.Candle{"AAPL": stock, "SPY": spy}},
		fakeFund{f: edgar.Fundamentals{Ticker: "AAPL", Shares: 1000, Revenue: 5000, NetIncome: 800, EPSDiluted: 4, Equity: 2000, AsOf: "2024-12-31"}},
		fakePrice{price: 50, ok: true},
		nil,
	)
	res := c.StockIndicators(context.Background(), "AAPL")
	si := byID(res.Indicators)["fundamental.beta"]
	if si.Status != StatusOK || si.Unit != unitMult || si.Value == nil {
		t.Fatalf("beta = %+v, want ok / 'x' / a value", si)
	}
	if !floatEq(*si.Value, 0, 1e-9) {
		t.Errorf("beta(flat stock vs moving SPY) = %v, want 0", *si.Value)
	}
}

// TestComputeBeta_NoSPYInsufficient asserts the Computer-level anti-fabrication guard:
// when the ohlcv source returns NO SPY candles, beta is insufficient (never fabricated),
// while the stock's own candle-driven technicals still compute.
func TestComputeBeta_NoSPYInsufficient(t *testing.T) {
	cat := MustLoad()
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	stkCloses := make([]float64, 120)
	for i := range stkCloses {
		stkCloses[i] = 50 + float64(i)
	}
	stock := mkCandles(base, stkCloses)

	// SPY entry absent from the map → DailyCandles("SPY") returns nil.
	c := NewComputer(cat,
		fakeMultiOHLCV{byTicker: map[string][]store.Candle{"AAPL": stock}},
		fakeFund{f: edgar.Fundamentals{Ticker: "AAPL", Shares: 1000, EPSDiluted: 4, Equity: 2000, Revenue: 5000, NetIncome: 800, AsOf: "2024-12-31"}},
		fakePrice{price: 100, ok: true},
		nil,
	)
	res := c.StockIndicators(context.Background(), "AAPL")
	m := byID(res.Indicators)
	if si := m["fundamental.beta"]; si.Status != StatusInsufficient || si.Value != nil {
		t.Fatalf("beta with no SPY = %+v, want insufficient/no value", si)
	}
	// A candle-driven technical still computes (the rest of the pipeline is unaffected).
	if si := m["technical.sma-ma"]; si.Status != StatusOK {
		t.Errorf("sma should still be ok with stock candles: %+v", si)
	}
}

// TestComputeBeta_SPYItselfSkipped asserts that when the ticker IS SPY, beta is NOT
// computed against itself (the degenerate β=1): the aligned series are skipped → the
// beta closure reports insufficient.
func TestComputeBeta_SPYItselfSkipped(t *testing.T) {
	cat := MustLoad()
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	closes := make([]float64, 90)
	for i := range closes {
		v := 400.0
		if i%2 == 1 {
			v = 404.0
		}
		closes[i] = v
	}
	spy := mkCandles(base, closes)
	c := NewComputer(cat,
		fakeMultiOHLCV{byTicker: map[string][]store.Candle{"SPY": spy}},
		fakeFund{f: edgar.Fundamentals{}},
		fakePrice{ok: false},
		nil,
	)
	res := c.StockIndicators(context.Background(), "SPY")
	if si := byID(res.Indicators)["fundamental.beta"]; si.Status != StatusInsufficient || si.Value != nil {
		t.Fatalf("beta for SPY-vs-itself = %+v, want insufficient (skipped)", si)
	}
}
