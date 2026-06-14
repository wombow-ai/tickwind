package indicators

import (
	"testing"

	"github.com/wombow-ai/tickwind/internal/edgar"
)

// inc2Fixture is a fully-populated, profitable fixture exercising every Increment-2
// ratio's ok path. Hand-derived intermediates (price=40, Shares=1000 → market cap
// 40000):
//
//	EBIT = OperatingIncomeLoss = 1000 (reported, preferred over the NI+int+tax derivation)
//	EBITDA = 1000 + 200 (D&A) = 1200
//	pre-tax income = 1100 (reported)
//	interest-bearing debt = LongTermDebt 1500 + DebtCurrent 500 = 2000
//	tangible equity = Equity 2000 − Goodwill 400 − Intangibles 100 = 1500
//	working capital = AssetsCurrent 3000 − LiabilitiesCurrent 1500 = 1500
//	EV = 40000 + 2000 − Cash 600 = 41400
//	FCF = OCF 1200 − CapEx 300 = 900
var inc2Fixture = edgar.Fundamentals{
	Shares: 1000, Revenue: 5000, NetIncome: 800, EPSDiluted: 4, Equity: 2000,
	GrossProfit: 2500, TotalAssets: 6000, TotalLiabilities: 4000,
	OperatingCashFlow: 1200, CapEx: 300, DividendsPaid: 100,
	RevenuePrior: 4000, NetIncomePrior: 500,
	// Group 1.
	OperatingIncomeLoss: 1000, InterestExpense: 200, IncomeTaxExpense: 300,
	DepreciationAmort: 200, PreTaxIncome: 1100, OperatingIncomeLossPrior: 800,
	// Group 2.
	CostOfRevenue: 2500, AssetsCurrent: 3000, LiabilitiesCurrent: 1500,
	InventoryNet: 500, CashAndEquivalents: 600, AccountsReceivable: 1000,
	AccountsPayable: 625, PropertyPlantNet: 2000,
	// Group 4.
	LongTermDebt: 1500, DebtCurrent: 500, Goodwill: 400, IntangiblesExGoodwill: 100,
	BuybackAmount: 800, ResearchDevelopment: 250,
	// Group 3.
	EPSBasic: 4.2, EPSDilutedPrior: 3.2, GrossProfitPrior: 2000,
	EquityPrior: 1600, TotalAssetsPrior: 5000,
}

const inc2Price = 40.0

// TestInc2Ratios checks every Increment-2 pure ratio against a hand-computed value
// over inc2Fixture, in the unit the compute layer assigns. The numbers are derived
// by hand in the want expression so the formula is auditable.
func TestInc2Ratios(t *testing.T) {
	f := inc2Fixture
	tests := []struct {
		name string
		eval func() (float64, bool)
		want float64
	}{
		// Group 1.
		{"opm%", func() (float64, bool) { return opm(f) }, 1000.0 / 5000 * 100},                     // 20%
		{"ebit", func() (float64, bool) { return ebit(f) }, 1000},                                   // reported op income
		{"ebit-margin%", func() (float64, bool) { return ebitMargin(f) }, 1000.0 / 5000 * 100},      // 20%
		{"pre-tax-margin%", func() (float64, bool) { return preTaxMargin(f) }, 1100.0 / 5000 * 100}, // 22%
		{"ebitda", func() (float64, bool) { return ebitda(f) }, 1200},
		{"ebitda-margin%", func() (float64, bool) { return ebitdaMargin(f) }, 1200.0 / 5000 * 100}, // 24%
		{"icr-tie", func() (float64, bool) { return icrTIE(f) }, 1000.0 / 200},                     // 5x
		// ROCE = EBIT / (assets − current liab) = 1000 / (6000 − 1500) = 22.222…%
		{"roce%", func() (float64, bool) { return roce(f) }, 1000.0 / 4500 * 100},
		// ROIC: taxRate = 300/1100 = 0.27272…; NOPAT = 1000·(1−0.27272…) = 727.27…
		// invested = 2000 + 2000 − 600 = 3400 → ROIC = 727.27…/3400·100 = 21.390…%
		{"roic%", func() (float64, bool) { return roic(f) }, 1000.0 * (1 - 300.0/1100.0) / 3400 * 100},
		{"op-growth%", func() (float64, bool) { return opGrowth(f) }, (1000.0 - 800) / 800 * 100}, // 25%

		// Group 2.
		{"current-ratio", func() (float64, bool) { return currentRatio(f) }, 3000.0 / 1500},               // 2x
		{"quick-ratio", func() (float64, bool) { return quickRatio(f) }, (3000.0 - 500) / 1500},           // 1.666…x
		{"cash-ratio", func() (float64, bool) { return cashRatio(f) }, 600.0 / 1500},                      // 0.4x
		{"inventory-turnover", func() (float64, bool) { return inventoryTurnover(f) }, 2500.0 / 500},      // 5x
		{"dio", func() (float64, bool) { return dio(f) }, 365.0 / (2500.0 / 500)},                         // 73 days
		{"receivables-turnover", func() (float64, bool) { return receivablesTurnover(f) }, 5000.0 / 1000}, // 5x
		{"dso", func() (float64, bool) { return dso(f) }, 365.0 / (5000.0 / 1000)},                        // 73 days
		{"payables-turnover", func() (float64, bool) { return payablesTurnover(f) }, 2500.0 / 625},        // 4x
		{"dpo", func() (float64, bool) { return dpoDays(f) }, 365.0 / (2500.0 / 625)},                     // 91.25 days
		// CCC = 73 + 73 − 91.25 = 54.75
		{"ccc", func() (float64, bool) { return ccc(f) }, 73 + 73 - 91.25},
		{"fixed-asset-turnover", func() (float64, bool) { return fixedAssetTurnover(f) }, 5000.0 / 2000},     // 2.5x
		{"current-asset-turnover", func() (float64, bool) { return currentAssetTurnover(f) }, 5000.0 / 3000}, // 1.666…x
		{"wc-turnover", func() (float64, bool) { return wcTurnover(f) }, 5000.0 / 1500},                      // 3.333…x
		{"ocf-ratio", func() (float64, bool) { return ocfRatio(f) }, 1200.0 / 1500},                          // 0.8x

		// Group 3.
		{"eps-growth%", func() (float64, bool) { return epsGrowth(f) }, (4.0 - 3.2) / 3.2 * 100},            // 25%
		{"equity-growth%", func() (float64, bool) { return equityGrowth(f) }, (2000.0 - 1600) / 1600 * 100}, // 25%
		{"asset-growth%", func() (float64, bool) { return assetGrowth(f) }, (6000.0 - 5000) / 5000 * 100},   // 20%
		{"gp-growth%", func() (float64, bool) { return gpGrowth(f) }, (2500.0 - 2000) / 2000 * 100},         // 25%
		{"eps-basic", func() (float64, bool) { return epsBasic(f) }, 4.2},

		// Group 4.
		{"lt-debt-ratio%", func() (float64, bool) { return ltDebtRatio(f) }, 1500.0 / (1500 + 2000) * 100}, // 42.857…%
		// net gearing = (2000 − 600) / 2000 = 70%
		{"net-gearing%", func() (float64, bool) { return netGearing(f) }, (2000.0 - 600) / 2000 * 100},
		{"cash-st-debt", func() (float64, bool) { return cashToSTDebt(f) }, 600.0 / 500},                      // 1.2x
		{"dtnw", func() (float64, bool) { return dtnw(f) }, 4000.0 / 1500},                                    // 2.666…x
		{"goodwill-equity%", func() (float64, bool) { return goodwillToEquity(f) }, 400.0 / 2000 * 100},       // 20%
		{"tbv (tangible PB)", func() (float64, bool) { return tangiblePB(inc2Price, f) }, 40000.0 / 1500},     // 26.666…x
		{"ev", func() (float64, bool) { return ev(inc2Price, f) }, 40000 + 2000 - 600},                        // 41400
		{"ev-sales", func() (float64, bool) { return evToSales(inc2Price, f) }, 41400.0 / 5000},               // 8.28x
		{"ev-fcf", func() (float64, bool) { return evToFCF(inc2Price, f) }, 41400.0 / 900},                    // 46x
		{"ev-ebitda", func() (float64, bool) { return evToEBITDA(inc2Price, f) }, 41400.0 / 1200},             // 34.5x
		{"r-d-intensity%", func() (float64, bool) { return rdIntensity(f) }, 250.0 / 5000 * 100},              // 5%
		{"buyback-yield%", func() (float64, bool) { return buybackYield(inc2Price, f) }, 800.0 / 40000 * 100}, // 2%
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tc.eval()
			if !ok {
				t.Fatalf("%s: ok=false, want ok with %v", tc.name, tc.want)
			}
			if !floatEq(got, tc.want, 1e-9) {
				t.Errorf("%s = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestInc2EBITDerivation checks EBIT's derived path: when OperatingIncomeLoss is
// absent, EBIT = net income + interest + tax (the catalog ebit-margin formula).
func TestInc2EBITDerivation(t *testing.T) {
	// No OperatingIncomeLoss → derive 800 + 200 + 300 = 1300.
	derived := edgar.Fundamentals{NetIncome: 800, InterestExpense: 200, IncomeTaxExpense: 300}
	if v, ok := ebit(derived); !ok || !floatEq(v, 1300, 1e-9) {
		t.Errorf("derived ebit = %v (ok=%v), want 1300", v, ok)
	}
	// No operating income AND no interest/tax → EBIT cannot be formed.
	if _, ok := ebit(edgar.Fundamentals{NetIncome: 800}); ok {
		t.Error("ebit ok with only net income; want insufficient (cannot form EBIT)")
	}
	// preTaxIncome derived path: NI + tax when no reported pre-tax concept.
	if v, ok := preTaxIncome(edgar.Fundamentals{NetIncome: 800, IncomeTaxExpense: 300}); !ok || !floatEq(v, 1100, 1e-9) {
		t.Errorf("derived pre-tax = %v (ok=%v), want 1100", v, ok)
	}
}

// TestInc2Ratios_EdgeInsufficient asserts every Increment-2 ratio returns ok=false
// on a missing input or a zero/negative denominator — never a fabricated value. The
// fixtures isolate the failing input per ratio.
func TestInc2Ratios_EdgeInsufficient(t *testing.T) {
	// A bare profitable firm with NONE of the Increment-2 concepts present.
	bare := edgar.Fundamentals{Shares: 1000, Revenue: 5000, NetIncome: 800, Equity: 2000, TotalAssets: 6000, TotalLiabilities: 4000}
	// A firm with negative equity / zero revenue for denominator guards.
	negEq := edgar.Fundamentals{Shares: 1000, Equity: -200, LongTermDebt: 100, DebtCurrent: 50, Goodwill: 50}
	// A debt-free firm (no debt concepts) — gearing/EV must be insufficient, not 0.
	debtFree := edgar.Fundamentals{Shares: 1000, Revenue: 5000, Equity: 2000, CashAndEquivalents: 100}

	tests := []struct {
		name string
		eval func() (float64, bool)
	}{
		// Group 1 — absent concepts on a bare firm.
		{"opm no op income", func() (float64, bool) { return opm(bare) }},
		{"ebit-margin no inputs", func() (float64, bool) { return ebitMargin(bare) }},
		{"pre-tax-margin no inputs", func() (float64, bool) { return preTaxMargin(bare) }},
		{"ebitda-margin no D&A", func() (float64, bool) { return ebitdaMargin(bare) }},
		{"icr-tie no interest", func() (float64, bool) { return icrTIE(bare) }},
		{"roce no current liab", func() (float64, bool) { return roce(bare) }},
		{"roic no debt", func() (float64, bool) { return roic(bare) }},
		{"op-growth no prior", func() (float64, bool) { return opGrowth(bare) }},
		// Group 2 — absent current balance sheet.
		{"current-ratio no AC", func() (float64, bool) { return currentRatio(bare) }},
		{"quick-ratio no inventory", func() (float64, bool) { return quickRatio(bare) }},
		{"cash-ratio no cash", func() (float64, bool) { return cashRatio(bare) }},
		{"inventory-turnover no inventory", func() (float64, bool) { return inventoryTurnover(bare) }},
		{"dio no inventory", func() (float64, bool) { return dio(bare) }},
		{"receivables-turnover no AR", func() (float64, bool) { return receivablesTurnover(bare) }},
		{"dso no AR", func() (float64, bool) { return dso(bare) }},
		{"payables-turnover no AP", func() (float64, bool) { return payablesTurnover(bare) }},
		{"dpo no AP", func() (float64, bool) { return dpoDays(bare) }},
		{"ccc missing components", func() (float64, bool) { return ccc(bare) }},
		{"fixed-asset-turnover no PP&E", func() (float64, bool) { return fixedAssetTurnover(bare) }},
		{"current-asset-turnover no AC", func() (float64, bool) { return currentAssetTurnover(bare) }},
		{"wc-turnover no AC", func() (float64, bool) { return wcTurnover(bare) }},
		{"ocf-ratio no current liab", func() (float64, bool) { return ocfRatio(bare) }},
		// Group 3 — no prior values on a bare firm.
		{"eps-growth no prior", func() (float64, bool) { return epsGrowth(bare) }},
		{"equity-growth no prior", func() (float64, bool) { return equityGrowth(bare) }},
		{"asset-growth no prior", func() (float64, bool) { return assetGrowth(bare) }},
		{"gp-growth no prior", func() (float64, bool) { return gpGrowth(bare) }},
		{"eps-basic absent", func() (float64, bool) { return epsBasic(bare) }},
		// Group 4 — debt-free / negative-equity / absent concepts.
		{"lt-debt-ratio no LT debt", func() (float64, bool) { return ltDebtRatio(bare) }},
		{"net-gearing neg equity", func() (float64, bool) { return netGearing(negEq) }},
		{"net-gearing debt-free", func() (float64, bool) { return netGearing(debtFree) }},
		{"cash-st-debt no ST debt", func() (float64, bool) { return cashToSTDebt(bare) }},
		{"dtnw no intangibles", func() (float64, bool) { return dtnw(bare) }}, // no goodwill/intangibles → insufficient
		{"goodwill-equity no goodwill", func() (float64, bool) { return goodwillToEquity(bare) }},
		{"tbv no intangibles", func() (float64, bool) { return tangiblePB(40, bare) }},
		{"ev debt-free", func() (float64, bool) { return ev(40, debtFree) }},
		{"ev-sales debt-free", func() (float64, bool) { return evToSales(40, debtFree) }},
		{"ev-fcf no fcf", func() (float64, bool) { return evToFCF(40, bare) }}, // ev insufficient (no debt)
		{"ev-ebitda no ebitda", func() (float64, bool) { return evToEBITDA(40, bare) }},
		{"r-d-intensity no R&D", func() (float64, bool) { return rdIntensity(bare) }},
		{"buyback-yield no buyback", func() (float64, bool) { return buybackYield(40, bare) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if v, ok := tc.eval(); ok {
				t.Errorf("%s: ok=true (value %v), want insufficient", tc.name, v)
			}
		})
	}
}

// TestFundamentalRegistryInc2Coverage asserts the Increment-2 registry contains
// exactly the expected Group 1/2/4 + faithful-Group-3 ids, and that each closure,
// run over the empty (no-fundamentals) input, reports insufficient with no Value
// (never a fabricated number).
func TestFundamentalRegistryInc2Coverage(t *testing.T) {
	want := []string{
		// Group 1.
		"fundamental.opm", "fundamental.ebit-margin", "fundamental.pre-tax-margin",
		"fundamental.ebitda-margin", "fundamental.icr-tie", "fundamental.roce",
		"fundamental.roic", "fundamental.op-growth",
		// Group 2.
		"fundamental.current-ratio", "fundamental.quick-ratio", "fundamental.cash-ratio",
		"fundamental.inventory-turnover", "fundamental.dio", "fundamental.receivables-turnover",
		"fundamental.dso", "fundamental.payables-turnover", "fundamental.dpo",
		"fundamental.ccc", "fundamental.fixed-asset-turnover", "fundamental.current-asset-turnover",
		"fundamental.wc-turnover", "fundamental.ocf-ratio",
		// Group 3 (faithful subset).
		"fundamental.eps-growth", "fundamental.equity-growth", "fundamental.asset-growth",
		"fundamental.gp-growth", "fundamental.eps-basic",
		// Group 4.
		"fundamental.lt-debt-ratio", "fundamental.net-gearing", "fundamental.cash-st-debt",
		"fundamental.dtnw", "fundamental.goodwill-equity", "fundamental.tbv",
		"fundamental.ev", "fundamental.ev-sales", "fundamental.ev-fcf",
		"fundamental.ev-ebitda", "fundamental.r-d-intensity", "fundamental.buyback-yield",
	}
	reg := fundamentalRegistryInc2()
	if len(reg) != len(want) {
		t.Fatalf("registry has %d ids, want %d", len(reg), len(want))
	}
	for _, id := range want {
		fn, ok := reg[id]
		if !ok {
			t.Errorf("missing closure for %q", id)
			continue
		}
		// No fundamentals available → insufficient, never a fabricated value.
		si := StockIndicator{Status: StatusInsufficient, Reason: "not computed"}
		fn(computeInput{hasFund: false}, &si)
		if si.Status != StatusInsufficient {
			t.Errorf("%s: status=%q on empty input, want insufficient", id, si.Status)
		}
		if si.Value != nil {
			t.Errorf("%s: carries a Value on empty input, want nil (no fabrication)", id)
		}
	}
}

// TestFundamentalRegistryInc2OKPath checks a handful of closures end-to-end over the
// rich inc2Fixture (with a price), asserting the correct Status / Unit / Value so the
// closure wiring (right pure fn + right Unit) is verified, not just the math.
func TestFundamentalRegistryInc2OKPath(t *testing.T) {
	reg := fundamentalRegistryInc2()
	in := computeInput{hasFund: true, fund: inc2Fixture, price: inc2Price}

	cases := []struct {
		id   string
		unit string
		want float64
	}{
		{"fundamental.ebit-margin", unitPercent, 20},
		{"fundamental.current-ratio", unitMult, 2},
		{"fundamental.dio", unitNone, 73},
		{"fundamental.ev", unitUSD, 41400}, // large dollar amount → compact "$" unit, not raw
		{"fundamental.ev-ebitda", unitMult, 41400.0 / 1200},
		{"fundamental.buyback-yield", unitPercent, 2},
		{"fundamental.eps-growth", unitPercent, 25},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			fn := reg[tc.id]
			si := StockIndicator{Status: StatusInsufficient, Reason: "not computed"}
			fn(in, &si)
			if si.Status != StatusOK {
				t.Fatalf("%s: status=%q, want ok", tc.id, si.Status)
			}
			if si.Unit != tc.unit {
				t.Errorf("%s: unit=%q, want %q", tc.id, si.Unit, tc.unit)
			}
			if si.Value == nil || !floatEq(*si.Value, tc.want, 1e-9) {
				t.Errorf("%s: value=%v, want %v", tc.id, si.Value, tc.want)
			}
		})
	}
}
