package indicators

import (
	"testing"

	"github.com/wombow-ai/tickwind/internal/edgar"
)

// TestGroup0Ratios checks each Group-0 pure ratio against a hand-computed value
// over a single profitable, dividend-paying fixture, in the unit the compute
// layer assigns. The numbers are derived by hand in the comment so the formula is
// auditable.
func TestGroup0Ratios(t *testing.T) {
	// Fixture (mirrors the existing TestFundamentalRatios style):
	//   Shares=1000, Revenue=5000, NetIncome=800, EPSDiluted=4, Equity=2000,
	//   GrossProfit=2500, TotalAssets=6000, TotalLiabilities=4000,
	//   OperatingCashFlow=1200, CapEx=300, DividendsPaid=100,
	//   NetIncomePrior=500. price=40 → market cap = 40*1000 = 40000, FCF = 900.
	f := edgar.Fundamentals{
		Shares: 1000, Revenue: 5000, NetIncome: 800, EPSDiluted: 4, Equity: 2000,
		GrossProfit: 2500, TotalAssets: 6000, TotalLiabilities: 4000,
		OperatingCashFlow: 1200, CapEx: 300, DividendsPaid: 100,
		RevenuePrior: 4000, NetIncomePrior: 500,
	}
	const price = 40.0

	tests := []struct {
		name string
		eval func() (float64, bool)
		want float64
	}{
		{"market-cap", func() (float64, bool) { return marketCap(price, f) }, 40000},              // 40*1000
		{"eps-diluted", func() (float64, bool) { return epsDiluted(f) }, 4},                       // stated
		{"bvps", func() (float64, bool) { return bvps(f) }, 2},                                    // 2000/1000
		{"sps", func() (float64, bool) { return sps(f) }, 5},                                      // 5000/1000
		{"ps", func() (float64, bool) { return ps(price, f) }, 8},                                 // 40000/5000
		{"d-e", func() (float64, bool) { return debtToEquity(f) }, 2},                             // 4000/2000
		{"equity-multiplier", func() (float64, bool) { return equityMultiplier(f) }, 3},           // 6000/2000
		{"roa%", func() (float64, bool) { return roa(f) }, 800.0 / 6000 * 100},                    // 13.333…%
		{"gp-a%", func() (float64, bool) { return gpToAssets(f) }, 2500.0 / 6000 * 100},           // 41.666…%
		{"asset-turnover", func() (float64, bool) { return assetTurnover(f) }, 5000.0 / 6000},     // 0.8333…x
		{"ocf-ni", func() (float64, bool) { return ocfToNI(f) }, 1200.0 / 800},                    // 1.5x
		{"ocf-cfo", func() (float64, bool) { return ocfCFO(f) }, 1200},                            // stated
		{"fcf-conversion", func() (float64, bool) { return fcfConversion(f) }, 900.0 / 800},       // 1.125x
		{"capex", func() (float64, bool) { return capex(f) }, 300},                                // stated
		{"capex-sales%", func() (float64, bool) { return capexToSales(f) }, 300.0 / 5000 * 100},   // 6%
		{"fcf-yield%", func() (float64, bool) { return fcfYield(price, f) }, 900.0 / 40000 * 100}, // 2.25%
		{"pcf", func() (float64, bool) { return pcf(price, f) }, 40000.0 / 1200},                  // 33.333…x
		{"cfps", func() (float64, bool) { return cfps(f) }, 1200.0 / 1000},                        // 1.2
		{"fcfps", func() (float64, bool) { return fcfps(f) }, 900.0 / 1000},                       // 0.9
		{"payout-ratio%", func() (float64, bool) { return payoutRatio(f) }, 100.0 / 800 * 100},    // 12.5%
		{"retention-ratio%", func() (float64, bool) { return retentionRatio(f) }, 100 - 12.5},     // 87.5%
		{"dps", func() (float64, bool) { return dps(f) }, 100.0 / 1000},                           // 0.1
		// SGR = ROE(40%) × retention(0.875) = 35%.
		{"sgr%", func() (float64, bool) { return sgr(f) }, 800.0 / 2000 * 100 * 0.875},              // 35%
		{"accruals%", func() (float64, bool) { return accruals(f) }, (800.0 - 1200) / 6000 * 100},   // -6.666…%
		{"tobin-s-q", func() (float64, bool) { return tobinsQ(price, f) }, (40000.0 + 4000) / 6000}, // 7.333…x
		{"pe-lyr", func() (float64, bool) { return peLYR(price, f) }, 40000.0 / 500},                // 80x
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

// TestGroup0Ratios_EdgeInsufficient asserts every Group-0 ratio returns ok=false
// on a missing input or a zero/negative denominator — never a fabricated value.
// The fixtures isolate the failing input per ratio.
func TestGroup0Ratios_EdgeInsufficient(t *testing.T) {
	// A loss-maker with no balance-sheet / cash-flow detail and no dividends.
	bare := edgar.Fundamentals{Shares: 100, Revenue: 1000, NetIncome: -50}
	// A negative-equity, zero-asset, zero-revenue firm.
	neg := edgar.Fundamentals{Shares: 100, Equity: -200, NetIncome: 50}
	// A no-price / no-shares context.
	noShares := edgar.Fundamentals{Revenue: 1000, NetIncome: 100, TotalAssets: 5000, OperatingCashFlow: 200}

	tests := []struct {
		name string
		eval func() (float64, bool)
	}{
		{"market-cap no price", func() (float64, bool) { return marketCap(0, bare) }},
		{"market-cap no shares", func() (float64, bool) { return marketCap(40, noShares) }},
		{"eps-diluted absent", func() (float64, bool) { return epsDiluted(bare) }},          // EPSDiluted 0
		{"bvps neg equity", func() (float64, bool) { return bvps(neg) }},                    // Equity<0
		{"sps no shares", func() (float64, bool) { return sps(noShares) }},                  // Shares 0
		{"ps no revenue", func() (float64, bool) { return ps(40, neg) }},                    // Revenue 0
		{"d-e neg equity", func() (float64, bool) { return debtToEquity(neg) }},             // Equity<0
		{"equity-mult no assets", func() (float64, bool) { return equityMultiplier(bare) }}, // Assets 0
		{"roa no assets", func() (float64, bool) { return roa(bare) }},                      // Assets 0
		{"gp-a no gross profit", func() (float64, bool) { return gpToAssets(noShares) }},    // GrossProfit 0
		{"asset-turnover no assets", func() (float64, bool) { return assetTurnover(bare) }}, // Assets 0
		{"ocf-ni loss", func() (float64, bool) { return ocfToNI(bare) }},                    // NetIncome<0
		{"ocf-cfo absent", func() (float64, bool) { return ocfCFO(bare) }},                  // OCF 0
		{"fcf-conversion loss", func() (float64, bool) { return fcfConversion(bare) }},      // NetIncome<0
		{"capex absent", func() (float64, bool) { return capex(bare) }},                     // CapEx 0
		{"capex-sales no capex", func() (float64, bool) { return capexToSales(bare) }},      // CapEx 0
		{"fcf-yield no ocf", func() (float64, bool) { return fcfYield(40, bare) }},          // OCF 0
		{"pcf no ocf", func() (float64, bool) { return pcf(40, bare) }},                     // OCF 0
		{"cfps no ocf", func() (float64, bool) { return cfps(bare) }},                       // OCF 0
		{"fcfps no ocf", func() (float64, bool) { return fcfps(bare) }},                     // OCF 0
		{"payout loss", func() (float64, bool) { return payoutRatio(bare) }},                // NetIncome<0
		{"retention loss", func() (float64, bool) { return retentionRatio(bare) }},          // via payout
		{"dps non-payer", func() (float64, bool) { return dps(bare) }},                      // DividendsPaid 0
		{"sgr neg equity", func() (float64, bool) { return sgr(neg) }},                      // via roe
		{"accruals no assets", func() (float64, bool) { return accruals(bare) }},            // Assets 0
		{"tobin-s-q no assets", func() (float64, bool) { return tobinsQ(40, bare) }},        // Assets 0
		{"pe-lyr no prior NI", func() (float64, bool) { return peLYR(40, bare) }},           // NetIncomePrior 0
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if v, ok := tc.eval(); ok {
				t.Errorf("%s: ok=true (value %v), want insufficient", tc.name, v)
			}
		})
	}
}

// TestFundamentalRegistryMoreCoversGroup0 asserts the registry contains exactly
// the 26 Group-0 ids and that each closure, run over the empty (no-fundamentals)
// input, reports insufficient with no Value (never a fabricated number).
func TestFundamentalRegistryMoreCoversGroup0(t *testing.T) {
	want := []string{
		"fundamental.market-cap", "fundamental.eps-diluted", "fundamental.bvps",
		"fundamental.sps", "fundamental.ps", "fundamental.d-e",
		"fundamental.equity-multiplier", "fundamental.roa", "fundamental.gp-a",
		"fundamental.total-asset-turnover", "fundamental.ocf-ni", "fundamental.ocf-cfo",
		"fundamental.fcf-conversion", "fundamental.capex", "fundamental.capex-sales",
		"fundamental.fcf-yield", "fundamental.pcf", "fundamental.cfps",
		"fundamental.fcfps", "fundamental.payout-ratio", "fundamental.retention-ratio",
		"fundamental.dps", "fundamental.sgr", "fundamental.accruals",
		"fundamental.tobin-s-q", "fundamental.pe-lyr",
	}
	reg := fundamentalRegistryMore()
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
