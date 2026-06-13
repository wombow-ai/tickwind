package research

import (
	"context"
	"errors"
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

// fakeEnricher is a controllable ResearchEnricher. When enabled, ComposeReport
// returns prose (or err). It records the material it was handed.
type fakeEnricher struct {
	enabled  bool
	prose    map[string]string
	err      error
	material string
	calls    int
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
	fs := Assemble(context.Background(), "aapl", src)

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
			fs := Assemble(context.Background(), "X", src)
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
			fs := Assemble(context.Background(), "X", src)
			pe, ok := findFact(fs, "pe")
			if !ok {
				t.Fatal("pe fact missing")
			}
			if pe.Value != loss {
				t.Errorf("pe value = %q, want %q (never 0)", pe.Value, loss)
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
	fs := Assemble(context.Background(), "SPY", src)
	pe, ok := findFact(fs, "pe")
	if !ok {
		t.Fatal("pe fact missing")
	}
	if pe.Value == loss {
		t.Errorf("pe value = %q for a no-fundamentals ticker; must NOT assert a loss", pe.Value)
	}
	if pe.Value != "数据不足" {
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
	fs := Assemble(context.Background(), "X", src)

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
	fs := Assemble(context.Background(), "AAPL", src)

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
	data := Assemble(context.Background(), "X", src)

	enr := &fakeEnricher{enabled: false, prose: map[string]string{"technical": "should never appear"}}
	composed := Compose(context.Background(), Assemble(context.Background(), "X", src), enr, "zh")

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
	composed := Compose(context.Background(), Assemble(context.Background(), "X", src), enr, "zh")

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
	composed := Compose(context.Background(), Assemble(context.Background(), "X", src), enr, "zh")

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
	dataOnly := Compose(context.Background(), Assemble(context.Background(), "X", src), off, "zh")
	if _, ok := section(dataOnly, overviewKey); ok {
		t.Error("overview section present on a disabled (data-only) report; want none")
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
	data := Assemble(context.Background(), "X", src)

	// The model "tries" to overwrite numbers via extra keys — Compose must ignore
	// everything except matching section-key prose.
	enr := &fakeEnricher{enabled: true, prose: map[string]string{
		"technical":    "动量中性。",
		"fundamentals": "营收增长,现金生成。",
		"market_cap":   "9999999",    // not a section key → ignored
		"fcf":          "0% (bogus)", // not a section key → ignored
	}}
	composed := Compose(context.Background(), Assemble(context.Background(), "X", src), enr, "zh")

	// Prose was filled for matching keys.
	if sec, ok := section(composed, "technical"); !ok || sec.Prose != "动量中性。" {
		t.Errorf("technical prose = %q, want filled", sec.Prose)
	}
	// Numbers untouched.
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
	svc := NewService(src, &fakeEnricher{enabled: false}, "deepseek-chat")
	if svc.Enabled() {
		t.Error("Enabled() = true, want false for a disabled enricher")
	}
	if svc.Model() != "" {
		t.Errorf("Model() = %q, want \"\" when disabled", svc.Model())
	}
	rep := svc.Report(context.Background(), "X")
	composed := svc.Compose(context.Background(), rep, "zh")
	assertSameFacts(t, rep, composed)
}
