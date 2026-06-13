package edgar

import (
	"fmt"
	"testing"
)

// gaap/dei builders keep the synthetic companyfacts terse.
func usd(pts ...factPoint) xbrlConcept { return xbrlConcept{Units: map[string][]factPoint{"USD": pts}} }
func shares(pts ...factPoint) xbrlConcept {
	return xbrlConcept{Units: map[string][]factPoint{"shares": pts}}
}
func perSh(pts ...factPoint) xbrlConcept {
	return xbrlConcept{Units: map[string][]factPoint{"USD/shares": pts}}
}

func TestExtractFundamentals_Profitable(t *testing.T) {
	resp := factsResp{EntityName: "Acme Corp"}
	resp.Facts.Dei = map[string]xbrlConcept{
		"EntityCommonStockSharesOutstanding": shares(
			factPoint{End: "2024-06-30", Val: 1000, Filed: "2024-07-01"},
			factPoint{End: "2025-03-31", Val: 1100, Filed: "2025-04-01"}, // latest
		),
	}
	resp.Facts.UsGaap = map[string]xbrlConcept{
		"RevenueFromContractWithCustomerExcludingAssessedTax": usd(
			factPoint{Start: "2023-01-01", End: "2023-12-31", Val: 500, FY: 2023, FP: "FY", Form: "10-K", Filed: "2024-02-01"},
			factPoint{Start: "2024-01-01", End: "2024-12-31", Val: 800, FY: 2024, FP: "FY", Form: "10-K", Filed: "2025-02-01"}, // latest annual
			factPoint{Start: "2024-01-01", End: "2024-03-31", Val: 180, FY: 2024, FP: "Q1", Form: "10-Q", Filed: "2024-05-01"}, // quarterly → ignored
		),
		"NetIncomeLoss": usd(
			factPoint{Start: "2024-01-01", End: "2024-12-31", Val: 120, FY: 2024, FP: "FY", Form: "10-K", Filed: "2025-02-01"},
			factPoint{Start: "2023-01-01", End: "2023-12-31", Val: 90, FY: 2023, FP: "FY", Form: "10-K", Filed: "2024-02-01"},
		),
		"EarningsPerShareDiluted": perSh(
			factPoint{Start: "2024-01-01", End: "2024-12-31", Val: 1.10, FY: 2024, FP: "FY", Form: "10-K"},
		),
		"StockholdersEquity": usd(
			factPoint{End: "2024-12-31", Val: 600, Filed: "2025-02-01"},
			factPoint{End: "2025-03-31", Val: 650, Filed: "2025-04-01"}, // latest instant
		),
	}

	f := extractFundamentals(resp)
	if f.Shares != 1100 {
		t.Errorf("Shares = %d, want 1100 (latest instant)", f.Shares)
	}
	if f.Revenue != 800 {
		t.Errorf("Revenue = %v, want 800 (latest FY, quarterly ignored)", f.Revenue)
	}
	if f.NetIncome != 120 {
		t.Errorf("NetIncome = %v, want 120", f.NetIncome)
	}
	if f.EPSDiluted != 1.10 {
		t.Errorf("EPSDiluted = %v, want 1.10", f.EPSDiluted)
	}
	if f.Equity != 650 {
		t.Errorf("Equity = %v, want 650 (latest instant)", f.Equity)
	}
	if f.Period != "FY2024" {
		t.Errorf("Period = %q, want FY2024", f.Period)
	}
	if f.AsOf != "2024-12-31" {
		t.Errorf("AsOf = %q, want 2024-12-31", f.AsOf)
	}
	if f.Currency != "USD" || f.Name != "Acme Corp" || !f.HasData() {
		t.Errorf("meta wrong: %+v", f)
	}
}

func TestExtractFundamentals_FallbackTagAndLoss(t *testing.T) {
	resp := factsResp{EntityName: "Loss Inc"}
	resp.Facts.UsGaap = map[string]xbrlConcept{
		// No contract-revenue tag → must fall back to "Revenues".
		"Revenues": usd(
			factPoint{Start: "2024-01-01", End: "2024-12-31", Val: 50, FY: 2024, FP: "FY", Form: "10-K"},
		),
		"NetIncomeLoss": usd(
			factPoint{Start: "2024-01-01", End: "2024-12-31", Val: -30, FY: 2024, FP: "FY", Form: "10-K"},
		),
	}
	f := extractFundamentals(resp)
	if f.Revenue != 50 {
		t.Errorf("Revenue = %v, want 50 (fallback tag)", f.Revenue)
	}
	if f.NetIncome != -30 {
		t.Errorf("NetIncome = %v, want -30 (loss preserved)", f.NetIncome)
	}
	if f.Period != "FY2024" {
		t.Errorf("Period = %q, want FY2024", f.Period)
	}
}

func TestExtractFundamentals_SharesFallbackWeightedAvg(t *testing.T) {
	resp := factsResp{EntityName: "MultiClass Inc"}
	// No point-in-time shares (multi-class issuer) → fall back to weighted-average.
	resp.Facts.UsGaap = map[string]xbrlConcept{
		"WeightedAverageNumberOfSharesOutstandingBasic": shares(
			factPoint{Start: "2024-10-01", End: "2024-12-31", Val: 250_000_000, FY: 2024, FP: "Q4", Filed: "2025-01-15"},
			factPoint{Start: "2025-01-01", End: "2025-03-31", Val: 333_913_000, FY: 2025, FP: "Q1", Filed: "2025-04-15"}, // latest
		),
		"Revenues": usd(factPoint{Start: "2024-01-01", End: "2024-12-31", Val: 477, FY: 2024, FP: "FY", Form: "10-K"}),
	}
	f := extractFundamentals(resp)
	if f.Shares != 333_913_000 {
		t.Errorf("Shares = %d, want 333913000 (latest weighted-avg fallback)", f.Shares)
	}
}

func TestExtractFundamentals_Empty(t *testing.T) {
	f := extractFundamentals(factsResp{EntityName: "Empty Co"})
	if f.HasData() {
		t.Errorf("HasData() = true for empty facts, want false")
	}
}

// fy is a terse builder for an annual (10-K) duration fact.
func fy(year int, val float64) factPoint {
	return factPoint{
		Start: fmt.Sprintf("%d-01-01", year),
		End:   fmt.Sprintf("%d-12-31", year),
		Val:   val,
		FY:    year,
		FP:    "FY",
		Form:  "10-K",
		Filed: fmt.Sprintf("%d-02-01", year+1),
	}
}

// TestExtractFundamentals_NewFields exercises the margin / leverage / cash-flow
// / YoY figures, including the reported-GrossProfit path, the COGS-derived
// fallback, and a dividend payer.
func TestExtractFundamentals_NewFields(t *testing.T) {
	resp := factsResp{EntityName: "Reporter Inc"}
	resp.Facts.UsGaap = map[string]xbrlConcept{
		"Revenues": usd(
			fy(2023, 500),
			fy(2024, 800), // latest annual
			// quarterly slice must be ignored by latestAnnual / annualForFY.
			factPoint{Start: "2024-01-01", End: "2024-03-31", Val: 180, FY: 2024, FP: "Q1", Form: "10-Q", Filed: "2024-05-01"},
		),
		"NetIncomeLoss": usd(
			fy(2023, 90),
			fy(2024, 120),
		),
		"GrossProfit": usd(
			fy(2024, 300), // reported → preferred over derivation
		),
		"Assets":      usd(factPoint{End: "2024-12-31", Val: 2000, Filed: "2025-02-01"}),
		"Liabilities": usd(factPoint{End: "2024-12-31", Val: 1200, Filed: "2025-02-01"}),
		"NetCashProvidedByUsedInOperatingActivities": usd(
			fy(2024, 250),
		),
		"PaymentsToAcquirePropertyPlantAndEquipment": usd(
			fy(2024, 60),
		),
		"PaymentsOfDividendsCommonStock": usd(
			fy(2024, 40),
		),
	}

	f := extractFundamentals(resp)
	tests := []struct {
		name string
		got  float64
		want float64
	}{
		{"GrossProfit (reported)", f.GrossProfit, 300},
		{"TotalAssets", f.TotalAssets, 2000},
		{"TotalLiabilities", f.TotalLiabilities, 1200},
		{"OperatingCashFlow", f.OperatingCashFlow, 250},
		{"CapEx", f.CapEx, 60},
		{"DividendsPaid", f.DividendsPaid, 40},
		{"RevenuePrior", f.RevenuePrior, 500},
		{"NetIncomePrior", f.NetIncomePrior, 90},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

// TestExtractFundamentals_DerivedGrossProfitNonPayer covers the COGS-derived
// GrossProfit fallback (no GrossProfit concept), the TotalLiabilities =
// Assets − Equity fallback (no Liabilities concept), and a NON-dividend payer
// (DividendsPaid stays 0). CapEx with a sign-flipped filer must store positive.
func TestExtractFundamentals_DerivedGrossProfitNonPayer(t *testing.T) {
	resp := factsResp{EntityName: "Derive Co"}
	resp.Facts.UsGaap = map[string]xbrlConcept{
		"Revenues": usd(
			fy(2023, 400),
			fy(2024, 1000), // latest annual
		),
		"NetIncomeLoss": usd(
			fy(2024, 50),
		),
		// No GrossProfit concept → derive Revenue − cost of revenue. The first
		// candidate (CostOfRevenue) is missing for FY2024; the SECOND
		// (CostOfGoodsAndServicesSold) supplies the same-period cost.
		"CostOfGoodsAndServicesSold": usd(
			fy(2023, 250),
			fy(2024, 620), // same FY as the chosen revenue
		),
		"Assets": usd(factPoint{End: "2024-12-31", Val: 1500, Filed: "2025-02-01"}),
		"StockholdersEquity": usd(
			factPoint{End: "2024-12-31", Val: 900, Filed: "2025-02-01"},
		),
		// CapEx reported negative by a quirky filer → must be stored positive.
		"PaymentsToAcquirePropertyPlantAndEquipment": usd(
			fy(2024, -80),
		),
		// No dividend concepts at all → non-payer.
	}

	f := extractFundamentals(resp)
	if f.GrossProfit != 380 { // 1000 − 620
		t.Errorf("GrossProfit = %v, want 380 (derived Revenue − COGS)", f.GrossProfit)
	}
	if f.TotalLiabilities != 600 { // 1500 − 900
		t.Errorf("TotalLiabilities = %v, want 600 (Assets − Equity fallback)", f.TotalLiabilities)
	}
	if f.CapEx != 80 {
		t.Errorf("CapEx = %v, want 80 (abs of -80)", f.CapEx)
	}
	if f.DividendsPaid != 0 {
		t.Errorf("DividendsPaid = %v, want 0 (non-payer)", f.DividendsPaid)
	}
	if f.RevenuePrior != 400 {
		t.Errorf("RevenuePrior = %v, want 400", f.RevenuePrior)
	}
}
