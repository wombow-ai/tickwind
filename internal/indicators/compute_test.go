package indicators

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/edgar"
	"github.com/wombow-ai/tickwind/internal/store"
)

// floatEq reports whether a and b are within a small absolute tolerance.
func floatEq(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

const tol = 1e-9

// rampCloses returns the ascending series [1, 2, ..., n].
func rampCloses(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = float64(i + 1)
	}
	return out
}

// --- (i) technical functions vs hand-computed / TA-reference values ---

func TestSMA(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		period int
		want   float64
		ok     bool
	}{
		{"mean of 11..30", rampCloses(30), 20, 20.5, true},
		{"exact-length window", []float64{2, 4, 6}, 3, 4, true},
		{"too few", []float64{1, 2}, 3, 0, false},
		{"zero period", []float64{1, 2, 3}, 0, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := sma(tc.values, tc.period)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && !floatEq(got, tc.want, tol) {
				t.Errorf("sma = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEMA(t *testing.T) {
	// On a linear ramp the SMA-seeded EMA converges to the same value as the SMA
	// of the final window (the series is symmetric about its window mean here).
	got, ok := ema(rampCloses(30), 20)
	if !ok {
		t.Fatal("ema not ok on 30-point ramp")
	}
	if !floatEq(got, 20.5, tol) {
		t.Errorf("ema = %v, want 20.5", got)
	}
	if _, ok := ema(rampCloses(5), 20); ok {
		t.Error("ema ok on too-short series; want insufficient")
	}
}

func TestRSIWilder(t *testing.T) {
	// Classic Wilder reference series (StockCharts/TA-Lib worked example).
	closes := []float64{
		44.34, 44.09, 44.15, 43.61, 44.33, 44.83, 45.10, 45.42, 45.84, 46.08,
		45.89, 46.03, 45.61, 46.28, 46.28, 46.00, 46.03, 46.41, 46.22, 45.64,
		46.21, 46.25, 45.71, 46.45, 45.78, 45.35, 44.03, 44.18, 44.22, 44.57,
		43.42, 42.66, 43.13,
	}
	got, ok := rsiWilder(closes, 14)
	if !ok {
		t.Fatal("rsi not ok")
	}
	if !floatEq(got, 37.788771982057824, 1e-6) {
		t.Errorf("rsi latest = %v, want ~37.7888", got)
	}

	// No-loss / no-gain edges.
	if v, ok := rsiWilder([]float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}, 14); !ok || v != 100 {
		t.Errorf("monotonic-up rsi = %v (ok=%v), want 100", v, ok)
	}
	if v, ok := rsiWilder([]float64{15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}, 14); !ok || v != 0 {
		t.Errorf("monotonic-down rsi = %v (ok=%v), want 0", v, ok)
	}
	if _, ok := rsiWilder([]float64{1, 2, 3}, 14); ok {
		t.Error("rsi ok on too-short series; want insufficient")
	}
}

func TestMACD(t *testing.T) {
	// On a linear ramp the fast/slow EMAs settle to a constant offset, so the
	// MACD line is constant and the histogram is ~0.
	got, ok := macd(rampCloses(40), 12, 26, 9)
	if !ok {
		t.Fatal("macd not ok on 40-point ramp")
	}
	if !floatEq(got.Line, 7.0, 1e-9) {
		t.Errorf("macd line = %v, want 7.0", got.Line)
	}
	if !floatEq(got.Signal, 7.0, 1e-9) {
		t.Errorf("macd signal = %v, want 7.0", got.Signal)
	}
	if !floatEq(got.Histogram, 0, 1e-9) {
		t.Errorf("macd hist = %v, want ~0", got.Histogram)
	}
	if _, ok := macd(rampCloses(10), 12, 26, 9); ok {
		t.Error("macd ok with too few closes; want insufficient")
	}
}

func TestBollinger(t *testing.T) {
	got, ok := bollinger(rampCloses(30), 20, 2)
	if !ok {
		t.Fatal("bollinger not ok")
	}
	if !floatEq(got.Middle, 20.5, tol) {
		t.Errorf("boll mid = %v, want 20.5", got.Middle)
	}
	// Population σ of the integers 11..30 is ~5.76628.
	if !floatEq(got.Upper, 32.03256259467079, 1e-6) {
		t.Errorf("boll upper = %v, want ~32.0326", got.Upper)
	}
	if !floatEq(got.Lower, 8.967437405329203, 1e-6) {
		t.Errorf("boll lower = %v, want ~8.9674", got.Lower)
	}
	if _, ok := bollinger(rampCloses(5), 20, 2); ok {
		t.Error("bollinger ok with too few closes; want insufficient")
	}
}

func TestATRWilder(t *testing.T) {
	highs := []float64{10, 11, 12, 13, 14, 13, 15, 16}
	lows := []float64{8, 9, 10, 11, 12, 11, 12, 14}
	closes := []float64{9, 10, 11, 12, 13, 12, 14, 15}
	got, ok := atrWilder(highs, lows, closes, 3)
	if !ok {
		t.Fatal("atr not ok")
	}
	if !floatEq(got, 2.2222222222222223, 1e-9) {
		t.Errorf("atr(3) = %v, want ~2.2222", got)
	}
	if _, ok := atrWilder(highs[:2], lows[:2], closes[:2], 3); ok {
		t.Error("atr ok with too few bars; want insufficient")
	}
}

func TestStochasticKDJ(t *testing.T) {
	highs := []float64{10, 11, 12, 13, 14, 13, 15, 16}
	lows := []float64{8, 9, 10, 11, 12, 11, 12, 14}
	closes := []float64{9, 10, 11, 12, 13, 12, 14, 15}
	// n=3, slowK=1, slowD=1 → %K = last RSV, %D = %K (hand-checkable).
	got, ok := stochasticKDJ(highs, lows, closes, 3, 1, 1)
	if !ok {
		t.Fatal("kdj(3,1,1) not ok")
	}
	if !floatEq(got.K, 80, 1e-9) || !floatEq(got.D, 80, 1e-9) || !floatEq(got.J, 80, 1e-9) {
		t.Errorf("kdj(3,1,1) = %+v, want K=D=J=80", got)
	}
	// n=3, slowK=3, slowD=3 → smoothed reference.
	got2, ok := stochasticKDJ(highs, lows, closes, 3, 3, 3)
	if !ok {
		t.Fatal("kdj(3,3,3) not ok")
	}
	if !floatEq(got2.K, 62.77777777777777, 1e-6) {
		t.Errorf("kdj(3,3,3) K = %v, want ~62.778", got2.K)
	}
	if !floatEq(got2.D, 61.666666666666664, 1e-6) {
		t.Errorf("kdj(3,3,3) D = %v, want ~61.667", got2.D)
	}
	if !floatEq(got2.J, 64.99999999999999, 1e-6) {
		t.Errorf("kdj(3,3,3) J = %v, want ~65.0", got2.J)
	}
	// Flat window → RSV = 50.
	flat := []float64{5, 5, 5, 5}
	if got3, ok := stochasticKDJ(flat, flat, flat, 3, 1, 1); !ok || !floatEq(got3.K, 50, 1e-9) {
		t.Errorf("flat kdj = %+v (ok=%v), want K=50", got3, ok)
	}
}

func TestVWAP(t *testing.T) {
	highs := []float64{10, 11, 12, 13, 14, 13, 15, 16}
	lows := []float64{8, 9, 10, 11, 12, 11, 12, 14}
	closes := []float64{9, 10, 11, 12, 13, 12, 14, 15}
	vols := []float64{100, 100, 100, 100, 100, 100, 100, 100}
	got, ok := vwap(highs, lows, closes, vols)
	if !ok {
		t.Fatal("vwap not ok")
	}
	if !floatEq(got, 11.958333333333332, 1e-9) {
		t.Errorf("vwap = %v, want ~11.9583", got)
	}
	if _, ok := vwap(nil, nil, nil, nil); ok {
		t.Error("vwap ok on empty bars; want insufficient")
	}
	zeroVol := []float64{0, 0, 0, 0, 0, 0, 0, 0}
	if _, ok := vwap(highs, lows, closes, zeroVol); ok {
		t.Error("vwap ok on zero total volume; want insufficient")
	}
}

func TestLatestVolume(t *testing.T) {
	if v, ok := latestVolume([]float64{1, 2, 3}); !ok || v != 3 {
		t.Errorf("latestVolume = %v (ok=%v), want 3", v, ok)
	}
	if _, ok := latestVolume(nil); ok {
		t.Error("latestVolume ok on empty; want insufficient")
	}
}

// --- (ii) fundamental ratios on hand-computed fixtures ---

func TestFundamentalRatios(t *testing.T) {
	// A profitable, dividend-paying fixture.
	profit := edgar.Fundamentals{
		Shares: 1000, Revenue: 5000, NetIncome: 800, EPSDiluted: 4, Equity: 2000,
		GrossProfit: 2500, TotalAssets: 6000, TotalLiabilities: 4000,
		OperatingCashFlow: 1200, CapEx: 300, DividendsPaid: 100,
		RevenuePrior: 4000, NetIncomePrior: 500,
	}
	const price = 40.0 // market cap = 40*1000 = 40000

	tests := []struct {
		name string
		eval func() (float64, bool)
		want float64
		ok   bool
	}{
		{"pe", func() (float64, bool) { return peTTM(price, profit) }, 10, true},               // 40/4
		{"pb", func() (float64, bool) { return pb(price, profit) }, 20, true},                  // 40000/2000
		{"roe%", func() (float64, bool) { return roe(profit) }, 40, true},                      // 800/2000*100
		{"npm%", func() (float64, bool) { return npm(profit) }, 16, true},                      // 800/5000*100
		{"gpm%", func() (float64, bool) { return gpm(profit) }, 50, true},                      // 2500/5000*100
		{"revGrowth%", func() (float64, bool) { return revenueGrowthYoY(profit) }, 25, true},   // (5000-4000)/4000*100
		{"earnGrowth%", func() (float64, bool) { return earningsGrowthYoY(profit) }, 60, true}, // (800-500)/500*100
		{"fcf", func() (float64, bool) { return fcf(profit) }, 900, true},                      // 1200-300
		{"dy%", func() (float64, bool) { return dividendYield(price, profit) }, 0.25, true},    // 100/40000*100
		{"debtToAsset%", func() (float64, bool) { return debtToAsset(profit) }, 66.66666666666666, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tc.eval()
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && !floatEq(got, tc.want, 1e-9) {
				t.Errorf("%s = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestFundamentalRatios_LossMaker(t *testing.T) {
	loss := edgar.Fundamentals{
		Shares: 350, Revenue: 463, NetIncome: -1200, EPSDiluted: -3.5, Equity: 5000,
		NetIncomePrior: -800,
	}
	// PE: non-positive EPS → insufficient (no fabrication of a meaningless ratio).
	if _, ok := peTTM(100, loss); ok {
		t.Error("pe ok on a loss-maker; want insufficient")
	}
	// NPM is negative but defined (revenue positive).
	if v, ok := npm(loss); !ok || !floatEq(v, -259.1792656587473, 1e-6) {
		t.Errorf("npm on loss = %v (ok=%v)", v, ok)
	}
	// Earnings growth uses |prior| as the base → (-1200 - -800)/800*100 = -50%.
	if v, ok := earningsGrowthYoY(loss); !ok || !floatEq(v, -50, 1e-9) {
		t.Errorf("earnings growth on loss = %v (ok=%v), want -50", v, ok)
	}
}

func TestFundamentalRatios_NonPayerAndMissing(t *testing.T) {
	// A non-dividend payer with no balance-sheet detail.
	bare := edgar.Fundamentals{Shares: 100, Revenue: 1000, NetIncome: 100, EPSDiluted: 1, Equity: 500}
	if _, ok := dividendYield(50, bare); ok {
		t.Error("dy ok for a non-payer; want insufficient")
	}
	if _, ok := debtToAsset(bare); ok {
		t.Error("debtToAsset ok with no total assets; want insufficient")
	}
	if _, ok := gpm(bare); ok {
		t.Error("gpm ok with no gross profit; want insufficient")
	}
	if _, ok := fcf(bare); ok {
		t.Error("fcf ok with no operating cash flow; want insufficient")
	}
	if _, ok := revenueGrowthYoY(bare); ok {
		t.Error("revenue growth ok with no prior revenue; want insufficient")
	}
	// Negative-equity firm → PB and ROE insufficient.
	neg := edgar.Fundamentals{Shares: 100, Revenue: 1000, NetIncome: 50, EPSDiluted: 0.5, Equity: -200}
	if _, ok := pb(50, neg); ok {
		t.Error("pb ok with negative equity; want insufficient")
	}
	if _, ok := roe(neg); ok {
		t.Error("roe ok with negative equity; want insufficient")
	}
}

// --- fake in-memory sources for Compute() tests ---

type fakeOHLCV struct {
	candles []store.Candle
	err     error
}

func (f fakeOHLCV) DailyCandles(context.Context, string) ([]store.Candle, error) {
	return f.candles, f.err
}

type fakeFund struct {
	f   edgar.Fundamentals
	err error
}

func (f fakeFund) Fundamentals(context.Context, string) (edgar.Fundamentals, error) {
	return f.f, f.err
}

type fakePrice struct {
	price float64
	ok    bool
}

func (f fakePrice) Price(context.Context, string) (float64, bool) { return f.price, f.ok }

type fakeMarket struct {
	vix     float64
	vixOK   bool
	fgScore int
	fgLabel string
	fgOK    bool
}

func (f fakeMarket) VIX() (float64, bool) { return f.vix, f.vixOK }
func (f fakeMarket) FearGreed() (int, string, bool) {
	return f.fgScore, f.fgLabel, f.fgOK
}

// makeCandles builds n ascending daily candles with a fixed +/- range and volume,
// dated consecutively so AsOf resolves to the last bar.
func makeCandles(n int) []store.Candle {
	out := make([]store.Candle, n)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		c := float64(i + 1)
		out[i] = store.Candle{
			Time:   base.AddDate(0, 0, i),
			Open:   c,
			High:   c + 0.5,
			Low:    c - 0.5,
			Close:  c,
			Volume: 1000 + float64(i),
		}
	}
	return out
}

func byID(sis []StockIndicator) map[string]StockIndicator {
	m := make(map[string]StockIndicator, len(sis))
	for _, si := range sis {
		m[si.ID] = si
	}
	return m
}

func TestCompute_FullPathOK(t *testing.T) {
	cat := MustLoad()
	candles := makeCandles(120) // plenty for every technical window
	fund := edgar.Fundamentals{
		Ticker: "AAPL", Shares: 1000, Revenue: 5000, NetIncome: 800, EPSDiluted: 4,
		Equity: 2000, GrossProfit: 2500, TotalAssets: 6000, TotalLiabilities: 4000,
		OperatingCashFlow: 1200, CapEx: 300, DividendsPaid: 100,
		RevenuePrior: 4000, NetIncomePrior: 500, AsOf: "2025-12-31",
	}
	c := NewComputer(cat,
		fakeOHLCV{candles: candles},
		fakeFund{f: fund},
		fakePrice{price: 40, ok: true},
		fakeMarket{vix: 14.2, vixOK: true, fgScore: 62, fgLabel: "Greed", fgOK: true},
	)
	res := c.StockIndicators(context.Background(), "AAPL")

	if res.Ticker != "AAPL" {
		t.Errorf("ticker = %q", res.Ticker)
	}
	if res.AsOf != "2026-04-30" { // 2026-01-01 + 119 days
		t.Errorf("as_of = %q, want 2026-04-30", res.AsOf)
	}
	if res.VIX == nil || !floatEq(*res.VIX, 14.2, 1e-9) {
		t.Errorf("vix = %v, want 14.2", res.VIX)
	}
	if res.FearGreed == nil || res.FearGreed.Score != 62 || res.FearGreed.Label != "Greed" {
		t.Errorf("fear&greed = %+v", res.FearGreed)
	}

	m := byID(res.Indicators)
	// Spot-check a few computed values.
	if si := m["fundamental.pe-ttm"]; si.Status != StatusOK || si.Value == nil || !floatEq(*si.Value, 10, 1e-9) || si.Unit != unitMult {
		t.Errorf("pe = %+v", si)
	}
	if si := m["fundamental.roe"]; si.Status != StatusOK || si.Unit != unitPercent || si.Value == nil || !floatEq(*si.Value, 40, 1e-9) {
		t.Errorf("roe = %+v", si)
	}
	if si := m["technical.macd"]; si.Status != StatusOK || si.Extra == nil {
		t.Errorf("macd = %+v", si)
	} else if _, hasSig := si.Extra["signal"]; !hasSig {
		t.Errorf("macd missing signal extra: %+v", si.Extra)
	}
	if si := m["technical.boll"]; si.Status != StatusOK || si.Extra["upper"] <= si.Extra["lower"] {
		t.Errorf("boll = %+v", si)
	}
	// Market context.
	if si := m[idVIX]; si.Status != StatusOK || si.Value == nil || !floatEq(*si.Value, 14.2, 1e-9) {
		t.Errorf("vix indicator = %+v", si)
	}
	if si := m[idFearGreed]; si.Status != StatusOK || si.Value == nil || !floatEq(*si.Value, 62, 1e-9) {
		t.Errorf("fear&greed indicator = %+v", si)
	}
	// Crypto-only ids are unsupported with the contract reason.
	for id := range cryptoOnlyIDs {
		si := m[id]
		if si.Status != StatusUnsupported || si.Reason != cryptoUnsupportedReason {
			t.Errorf("crypto id %s = %+v, want unsupported", id, si)
		}
		if si.Value != nil {
			t.Errorf("crypto id %s has a value; want nil", id)
		}
	}

	// Ordering: ok first, then insufficient, then unsupported.
	assertSorted(t, res.Indicators)
}

func TestCompute_NoFundamentals_TechnicalsStillOK(t *testing.T) {
	cat := MustLoad()
	c := NewComputer(cat,
		fakeOHLCV{candles: makeCandles(120)},
		fakeFund{f: edgar.Fundamentals{}}, // HasData() false → no fundamentals
		fakePrice{ok: false},
		nil, // no market context
	)
	res := c.StockIndicators(context.Background(), "ZZZZ")
	m := byID(res.Indicators)

	if si := m["technical.sma-ma"]; si.Status != StatusOK {
		t.Errorf("sma should be ok with bars: %+v", si)
	}
	if si := m["fundamental.pe-ttm"]; si.Status != StatusInsufficient {
		t.Errorf("pe should be insufficient without XBRL: %+v", si)
	}
	// Market-context ids degrade to insufficient when the provider is nil.
	if si := m[idVIX]; si.Status != StatusInsufficient {
		t.Errorf("vix should be insufficient without a provider: %+v", si)
	}
}

func TestCompute_NoData_AllNonTechnicalDegrade(t *testing.T) {
	cat := MustLoad()
	c := NewComputer(cat, nil, nil, nil, nil) // every source nil
	res := c.StockIndicators(context.Background(), "ZZZZ")
	if res.AsOf != "" {
		t.Errorf("as_of = %q, want empty", res.AsOf)
	}
	for _, si := range res.Indicators {
		if si.Status == StatusOK {
			t.Errorf("indicator %s is ok with no data: %+v", si.ID, si)
		}
	}
}

// assertSorted verifies the ok → insufficient → unsupported ordering.
func assertSorted(t *testing.T, sis []StockIndicator) {
	t.Helper()
	last := -1
	for _, si := range sis {
		r := statusRank[si.Status]
		if r < last {
			t.Fatalf("indicators not sorted by status: %s (rank %d) after rank %d", si.ID, r, last)
		}
		last = r
	}
}

// --- (iii) registry-coverage: every P0 stock-applicable id is handled ---

func TestRegistryCoversAllP0(t *testing.T) {
	cat := MustLoad()
	c := NewComputer(cat, nil, nil, nil, nil)

	p0 := cat.Filter(Query{Priority: "P0"})
	if len(p0) == 0 {
		t.Fatal("no P0 stock-applicable indicators in the catalog")
	}

	for _, rec := range p0 {
		_, registered := c.registry[rec.ID]
		_, crypto := cryptoOnlyIDs[rec.ID]
		isMarketCtx := rec.ID == idVIX || rec.ID == idFearGreed
		if !registered && !crypto && !isMarketCtx {
			t.Errorf("P0 id %q is silently unhandled: register a compute impl, add it to "+
				"cryptoOnlyIDs, or mark it a market-context id", rec.ID)
		}
	}
}

// TestComputeNeverFabricates asserts that an insufficient/unsupported indicator
// never carries a value (the dataset guardrail: no fabricated numbers).
func TestComputeNeverFabricates(t *testing.T) {
	cat := MustLoad()
	c := NewComputer(cat, nil, nil, nil, nil)
	res := c.StockIndicators(context.Background(), "ZZZZ")
	for _, si := range res.Indicators {
		if si.Status != StatusOK && si.Value != nil {
			t.Errorf("indicator %s is %s but carries a value %v", si.ID, si.Status, *si.Value)
		}
		if si.Status == StatusOK && si.Value == nil {
			t.Errorf("indicator %s is ok but has no value", si.ID)
		}
	}
}

// TestRegistryNoDuplicateIDs asserts the four sub-registries merged in NewComputer
// have disjoint ids. A double-registered id would silently shadow one closure when
// the maps are copied in, so this fails loudly if any id appears in more than one
// registry (the merge itself cannot detect the collision).
func TestRegistryNoDuplicateIDs(t *testing.T) {
	subs := []struct {
		name string
		reg  map[string]computeFn
	}{
		{"technicalRegistry", technicalRegistry()},
		{"fundamentalRegistry", fundamentalRegistry()},
		{"technicalRegistryMore", technicalRegistryMore()},
		{"fundamentalRegistryMore", fundamentalRegistryMore()},
	}
	seen := make(map[string]string)
	for _, s := range subs {
		for id := range s.reg {
			if prev, dup := seen[id]; dup {
				t.Errorf("indicator id %q is registered in both %s and %s", id, prev, s.name)
				continue
			}
			seen[id] = s.name
		}
	}
}

// TestRegistryIDsAreRealCatalogIDs asserts every registered closure key is a real
// catalog id (catches typos: a key that no catalog record carries would silently
// never be evaluated, since computedIDs iterates the catalog).
func TestRegistryIDsAreRealCatalogIDs(t *testing.T) {
	cat := MustLoad()
	c := NewComputer(cat, nil, nil, nil, nil)

	catIDs := make(map[string]struct{}, cat.Len())
	for _, rec := range cat.All() {
		catIDs[rec.ID] = struct{}{}
	}
	for id := range c.registry {
		if _, ok := catIDs[id]; !ok {
			t.Errorf("registry id %q is not a real catalog id (typo?)", id)
		}
	}
}

// syntheticInput builds a fully-populated computeInput large enough to satisfy
// every registered closure's longest window (Connors RSI needs ~102 bars), with a
// mildly varying OHLCV so deltas, ranges, and volumes are all non-zero, plus a
// profitable, dividend-paying fundamentals fixture and a price. It is "valid" input
// — every closure should return ok or insufficient over it, never panic or fabricate.
func syntheticInput() computeInput {
	const n = 140
	opens := make([]float64, n)
	highs := make([]float64, n)
	lows := make([]float64, n)
	closes := make([]float64, n)
	volumes := make([]float64, n)
	for i := 0; i < n; i++ {
		// A gentle wave on a rising trend so windows are never flat (zero range /
		// zero deviation would legitimately be insufficient, which is fine, but the
		// wave exercises the ok path of more closures).
		base := 100 + float64(i)*0.5 + 5*math.Sin(float64(i)/4)
		opens[i] = base
		closes[i] = base + math.Cos(float64(i)/3)
		highs[i] = math.Max(opens[i], closes[i]) + 1.5
		lows[i] = math.Min(opens[i], closes[i]) - 1.5
		volumes[i] = 1_000_000 + float64(i*1000) + 5000*math.Sin(float64(i)/2)
	}
	return computeInput{
		opens:   opens,
		highs:   highs,
		lows:    lows,
		closes:  closes,
		volumes: volumes,
		fund: edgar.Fundamentals{
			Ticker: "SYNT", Shares: 1000, Revenue: 5000, NetIncome: 800, EPSDiluted: 4,
			Equity: 2000, GrossProfit: 2500, TotalAssets: 6000, TotalLiabilities: 4000,
			OperatingCashFlow: 1200, CapEx: 300, DividendsPaid: 100,
			RevenuePrior: 4000, NetIncomePrior: 500, AsOf: "2025-12-31",
		},
		hasFund: true,
		price:   40,
	}
}

// TestComputedClosuresNeverPanicOrFabricate runs EVERY registered closure over a
// synthetic-but-valid computeInput and over an empty one, asserting (a) no closure
// panics on either input and (b) any non-ok result carries no Value (the
// no-fabrication guarantee). It complements TestComputeNeverFabricates by exercising
// each closure directly (including the ok path on rich data) rather than only the
// empty-data path through StockIndicators.
func TestComputedClosuresNeverPanicOrFabricate(t *testing.T) {
	cat := MustLoad()
	c := NewComputer(cat, nil, nil, nil, nil)

	// Attach each registry id's catalog record so closures that read DefaultParams
	// see the real default_params, exactly as StockIndicators would.
	recByID := make(map[string]Indicator, cat.Len())
	for _, rec := range cat.All() {
		recByID[rec.ID] = rec
	}

	inputs := map[string]computeInput{
		"synthetic": syntheticInput(),
		"empty":     {},
	}

	for id, fn := range c.registry {
		rec := recByID[id] // zero Indicator if absent; TestRegistryIDsAreRealCatalogIDs guards that
		for label, in := range inputs {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("closure %s panicked on %s input: %v", id, label, r)
					}
				}()
				si := StockIndicator{Indicator: rec, Status: StatusInsufficient, Reason: "not computed"}
				fn(in, &si)
				if si.Status != StatusOK && si.Value != nil {
					t.Errorf("closure %s on %s input is %s but carries a value %v",
						id, label, si.Status, *si.Value)
				}
				if si.Status == StatusOK && si.Value == nil {
					t.Errorf("closure %s on %s input is ok but has no value", id, label)
				}
			}()
		}
	}
}
