package edgar

import (
	"encoding/json"
	"fmt"
	"testing"
)

// TestFactsRespDecode_StringCIK is the regression test for the recent-IPO bug:
// SEC's companyfacts API emits the top-level "cik" as a zero-padded STRING for
// newer filers (RDDT, CART/Instacart, ARM, CRWV/CoreWeave, RBRK, CAVA, DKNG…)
// but as a NUMBER for older ones. The strict json decoder used to fail the WHOLE
// payload on a string-vs-int mismatch, so every recent IPO's fundamentals errored
// (→ /fundamentals 404, all 78 indicators "insufficient"). The fix dropped the
// unused CIK field so the decode tolerates BOTH shapes. This feeds a recent-IPO-
// shaped payload (string "cik") and asserts the facts still decode + extract.
func TestFactsRespDecode_StringCIK(t *testing.T) {
	// A minimal companyfacts payload shaped like a newer filer's: "cik" is a
	// zero-padded STRING, with one revenue + net-income annual fact.
	const stringCIK = `{
		"cik": "0001713445",
		"entityName": "Reddit, Inc.",
		"facts": {
			"us-gaap": {
				"Revenues": {"units": {"USD": [
					{"start":"2023-01-01","end":"2023-12-31","val":804000000,"fy":2023,"fp":"FY","form":"10-K","filed":"2024-02-15"}
				]}},
				"NetIncomeLoss": {"units": {"USD": [
					{"start":"2023-01-01","end":"2023-12-31","val":-90800000,"fy":2023,"fp":"FY","form":"10-K","filed":"2024-02-15"}
				]}}
			}
		}
	}`

	var resp factsResp
	if err := json.Unmarshal([]byte(stringCIK), &resp); err != nil {
		t.Fatalf("decode string-cik payload failed: %v (recent IPOs would lose all fundamentals)", err)
	}
	if resp.EntityName != "Reddit, Inc." {
		t.Errorf("EntityName = %q, want %q", resp.EntityName, "Reddit, Inc.")
	}
	f := extractFundamentals(resp)
	if !f.HasData() {
		t.Fatalf("HasData() = false for a string-cik payload, want true (fundamentals recovered)")
	}
	if f.Revenue != 804000000 {
		t.Errorf("Revenue = %v, want 804000000", f.Revenue)
	}
	if f.NetIncome != -90800000 {
		t.Errorf("NetIncome = %v, want -90800000 (loss preserved)", f.NetIncome)
	}

	// And the legacy numeric "cik" shape must STILL decode (no regression).
	const numericCIK = `{"cik": 320193, "entityName": "Apple Inc.", "facts": {"us-gaap": {}}}`
	var resp2 factsResp
	if err := json.Unmarshal([]byte(numericCIK), &resp2); err != nil {
		t.Fatalf("decode numeric-cik payload failed: %v", err)
	}
	if resp2.EntityName != "Apple Inc." {
		t.Errorf("EntityName = %q, want %q", resp2.EntityName, "Apple Inc.")
	}
}

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

// TestExtractFundamentals_TTM uses Micron's real FY2025/FY2026 reporting shape (the case
// that motivated the feature: the FY2025 10-K is stale vs the far larger FY2026 quarters).
// It asserts the trailing-twelve-month roll (annual + current-YTD − prior-YTD) for EPS,
// revenue and net income, plus the standalone-quarter pick + labels.
func TestExtractFundamentals_TTM(t *testing.T) {
	resp := factsResp{EntityName: "Micron Technology, Inc."}
	resp.Facts.UsGaap = map[string]xbrlConcept{
		// Diluted EPS: FY2025 annual 7.59; FY2026 Q3 standalone 24.67 (90d) + 9-mo YTD 41.40
		// (272d); FY2025 prior-year 9-mo YTD 4.75. TTM = 7.59 + 41.40 − 4.75 = 44.24.
		"EarningsPerShareDiluted": perSh(
			factPoint{Start: "2024-08-30", End: "2025-08-28", Val: 7.59, FY: 2025, FP: "FY", Form: "10-K", Filed: "2025-10-03"},
			factPoint{Start: "2025-11-28", End: "2026-02-26", Val: 12.07, FY: 2026, FP: "Q2", Form: "10-Q", Filed: "2026-03-19"},
			factPoint{Start: "2026-02-27", End: "2026-05-28", Val: 24.67, FY: 2026, FP: "Q3", Form: "10-Q", Filed: "2026-06-25"}, // latest standalone
			factPoint{Start: "2025-08-29", End: "2026-05-28", Val: 41.40, FY: 2026, FP: "Q3", Form: "10-Q", Filed: "2026-06-25"}, // current YTD
			factPoint{Start: "2024-08-30", End: "2025-05-29", Val: 4.75, FY: 2025, FP: "Q3", Form: "10-Q", Filed: "2025-06-26"},  // prior-year YTD
		),
		// Revenue (dollar flow, exact roll): FY 37380 + cur-YTD 30000 − prior-YTD 18000 = 49380.
		"RevenueFromContractWithCustomerExcludingAssessedTax": usd(
			factPoint{Start: "2024-08-30", End: "2025-08-28", Val: 37380, FY: 2025, FP: "FY", Form: "10-K", Filed: "2025-10-03"},
			factPoint{Start: "2025-08-29", End: "2026-05-28", Val: 30000, FY: 2026, FP: "Q3", Form: "10-Q", Filed: "2026-06-25"},
			factPoint{Start: "2026-02-27", End: "2026-05-28", Val: 11000, FY: 2026, FP: "Q3", Form: "10-Q", Filed: "2026-06-25"}, // standalone → not the YTD
			factPoint{Start: "2024-08-30", End: "2025-05-29", Val: 18000, FY: 2025, FP: "Q3", Form: "10-Q", Filed: "2025-06-26"},
		),
		// Net income: FY 8540 + cur-YTD 9000 − prior-YTD 1500 = 16040.
		"NetIncomeLoss": usd(
			factPoint{Start: "2024-08-30", End: "2025-08-28", Val: 8540, FY: 2025, FP: "FY", Form: "10-K", Filed: "2025-10-03"},
			factPoint{Start: "2025-08-29", End: "2026-05-28", Val: 9000, FY: 2026, FP: "Q3", Form: "10-Q", Filed: "2026-06-25"},
			factPoint{Start: "2024-08-30", End: "2025-05-29", Val: 1500, FY: 2025, FP: "Q3", Form: "10-Q", Filed: "2025-06-26"},
		),
	}

	f := extractFundamentals(resp)
	const eps = 1e-6
	if f.EPSDiluted != 7.59 {
		t.Errorf("EPSDiluted (static, latest FY) = %v, want 7.59", f.EPSDiluted)
	}
	if abs(f.EPSDilutedTTM-44.24) > eps {
		t.Errorf("EPSDilutedTTM = %v, want 44.24 (7.59 + 41.40 − 4.75)", f.EPSDilutedTTM)
	}
	if f.EPSDilutedQuarterly != 24.67 {
		t.Errorf("EPSDilutedQuarterly = %v, want 24.67 (latest standalone Q3)", f.EPSDilutedQuarterly)
	}
	if f.LatestQuarter != "Q3 FY2026" {
		t.Errorf("LatestQuarter = %q, want %q", f.LatestQuarter, "Q3 FY2026")
	}
	if f.TTMAsOf != "2026-05-28" {
		t.Errorf("TTMAsOf = %q, want 2026-05-28", f.TTMAsOf)
	}
	if abs(f.RevenueTTM-49380) > eps {
		t.Errorf("RevenueTTM = %v, want 49380", f.RevenueTTM)
	}
	if abs(f.NetIncomeTTM-16040) > eps {
		t.Errorf("NetIncomeTTM = %v, want 16040", f.NetIncomeTTM)
	}
}

// TestTrailingTwelveMonths_Fallbacks covers the insufficient-not-wrong edges: a fresh 10-K
// with no newer quarter → TTM is the annual itself; a single year with no prior-year YTD →
// TTM falls back to the annual rather than fabricating a roll.
func TestTrailingTwelveMonths_Fallbacks(t *testing.T) {
	annualOnly := []factPoint{
		{Start: "2023-01-01", End: "2023-12-31", Val: 100, FY: 2023, FP: "FY", Form: "10-K", Filed: "2024-02-01"},
		{Start: "2024-01-01", End: "2024-12-31", Val: 120, FY: 2024, FP: "FY", Form: "10-K", Filed: "2025-02-01"}, // newest, no later quarter
	}
	if ttm, asOf, ok := trailingTwelveMonths(annualOnly); !ok || ttm != 120 || asOf != "2024-12-31" {
		t.Errorf("annual-only TTM = (%v, %q, %v), want (120, 2024-12-31, true)", ttm, asOf, ok)
	}

	// A current-FY quarter but NO prior-year YTD to subtract → fall back to the annual.
	noPrior := []factPoint{
		{Start: "2024-01-01", End: "2024-12-31", Val: 200, FY: 2024, FP: "FY", Form: "10-K", Filed: "2025-02-01"},
		{Start: "2025-01-01", End: "2025-03-31", Val: 60, FY: 2025, FP: "Q1", Form: "10-Q", Filed: "2025-05-01"},
	}
	if ttm, _, ok := trailingTwelveMonths(noPrior); !ok || ttm != 200 {
		t.Errorf("no-prior-YTD TTM = (%v, %v), want (200, true)", ttm, ok)
	}

	if _, _, ok := trailingTwelveMonths(nil); ok {
		t.Error("empty points: ok should be false")
	}
}

// TestExtractFundamentals_TTMGuards covers the recency/cumulative guards from the adversarial
// review: a post-10-K stale quarter must NOT seed the run-rate P/E, and a missing current-YTD
// must NOT produce a quarter-over-quarter "TTM".
func TestExtractFundamentals_TTMGuards(t *testing.T) {
	const eps = 1e-6

	// (A) Post-10-K: the latest annual covers the full year; the newest standalone quarter
	// (prior FY's Q3) ENDS BEFORE the annual → no run-rate quarter, TTM = the annual itself.
	postAnnual := factsResp{EntityName: "Calendar Co"}
	postAnnual.Facts.UsGaap = map[string]xbrlConcept{
		"EarningsPerShareDiluted": perSh(
			factPoint{Start: "2024-01-01", End: "2024-12-31", Val: 5.00, FY: 2024, FP: "FY", Form: "10-K", Filed: "2025-02-15"},
			factPoint{Start: "2024-07-01", End: "2024-09-30", Val: 1.30, FY: 2024, FP: "Q3", Form: "10-Q", Filed: "2024-11-01"}, // older than the annual
		),
	}
	a := extractFundamentals(postAnnual)
	if a.EPSDilutedQuarterly != 0 || a.LatestQuarter != "" {
		t.Errorf("post-10-K: want no run-rate quarter, got Q=%v label=%q", a.EPSDilutedQuarterly, a.LatestQuarter)
	}
	if abs(a.EPSDilutedTTM-5.00) > eps {
		t.Errorf("post-10-K: TTM = %v, want 5.00 (the annual)", a.EPSDilutedTTM)
	}

	// (B) A current-FY Q3 standalone exists (newer than the annual → valid run-rate) but its
	// 9-month YTD is ABSENT → TTM must fall back to the annual, NOT roll a Q-over-Q figure.
	cumMissing := factsResp{EntityName: "Sparse Inc"}
	cumMissing.Facts.UsGaap = map[string]xbrlConcept{
		"EarningsPerShareDiluted": perSh(
			factPoint{Start: "2024-08-30", End: "2025-08-28", Val: 7.59, FY: 2025, FP: "FY", Form: "10-K", Filed: "2025-10-03"},
			factPoint{Start: "2026-02-27", End: "2026-05-28", Val: 24.67, FY: 2026, FP: "Q3", Form: "10-Q", Filed: "2026-06-25"}, // standalone, no YTD
			factPoint{Start: "2025-02-27", End: "2025-05-29", Val: 1.68, FY: 2025, FP: "Q3", Form: "10-Q", Filed: "2025-06-26"},
		),
	}
	b := extractFundamentals(cumMissing)
	if abs(b.EPSDilutedTTM-7.59) > eps {
		t.Errorf("cumulative-missing: TTM = %v, want 7.59 (annual fallback, not a Q-over-Q roll)", b.EPSDilutedTTM)
	}
	if b.EPSDilutedQuarterly != 24.67 { // the run-rate quarter IS valid (newer than the annual)
		t.Errorf("cumulative-missing: run-rate quarter = %v, want 24.67", b.EPSDilutedQuarterly)
	}
}

// TestExtractFundamentals_CommonDividends asserts the dividend yield uses COMMON-only dividends
// (the general PaymentsOfDividends concept lumps in preferred for big banks → would overstate).
func TestExtractFundamentals_CommonDividends(t *testing.T) {
	withCommon := factsResp{EntityName: "Payer Inc"}
	withCommon.Facts.UsGaap = map[string]xbrlConcept{
		"NetIncomeLoss":                  usd(factPoint{Start: "2024-01-01", End: "2024-12-31", Val: 100, FY: 2024, FP: "FY", Form: "10-K"}),
		"PaymentsOfDividends":            usd(factPoint{Start: "2024-01-01", End: "2024-12-31", Val: 30, FY: 2024, FP: "FY", Form: "10-K"}), // general (incl. preferred)
		"PaymentsOfDividendsCommonStock": usd(factPoint{Start: "2024-01-01", End: "2024-12-31", Val: 22, FY: 2024, FP: "FY", Form: "10-K"}), // common-only
	}
	f := extractFundamentals(withCommon)
	if f.CommonDividendsPaid != 22 {
		t.Errorf("CommonDividendsPaid = %v, want 22 (common-only, drives the yield)", f.CommonDividendsPaid)
	}

	// A filer tagging ONLY the general concept (preferred+common) → CommonDividendsPaid 0 → the
	// yield is omitted rather than overstated by the preferred fraction.
	generalOnly := factsResp{EntityName: "Bank Inc"}
	generalOnly.Facts.UsGaap = map[string]xbrlConcept{
		"NetIncomeLoss":       usd(factPoint{Start: "2024-01-01", End: "2024-12-31", Val: 100, FY: 2024, FP: "FY", Form: "10-K"}),
		"PaymentsOfDividends": usd(factPoint{Start: "2024-01-01", End: "2024-12-31", Val: 50, FY: 2024, FP: "FY", Form: "10-K"}),
	}
	g := extractFundamentals(generalOnly)
	if g.CommonDividendsPaid != 0 {
		t.Errorf("CommonDividendsPaid = %v, want 0 (general-only → yield omitted)", g.CommonDividendsPaid)
	}
}

// TestAnnualSeriesMerged covers the multi-year trend extraction: concept-MERGE across a tag
// change (e.g. revenue's pre/post-ASC-606 tags), end-year keying, the primary-tag-wins rule when
// both tags report a year, the newest-filed restatement tie-break, oldest-first order, and the
// maxYears most-recent truncation.
func TestAnnualSeriesMerged(t *testing.T) {
	gaap := map[string]xbrlConcept{
		"RevenueFromContractWithCustomerExcludingAssessedTax": usd(
			factPoint{Start: "2023-01-01", End: "2023-12-31", Val: 1000, Form: "10-K", Filed: "2024-02-01"},
			factPoint{Start: "2022-01-01", End: "2022-12-31", Val: 900, Form: "10-K", Filed: "2024-02-01"}, // comparative col
		),
		"Revenues": usd(
			factPoint{Start: "2021-01-01", End: "2021-12-31", Val: 800, Form: "10-K", Filed: "2022-02-01"},
			factPoint{Start: "2020-01-01", End: "2020-12-31", Val: 700, Form: "10-K", Filed: "2021-02-01"},
			factPoint{Start: "2022-01-01", End: "2022-12-31", Val: 850, Form: "10-K", Filed: "2023-02-01"}, // 2022 also here → primary must win
			factPoint{Start: "2021-01-01", End: "2021-12-31", Val: 810, Form: "10-K", Filed: "2023-02-01"}, // restated 2021 (newer filed)
		),
	}
	tags := []string{"RevenueFromContractWithCustomerExcludingAssessedTax", "Revenues"}
	got := annualSeriesMerged(gaap, "USD", 10, tags...)
	// FY == Year here (no fy field on the fixtures → fiscalYear falls back to end-year → offset 0).
	want := []YearValue{{2020, 2020, 700}, {2021, 2021, 810}, {2022, 2022, 900}, {2023, 2023, 1000}}
	if len(got) != len(want) {
		t.Fatalf("series = %+v, want %+v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("series[%d] = %+v, want %+v (merge/primary-wins/restatement/order)", i, got[i], w)
		}
	}
	if s2 := annualSeriesMerged(gaap, "USD", 2, tags...); len(s2) != 2 || s2[0].Year != 2022 || s2[1].Year != 2023 {
		t.Errorf("maxYears=2 = %+v, want the 2 most-recent [{2022 900} {2023 1000}]", s2)
	}

	// Off-calendar FYE: a filer whose periods END in early Feb but are labeled the PRIOR fiscal
	// year (fy = endYear − 1, e.g. Target). The DISPLAY FY must follow the company's fy (derived
	// from the latest year's authoritative fy), not the end-date calendar year.
	febFYE := map[string]xbrlConcept{
		"Revenues": usd(
			factPoint{Start: "2022-01-30", End: "2023-01-28", Val: 100, FY: 2022, Form: "10-K", Filed: "2023-03-01"},
			factPoint{Start: "2023-01-29", End: "2024-02-03", Val: 110, FY: 2023, Form: "10-K", Filed: "2024-03-01"}, // latest: endYear 2024, fy 2023
		),
	}
	if fs := annualSeriesMerged(febFYE, "USD", 10, "Revenues"); len(fs) != 2 ||
		fs[0].Year != 2023 || fs[0].FY != 2022 || fs[1].Year != 2024 || fs[1].FY != 2023 {
		t.Errorf("febFYE = %+v, want display FY = end-year − 1 (offset −1 from the latest year's fy 2023)", fs)
	}
}

// TestExtractFundamentals_History asserts the end-to-end wiring + that an untagged line is absent
// (never fabricated).
func TestExtractFundamentals_History(t *testing.T) {
	resp := factsResp{EntityName: "Trend Co"}
	resp.Facts.UsGaap = map[string]xbrlConcept{
		"Revenues": usd(
			factPoint{Start: "2021-01-01", End: "2021-12-31", Val: 800, FP: "FY", Form: "10-K", Filed: "2022-02-01"},
			factPoint{Start: "2022-01-01", End: "2022-12-31", Val: 900, FP: "FY", Form: "10-K", Filed: "2023-02-01"},
			factPoint{Start: "2023-01-01", End: "2023-12-31", Val: 1000, FP: "FY", Form: "10-K", Filed: "2024-02-01"},
		),
		"NetIncomeLoss": usd(
			factPoint{Start: "2022-01-01", End: "2022-12-31", Val: 90, FP: "FY", Form: "10-K", Filed: "2023-02-01"},
			factPoint{Start: "2023-01-01", End: "2023-12-31", Val: 120, FP: "FY", Form: "10-K", Filed: "2024-02-01"},
		),
	}
	f := extractFundamentals(resp)
	if f.History == nil {
		t.Fatal("History is nil, want populated")
	}
	if len(f.History.Revenue) != 3 || f.History.Revenue[0] != (YearValue{2021, 2021, 800}) || f.History.Revenue[2] != (YearValue{2023, 2023, 1000}) {
		t.Errorf("History.Revenue = %+v, want 3 years oldest-first ending {2023 2023 1000}", f.History.Revenue)
	}
	if len(f.History.NetIncome) != 2 {
		t.Errorf("History.NetIncome len = %d, want 2", len(f.History.NetIncome))
	}
	if len(f.History.GrossProfit) != 0 { // untagged → absent, NOT fabricated
		t.Errorf("History.GrossProfit = %+v, want empty (untagged)", f.History.GrossProfit)
	}
}

// TestQuarterlySeries validates the standalone single-quarter extraction on a Micron-shaped
// off-August fiscal year: directly-reported Q1-Q3 (~90d), a DERIVED Q4 (full year − 9-month YTD),
// and the count-back fiscal labeling anchored on the newest standalone (companyfacts fp/fy are
// unreliable on comparative columns). Plus the only-cumulative-filer → empty guard.
func TestQuarterlySeries(t *testing.T) {
	gaap := map[string]xbrlConcept{
		"Revenues": usd(
			factPoint{Start: "2024-08-31", End: "2024-11-30", Val: 23, FY: 2025, FP: "Q1", Form: "10-Q", Filed: "2024-12-15"},
			factPoint{Start: "2024-12-01", End: "2025-02-28", Val: 22, FY: 2025, FP: "Q2", Form: "10-Q", Filed: "2025-03-15"},
			factPoint{Start: "2025-03-01", End: "2025-05-31", Val: 25, FY: 2025, FP: "Q3", Form: "10-Q", Filed: "2025-06-15"},
			factPoint{Start: "2024-08-31", End: "2025-05-31", Val: 70, FY: 2025, FP: "Q3", Form: "10-Q", Filed: "2025-06-15"},  // 9-month YTD
			factPoint{Start: "2024-08-31", End: "2025-08-30", Val: 100, FY: 2025, FP: "FY", Form: "10-K", Filed: "2025-10-01"}, // full year → Q4 = 100−70 = 30
			factPoint{Start: "2025-08-31", End: "2025-11-30", Val: 30, FY: 2026, FP: "Q1", Form: "10-Q", Filed: "2025-12-15"},  // newest standalone → anchor
		),
	}
	got := quarterlySeries(gaap, "USD", 8, "Revenues")
	want := []QuarterValue{
		{Label: "Q1 FY2025", End: "2024-11-30", Val: 23},
		{Label: "Q2 FY2025", End: "2025-02-28", Val: 22},
		{Label: "Q3 FY2025", End: "2025-05-31", Val: 25},
		{Label: "Q4 FY2025", End: "2025-08-30", Val: 30, Derived: true},
		{Label: "Q1 FY2026", End: "2025-11-30", Val: 30},
	}
	if len(got) != len(want) {
		t.Fatalf("quarterlySeries len = %d, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("quarter[%d] = %+v, want %+v (standalone / derived-Q4 / count-back label)", i, got[i], w)
		}
	}

	// Only-cumulative filer (no standalone 90d points) → no anchor → empty (insufficient-not-wrong).
	cumOnly := map[string]xbrlConcept{
		"Revenues": usd(
			factPoint{Start: "2024-08-31", End: "2025-05-31", Val: 70, FY: 2025, FP: "Q3", Form: "10-Q", Filed: "2025-06-15"},
			factPoint{Start: "2024-08-31", End: "2025-08-30", Val: 100, FY: 2025, FP: "FY", Form: "10-K", Filed: "2025-10-01"},
		),
	}
	if q := quarterlySeries(cumOnly, "USD", 8, "Revenues"); q != nil {
		t.Errorf("only-cumulative filer → want nil, got %+v", q)
	}
}

// TestQuarterlySeries_TagPriority asserts the quarterly merge matches the annual one: a strictly
// higher-priority tag wins a shared end-date even if a lower-priority tag filed later (so Annual
// and Quarterly never disagree on which concept a period's value came from).
func TestQuarterlySeries_TagPriority(t *testing.T) {
	gaap := map[string]xbrlConcept{
		"RevenueFromContractWithCustomerExcludingAssessedTax": usd(
			factPoint{Start: "2024-08-31", End: "2024-11-30", Val: 23, FY: 2025, FP: "Q1", Form: "10-Q", Filed: "2024-12-15"},
		),
		"Revenues": usd( // lower priority, filed LATER — must NOT win
			factPoint{Start: "2024-08-31", End: "2024-11-30", Val: 99, FY: 2025, FP: "Q1", Form: "10-Q", Filed: "2025-06-01"},
		),
	}
	q := quarterlySeries(gaap, "USD", 8, "RevenueFromContractWithCustomerExcludingAssessedTax", "Revenues")
	if len(q) != 1 || q[0].Val != 23 {
		t.Errorf("tag-priority: got %+v, want the primary tag's 23 (not the later-filed lower-priority 99)", q)
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
			// quarterly slice must be ignored by latestAnnual / annualForEndYear.
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

// inst is a terse builder for a point-in-time (instant) balance-sheet fact.
func inst(end string, val float64) factPoint {
	return factPoint{End: end, Val: val, Filed: end}
}

// TestExtractFundamentals_Inc2Fields exercises every NEW Increment-2 concept
// (design §1.2 Groups 1/2/4 income-statement + current-balance-sheet + debt/EV
// fields) plus the Group-3 prior-FY values (prior diluted EPS / gross profit via
// annualForEndYear, prior equity / assets via priorInstant). Each must extract with the
// chosen tag priority and prefer the latest annual / instant.
func TestExtractFundamentals_Inc2Fields(t *testing.T) {
	resp := factsResp{EntityName: "Full Inc"}
	resp.Facts.UsGaap = map[string]xbrlConcept{
		"Revenues":      usd(fy(2023, 600), fy(2024, 1000)),
		"NetIncomeLoss": usd(fy(2023, 80), fy(2024, 150)),
		"EarningsPerShareDiluted": perSh(
			factPoint{Start: "2023-01-01", End: "2023-12-31", Val: 1.0, FY: 2023, FP: "FY", Form: "10-K", Filed: "2024-02-01"},
			factPoint{Start: "2024-01-01", End: "2024-12-31", Val: 1.5, FY: 2024, FP: "FY", Form: "10-K", Filed: "2025-02-01"},
		),
		"EarningsPerShareBasic": perSh(
			factPoint{Start: "2024-01-01", End: "2024-12-31", Val: 1.55, FY: 2024, FP: "FY", Form: "10-K", Filed: "2025-02-01"},
		),
		"GrossProfit": usd(fy(2023, 240), fy(2024, 420)),
		// Balance-sheet instants: prior FY-end + latest FY-end.
		"StockholdersEquity": usd(inst("2023-12-31", 500), inst("2024-12-31", 700)),
		"Assets":             usd(inst("2023-12-31", 1800), inst("2024-12-31", 2400)),
		// Group 1 income-statement flows.
		"OperatingIncomeLoss":                  usd(fy(2023, 120), fy(2024, 220)),
		"InterestExpense":                      usd(fy(2024, 30)),
		"IncomeTaxExpenseBenefit":              usd(fy(2024, 40)),
		"DepreciationDepletionAndAmortization": usd(fy(2024, 90)),
		"IncomeLossFromContinuingOperationsBeforeIncomeTaxesExtraordinaryItemsNoncontrollingInterest": usd(fy(2024, 190)),
		// Group 2 current balance-sheet + cost of revenue.
		"CostOfRevenue":                         usd(fy(2024, 580)),
		"AssetsCurrent":                         usd(inst("2024-12-31", 900)),
		"LiabilitiesCurrent":                    usd(inst("2024-12-31", 400)),
		"InventoryNet":                          usd(inst("2024-12-31", 150)),
		"CashAndCashEquivalentsAtCarryingValue": usd(inst("2024-12-31", 300)),
		"AccountsReceivableNetCurrent":          usd(inst("2024-12-31", 200)),
		"AccountsPayableCurrent":                usd(inst("2024-12-31", 120)),
		"PropertyPlantAndEquipmentNet":          usd(inst("2024-12-31", 800)),
		// Group 4 debt / EV / capital structure.
		"LongTermDebtNoncurrent":               usd(inst("2024-12-31", 600)),
		"DebtCurrent":                          usd(inst("2024-12-31", 100)),
		"Goodwill":                             usd(inst("2024-12-31", 250)),
		"IntangibleAssetsNetExcludingGoodwill": usd(inst("2024-12-31", 80)),
		"PaymentsForRepurchaseOfCommonStock":   usd(fy(2024, 70)),
		"ResearchAndDevelopmentExpense":        usd(fy(2024, 110)),
	}

	f := extractFundamentals(resp)
	tests := []struct {
		name string
		got  float64
		want float64
	}{
		// Group 1.
		{"OperatingIncomeLoss", f.OperatingIncomeLoss, 220},
		{"OperatingIncomeLossPrior", f.OperatingIncomeLossPrior, 120},
		{"InterestExpense", f.InterestExpense, 30},
		{"IncomeTaxExpense", f.IncomeTaxExpense, 40},
		{"DepreciationAmort", f.DepreciationAmort, 90},
		{"PreTaxIncome", f.PreTaxIncome, 190},
		// Group 2.
		{"CostOfRevenue", f.CostOfRevenue, 580},
		{"AssetsCurrent", f.AssetsCurrent, 900},
		{"LiabilitiesCurrent", f.LiabilitiesCurrent, 400},
		{"InventoryNet", f.InventoryNet, 150},
		{"CashAndEquivalents", f.CashAndEquivalents, 300},
		{"AccountsReceivable", f.AccountsReceivable, 200},
		{"AccountsPayable", f.AccountsPayable, 120},
		{"PropertyPlantNet", f.PropertyPlantNet, 800},
		// Group 4.
		{"LongTermDebt", f.LongTermDebt, 600},
		{"DebtCurrent", f.DebtCurrent, 100},
		{"Goodwill", f.Goodwill, 250},
		{"IntangiblesExGoodwill", f.IntangiblesExGoodwill, 80},
		{"BuybackAmount", f.BuybackAmount, 70},
		{"ResearchDevelopment", f.ResearchDevelopment, 110},
		// Group 3 (prior-FY + basic EPS).
		{"EPSBasic", f.EPSBasic, 1.55},
		{"EPSDilutedPrior", f.EPSDilutedPrior, 1.0},
		{"GrossProfitPrior", f.GrossProfitPrior, 240},
		{"EquityPrior", f.EquityPrior, 500},
		{"TotalAssetsPrior", f.TotalAssetsPrior, 1800},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

// TestExtractFundamentals_Inc2FallbacksAndDerivations covers the tag-priority
// fallbacks and the sign / derivation guards: long-term debt staying 0 when only the
// non-debt LiabilitiesNoncurrent block is present (it is NOT substituted as debt), the
// NetIncome+tax pre-tax derivation when the reported concept is absent, the
// EBIT-input-only path (no OperatingIncomeLoss), a sign-flipped buyback, and a DERIVED
// gross profit leaving GrossProfitPrior 0 (no faithful prior pair).
func TestExtractFundamentals_Inc2FallbacksAndDerivations(t *testing.T) {
	resp := factsResp{EntityName: "Fallback Inc"}
	resp.Facts.UsGaap = map[string]xbrlConcept{
		"Revenues":      usd(fy(2023, 400), fy(2024, 900)),
		"NetIncomeLoss": usd(fy(2024, 100)),
		// Gross profit DERIVED (no GrossProfit concept) → GrossProfitPrior stays 0.
		"CostOfGoodsAndServicesSold": usd(fy(2023, 300), fy(2024, 560)),
		// No LongTermDebtNoncurrent / LongTermDebt, only the (non-debt) total
		// non-current-liability block → f.LongTermDebt must stay 0 (NOT substituted),
		// so the EV/gearing/ROIC family reports insufficient rather than inflating debt.
		"LiabilitiesNoncurrent": usd(inst("2024-12-31", 350)),
		// Income tax present but NO reported pre-tax concept → derive NI + tax.
		"IncomeTaxExpenseBenefit": usd(fy(2024, 25)),
		// Sign-flipped buyback by a quirky filer → stored positive.
		"PaymentsForRepurchaseOfCommonStock": usd(fy(2024, -55)),
	}
	f := extractFundamentals(resp)
	if f.GrossProfit != 340 { // 900 − 560 derived
		t.Errorf("GrossProfit = %v, want 340 (derived)", f.GrossProfit)
	}
	if f.GrossProfitPrior != 0 {
		t.Errorf("GrossProfitPrior = %v, want 0 (derived path has no faithful prior)", f.GrossProfitPrior)
	}
	if f.LongTermDebt != 0 {
		t.Errorf("LongTermDebt = %v, want 0 (LiabilitiesNoncurrent is NOT interest-bearing debt; never substituted)", f.LongTermDebt)
	}
	if f.PreTaxIncome != 0 {
		t.Errorf("PreTaxIncome field = %v, want 0 (no reported concept; derivation happens in the ratio)", f.PreTaxIncome)
	}
	if f.IncomeTaxExpense != 25 {
		t.Errorf("IncomeTaxExpense = %v, want 25", f.IncomeTaxExpense)
	}
	if f.BuybackAmount != 55 {
		t.Errorf("BuybackAmount = %v, want 55 (abs of -55)", f.BuybackAmount)
	}
}

// TestExtractFundamentals_Inc2Absent asserts that when NONE of the Increment-2
// concepts are present, every new field stays 0 (never invented) — the
// anti-fabrication guarantee at the extraction layer.
func TestExtractFundamentals_Inc2Absent(t *testing.T) {
	resp := factsResp{EntityName: "Minimal Inc"}
	resp.Facts.UsGaap = map[string]xbrlConcept{
		"Revenues":      usd(fy(2024, 100)),
		"NetIncomeLoss": usd(fy(2024, 10)),
	}
	f := extractFundamentals(resp)
	zero := map[string]float64{
		"OperatingIncomeLoss": f.OperatingIncomeLoss, "OperatingIncomeLossPrior": f.OperatingIncomeLossPrior,
		"InterestExpense": f.InterestExpense, "IncomeTaxExpense": f.IncomeTaxExpense,
		"DepreciationAmort": f.DepreciationAmort, "PreTaxIncome": f.PreTaxIncome,
		"CostOfRevenue": f.CostOfRevenue, "AssetsCurrent": f.AssetsCurrent,
		"LiabilitiesCurrent": f.LiabilitiesCurrent, "InventoryNet": f.InventoryNet,
		"CashAndEquivalents": f.CashAndEquivalents, "AccountsReceivable": f.AccountsReceivable,
		"AccountsPayable": f.AccountsPayable, "PropertyPlantNet": f.PropertyPlantNet,
		"LongTermDebt": f.LongTermDebt, "DebtCurrent": f.DebtCurrent,
		"Goodwill": f.Goodwill, "IntangiblesExGoodwill": f.IntangiblesExGoodwill,
		"BuybackAmount": f.BuybackAmount, "ResearchDevelopment": f.ResearchDevelopment,
		"EPSBasic": f.EPSBasic, "EPSDilutedPrior": f.EPSDilutedPrior,
		"GrossProfitPrior": f.GrossProfitPrior, "EquityPrior": f.EquityPrior,
		"TotalAssetsPrior": f.TotalAssetsPrior,
	}
	for name, v := range zero {
		if v != 0 {
			t.Errorf("%s = %v on absent concepts, want 0 (never invented)", name, v)
		}
	}
}

// TestExtractFundamentals_Inc3PriorFields exercises the Increment-3 (Piotroski Group 5)
// prior period-end fields: LongTermDebtPrior / AssetsCurrentPrior /
// LiabilitiesCurrentPrior via priorInstant of their respective concept series, and
// SharesPrior via priorInstant of the dei share series the current Shares prefers. Two
// annual period-ends are supplied (the prior cleanly ≥300 days before the latest).
func TestExtractFundamentals_Inc3PriorFields(t *testing.T) {
	resp := factsResp{EntityName: "TwoYear Inc"}
	resp.Facts.Dei = map[string]xbrlConcept{
		"EntityCommonStockSharesOutstanding": shares(
			inst("2023-12-31", 1000), // prior
			inst("2024-12-31", 1100), // latest
		),
	}
	resp.Facts.UsGaap = map[string]xbrlConcept{
		"Revenues":      usd(fy(2023, 900), fy(2024, 1000)),
		"NetIncomeLoss": usd(fy(2023, 50), fy(2024, 120)),
		// Long-term debt: prior + latest instants (same concept → priorInstant).
		"LongTermDebtNoncurrent": usd(inst("2023-12-31", 250), inst("2024-12-31", 200)),
		// Current assets / liabilities: prior + latest instants.
		"AssetsCurrent":      usd(inst("2023-12-31", 300), inst("2024-12-31", 400)),
		"LiabilitiesCurrent": usd(inst("2023-12-31", 200), inst("2024-12-31", 220)),
	}

	f := extractFundamentals(resp)
	tests := []struct {
		name string
		got  float64
		want float64
	}{
		{"Shares", float64(f.Shares), 1100},
		{"SharesPrior", float64(f.SharesPrior), 1000},
		{"LongTermDebt", f.LongTermDebt, 200},
		{"LongTermDebtPrior", f.LongTermDebtPrior, 250},
		{"AssetsCurrent", f.AssetsCurrent, 400},
		{"AssetsCurrentPrior", f.AssetsCurrentPrior, 300},
		{"LiabilitiesCurrent", f.LiabilitiesCurrent, 220},
		{"LiabilitiesCurrentPrior", f.LiabilitiesCurrentPrior, 200},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

// TestExtractFundamentals_Inc3PriorAbsentSinglePeriod asserts that with only ONE
// period-end for each concept, the Increment-3 prior fields stay 0 (no prior instant
// to match) — so the Piotroski F-score downstream reports insufficient, never a
// fabricated partial score from a single year of data.
func TestExtractFundamentals_Inc3PriorAbsentSinglePeriod(t *testing.T) {
	resp := factsResp{EntityName: "OneYear Inc"}
	resp.Facts.Dei = map[string]xbrlConcept{
		"EntityCommonStockSharesOutstanding": shares(inst("2024-12-31", 1100)),
	}
	resp.Facts.UsGaap = map[string]xbrlConcept{
		"Revenues":               usd(fy(2024, 1000)),
		"NetIncomeLoss":          usd(fy(2024, 120)),
		"LongTermDebtNoncurrent": usd(inst("2024-12-31", 200)),
		"AssetsCurrent":          usd(inst("2024-12-31", 400)),
		"LiabilitiesCurrent":     usd(inst("2024-12-31", 220)),
	}

	f := extractFundamentals(resp)
	// Current values present...
	if f.Shares != 1100 || f.LongTermDebt != 200 || f.AssetsCurrent != 400 || f.LiabilitiesCurrent != 220 {
		t.Fatalf("current values not extracted: %+v", f)
	}
	// ...but every prior field 0 (no prior period to match).
	priors := map[string]float64{
		"SharesPrior":             float64(f.SharesPrior),
		"LongTermDebtPrior":       f.LongTermDebtPrior,
		"AssetsCurrentPrior":      f.AssetsCurrentPrior,
		"LiabilitiesCurrentPrior": f.LiabilitiesCurrentPrior,
	}
	for name, v := range priors {
		if v != 0 {
			t.Errorf("%s = %v on a single period, want 0 (no prior to invent)", name, v)
		}
	}
}

// TestExtractFundamentals_StaleSharesGuard is the regression test for the
// wrong-market-cap bug: when the undimensioned shares-outstanding concept stopped
// updating years before the financials (Berkshire BRK.B's dei concept's last point
// is 2011-04-29 = 941,481 while it still files current 10-Ks → 941,481 × price ≈
// $460M for a ~$1T company), the stale share count must be treated as ABSENT so
// market_cap and every per-share metric report "insufficient" rather than a wildly
// wrong number. A FRESH payload (shares within a quarter of the financials) must
// keep its share count. Both are checked. The guard is clock-free — it compares the
// shares date to the company's own latest financial-period end, not wall-clock now.
func TestExtractFundamentals_StaleSharesGuard(t *testing.T) {
	// BRK-shaped: a 2011 cover-page share count alongside CURRENT (2024) financials.
	stale := factsResp{EntityName: "Berkshire-shaped Inc"}
	stale.Facts.Dei = map[string]xbrlConcept{
		"EntityCommonStockSharesOutstanding": shares(
			inst("2011-04-29", 941_481), // 14 years stale vs the financials below
		),
	}
	stale.Facts.UsGaap = map[string]xbrlConcept{
		"Revenues":           usd(fy(2023, 364_000_000_000), fy(2024, 371_000_000_000)),
		"NetIncomeLoss":      usd(fy(2024, 89_000_000_000)),
		"StockholdersEquity": usd(inst("2024-12-31", 649_000_000_000)),
		"Assets":             usd(inst("2024-12-31", 1_153_000_000_000)),
	}
	sf := extractFundamentals(stale)
	if sf.Shares != 0 {
		t.Errorf("stale Shares = %d, want 0 (a 2011 count must be treated as absent vs 2024 financials, "+
			"so market_cap can't be derived wrong)", sf.Shares)
	}
	if sf.SharesAsOf != "" {
		t.Errorf("stale SharesAsOf = %q, want empty (no usable shares date)", sf.SharesAsOf)
	}
	if sf.SharesPrior != 0 {
		t.Errorf("stale SharesPrior = %d, want 0 (prior of a stale series is also dropped)", sf.SharesPrior)
	}
	// The non-shares financials must be UNTOUCHED — revenue / net income / equity stay
	// present, so /fundamentals still serves them (only the shares-derived numbers vanish).
	if sf.Revenue != 371_000_000_000 || sf.NetIncome != 89_000_000_000 || sf.Equity != 649_000_000_000 {
		t.Errorf("non-shares financials altered by the guard: rev=%v ni=%v eq=%v",
			sf.Revenue, sf.NetIncome, sf.Equity)
	}
	if !sf.HasData() { // revenue alone keeps HasData true → the card/endpoint still renders
		t.Error("HasData() = false after nulling stale shares, want true (revenue/NI still present)")
	}

	// Fresh-shaped: shares dated within a quarter of the financials → kept.
	fresh := factsResp{EntityName: "Healthy Filer Inc"}
	fresh.Facts.Dei = map[string]xbrlConcept{
		"EntityCommonStockSharesOutstanding": shares(
			inst("2025-01-31", 15_000_000_000), // ~1 month after the FY2024 financials
		),
	}
	fresh.Facts.UsGaap = map[string]xbrlConcept{
		"Revenues":           usd(fy(2024, 391_000_000_000)),
		"NetIncomeLoss":      usd(fy(2024, 94_000_000_000)),
		"StockholdersEquity": usd(inst("2024-12-31", 57_000_000_000)),
	}
	ff := extractFundamentals(fresh)
	if ff.Shares != 15_000_000_000 {
		t.Errorf("fresh Shares = %d, want 15000000000 (a current count must be kept)", ff.Shares)
	}
	if ff.SharesAsOf != "2025-01-31" {
		t.Errorf("fresh SharesAsOf = %q, want 2025-01-31", ff.SharesAsOf)
	}

	// Edge: no financial anchor at all (only a shares concept) → cannot judge staleness,
	// so the shares are KEPT (we never null a count we can't prove stale).
	noAnchor := factsResp{EntityName: "Shares-only Inc"}
	noAnchor.Facts.Dei = map[string]xbrlConcept{
		"EntityCommonStockSharesOutstanding": shares(inst("2011-04-29", 941_481)),
	}
	na := extractFundamentals(noAnchor)
	if na.Shares != 941_481 {
		t.Errorf("no-anchor Shares = %d, want 941481 (no financial anchor → cannot prove stale, keep)", na.Shares)
	}
}

// comp builds a comparative income-statement column as SEC actually emits it in
// companyfacts: a full-year (10-K) duration whose own Start/End mark the GENUINE
// fiscal year (year), but whose report-context fy + filed date are those of the
// FILING that carried the column (filingFY / filed). An annual 10-K re-stamps
// EVERY embedded prior-year comparative column with the FILING's fy and the SAME
// filed date — so two different fiscal years' columns inside one 10-K share both
// fy and filed.
func comp(year, filingFY int, filed string, val float64) factPoint {
	return factPoint{
		Start: fmt.Sprintf("%d-01-01", year),
		End:   fmt.Sprintf("%d-12-31", year),
		Val:   val,
		FY:    filingFY, // SEC re-stamps the comparative column with the FILING's fy
		FP:    "FY",
		Form:  "10-K",
		Filed: filed, // …and the SAME filed date as the rest of that 10-K
	}
}

// TestExtractFundamentals_ComparativeColumnPriorYear is the regression test for the
// prior-year XBRL selection bug. In SEC companyfacts, an annual 10-K embeds its 2-3
// prior fiscal years as comparative income-statement columns and re-stamps EVERY
// column with the FILING's fy and the SAME filed date. Selecting the prior year by
// the report-context fy (the old annualForFY) therefore matched ALL columns sharing
// the target fy, and the only tie-break (latest Filed) could not separate columns
// that share a filed date — SEC orders the array ascending by end-date, so the
// OLDEST (wrong) comparative column won deterministically.
//
// Modeled on the LIVE-verified Apple shape: the FY2024 10-K (filed 2024-11-01)
// carries FY2024 / FY2023 / FY2022 revenue columns, ALL stamped fy:2024,
// filed:2024-11-01; the prior FY2023 10-K (filed 2023-11-03) carries FY2023 /
// FY2022 / FY2021 columns, ALL stamped fy:2023, filed:2023-11-03. The OLD code's
// annualForFY(revPts, 2024-1=2023) matched every column stamped fy:2023 (the three
// from the FY2023 filing) AND returned the oldest of them (FY2021, 365817) — two
// fiscal years off. The fix keys on the period's END-DATE YEAR, so the prior of the
// FY2024 current period is the FY2023 period (383285), regardless of fy/filed
// collisions. The latest/primary value must stay FY2024 (391035), unchanged.
func TestExtractFundamentals_ComparativeColumnPriorYear(t *testing.T) {
	resp := factsResp{EntityName: "Apple-shaped Inc"}
	resp.Facts.UsGaap = map[string]xbrlConcept{
		"Revenues": usd(
			// FY2023 10-K (filed 2023-11-03): three comparative columns, all fy:2023.
			comp(2021, 2023, "2023-11-03", 365817),
			comp(2022, 2023, "2023-11-03", 394328),
			comp(2023, 2023, "2023-11-03", 383285),
			// FY2024 10-K (filed 2024-11-01): three comparative columns, all fy:2024.
			comp(2022, 2024, "2024-11-01", 394328),
			comp(2023, 2024, "2024-11-01", 383285),
			comp(2024, 2024, "2024-11-01", 391035), // genuine current FY2024
		),
		// Net income, same comparative-column shape (current 2024 / prior 2023).
		"NetIncomeLoss": usd(
			comp(2021, 2023, "2023-11-03", 94680),
			comp(2022, 2023, "2023-11-03", 99803),
			comp(2023, 2023, "2023-11-03", 96995),
			comp(2022, 2024, "2024-11-01", 99803),
			comp(2023, 2024, "2024-11-01", 96995),
			comp(2024, 2024, "2024-11-01", 93736),
		),
		// COGS in the SAME comparative-column shape — exercises the same-FY pairing:
		// CostOfRevenue must match the chosen revenue's END-YEAR (2024 → 210352), so
		// gross margin / turnover use the right year's cost, NOT an older column.
		"CostOfRevenue": usd(
			comp(2022, 2024, "2024-11-01", 223546),
			comp(2023, 2024, "2024-11-01", 214137),
			comp(2024, 2024, "2024-11-01", 210352),
		),
	}

	f := extractFundamentals(resp)

	// Primary/latest values are unchanged (latestAnnual sorts by End → FY2024).
	if f.Revenue != 391035 {
		t.Errorf("Revenue = %v, want 391035 (latest FY2024 primary value must be unchanged)", f.Revenue)
	}
	if f.Period != "FY2024" {
		t.Errorf("Period = %q, want FY2024", f.Period)
	}
	if f.NetIncome != 93736 {
		t.Errorf("NetIncome = %v, want 93736 (latest FY2024)", f.NetIncome)
	}

	// THE BUG: prior-year must be the period ending one calendar year before the
	// current period (FY2023), NOT an older comparative column. The old annualForFY
	// returned FY2022 (394328) — or worse, FY2021 (365817) from the FY2023 filing's
	// columns — because every fy:2023-stamped column collided on fy + filed.
	if f.RevenuePrior != 383285 {
		t.Errorf("RevenuePrior = %v, want 383285 (FY2023, the period ending one year "+
			"before FY2024); the comparative-column collision must NOT return an older "+
			"column (394328=FY2022 or 365817=FY2021)", f.RevenuePrior)
	}
	if f.NetIncomePrior != 96995 {
		t.Errorf("NetIncomePrior = %v, want 96995 (FY2023, not an older comparative column)", f.NetIncomePrior)
	}

	// Derived gross profit must pair the current revenue with the SAME-FY COGS
	// (FY2024 revenue 391035 − FY2024 COGS 210352 = 180683), not an older column.
	if f.GrossProfit != 391035-210352 {
		t.Errorf("GrossProfit = %v, want %v (Revenue FY2024 − COGS FY2024, same end-year)",
			f.GrossProfit, 391035-210352)
	}
	if f.CostOfRevenue != 210352 {
		t.Errorf("CostOfRevenue = %v, want 210352 (FY2024, matched to the revenue end-year)", f.CostOfRevenue)
	}
}
