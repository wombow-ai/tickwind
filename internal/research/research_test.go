package research

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wombow-ai/tickwind/internal/edgar"
	"github.com/wombow-ai/tickwind/internal/indicators"
	"github.com/wombow-ai/tickwind/internal/store"
)

// --- fakes ---

// fakeIndicators returns a fixed indicator result.
type fakeIndicators struct {
	res   indicators.StockIndicatorsResult
	calls int
}

func (f *fakeIndicators) StockIndicators(context.Context, string) indicators.StockIndicatorsResult {
	f.calls++
	return f.res
}

// fakeFund returns fixed fundamentals (or an error when err != nil).
type fakeFund struct {
	f   edgar.Fundamentals
	err error
}

func (f fakeFund) Fundamentals(context.Context, string) (edgar.Fundamentals, error) {
	if f.err != nil {
		return edgar.Fundamentals{}, f.err
	}
	return f.f, nil
}

// fakeQuote returns a fixed quote.
type fakeQuote struct {
	q  store.Quote
	ok bool
}

func (f fakeQuote) Quote(context.Context, string) (store.Quote, bool) { return f.q, f.ok }

// fakeEnricher is a controllable ResearchEnricher. When enabled, ComposeReport /
// ComposeDeepReport return prose (or err). It records the material it was handed
// and how many normal vs deep calls were made (so the deep routing can be
// asserted). deepProse, when set, is returned by ComposeDeepReport instead of
// prose.
type fakeEnricher struct {
	enabled   bool
	prose     map[string]string
	deepProse map[string]string
	err       error
	material  string
	calls     int
	deepCalls int
}

func (f *fakeEnricher) Enabled() bool { return f.enabled }

func (f *fakeEnricher) ComposeReport(_ context.Context, material, _ string) (map[string]string, error) {
	f.calls++
	f.material = material
	if f.err != nil {
		return nil, f.err
	}
	return f.prose, nil
}

func (f *fakeEnricher) ComposeDeepReport(_ context.Context, material, _ string) (map[string]string, error) {
	f.deepCalls++
	f.material = material
	if f.err != nil {
		return nil, f.err
	}
	if f.deepProse != nil {
		return f.deepProse, nil
	}
	return f.prose, nil
}

// --- builders ---

func ptr(v float64) *float64 { return &v }

// okIndicator builds an ok StockIndicator with a value and unit.
func okIndicator(id string, v float64, unit string) indicators.StockIndicator {
	return indicators.StockIndicator{
		Indicator: indicators.Indicator{ID: id},
		Status:    indicators.StatusOK,
		Value:     ptr(v),
		Unit:      unit,
	}
}

// insufficientIndicator builds an insufficient StockIndicator with a reason.
func insufficientIndicator(id, reason string) indicators.StockIndicator {
	return indicators.StockIndicator{
		Indicator: indicators.Indicator{ID: id},
		Status:    indicators.StatusInsufficient,
		Reason:    reason,
	}
}

// unsupportedIndicator builds an unsupported (crypto) StockIndicator.
func unsupportedIndicator(id string) indicators.StockIndicator {
	return indicators.StockIndicator{
		Indicator: indicators.Indicator{ID: id},
		Status:    indicators.StatusUnsupported,
		Reason:    "crypto-market data source; not applicable to US equities",
	}
}

// findFact returns the fact with key k in any section (and whether found).
func findFact(fs FactSheet, k string) (Fact, bool) {
	for _, sec := range fs.Sections {
		for _, f := range sec.Facts {
			if f.Key == k {
				return f, true
			}
		}
	}
	return Fact{}, false
}

// section returns the section with key k (and whether present).
func section(fs FactSheet, k string) (SectionFacts, bool) {
	for _, sec := range fs.Sections {
		if sec.Key == k {
			return sec, true
		}
	}
	return SectionFacts{}, false
}

// --- tests ---

// TestAssembleStatusGating asserts (a) an insufficient indicator yields
// Value=="数据不足" with NO Raw number + the verbatim reason, and a crypto/
// unsupported id never appears.
func TestAssembleStatusGating(t *testing.T) {
	ind := &fakeIndicators{res: indicators.StockIndicatorsResult{
		Ticker: "AAPL", AsOf: "2024-09-28",
		Indicators: []indicators.StockIndicator{
			okIndicator("fundamental.roe", 42.0, "%"),
			insufficientIndicator("fundamental.dy", "non-dividend payer or no price"),
			unsupportedIndicator("sentiment.fr"),
		},
	}}
	src := Sources{
		Indicators:   ind,
		Fundamentals: fakeFund{f: edgar.Fundamentals{Ticker: "AAPL", Revenue: 1e11, NetIncome: 9e10, EPSDiluted: 6.1, Shares: 15e9, Period: "FY2024", AsOf: "2024-09-28"}},
		Quote:        fakeQuote{q: store.Quote{Ticker: "AAPL", Price: 190.12, Source: "alpaca", Session: "regular"}, ok: true},
	}
	fs := Assemble(context.Background(), "aapl", "zh", src)

	if ind.calls != 1 {
		t.Fatalf("StockIndicators called %d times, want exactly 1", ind.calls)
	}

	dy, ok := findFact(fs, "dy")
	if !ok {
		t.Fatal("dy fact missing; insufficient facts must still be emitted")
	}
	if dy.Status != StatusInsufficient {
		t.Errorf("dy status = %q, want %q", dy.Status, StatusInsufficient)
	}
	if dy.Value != "数据不足" {
		t.Errorf("dy value = %q, want 数据不足", dy.Value)
	}
	if dy.Raw != nil {
		t.Errorf("dy Raw = %v, want nil (no number for an absent field)", *dy.Raw)
	}
	if dy.Reason != "non-dividend payer or no price" {
		t.Errorf("dy reason = %q, want verbatim indicator reason", dy.Reason)
	}

	// The crypto/unsupported id must be absent everywhere.
	for _, sec := range fs.Sections {
		for _, f := range sec.Facts {
			if f.Key == "sentiment.fr" || f.Key == "fr" {
				t.Errorf("unsupported crypto id leaked into section %q", sec.Key)
			}
		}
	}
}

// TestAssembleFCFFormatsAsDollars asserts FCF (indicator unit "" but DOLLARS)
// renders as compact USD, sign-aware, not as a percentage.
func TestAssembleFCFFormatsAsDollars(t *testing.T) {
	tests := []struct {
		name string
		raw  float64
		want string
	}{
		{"positive billions", 4.5e9, "$4.50B"},
		{"burn (negative)", -1.2e9, "-$1.20B"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ind := &fakeIndicators{res: indicators.StockIndicatorsResult{
				Indicators: []indicators.StockIndicator{
					okIndicator("fundamental.fcf", tc.raw, ""), // indicator reports unit ""
				},
			}}
			src := Sources{
				Indicators:   ind,
				Fundamentals: fakeFund{f: edgar.Fundamentals{Revenue: 1, Period: "FY2024"}},
			}
			fs := Assemble(context.Background(), "X", "zh", src)
			fcf, ok := findFact(fs, "fcf")
			if !ok {
				t.Fatal("fcf fact missing")
			}
			if fcf.Unit != unitUSD {
				t.Errorf("fcf unit = %q, want %q (dollars override, not the indicator's empty unit)", fcf.Unit, unitUSD)
			}
			if fcf.Value != tc.want {
				t.Errorf("fcf value = %q, want %q", fcf.Value, tc.want)
			}
		})
	}
}

// TestAssembleFundamentalsTTMFirst asserts the report's headline revenue / net
// income / diluted EPS use the TRAILING-TWELVE-MONTH figure when available (so
// they reconcile with the TTM-based P/E and match the free FundamentalsCard),
// carry a "(TTM)" label + a TTM as-of (NOT the fiscal year), and fall back to the
// latest FY value + FY as-of + plain label when no TTM is available.
func TestAssembleFundamentalsTTMFirst(t *testing.T) {
	t.Run("TTM available — used + disclosed", func(t *testing.T) {
		ind := &fakeIndicators{res: indicators.StockIndicatorsResult{
			Indicators: []indicators.StockIndicator{okIndicator("fundamental.pe-ttm", 25.5, "x")},
		}}
		src := Sources{
			Indicators: ind,
			Fundamentals: fakeFund{f: edgar.Fundamentals{
				Ticker: "MU", Period: "FY2025", AsOf: "2025-08-28", Shares: 1.1e9,
				Revenue: 37378000000, NetIncome: 8539000000, EPSDiluted: 7.59,
				RevenueTTM: 90274000000, NetIncomeTTM: 50469000000, EPSDilutedTTM: 44.24,
				TTMAsOf: "2026-05-28", LatestQuarter: "Q3 FY2026",
			}},
			Quote: fakeQuote{q: store.Quote{Ticker: "MU", Price: 1129.73, Source: "alpaca", Session: "regular"}, ok: true},
		}
		fs := Assemble(context.Background(), "MU", "en", src)
		for _, c := range []struct {
			key, wantLabel string
			wantRaw        float64
		}{
			{"revenue", "Revenue (TTM)", 90274000000},
			{"net_income", "Net Income (TTM)", 50469000000},
			{"eps_diluted", "Diluted EPS (TTM)", 44.24},
		} {
			f, ok := findFact(fs, c.key)
			if !ok {
				t.Fatalf("%s fact missing", c.key)
			}
			if f.Raw == nil || *f.Raw != c.wantRaw {
				t.Errorf("%s Raw = %v, want %v (TTM, not FY)", c.key, f.Raw, c.wantRaw)
			}
			if f.LabelEN != c.wantLabel {
				t.Errorf("%s LabelEN = %q, want %q", c.key, f.LabelEN, c.wantLabel)
			}
			if f.AsOf != "Q3 FY2026" {
				t.Errorf("%s AsOf = %q, want the TTM stamp Q3 FY2026 (not FY2025)", c.key, f.AsOf)
			}
		}
		// The headline EPS must now RECONCILE with the report's own P/E (TTM):
		// price / EPS ≈ the pe value, not 6× off.
		eps, _ := findFact(fs, "eps_diluted")
		if got := 1129.73 / *eps.Raw; got < 24 || got > 27 {
			t.Errorf("price/EPS(TTM) = %.1f, want ≈25.5 (reconciles with P/E TTM)", got)
		}
		// The P/E (TTM) fact's as-of must match the TTM EPS it divides by (not FY).
		if pe, ok := findFact(fs, "pe"); ok && pe.AsOf != "Q3 FY2026" {
			t.Errorf("pe AsOf = %q, want the TTM stamp Q3 FY2026 (matches its TTM EPS numerator)", pe.AsOf)
		}
	})

	t.Run("no TTM — FY fallback, plain label", func(t *testing.T) {
		ind := &fakeIndicators{res: indicators.StockIndicatorsResult{
			Indicators: []indicators.StockIndicator{okIndicator("fundamental.roe", 30, "%")},
		}}
		src := Sources{
			Indicators: ind,
			Fundamentals: fakeFund{f: edgar.Fundamentals{
				Ticker: "OLD", Period: "FY2025", AsOf: "2025-12-31", Shares: 1e9,
				Revenue: 1e10, NetIncome: 5e9, EPSDiluted: 5.0, // no *TTM fields
			}},
			Quote: fakeQuote{q: store.Quote{Ticker: "OLD", Price: 100, Source: "alpaca", Session: "regular"}, ok: true},
		}
		fs := Assemble(context.Background(), "OLD", "en", src)
		eps, ok := findFact(fs, "eps_diluted")
		if !ok {
			t.Fatal("eps_diluted fact missing")
		}
		if eps.Raw == nil || *eps.Raw != 5.0 {
			t.Errorf("eps Raw = %v, want the FY value 5.0", eps.Raw)
		}
		if eps.LabelEN != "Diluted EPS" {
			t.Errorf("eps LabelEN = %q, want the plain (non-TTM) label", eps.LabelEN)
		}
		if eps.AsOf != "FY2025" {
			t.Errorf("eps AsOf = %q, want FY2025 (the fiscal-year stamp)", eps.AsOf)
		}
		rev, _ := findFact(fs, "revenue")
		if rev.LabelEN != "Revenue" {
			t.Errorf("revenue LabelEN = %q, want plain Revenue (no TTM)", rev.LabelEN)
		}
	})
}

// TestAssemblePELossMaker asserts a loss-maker P/E renders "亏损" (never 0): both
// the insufficient path (the indicator's documented behaviour) and the defensive
// non-positive-value path.
func TestAssemblePELossMaker(t *testing.T) {
	tests := []struct {
		name string
		si   indicators.StockIndicator
	}{
		{"insufficient (loss)", insufficientIndicator("fundamental.pe-ttm", "non-positive EPS (loss) or no price")},
		{"non-positive value", okIndicator("fundamental.pe-ttm", -3, "x")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ind := &fakeIndicators{res: indicators.StockIndicatorsResult{
				Indicators: []indicators.StockIndicator{tc.si},
			}}
			// Provide a price + fundamentals so the valuation section has another
			// ok fact and is not omitted (so the pe fact survives to be checked).
			src := Sources{
				Indicators:   ind,
				Fundamentals: fakeFund{f: edgar.Fundamentals{Shares: 1e9, Revenue: 1, Period: "FY2024"}},
				Quote:        fakeQuote{q: store.Quote{Price: 10, Source: "alpaca", Session: "regular"}, ok: true},
			}
			fs := Assemble(context.Background(), "X", "zh", src)
			pe, ok := findFact(fs, "pe")
			if !ok {
				t.Fatal("pe fact missing")
			}
			if pe.Value != lossLabel("zh") {
				t.Errorf("pe value = %q, want %q (never 0)", pe.Value, lossLabel("zh"))
			}
			if pe.Value == "0" || pe.Value == "0.0x" || pe.Value == "0x" {
				t.Errorf("pe value rendered as zero: %q", pe.Value)
			}
		})
	}
}

// TestAssemblePEMissingFundamentalsNotLoss guards the anti-fabrication fix: an
// insufficient P/E caused by MISSING fundamentals (an ETF/ADR/foreign name with
// price bars but no SEC XBRL) must render "数据不足", NOT "亏损" — otherwise the
// valuation section (kept alive by the ok price fact) would assert a fabricated
// "loss-making" claim about a company we have no earnings data for.
func TestAssemblePEMissingFundamentalsNotLoss(t *testing.T) {
	ind := &fakeIndicators{res: indicators.StockIndicatorsResult{
		Indicators: []indicators.StockIndicator{
			insufficientIndicator("fundamental.pe-ttm", "no SEC fundamentals available"),
		},
	}}
	// A price keeps the valuation section alive (so the pe fact survives), but
	// there are no fundamentals.
	src := Sources{
		Indicators: ind,
		Quote:      fakeQuote{q: store.Quote{Price: 25, Source: "alpaca", Session: "regular"}, ok: true},
	}
	fs := Assemble(context.Background(), "SPY", "zh", src)
	pe, ok := findFact(fs, "pe")
	if !ok {
		t.Fatal("pe fact missing")
	}
	if pe.Value == lossLabel("zh") {
		t.Errorf("pe value = %q for a no-fundamentals ticker; must NOT assert a loss", pe.Value)
	}
	if pe.Value != insufficientLabel("zh") {
		t.Errorf("pe value = %q, want 数据不足", pe.Value)
	}
	if pe.Raw != nil {
		t.Errorf("pe Raw = %v, want nil (no number for an absent field)", *pe.Raw)
	}
}

// TestAssembleOmitsEmptySection asserts a section with zero ok facts is omitted.
func TestAssembleOmitsEmptySection(t *testing.T) {
	// Only an insufficient technical indicator and no fundamentals/quote → every
	// section ends up with no ok fact → no sections at all.
	ind := &fakeIndicators{res: indicators.StockIndicatorsResult{
		Indicators: []indicators.StockIndicator{
			insufficientIndicator("technical.rsi", "need more daily closes for RSI"),
		},
	}}
	src := Sources{Indicators: ind} // no fundamentals, no quote
	fs := Assemble(context.Background(), "X", "zh", src)

	if _, ok := section(fs, "technical"); ok {
		t.Error("technical section present despite zero ok facts; want omitted")
	}
	if _, ok := section(fs, "valuation"); ok {
		t.Error("valuation section present with no data; want omitted")
	}
	if _, ok := section(fs, "fundamentals"); ok {
		t.Error("fundamentals section present with no data; want omitted")
	}
	if len(fs.Sections) != 0 {
		t.Errorf("got %d sections, want 0", len(fs.Sections))
	}
}

// TestAssembleMarketCapAndPrice asserts MarketCap = price × Shares and the price
// fact + label come from the quote (de-dup: market cap is never from the
// indicator set).
func TestAssembleMarketCapAndPrice(t *testing.T) {
	ind := &fakeIndicators{res: indicators.StockIndicatorsResult{
		Indicators: []indicators.StockIndicator{okIndicator("fundamental.pe-ttm", 31.2, "x")},
	}}
	src := Sources{
		Indicators:   ind,
		Fundamentals: fakeFund{f: edgar.Fundamentals{Shares: 15_000_000_000, Revenue: 1, Period: "FY2024"}},
		Quote:        fakeQuote{q: store.Quote{Price: 190.12, Source: "alpaca", Session: "regular"}, ok: true},
	}
	fs := Assemble(context.Background(), "AAPL", "zh", src)

	mc, ok := findFact(fs, "market_cap")
	if !ok {
		t.Fatal("market_cap fact missing")
	}
	wantMC := 190.12 * 15_000_000_000
	if mc.Raw == nil || *mc.Raw != wantMC {
		t.Errorf("market_cap raw = %v, want %v (price × shares)", mc.Raw, wantMC)
	}
	if mc.Value != "$2.85T" {
		t.Errorf("market_cap value = %q, want $2.85T", mc.Value)
	}
	if fs.PriceLabel != "$190.12 · alpaca · delayed · regular" {
		t.Errorf("price label = %q", fs.PriceLabel)
	}
}

// TestComposeDisabledIsDataOnly asserts Compose with a disabled enricher returns
// prose=="" everywhere and Facts identical to the data-only Assemble output.
func TestComposeDisabledIsDataOnly(t *testing.T) {
	src := Sources{
		Indicators: &fakeIndicators{res: indicators.StockIndicatorsResult{
			Indicators: []indicators.StockIndicator{okIndicator("technical.rsi", 56.3, "")},
		}},
	}
	data := Assemble(context.Background(), "X", "zh", src)

	enr := &fakeEnricher{enabled: false, prose: map[string]string{"technical": "should never appear"}}
	composed := Compose(context.Background(), Assemble(context.Background(), "X", "zh", src), enr, "zh")

	if enr.calls != 0 {
		t.Errorf("ComposeReport called %d times on a disabled enricher, want 0", enr.calls)
	}
	for _, sec := range composed.Sections {
		if sec.Prose != "" {
			t.Errorf("section %q prose = %q, want \"\" (data-only)", sec.Key, sec.Prose)
		}
	}
	assertSameFacts(t, data, composed)
}

// TestComposeErrorIsDataOnly asserts a ComposeReport error degrades to the
// unchanged data-only sheet (no error, prose stays "").
func TestComposeErrorIsDataOnly(t *testing.T) {
	src := Sources{
		Indicators: &fakeIndicators{res: indicators.StockIndicatorsResult{
			Indicators: []indicators.StockIndicator{okIndicator("technical.rsi", 56.3, "")},
		}},
	}
	enr := &fakeEnricher{enabled: true, err: errors.New("provider down")}
	composed := Compose(context.Background(), Assemble(context.Background(), "X", "zh", src), enr, "zh")

	if enr.calls != 1 {
		t.Errorf("ComposeReport called %d times, want 1", enr.calls)
	}
	for _, sec := range composed.Sections {
		if sec.Prose != "" {
			t.Errorf("section %q prose = %q after error, want \"\"", sec.Key, sec.Prose)
		}
	}
}

// TestComposeOverviewPrepended asserts the LLM's "overview" synthesis becomes a
// prose-only section rendered FIRST (prepended), while a disabled enricher (the
// data-only report) gets no overview at all.
func TestComposeOverviewPrepended(t *testing.T) {
	src := Sources{
		Indicators: &fakeIndicators{res: indicators.StockIndicatorsResult{
			Indicators: []indicators.StockIndicator{okIndicator("technical.rsi", 56.3, "")},
		}},
	}
	// Enabled: an "overview" key (not a section in the material) is prepended as a
	// prose-only section; the real "technical" section gets its prose too.
	enr := &fakeEnricher{enabled: true, prose: map[string]string{
		"overview":  "综合来看,基本面稳健但情绪偏谨慎。以上为基于公开数据的客观梳理,非投资建议。",
		"technical": "RSI 处于中性区间。",
	}}
	composed := Compose(context.Background(), Assemble(context.Background(), "X", "zh", src), enr, "zh")

	var keys []string
	for _, s := range composed.Sections {
		keys = append(keys, s.Key)
	}
	if len(composed.Sections) == 0 || composed.Sections[0].Key != overviewKey {
		t.Fatalf("section order = %v, want overview first", keys)
	}
	ov := composed.Sections[0]
	if ov.Prose == "" || len(ov.Facts) != 0 {
		t.Errorf("overview section = %+v, want prose-only (no facts)", ov)
	}
	if tech, ok := section(composed, "technical"); !ok || tech.Prose == "" {
		t.Error("technical section missing its prose")
	}

	// Disabled → data-only → NO overview section.
	off := &fakeEnricher{enabled: false, prose: map[string]string{"overview": "should never appear"}}
	dataOnly := Compose(context.Background(), Assemble(context.Background(), "X", "zh", src), off, "zh")
	if _, ok := section(dataOnly, overviewKey); ok {
		t.Error("overview section present on a disabled (data-only) report; want none")
	}
}

// TestComposeBullBear asserts the composer parses the model's newline-joined
// bull/bear strings into clean points on the overview: list markers are stripped, a
// bare descriptive "买入" (insider buy) survives, and any point that trips the
// deterministic advice/target guard is dropped.
func TestComposeBullBear(t *testing.T) {
	src := Sources{
		Indicators: &fakeIndicators{res: indicators.StockIndicatorsResult{
			Indicators: []indicators.StockIndicator{okIndicator("technical.rsi", 56.3, "")},
		}},
	}
	enr := &fakeEnricher{enabled: true, prose: map[string]string{
		"overview": "综合来看,基本面稳健。以上为基于公开数据的客观梳理,非投资建议。",
		"bull":     "- 营收同比增长,显示需求稳健\n• 内部人近期买入,内部信心较强\n3. 目标价 $250,强烈推荐买入",
		"bear":     "估值处于历史高位\n做空占比上升",
	}}
	composed := Compose(context.Background(), Assemble(context.Background(), "X", "zh", src), enr, "zh")

	ov := composed.Sections[0]
	if ov.Key != overviewKey {
		t.Fatalf("first section = %q, want overview", ov.Key)
	}
	// Two bull points kept (markers stripped); the "目标价/强烈推荐" point dropped.
	if len(ov.Bull) != 2 {
		t.Fatalf("bull = %v, want 2 points (advice/target dropped)", ov.Bull)
	}
	if ov.Bull[0] != "营收同比增长,显示需求稳健" || ov.Bull[1] != "内部人近期买入,内部信心较强" {
		t.Errorf("bull points not cleaned: %v", ov.Bull)
	}
	for _, p := range ov.Bull {
		if hasAdvice(p) {
			t.Errorf("advice point survived the guard: %q", p)
		}
	}
	if len(ov.Bear) != 2 {
		t.Fatalf("bear = %v, want 2 points", ov.Bear)
	}

	// Disabled → no overview, hence no bull/bear.
	off := &fakeEnricher{enabled: false, prose: map[string]string{"bull": "x", "bear": "y"}}
	dataOnly := Compose(context.Background(), Assemble(context.Background(), "X", "zh", src), off, "zh")
	if _, ok := section(dataOnly, overviewKey); ok {
		t.Error("data-only report carries an overview/bull-bear; want none")
	}
}

// TestHasAdviceAnalystTargets locks in the analystRe additions: third-party analyst
// price-targets / ratings / quantified upside-downside are flagged as advice (so the
// chat web-search scrub and the final-prose backstop drop them), while factual prose
// that merely contains "target"/a number/"rating" is NOT mis-flagged.
func TestHasAdviceAnalystTargets(t *testing.T) {
	advice := []string{
		"Morgan Stanley raised its target to $250",     // bare target + price
		"Bank of America cut its price target to $180", // "price target" phrase
		"Goldman maintains a $300 target on the stock", // "$300 target"
		"analysts see ~30% upside from here",           // quantified upside
		"the stock has 15% downside to fair value",     // downside + "fair value" phrase
		"Goldman Sachs initiated with a Buy rating",    // analyst rating
		"downgraded to an Underweight rating",          // analyst rating
		"目标价 $250,上涨空间可观",                              // ZH target + upside room
		"给予买入评级",                                       // ZH analyst rating
	}
	for _, p := range advice {
		if !hasAdvice(p) {
			t.Errorf("hasAdvice(%q) = false, want true (analyst advice)", p)
		}
	}
	// Factual prose that must NOT trip the (high-precision) analyst patterns.
	factual := []string{
		"its total addressable market target is $5 billion", // "target ... market", not a price target
		"revenue grew 30% year over year",                   // a growth %, not "% upside/downside"
		"the company holds a strong credit rating",          // "credit rating", not an analyst call
		"an insider bought 1,000 shares at $50",             // past-tense insider fact
		"shares reached $400 in regular trading",            // factual print, not "to reach $400"
	}
	for _, p := range factual {
		if hasAdvice(p) {
			t.Errorf("hasAdvice(%q) = true, want false (factual prose mis-flagged)", p)
		}
	}
}

// TestHasAdviceSoftAdvice locks the Explore-mode firewall hardening: the soft "...therefore
// it's a good position" verdicts the keyword net used to miss are now caught, while the deep
// two-sided ANALYSIS that Explore mode is meant to produce still SURVIVES (so the relaxation
// can't be undone by over-blocking).
func TestHasAdviceSoftAdvice(t *testing.T) {
	// Aggregate desirability verdicts on the security — must be CAUGHT.
	advice := []string{
		"on balance this looks like a compelling entry",
		"the setup looks attractive here",
		"the risk/reward skews favorably from here",
		"it's the best opportunity in the sector",
		"a high-conviction top pick",
		"worth owning at these levels",
		"poised to outperform the market",
		"the stock looks cheap on these numbers",
		"limited downside with asymmetric upside",
		"投资者可以建仓",
		"目前性价比很高,很有吸引力",
		"是该板块的最佳机会,有望跑赢",
		"建议逢低布局,这是个好的买点",
		"上行空间大于下行,值得拥有",
		"the chart is begging to break out",
		"the path of least resistance is higher",
		"it checks every box for the bulls",
		"数据会说话,目前上涨是大概率",
	}
	for _, p := range advice {
		if !hasAdvice(p) {
			t.Errorf("hasAdvice(%q) = false, want true (soft advice must be caught)", p)
		}
	}
	// Deep two-sided ANALYSIS — descriptive, never a recommendation — must SURVIVE.
	compliant := []string{
		"the bear case is the more compelling of the two readings",      // bare 'compelling', not 'compelling entry'
		"dividend yield is attractive vs the sector at the disclosed %", // bare 'attractive', not 'looks attractive'
		"ranks in the 91st percentile for quality",
		"gross margin expanded to 46.9% year over year",
		"catalysts to watch: the Q3 earnings print and the next 10-Q",  // 'to watch', not 'worth watching'
		"the next earnings print is the key event to watch this month", // ditto
		"if margins hold at the disclosed 46.9%, the quality percentile stays top-decile",
		"bull case: revenue accelerating; bear case: 20th-percentile value",
		"an insider initiated a new stake last quarter, per Form 4", // past-tense position FACT
		"RSI sits at 72, overbought by the standard threshold",
		"该催化剂值得关注,临近财报",                                              // '值得关注' is NOT in the list (catalyst-level, legit)
		"从风险收益比的角度,看多看空各有支撑",                                         // bare '风险收益比' concept is NOT in the list
		"the coverage ratio leaves limited downside to the dividend", // 'limited downside' is anchored → coverage fact survives
		"best-in-class operating margins of 46.9%",                   // 'best-in-class' ≠ 'best opportunity'
	}
	for _, p := range compliant {
		if hasAdvice(p) {
			t.Errorf("hasAdvice(%q) = true, want false (compliant two-sided analysis over-stripped)", p)
		}
	}
}

// TestComposeScrubsAdviceFromProse asserts the deterministic advice backstop now also
// covers the report BODY (section prose + overview), not just bull/bear points: an advice
// line in the overview is dropped (clean lines kept), and an all-advice section paragraph
// degrades to its data-only facts (Prose == "").
func TestComposeScrubsAdviceFromProse(t *testing.T) {
	src := Sources{
		Indicators: &fakeIndicators{res: indicators.StockIndicatorsResult{
			Indicators: []indicators.StockIndicator{okIndicator("technical.rsi", 56.3, "")},
		}},
	}
	enr := &fakeEnricher{enabled: true, prose: map[string]string{
		"overview":  "基本面稳健,营收同比增长。\n目标价 $250,强烈推荐买入。",
		"technical": "RSI 56.3 处于中性区间。建议逢低买入,fair value $300。",
	}}
	composed := Compose(context.Background(), Assemble(context.Background(), "X", "zh", src), enr, "zh")

	ov, ok := section(composed, overviewKey)
	if !ok {
		t.Fatal("overview missing")
	}
	if hasAdvice(ov.Prose) || strings.Contains(ov.Prose, "目标价") {
		t.Errorf("advice leaked into overview prose: %q", ov.Prose)
	}
	if !strings.Contains(ov.Prose, "营收同比增长") {
		t.Errorf("clean overview line was dropped: %q", ov.Prose)
	}
	tech, ok := section(composed, "technical")
	if !ok {
		t.Fatal("technical section missing")
	}
	if tech.Prose != "" {
		t.Errorf("all-advice section prose should degrade to data-only, got: %q", tech.Prose)
	}
}

// TestComposeNeverMutatesNumbers asserts the LLM map only touches Prose — every
// Fact.Value and Fact.Raw is byte-for-byte identical before and after Compose,
// even when the model returns prose AND a stray bogus number-looking key.
func TestComposeNeverMutatesNumbers(t *testing.T) {
	src := Sources{
		Indicators: &fakeIndicators{res: indicators.StockIndicatorsResult{
			Indicators: []indicators.StockIndicator{
				okIndicator("technical.rsi", 56.3, ""),
				okIndicator("fundamental.fcf", 4.5e9, ""),
			},
		}},
		Fundamentals: fakeFund{f: edgar.Fundamentals{Revenue: 1e11, NetIncome: 9e10, EPSDiluted: 6.1, Shares: 15e9, Period: "FY2024"}},
		Quote:        fakeQuote{q: store.Quote{Price: 190.12, Source: "alpaca", Session: "regular"}, ok: true},
	}
	data := Assemble(context.Background(), "X", "zh", src)

	// The model "tries" to overwrite numbers via extra keys — Compose must ignore
	// everything except matching section-key prose.
	enr := &fakeEnricher{enabled: true, prose: map[string]string{
		"technical":    "动量中性。",
		"fundamentals": "营收增长,现金生成。",
		"market_cap":   "9999999",    // not a section key → ignored
		"fcf":          "0% (bogus)", // not a section key → ignored
	}}
	composed := Compose(context.Background(), Assemble(context.Background(), "X", "zh", src), enr, "zh")

	// Prose was filled for matching keys.
	if sec, ok := section(composed, "technical"); !ok || sec.Prose != "动量中性。" {
		t.Errorf("technical prose = %q, want filled", sec.Prose)
	}
	// Numbers untouched.
	assertSameFacts(t, data, composed)
}

// TestComposeDeepReportNeverMutatesNumbers is the deep-path twin of
// TestComposeNeverMutatesNumbers: the richer ComposeDeepReport compose must obey
// the SAME anti-hallucination contract — it only ever fills Prose, every
// Fact.Value/Raw is byte-for-byte identical before and after, stray numeric keys
// in the reply are ignored, and the LLM never sees a raw number in the material.
func TestComposeDeepReportNeverMutatesNumbers(t *testing.T) {
	src := Sources{
		Indicators: &fakeIndicators{res: indicators.StockIndicatorsResult{
			Indicators: []indicators.StockIndicator{
				okIndicator("technical.rsi", 56.3, ""),
				okIndicator("fundamental.fcf", 4.5e9, ""),
			},
		}},
		Fundamentals: fakeFund{f: edgar.Fundamentals{Revenue: 1e11, NetIncome: 9e10, EPSDiluted: 6.1, Shares: 15e9, Period: "FY2024"}},
		Quote:        fakeQuote{q: store.Quote{Price: 190.12, Source: "alpaca", Session: "regular"}, ok: true},
	}
	data := Assemble(context.Background(), "X", "zh", src)

	// The deep model returns richer prose AND tries to inject numbers via stray
	// keys — ComposeDeep must ignore everything except matching section-key prose.
	enr := &fakeEnricher{enabled: true, deepProse: map[string]string{
		"technical":    "动量信号中性,RSI 处于其历史区间的中段,未显示超买或超卖。",
		"fundamentals": "据最新年报,营收规模可观且现金生成稳健,但需注意这是滞后的年度数据。",
		"overview":     "综合来看,基本面稳健而估值需结合行业背景看待。以上为基于公开数据的客观梳理,非投资建议。",
		"market_cap":   "9999999",    // not a section key → ignored
		"fcf":          "0% (bogus)", // not a section key → ignored
		"pe":           "重算的市盈率 12x", // a number the model tried to assert → ignored
	}}
	composed := ComposeDeep(context.Background(), Assemble(context.Background(), "X", "zh", src), enr, "zh")

	if enr.deepCalls != 1 || enr.calls != 0 {
		t.Fatalf("deepCalls=%d calls=%d; want ComposeDeep to use ComposeDeepReport exactly once", enr.deepCalls, enr.calls)
	}
	// Prose was filled for matching section keys.
	if sec, ok := section(composed, "technical"); !ok || sec.Prose == "" {
		t.Errorf("technical prose = %q, want the richer deep prose", sec.Prose)
	}
	// The deep compose adds the same overview-synthesis section the normal path does.
	if _, ok := section(composed, overviewKey); !ok {
		t.Error("deep compose produced no overview section; want one when the model returns an overview")
	}
	// The material handed to the LLM carries only formatted strings — never a raw
	// number. Assert the raw FCF (4500000000) and the raw RSI (56.3) do NOT appear
	// verbatim; only their formatted forms ($4.50B / 56.3) are present.
	if strings.Contains(enr.material, "4500000000") {
		t.Errorf("material leaked a raw number (4500000000): %q", enr.material)
	}
	// Numbers untouched: every Fact.Value/Raw/Unit/Status is identical to data-only.
	assertSameFacts(t, data, composed)
}

// assertSameFacts checks every Fact's Key/Value/Raw/Unit/Status matches between
// two sheets (order-independent by key) — the anti-hallucination invariant.
func assertSameFacts(t *testing.T, want, got FactSheet) {
	t.Helper()
	index := func(fs FactSheet) map[string]Fact {
		m := map[string]Fact{}
		for _, sec := range fs.Sections {
			for _, f := range sec.Facts {
				m[sec.Key+"/"+f.Key] = f
			}
		}
		return m
	}
	wi, gi := index(want), index(got)
	if len(wi) != len(gi) {
		t.Fatalf("fact count differs: data-only %d, composed %d", len(wi), len(gi))
	}
	for k, wf := range wi {
		gf, ok := gi[k]
		if !ok {
			t.Errorf("fact %q missing after compose", k)
			continue
		}
		if wf.Value != gf.Value {
			t.Errorf("fact %q value mutated: %q → %q", k, wf.Value, gf.Value)
		}
		if wf.Unit != gf.Unit {
			t.Errorf("fact %q unit mutated: %q → %q", k, wf.Unit, gf.Unit)
		}
		if wf.Status != gf.Status {
			t.Errorf("fact %q status mutated: %q → %q", k, wf.Status, gf.Status)
		}
		switch {
		case wf.Raw == nil && gf.Raw != nil:
			t.Errorf("fact %q raw appeared: %v", k, *gf.Raw)
		case wf.Raw != nil && gf.Raw == nil:
			t.Errorf("fact %q raw vanished", k)
		case wf.Raw != nil && gf.Raw != nil && *wf.Raw != *gf.Raw:
			t.Errorf("fact %q raw mutated: %v → %v", k, *wf.Raw, *gf.Raw)
		}
	}
}

// TestFormatValue covers the unit-aware formatters incl. nil → em-dash.
func TestFormatValue(t *testing.T) {
	tests := []struct {
		name string
		raw  *float64
		unit string
		want string
	}{
		{"nil → dash", nil, unitUSD, dash},
		{"percent", ptr(42.16), unitPercent, "42.2%"},
		{"negative percent", ptr(-3.2), unitPercent, "-3.2%"},
		{"multiple", ptr(41.23), unitMult, "41.2x"},
		{"price over 10", ptr(190.125), unitPrice, "$190.12"},
		{"price under 10", ptr(4.567), unitPrice, "$4.567"},
		{"price under 1", ptr(0.4567), unitPrice, "$0.4567"},
		{"usd trillions", ptr(4.51e12), unitUSD, "$4.51T"},
		{"usd billions", ptr(98.8e9), unitUSD, "$98.80B"},
		{"usd negative", ptr(-1.2e9), unitUSD, "-$1.20B"},
		{"plain int (volume)", ptr(1234567), unitNone, "1234567"},
		{"plain decimal (rsi)", ptr(56.34), unitNone, "56.34"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatValue(tc.raw, tc.unit); got != tc.want {
				t.Errorf("formatValue(%v, %q) = %q, want %q", tc.raw, tc.unit, got, tc.want)
			}
		})
	}
}

// TestServiceDataOnlyEqualsReport asserts the Service plumbs Assemble/Compose and
// reports Enabled/Model correctly when disabled.
func TestServiceDataOnly(t *testing.T) {
	src := Sources{
		Indicators: &fakeIndicators{res: indicators.StockIndicatorsResult{
			Indicators: []indicators.StockIndicator{okIndicator("technical.rsi", 56.3, "")},
		}},
	}
	svc := NewService(src, &fakeEnricher{enabled: false}, "deepseek-chat", "")
	if svc.Enabled() {
		t.Error("Enabled() = true, want false for a disabled enricher")
	}
	if svc.Model() != "" {
		t.Errorf("Model() = %q, want \"\" when disabled", svc.Model())
	}
	if svc.DeepModel() != "" {
		t.Errorf("DeepModel() = %q, want \"\" when disabled", svc.DeepModel())
	}
	rep := svc.Report(context.Background(), "X", "zh")
	composed := svc.Compose(context.Background(), rep, "zh")
	assertSameFacts(t, rep, composed)
	deepComposed := svc.ComposeDeep(context.Background(), rep, "zh")
	assertSameFacts(t, rep, deepComposed)
}

// TestServiceDeepModelFallback asserts an empty deepModel falls back to the normal
// model (and to "" when the LLM is disabled), so depth=deep costs/behaves exactly
// like the normal path until LLM_DEEP_MODEL is set.
func TestServiceDeepModelFallback(t *testing.T) {
	src := Sources{Indicators: &fakeIndicators{}}
	// Empty deepModel → falls back to the normal model when enabled.
	svc := NewService(src, &fakeEnricher{enabled: true}, "deepseek-chat", "")
	if got := svc.DeepModel(); got != "deepseek-chat" {
		t.Errorf("DeepModel() = %q, want the fallback to the normal model", got)
	}
	// An explicit deepModel is surfaced as-is.
	svc2 := NewService(src, &fakeEnricher{enabled: true}, "deepseek-chat", "anthropic/claude-opus")
	if got := svc2.DeepModel(); got != "anthropic/claude-opus" {
		t.Errorf("DeepModel() = %q, want the configured deep model", got)
	}
}
