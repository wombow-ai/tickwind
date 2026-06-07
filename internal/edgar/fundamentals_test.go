package edgar

import "testing"

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

func TestExtractFundamentals_Empty(t *testing.T) {
	f := extractFundamentals(factsResp{EntityName: "Empty Co"})
	if f.HasData() {
		t.Errorf("HasData() = true for empty facts, want false")
	}
}
