package edgar

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const companyFactsURL = "https://data.sec.gov/api/xbrl/companyfacts/CIK%s.json"

// Fundamentals is a compact set of XBRL-derived figures for the stock detail
// page. Revenue / NetIncome / EPSDiluted are the latest reported fiscal-year
// values, labelled by Period (e.g. "FY2024"); Shares and Equity are the latest
// point-in-time values. Market cap and P/E are NOT here — they depend on a live
// price and are derived at the API layer (market cap = price × Shares, P/E =
// price ÷ EPSDiluted). All figures come from free, public SEC data.
type Fundamentals struct {
	Ticker     string  `json:"ticker"`
	Name       string  `json:"name,omitempty"`
	Currency   string  `json:"currency"`
	Shares     int64   `json:"shares"`      // common shares outstanding (latest)
	Revenue    float64 `json:"revenue"`     // latest fiscal-year revenue
	NetIncome  float64 `json:"net_income"`  // latest fiscal-year net income (can be <0)
	EPSDiluted float64 `json:"eps_diluted"` // latest fiscal-year diluted EPS
	Equity     float64 `json:"equity"`      // stockholders' equity (latest)
	Period     string  `json:"period"`      // fiscal period the income figures cover, e.g. "FY2024"
	AsOf       string  `json:"as_of"`       // newest underlying fact date (YYYY-MM-DD)

	// SharesAsOf is the end date (YYYY-MM-DD) of the shares-outstanding instant that
	// Shares was taken from — i.e. how current the share count is. It is exposed so a
	// downstream consumer can reason about share-count freshness; extractFundamentals
	// itself enforces a staleness guard (see sharesStaleAfterDays) and ZEROES Shares
	// when the chosen instant is far older than the company's latest financial period,
	// so a 14-year-stale cover-page count (e.g. Berkshire's undimensioned dei concept,
	// last reported 2011) can never yield a wildly-wrong market cap. Empty when Shares
	// is 0 / absent.
	SharesAsOf string `json:"shares_as_of,omitempty"`

	// Additional figures for margin / leverage / cash-flow / YoY indicators.
	// Each is 0 when the underlying concept is absent (best-effort).
	GrossProfit         float64 `json:"gross_profit,omitempty"`          // latest-FY gross profit; us-gaap:GrossProfit, else Revenue − cost of revenue
	TotalAssets         float64 `json:"total_assets,omitempty"`          // us-gaap:Assets (latest instant)
	TotalLiabilities    float64 `json:"total_liabilities,omitempty"`     // us-gaap:Liabilities (latest instant), else TotalAssets − Equity
	OperatingCashFlow   float64 `json:"operating_cash_flow,omitempty"`   // latest-FY cash from operations (can be <0)
	CapEx               float64 `json:"capex,omitempty"`                 // latest-FY capital expenditure, stored POSITIVE
	DividendsPaid       float64 `json:"dividends_paid,omitempty"`        // latest-FY dividends paid (general concept; may include preferred), stored POSITIVE; 0 for non-payers
	DividendsPaidPrior  float64 `json:"dividends_paid_prior,omitempty"`  // prior-FY dividends paid (for YoY dividend growth)
	CommonDividendsPaid float64 `json:"common_dividends_paid,omitempty"` // latest-FY COMMON-only dividends paid (for the dividend yield); 0 when the filer reports only the general concept
	RevenuePrior        float64 `json:"revenue_prior,omitempty"`         // prior-FY revenue (for YoY growth)
	NetIncomePrior      float64 `json:"net_income_prior,omitempty"`      // prior-FY net income (for YoY growth)

	// --- Increment 2 (design §1.2 Groups 1/2/4): additional XBRL concepts that
	// unlock the income-statement, current-balance-sheet, and debt/EV ratio
	// families. Each is 0 when the underlying concept is absent from a filer's
	// companyfacts (best-effort, NEVER invented), so a dependent ratio that needs
	// a missing concept reports insufficient rather than a fabricated number.

	// Group 1 — income-statement flows (latest-FY, via latestAnnual).
	OperatingIncomeLoss      float64 `json:"operating_income_loss,omitempty"`       // us-gaap:OperatingIncomeLoss (latest-FY); can be <0
	InterestExpense          float64 `json:"interest_expense,omitempty"`            // us-gaap:InterestExpense (latest-FY); stored POSITIVE
	IncomeTaxExpense         float64 `json:"income_tax_expense,omitempty"`          // us-gaap:IncomeTaxExpenseBenefit (latest-FY); can be <0 (a tax benefit)
	DepreciationAmort        float64 `json:"depreciation_amort,omitempty"`          // us-gaap:DepreciationDepletionAndAmortization (latest-FY); for EBITDA
	PreTaxIncome             float64 `json:"pretax_income,omitempty"`               // us-gaap:IncomeLossFromContinuingOperationsBeforeIncomeTaxes... (latest-FY); else NetIncome + tax
	OperatingIncomeLossPrior float64 `json:"operating_income_loss_prior,omitempty"` // prior-FY OperatingIncomeLoss (for op-growth)

	// Group 2 — current balance-sheet instants + cost of revenue flow.
	CostOfRevenue      float64 `json:"cost_of_revenue,omitempty"`      // latest-FY cost of revenue (matched to the Revenue FY); for inventory/payables turnover
	AssetsCurrent      float64 `json:"assets_current,omitempty"`       // us-gaap:AssetsCurrent (latest instant)
	LiabilitiesCurrent float64 `json:"liabilities_current,omitempty"`  // us-gaap:LiabilitiesCurrent (latest instant)
	InventoryNet       float64 `json:"inventory_net,omitempty"`        // us-gaap:InventoryNet (latest instant)
	CashAndEquivalents float64 `json:"cash_and_equivalents,omitempty"` // us-gaap:CashAndCashEquivalentsAtCarryingValue (latest instant)
	AccountsReceivable float64 `json:"accounts_receivable,omitempty"`  // us-gaap:AccountsReceivableNetCurrent (latest instant)
	AccountsPayable    float64 `json:"accounts_payable,omitempty"`     // us-gaap:AccountsPayableCurrent (latest instant)
	PropertyPlantNet   float64 `json:"property_plant_net,omitempty"`   // us-gaap:PropertyPlantAndEquipmentNet (latest instant)
	RetainedEarnings   float64 `json:"retained_earnings,omitempty"`    // us-gaap:RetainedEarningsAccumulatedDeficit (latest instant); can be <0; Altman-Z X2

	// Group 4 — debt / EV / capital-structure instants + flows.
	LongTermDebt          float64 `json:"long_term_debt,omitempty"`          // interest-bearing long-term debt: us-gaap:LongTermDebtNoncurrent, else us-gaap:LongTermDebt (latest instant). 0 when no debt-specific tag exists (NOT substituted from total non-current liabilities) so the EV/gearing/ROIC family reports insufficient rather than inflating debt.
	DebtCurrent           float64 `json:"debt_current,omitempty"`            // us-gaap:DebtCurrent, else ShortTermBorrowings (latest instant)
	Goodwill              float64 `json:"goodwill,omitempty"`                // us-gaap:Goodwill (latest instant)
	IntangiblesExGoodwill float64 `json:"intangibles_ex_goodwill,omitempty"` // us-gaap:IntangibleAssetsNetExcludingGoodwill (latest instant)
	BuybackAmount         float64 `json:"buyback_amount,omitempty"`          // us-gaap:PaymentsForRepurchaseOfCommonStock (latest-FY); stored POSITIVE
	ResearchDevelopment   float64 `json:"research_development,omitempty"`    // us-gaap:ResearchAndDevelopmentExpense (latest-FY)

	// Group 3 — prior-FY values for growth ratios (faithfully extracted: flows via
	// annualForEndYear(endYear−1), balance-sheet instants via the prior period-end
	// instant).
	// 0 when the prior period is absent → the growth ratio reports insufficient.
	EPSBasic         float64 `json:"eps_basic,omitempty"`          // us-gaap:EarningsPerShareBasic (latest-FY)
	EPSDilutedPrior  float64 `json:"eps_diluted_prior,omitempty"`  // prior-FY diluted EPS (for eps-growth)
	GrossProfitPrior float64 `json:"gross_profit_prior,omitempty"` // prior-FY gross profit (for gp-growth)
	EquityPrior      float64 `json:"equity_prior,omitempty"`       // prior period-end equity (for equity-growth)
	TotalAssetsPrior float64 `json:"total_assets_prior,omitempty"` // prior period-end total assets (for asset-growth)

	// --- Increment 3 (design §1.2 Group 5): prior-FY balance-sheet instants the
	// Piotroski F-score's leverage/liquidity/dilution tests compare against the
	// current period. Each is the prior period-end instant (priorInstant) of the
	// SAME concept/tag-priority its current sibling uses, so the YoY pair is
	// consistent. 0 when the prior period is absent → the F-score reports
	// insufficient (the score is all-or-nothing; a partial score would mislead).
	LongTermDebtPrior       float64 `json:"long_term_debt_prior,omitempty"`      // prior period-end long-term debt (same pick as LongTermDebt); for ΔLeverage
	AssetsCurrentPrior      float64 `json:"assets_current_prior,omitempty"`      // prior period-end current assets; for ΔCurrentRatio
	LiabilitiesCurrentPrior float64 `json:"liabilities_current_prior,omitempty"` // prior period-end current liabilities; for ΔCurrentRatio
	SharesPrior             int64   `json:"shares_prior,omitempty"`              // prior period-end common shares outstanding (dei series); for the no-dilution test

	// --- TTM / quarterly figures. The latest 10-K is often several quarters stale (e.g. a
	// memory-cycle earnings boom — MU's FY2025 10-K predates its far larger FY2026 quarters);
	// these surface the MOST RECENT reported earnings so valuation reflects current results,
	// not a year-old annual. Each is computed from the SAME concept series its annual sibling
	// uses (concept-consistent) and is insufficient-not-wrong: left 0/empty when the quarterly
	// history is too thin to roll a trailing-twelve-month total (then callers fall back to the
	// annual figure). TTM = latest full FY + current-fiscal-year-to-date − prior-year-to-date.
	RevenueTTM          float64 `json:"revenue_ttm,omitempty"`           // trailing-12-month revenue
	NetIncomeTTM        float64 `json:"net_income_ttm,omitempty"`        // trailing-12-month net income (can be <0)
	EPSDilutedTTM       float64 `json:"eps_diluted_ttm,omitempty"`       // trailing-12-month diluted EPS — the TTM-P/E numerator
	EPSDilutedQuarterly float64 `json:"eps_diluted_quarterly,omitempty"` // latest STANDALONE fiscal-quarter diluted EPS — annualized (×4) for the run-rate/forward P/E
	LatestQuarter       string  `json:"latest_quarter,omitempty"`        // the latest standalone quarter's label, e.g. "Q3 FY2026"
	TTMAsOf             string  `json:"ttm_as_of,omitempty"`             // the TTM period-end date (YYYY-MM-DD) — the latest quarter end

	// History is the multi-year ANNUAL trend (up to ~10 fiscal years) for the headline
	// income-statement / cash-flow lines, for a per-stock financial-history table. nil for
	// filers with no annual XBRL series (ETFs / non-US). See FinancialsHistory.
	History *FinancialsHistory `json:"history,omitempty"`
}

// YearValue is one (fiscal-year, reported value) point in a multi-year series. Year is the
// END-DATE calendar year — the collision-proof SERIES KEY (see annualForEndYear). FY is the
// company-DESIGNATED fiscal year used for the DISPLAY label: it equals Year for a Dec/Sept
// year-end, but for an off-calendar end (e.g. a late-Jan/Feb retail FYE, where a period ending
// 2024-02-03 is "fiscal 2023") it carries the company's own label so the table matches the
// snapshot card's fiscalLabel. Val is the real reported 10-K figure for that year.
type YearValue struct {
	Year int     `json:"year"`
	FY   int     `json:"fy"`
	Val  float64 `json:"val"`
}

// FinancialsHistory holds the per-fiscal-year ANNUAL series for the headline financial lines —
// the data behind the multi-year trend table. Each series is up to ~financialsHistoryYears most-
// recent fiscal years, OLDEST-FIRST, one value per year; a year with no annual filing is simply
// absent (NEVER interpolated/fabricated — anti-hallucination). Most series concept-MERGE across
// the same tag-priority their snapshot sibling uses (so e.g. revenue spans the pre/post-ASC-606
// concept change). TWO differ deliberately: GrossProfit here is the tagged us-gaap:GrossProfit
// ONLY (no Revenue−COGS derivation the snapshot falls back to), and OperatingCashFlow adds a
// continuing-operations fallback the snapshot omits — so a filer may show a snapshot line with no
// matching history row (or vice-versa) WITHOUT contradiction (both insufficient-not-wrong). An
// empty series is omitted; the whole struct is nil when nothing is present.
//
// Only company-level DOLLAR totals are kept — they are split-immune and comparable across a
// decade. PER-SHARE metrics (EPS, DPS) are deliberately EXCLUDED: a multi-year series would mix
// pre/post-stock-split bases (companyfacts restates only the ~3 years a later 10-K re-reports as
// comparatives, leaving older years on the un-split basis), which would read as a phantom cliff.
// The current/TTM EPS lives in the snapshot card instead.
type FinancialsHistory struct {
	Revenue           []YearValue `json:"revenue,omitempty"`
	NetIncome         []YearValue `json:"net_income,omitempty"`
	GrossProfit       []YearValue `json:"gross_profit,omitempty"`
	OperatingIncome   []YearValue `json:"operating_income,omitempty"`
	OperatingCashFlow []YearValue `json:"operating_cash_flow,omitempty"`

	// Quarterly STANDALONE single-quarter values (last ~financialsQuartersMax quarters, oldest-
	// first), the same DOLLAR lines as above — the directly-reported ~90-day quarters plus a
	// derived Q4 (full year − the 9-month YTD). Empty when a filer reports no standalone quarter
	// to anchor the labeling (insufficient-not-wrong). Additive to the annual series above.
	RevenueQ           []QuarterValue `json:"revenue_q,omitempty"`
	NetIncomeQ         []QuarterValue `json:"net_income_q,omitempty"`
	GrossProfitQ       []QuarterValue `json:"gross_profit_q,omitempty"`
	OperatingIncomeQ   []QuarterValue `json:"operating_income_q,omitempty"`
	OperatingCashFlowQ []QuarterValue `json:"operating_cash_flow_q,omitempty"`

	// Balance-sheet year-END series (INSTANT concepts, aligned to the income fiscal-year-ends).
	TotalAssets        []YearValue `json:"total_assets,omitempty"`
	TotalLiabilities   []YearValue `json:"total_liabilities,omitempty"`
	StockholdersEquity []YearValue `json:"stockholders_equity,omitempty"`

	// Cash-flow & capital-returns ANNUAL series (dollar flows; capex/buybacks/dividends stored
	// POSITIVE). FreeCashFlow is DERIVED = operating cash flow − capex, per fiscal year.
	FreeCashFlow []YearValue `json:"free_cash_flow,omitempty"`
	CapEx        []YearValue `json:"capex,omitempty"`
	Buybacks     []YearValue `json:"buybacks,omitempty"`
	Dividends    []YearValue `json:"dividends,omitempty"`
}

// QuarterValue is one STANDALONE single-quarter point. Label is the fiscal quarter (e.g.
// "Q3 FY2026"), derived by counting back from the latest authoritative quarter — companyfacts
// fp/fy are restamped on comparative columns, unreliable except on the newest quarter. End is the
// period end (YYYY-MM-DD); Val is the single-quarter value; Derived is true for a Q4 computed as
// the full year minus the 9-month YTD (10-Ks never tag a standalone Q4).
type QuarterValue struct {
	Label   string  `json:"label"`
	End     string  `json:"end"`
	Val     float64 `json:"val"`
	Derived bool    `json:"derived,omitempty"`
}

// hasData reports whether any annual OR quarterly series carries at least one point.
func (h FinancialsHistory) hasData() bool {
	return len(h.Revenue) > 0 || len(h.NetIncome) > 0 || len(h.GrossProfit) > 0 ||
		len(h.OperatingIncome) > 0 || len(h.OperatingCashFlow) > 0 ||
		len(h.RevenueQ) > 0 || len(h.NetIncomeQ) > 0 || len(h.GrossProfitQ) > 0 ||
		len(h.OperatingIncomeQ) > 0 || len(h.OperatingCashFlowQ) > 0 ||
		len(h.TotalAssets) > 0 || len(h.TotalLiabilities) > 0 || len(h.StockholdersEquity) > 0 ||
		len(h.FreeCashFlow) > 0 || len(h.CapEx) > 0 || len(h.Buybacks) > 0 || len(h.Dividends) > 0
}

// HasData reports whether any meaningful figure was extracted.
func (f Fundamentals) HasData() bool {
	return f.Shares > 0 || f.Revenue != 0 || f.NetIncome != 0 || f.EPSDiluted != 0
}

type factsResp struct {
	// NOTE: the companyfacts payload also carries a top-level "cik", but SEC emits
	// it as a NUMBER for old filers and a zero-padded STRING (e.g. "0001713445")
	// for newer ones (RDDT, CART, ARM, CRWV, RBRK, CAVA, DKNG…). The strict decoder
	// (encoding/json via Client.get) fails the WHOLE payload on the type it doesn't
	// expect, which used to error every recent-IPO's fundamentals. The field is
	// never read here (the CIK is already known from the ticker lookup), so it is
	// intentionally OMITTED — an absent struct field is skipped regardless of the
	// JSON type, making the decode tolerant of both shapes.
	EntityName string `json:"entityName"`
	Facts      struct {
		Dei    map[string]xbrlConcept `json:"dei"`
		UsGaap map[string]xbrlConcept `json:"us-gaap"`
	} `json:"facts"`
}

type xbrlConcept struct {
	Units map[string][]factPoint `json:"units"`
}

type factPoint struct {
	Start string  `json:"start"` // empty for instantaneous facts (e.g. shares, equity)
	End   string  `json:"end"`
	Val   float64 `json:"val"`
	FY    int     `json:"fy"`
	FP    string  `json:"fp"`
	Form  string  `json:"form"`
	Filed string  `json:"filed"`
}

// Fundamentals fetches XBRL company facts for a US-listed ticker and extracts a
// compact figure set. Returns an error for non-US/unknown tickers (not in the
// SEC ticker directory).
func (c *Client) Fundamentals(ctx context.Context, ticker string) (Fundamentals, error) {
	info, err := c.lookup(ctx, ticker)
	if err != nil {
		return Fundamentals{}, err
	}
	var resp factsResp
	if err := c.get(ctx, fmt.Sprintf(companyFactsURL, info.CIK), &resp); err != nil {
		return Fundamentals{}, err
	}
	f := extractFundamentals(resp)
	f.Ticker = strings.ToUpper(ticker)
	if f.Name == "" {
		f.Name = info.Title
	}
	return f, nil
}

// extractFundamentals pulls the figures from a decoded companyfacts response.
// It is pure (no I/O) so it is unit-testable. Each metric tries a priority list
// of XBRL tags (companies/eras use different ones) and takes the first present.
func extractFundamentals(resp factsResp) Fundamentals {
	gaap := resp.Facts.UsGaap
	dei := resp.Facts.Dei
	f := Fundamentals{Name: resp.EntityName, Currency: "USD"}

	// The latest financial-period end in this payload (newest revenue / net-income /
	// equity / assets date) is the anchor for the shares-staleness guard below. It is
	// derived purely from the company's own facts (clock-free → testable) so a
	// historical-data fixture stays valid regardless of when the test runs.
	latestFinEnd := latestFinancialEnd(gaap)

	// Shares outstanding (point-in-time): dei is canonical, us-gaap as fallback.
	// Keep the dei series so the prior period-end share count can be matched for the
	// Piotroski no-dilution test (Group 5). The prior is taken from the SAME primary
	// dei series the current count prefers; if that primary path is absent (only the
	// us-gaap/weighted-avg fallbacks have a current value) SharesPrior stays 0 and the
	// F-score reports insufficient rather than mixing series.
	deiSharePts := pick(dei, "shares", "EntityCommonStockSharesOutstanding")
	if p, ok := latestInstant(deiSharePts); ok {
		f.Shares, f.SharesAsOf = int64(p.Val), p.End
		if pp, ok := priorInstant(deiSharePts, p.End); ok {
			f.SharesPrior = int64(pp.Val)
		}
	} else if p, ok := latestInstant(pick(gaap, "shares", "CommonStockSharesOutstanding")); ok {
		f.Shares, f.SharesAsOf = int64(p.Val), p.End
	}
	// Fallback for multi-class / oddly-tagged issuers (e.g. MSTR) that omit a
	// point-in-time cover-page count: the latest weighted-average share count
	// (a clean single total — it's also the EPS denominator).
	if f.Shares == 0 {
		if p, ok := latestInstant(pick(gaap, "shares",
			"WeightedAverageNumberOfSharesOutstandingBasic",
			"WeightedAverageNumberOfDilutedSharesOutstanding")); ok && p.Val > 0 {
			f.Shares, f.SharesAsOf = int64(p.Val), p.End
		}
	}

	// STALENESS GUARD (anti-hallucination): never derive a shares-dependent number
	// (market cap, P/B, P/S, EV, Altman-Z, every per-share metric) from a share count
	// that is far older than the company's latest financial period. A current filer
	// refreshes its cover-page share count every 10-Q (~quarterly), so a healthy issuer's
	// SharesAsOf is at most a few months behind latestFinEnd; a wide window therefore
	// never nulls a current filer but catches the pathological gaps where an undimensioned
	// dei/us-gaap concept stopped updating years ago (Berkshire BRK.B's last point is
	// 2011 → 941,481 × ~$489 ≈ $460M for a ~$1T company). When the chosen shares instant
	// is more than sharesStaleAfterDays older than latestFinEnd, treat Shares as ABSENT
	// (0) — every downstream guard already turns a 0 share count into "insufficient", so
	// the consumer reports no value instead of a wildly-wrong one.
	if f.Shares > 0 && sharesIsStale(f.SharesAsOf, latestFinEnd) {
		f.Shares, f.SharesPrior, f.SharesAsOf = 0, 0, ""
	}

	// Stockholders' equity (point-in-time) — for P/B at the API layer. Keep the
	// concept's full series + chosen point so the prior period-end equity can be
	// matched (Group 3 equity-growth).
	eqPts := pick(gaap, "USD",
		"StockholdersEquity",
		"StockholdersEquityIncludingPortionAttributableToNoncontrollingInterest")
	if p, ok := latestInstant(eqPts); ok {
		f.Equity = p.Val
		if pp, ok := priorInstant(eqPts, p.End); ok {
			f.EquityPrior = pp.Val
		}
	}

	// Revenue (annual flow). Keep the chosen point + its concept's full series so
	// the prior fiscal year and same-period cost of revenue can be matched.
	revPts := pick(gaap, "USD",
		"RevenueFromContractWithCustomerExcludingAssessedTax",
		"Revenues",
		"RevenueFromContractWithCustomerIncludingAssessedTax",
		"SalesRevenueNet")
	var revPt factPoint
	var revOK bool
	if revPt, revOK = latestAnnual(revPts); revOK {
		f.Revenue, f.Period, f.AsOf = revPt.Val, fiscalLabel(revPt), revPt.End
		// Prior-year revenue (same concept, the annual period ending one calendar
		// year before the chosen current period) for YoY growth — keyed on the
		// END-DATE YEAR, robust to the comparative-column FY-restamp collision.
		if p, ok := annualForEndYear(revPts, endYear(revPt)-1); ok {
			f.RevenuePrior = p.Val
		}
	}

	// Net income (annual flow) — can be negative (loss).
	niPts := pick(gaap, "USD", "NetIncomeLoss", "ProfitLoss")
	if niPt, ok := latestAnnual(niPts); ok {
		f.NetIncome = niPt.Val
		if f.Period == "" {
			f.Period, f.AsOf = fiscalLabel(niPt), niPt.End
		}
		// Prior-year net income (same concept, the period ending one year before
		// the chosen current period) for YoY growth — keyed on the END-DATE YEAR.
		if p, ok := annualForEndYear(niPts, endYear(niPt)-1); ok {
			f.NetIncomePrior = p.Val
		}
	}

	// Diluted EPS (annual flow) — drives P/E at the API layer. Keep the concept's
	// full series + chosen point so the prior-FY diluted EPS can be matched
	// (Group 3 eps-growth).
	epsPts := pick(gaap, "USD/shares", "EarningsPerShareDiluted", "EarningsPerShareBasic")
	if p, ok := latestAnnual(epsPts); ok {
		f.EPSDiluted = p.Val
		if pp, ok := annualForEndYear(epsPts, endYear(p)-1); ok {
			f.EPSDilutedPrior = pp.Val
		}
	}

	// Basic EPS (annual flow) — exposed as fundamental.eps-basic.
	if p, ok := latestAnnual(pick(gaap, "USD/shares", "EarningsPerShareBasic")); ok {
		f.EPSBasic = p.Val
	}

	// Quarterly + trailing-twelve-month figures (see the struct fields' doc). Each TTM rolls
	// the SAME concept series its annual sibling uses, so the total is concept-consistent;
	// each falls back gracefully (0 → callers use the annual) when the quarterly history is
	// too thin. revPts / niPts / epsPts already hold the full concept series (annual +
	// quarterly + year-to-date points), so no extra fetch is needed.
	epsAnnualEnd := ""
	if a, ok := latestAnnual(epsPts); ok {
		epsAnnualEnd = a.End
	}
	if q, ok := latestQuarterly(epsPts, epsAnnualEnd); ok {
		f.EPSDilutedQuarterly = q.Val
		f.LatestQuarter = quarterLabel(q)
	}
	if ttm, asOf, ok := trailingTwelveMonths(epsPts); ok {
		f.EPSDilutedTTM, f.TTMAsOf = ttm, asOf
	}
	if ttm, _, ok := trailingTwelveMonths(revPts); ok {
		f.RevenueTTM = ttm
	}
	if ttm, _, ok := trailingTwelveMonths(niPts); ok {
		f.NetIncomeTTM = ttm
	}

	// Multi-year ANNUAL history (up to ~10 fiscal years) for the headline lines — the per-stock
	// financial-trend table. Concept-merged by end-year (the SAME tag priority as the snapshot
	// figures, so the latest year matches), oldest-first; absent years are skipped, never
	// fabricated. nil when nothing is present (ETF / non-US).
	revTags := []string{"RevenueFromContractWithCustomerExcludingAssessedTax", "Revenues", "RevenueFromContractWithCustomerIncludingAssessedTax", "SalesRevenueNet"}
	niTags := []string{"NetIncomeLoss", "ProfitLoss"}
	gpTags := []string{"GrossProfit"}
	opTags := []string{"OperatingIncomeLoss"}
	ocfTags := []string{"NetCashProvidedByUsedInOperatingActivities", "NetCashProvidedByUsedInOperatingActivitiesContinuingOperations"}
	// Balance-sheet year-end series — instants aligned to the income FY-ends. Accepted gaps
	// (insufficient-not-wrong): fyEnds come from revenue, so a no-revenue filer gets no balance
	// history; and the instant must sit at the EXACT FY-end date (a rare 1-day mismatch drops a year).
	fyEnds := fiscalYearEnds(gaap, financialsHistoryYears, revTags...)
	bsAssets := annualBalanceSeries(gaap, fyEnds, "Assets")
	bsEquity := annualBalanceSeries(gaap, fyEnds, "StockholdersEquity", "StockholdersEquityIncludingPortionAttributableToNoncontrollingInterest")
	// Total liabilities: tagged us-gaap:Liabilities, else Assets − Equity per year (the snapshot
	// card's own rule, real reported components at the same FY-end) so the row matches the card.
	bsLiab := fillLiabilities(annualBalanceSeries(gaap, fyEnds, "Liabilities"), bsAssets, bsEquity)
	// Cash-flow & capital-returns annual series. CapEx / buybacks / dividends are stored POSITIVE
	// (like the snapshot scalars). FREE CASH FLOW is derived per fiscal year = operating cash flow −
	// capex, aligned by year (a year missing either side is dropped, never shown partial).
	ocfS := annualSeriesMerged(gaap, "USD", financialsHistoryYears, ocfTags...)
	capexS := absYearValues(annualSeriesMerged(gaap, "USD", financialsHistoryYears, "PaymentsToAcquirePropertyPlantAndEquipment"))
	buybackS := absYearValues(annualSeriesMerged(gaap, "USD", financialsHistoryYears, "PaymentsForRepurchaseOfCommonStock"))
	// Dividends-PAID cash-flow line — the GENERAL tag (total cash dividends, incl. the cash-flow
	// statement's own line) so coverage spans filers that report only the umbrella concept (e.g.
	// Apple tags the common-specific concept only through 2017, then the general one). This is the
	// total-dividends display, NOT the common-only per-share yield (which lives on the snapshot card).
	divS := absYearValues(annualSeriesMerged(gaap, "USD", financialsHistoryYears, "PaymentsOfDividendsCommonStock", "PaymentsOfDividends", "PaymentsOfOrdinaryDividends"))
	if h := (FinancialsHistory{
		Revenue:           annualSeriesMerged(gaap, "USD", financialsHistoryYears, revTags...),
		NetIncome:         annualSeriesMerged(gaap, "USD", financialsHistoryYears, niTags...),
		GrossProfit:       annualSeriesMerged(gaap, "USD", financialsHistoryYears, gpTags...),
		OperatingIncome:   annualSeriesMerged(gaap, "USD", financialsHistoryYears, opTags...),
		OperatingCashFlow: ocfS,
		// Quarterly single-quarter series (same concept tags, additive).
		RevenueQ:           quarterlySeries(gaap, "USD", financialsQuartersMax, revTags...),
		NetIncomeQ:         quarterlySeries(gaap, "USD", financialsQuartersMax, niTags...),
		GrossProfitQ:       quarterlySeries(gaap, "USD", financialsQuartersMax, gpTags...),
		OperatingIncomeQ:   quarterlySeries(gaap, "USD", financialsQuartersMax, opTags...),
		OperatingCashFlowQ: quarterlySeries(gaap, "USD", financialsQuartersMax, ocfTags...),
		TotalAssets:        bsAssets,
		TotalLiabilities:   bsLiab,
		StockholdersEquity: bsEquity,
		// Cash flow & capital returns (annual): derived FCF + capex + buybacks + dividends.
		FreeCashFlow: deriveFCF(ocfS, capexS),
		CapEx:        capexS,
		Buybacks:     buybackS,
		Dividends:    divS,
	}); h.hasData() {
		f.History = &h
	}

	// Gross profit (annual flow): prefer the reported concept; else derive
	// Revenue − cost of revenue for the SAME fiscal period as Revenue. Keep the
	// reported concept's series so the prior-FY gross profit can be matched
	// (Group 3 gp-growth); the derived path has no clean prior pair, so
	// GrossProfitPrior stays 0 (insufficient) for derived filers.
	gpPts := pick(gaap, "USD", "GrossProfit")
	if p, ok := latestAnnual(gpPts); ok {
		f.GrossProfit = p.Val
		if pp, ok := annualForEndYear(gpPts, endYear(p)-1); ok {
			f.GrossProfitPrior = pp.Val
		}
	} else if revOK {
		for _, cogsTag := range []string{"CostOfRevenue", "CostOfGoodsAndServicesSold", "CostOfGoodsSold"} {
			// Same-FY COGS: match the revenue period's END-DATE YEAR so the derived
			// GrossProfit = Revenue(FY) − COGS(same FY) pairs correctly (revenue and
			// COGS of one FY share an End date → same end-year).
			if cp, ok := annualForEndYear(pick(gaap, "USD", cogsTag), endYear(revPt)); ok {
				f.GrossProfit = revPt.Val - cp.Val
				break
			}
		}
	}

	// Total assets / liabilities (point-in-time) — for leverage at the API layer.
	// Keep the Assets series so the prior period-end assets can be matched
	// (Group 3 asset-growth).
	assetPts := pick(gaap, "USD", "Assets")
	if p, ok := latestInstant(assetPts); ok {
		f.TotalAssets = p.Val
		if pp, ok := priorInstant(assetPts, p.End); ok {
			f.TotalAssetsPrior = pp.Val
		}
	}
	if p, ok := latestInstant(pick(gaap, "USD", "Liabilities")); ok {
		f.TotalLiabilities = p.Val
	} else if f.TotalAssets != 0 && f.Equity != 0 {
		f.TotalLiabilities = f.TotalAssets - f.Equity
	}

	// Operating cash flow (annual flow) — can be negative.
	if p, ok := latestAnnual(pick(gaap, "USD",
		"NetCashProvidedByUsedInOperatingActivities")); ok {
		f.OperatingCashFlow = p.Val
	}

	// Capital expenditure (annual flow) — stored positive (XBRL reports it
	// positive, but guard against a sign-flipped filer).
	if p, ok := latestAnnual(pick(gaap, "USD",
		"PaymentsToAcquirePropertyPlantAndEquipment")); ok {
		f.CapEx = abs(p.Val)
	}

	// Common dividends paid (annual cash-flow outflow) — stored positive; 0 for non-payers. Tries
	// the common-stock concept, then the general one, then PaymentsOfOrdinaryDividends (the
	// cash-paid concept large filers like JNJ/PFE use instead — same paid semantics).
	divPts := pick(gaap, "USD", "PaymentsOfDividendsCommonStock", "PaymentsOfDividends", "PaymentsOfOrdinaryDividends")
	if p, ok := latestAnnual(divPts); ok {
		f.DividendsPaid = abs(p.Val)
		// Prior-FY dividends (same concept, end-date year − 1) for YoY dividend growth — matched
		// to the period ending one year before the chosen current period (mirrors RevenuePrior).
		if pp, ok := annualForEndYear(divPts, endYear(p)-1); ok {
			f.DividendsPaidPrior = abs(pp.Val)
		}
	}

	// COMMON-only dividends paid — for the dividend YIELD, which divides by the common-only
	// market cap. The general PaymentsOfDividends concept (used above) lumps in PREFERRED
	// dividends for issuers with preferred stock (big banks), which would overstate the common
	// yield. Restricted to the common-specific concepts (PaymentsOfDividendsCommonStock, and
	// PaymentsOfOrdinaryDividends = ordinary/common shares); a filer that reports ONLY the
	// general concept leaves this 0 → the yield is omitted (insufficient-not-wrong) rather than
	// served contaminated by preferred dividends.
	if p, ok := latestAnnual(pick(gaap, "USD", "PaymentsOfDividendsCommonStock", "PaymentsOfOrdinaryDividends")); ok {
		f.CommonDividendsPaid = abs(p.Val)
	}

	// --- Increment 2 (design §1.2 Groups 1/2/4) ---

	// Group 1 — income-statement flows (latest annual, same ~365-day filter as
	// Revenue). Each concept absent → the field stays 0 and dependent ratios
	// (EBIT/EBITDA/operating margins, interest coverage, ROCE/ROIC) report
	// insufficient rather than fabricate.
	opPts := pick(gaap, "USD", "OperatingIncomeLoss")
	if p, ok := latestAnnual(opPts); ok {
		f.OperatingIncomeLoss = p.Val
		// Prior-FY operating income (same concept) for op-growth — matched to the
		// period ending one year before the chosen current period (END-DATE YEAR),
		// so the YoY pair is consistent (mirrors RevenuePrior).
		if pp, ok := annualForEndYear(opPts, endYear(p)-1); ok {
			f.OperatingIncomeLossPrior = pp.Val
		}
	}
	// Interest expense — stored positive (XBRL reports it positive; guard a
	// sign-flipped filer). Try the plain concept, then the net variant.
	if p, ok := latestAnnual(pick(gaap, "USD",
		"InterestExpense", "InterestExpenseNonoperating", "InterestAndDebtExpense")); ok {
		f.InterestExpense = abs(p.Val)
	}
	// Income-tax expense/benefit — can be negative (a net tax benefit), kept as
	// stated so EBIT = NetIncome + interest + tax is faithful.
	if p, ok := latestAnnual(pick(gaap, "USD", "IncomeTaxExpenseBenefit")); ok {
		f.IncomeTaxExpense = p.Val
	}
	// Depreciation, depletion & amortization (annual flow) — for EBITDA. Prefer
	// the combined concept, then the cash-flow-statement variant.
	if p, ok := latestAnnual(pick(gaap, "USD",
		"DepreciationDepletionAndAmortization",
		"DepreciationAmortizationAndAccretionNet",
		"DepreciationAndAmortization")); ok {
		f.DepreciationAmort = p.Val
	}
	// Pre-tax income from continuing operations (annual flow) — for pre-tax
	// margin. Prefer the reported concept; else derive NetIncome + tax (the
	// catalog's own pre-tax fallback) when the tax line is present.
	if p, ok := latestAnnual(pick(gaap, "USD",
		"IncomeLossFromContinuingOperationsBeforeIncomeTaxesExtraordinaryItemsNoncontrollingInterest",
		"IncomeLossFromContinuingOperationsBeforeIncomeTaxesMinorityInterestAndIncomeLossFromEquityMethodInvestments",
		"IncomeLossFromContinuingOperationsBeforeIncomeTaxesMinorityInterestAndDiscontinuedOperations")); ok {
		f.PreTaxIncome = p.Val
	}

	// Group 2 — current balance-sheet instants (latest point-in-time) + cost of
	// revenue (annual flow, matched to the Revenue FY for turnover ratios).
	if revOK {
		// Cost of revenue for the SAME fiscal year as Revenue — the inventory /
		// payables turnover numerator. Reuse the COGS tag priority already used to
		// derive GrossProfit so the figure is consistent.
		for _, cogsTag := range []string{"CostOfRevenue", "CostOfGoodsAndServicesSold", "CostOfGoodsSold"} {
			// Match the revenue period's END-DATE YEAR so CostOfRevenue is the SAME
			// FY as Revenue (the turnover-ratio numerator).
			if cp, ok := annualForEndYear(pick(gaap, "USD", cogsTag), endYear(revPt)); ok {
				f.CostOfRevenue = cp.Val
				break
			}
		}
	}
	// Current assets / liabilities — keep each concept's series so the prior
	// period-end instant can be matched for the Piotroski ΔCurrentRatio test (Group 5).
	acPts := pick(gaap, "USD", "AssetsCurrent")
	if p, ok := latestInstant(acPts); ok {
		f.AssetsCurrent = p.Val
		if pp, ok := priorInstant(acPts, p.End); ok {
			f.AssetsCurrentPrior = pp.Val
		}
	}
	lcPts := pick(gaap, "USD", "LiabilitiesCurrent")
	if p, ok := latestInstant(lcPts); ok {
		f.LiabilitiesCurrent = p.Val
		if pp, ok := priorInstant(lcPts, p.End); ok {
			f.LiabilitiesCurrentPrior = pp.Val
		}
	}
	if p, ok := latestInstant(pick(gaap, "USD", "RetainedEarningsAccumulatedDeficit")); ok {
		f.RetainedEarnings = p.Val // can be negative (accumulated deficit / heavy buybacks); for Altman-Z X2
	}
	if p, ok := latestInstant(pick(gaap, "USD",
		"InventoryNet", "InventoryFinishedGoodsNetOfReserves")); ok {
		f.InventoryNet = p.Val
	}
	if p, ok := latestInstant(pick(gaap, "USD",
		"CashAndCashEquivalentsAtCarryingValue",
		"CashCashEquivalentsRestrictedCashAndRestrictedCashEquivalents",
		"CashAndCashEquivalentsAtCarryingValueIncludingDiscontinuedOperations")); ok {
		f.CashAndEquivalents = p.Val
	}
	if p, ok := latestInstant(pick(gaap, "USD",
		"AccountsReceivableNetCurrent", "ReceivablesNetCurrent")); ok {
		f.AccountsReceivable = p.Val
	}
	if p, ok := latestInstant(pick(gaap, "USD",
		"AccountsPayableCurrent", "AccountsPayableTradeCurrent")); ok {
		f.AccountsPayable = p.Val
	}
	if p, ok := latestInstant(pick(gaap, "USD",
		"PropertyPlantAndEquipmentNet")); ok {
		f.PropertyPlantNet = p.Val
	}

	// Group 4 — debt / EV / capital-structure instants + flows. Keep the long-term-debt
	// series so the prior period-end instant can be matched for the Piotroski ΔLeverage
	// test (Group 5) — the prior MUST come from the SAME concept the current value was
	// picked from (priorInstant of the chosen series), never a different tag.
	//
	// Tag-priority is GENUINE long-term DEBT only (us-gaap:LongTermDebtNoncurrent, else
	// us-gaap:LongTermDebt). us-gaap:LiabilitiesNoncurrent is deliberately NOT a fallback:
	// it is the entire non-current-liability block (deferred taxes, operating leases,
	// pensions, deferred revenue, …), NOT interest-bearing debt, so substituting it would
	// inflate interestBearingDebt() / ev() and cascade into netGearing / roic invested
	// capital / evTo*. When neither debt-specific tag is present we leave LongTermDebt = 0:
	// the consumers' existing <=0 guards then report INSUFFICIENT (or, for ev(), correctly
	// treat the firm as carrying no long-term debt) rather than fabricating a debt figure
	// from total liabilities. (DebtLongtermAndShorttermCombinedAmount is intentionally
	// omitted: it bundles current + long-term debt, which DebtCurrent already adds
	// separately in interestBearingDebt(), so including it here would double-count.)
	ltdPts := pick(gaap, "USD",
		"LongTermDebtNoncurrent", "LongTermDebt")
	if p, ok := latestInstant(ltdPts); ok {
		f.LongTermDebt = p.Val
		if pp, ok := priorInstant(ltdPts, p.End); ok {
			f.LongTermDebtPrior = pp.Val
		}
	}
	if p, ok := latestInstant(pick(gaap, "USD",
		"DebtCurrent", "ShortTermBorrowings", "LongTermDebtCurrent")); ok {
		f.DebtCurrent = p.Val
	}
	if p, ok := latestInstant(pick(gaap, "USD", "Goodwill")); ok {
		f.Goodwill = p.Val
	}
	if p, ok := latestInstant(pick(gaap, "USD",
		"IntangibleAssetsNetExcludingGoodwill",
		"FiniteLivedIntangibleAssetsNet")); ok {
		f.IntangiblesExGoodwill = p.Val
	}
	// Buyback amount (annual flow) — stored positive (XBRL reports it positive;
	// guard a sign-flipped filer); 0 for non-repurchasers.
	if p, ok := latestAnnual(pick(gaap, "USD",
		"PaymentsForRepurchaseOfCommonStock")); ok {
		f.BuybackAmount = abs(p.Val)
	}
	// R&D expense (annual flow) — for R&D intensity; 0 for non-R&D filers.
	if p, ok := latestAnnual(pick(gaap, "USD",
		"ResearchAndDevelopmentExpense")); ok {
		f.ResearchDevelopment = p.Val
	}

	return f
}

// abs returns the absolute value of a float64.
func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// sharesStaleAfterDays is the staleness threshold for the share count: if the
// chosen shares-outstanding instant is more than this many days OLDER than the
// company's latest financial period (latestFinancialEnd), the share count is
// treated as absent rather than used to derive a market cap / per-share metric.
//
// ~15 months. Justification: a current SEC filer restates its cover-page shares
// outstanding on every quarterly 10-Q, so a healthy issuer's shares date trails
// its latest financial-statement date by at most ~one quarter (≤~100 days) — even
// allowing for a late annual-only filer the gap stays under a year. 15 months
// (450 days) leaves comfortable slack so NO current filer is ever nulled, while
// still catching the pathological cases where an undimensioned shares concept
// stopped updating years before the financials did (Berkshire's dei concept: last
// 2011, financials current → a ~14-year gap; HEICO 2015; Bio-Rad 2010; Comcast
// 2009 — a cohort, not a one-off). The window is intentionally generous: the goal
// is "insufficient, not wrong", so we only act on an unambiguous multi-quarter gap.
const sharesStaleAfterDays = 450

// latestFinancialEnd returns the newest period-end date (YYYY-MM-DD) among the
// company's core financial facts — annual revenue / net income (duration flows)
// and the latest stockholders'-equity / total-assets instants. It is the anchor
// the shares-staleness guard compares the share-count date against. Returns "" when
// none of these concepts are present (then sharesIsStale is a no-op — we cannot
// judge staleness without a financial anchor, so we keep whatever shares we found
// rather than null a count we cannot prove stale). Pure: derived only from the
// payload, so it is clock-free and unit-testable.
func latestFinancialEnd(gaap map[string]xbrlConcept) string {
	best := ""
	consider := func(p factPoint, ok bool) {
		if ok && p.End > best {
			best = p.End
		}
	}
	consider(latestAnnual(pick(gaap, "USD",
		"RevenueFromContractWithCustomerExcludingAssessedTax",
		"Revenues",
		"RevenueFromContractWithCustomerIncludingAssessedTax",
		"SalesRevenueNet")))
	consider(latestAnnual(pick(gaap, "USD", "NetIncomeLoss", "ProfitLoss")))
	consider(latestInstant(pick(gaap, "USD",
		"StockholdersEquity",
		"StockholdersEquityIncludingPortionAttributableToNoncontrollingInterest")))
	consider(latestInstant(pick(gaap, "USD", "Assets")))
	return best
}

// sharesIsStale reports whether a share count dated sharesEnd is too old to trust
// relative to the company's latest financial period latestFinEnd — i.e. sharesEnd
// is more than sharesStaleAfterDays before latestFinEnd. Returns false (NOT stale)
// when either date is empty or unparseable, or when the shares date is not older
// than the financial date, so the guard only ever fires on a provable multi-quarter
// gap and never nulls a count it cannot judge.
func sharesIsStale(sharesEnd, latestFinEnd string) bool {
	if sharesEnd == "" || latestFinEnd == "" {
		return false
	}
	se, err1 := time.Parse("2006-01-02", sharesEnd)
	fe, err2 := time.Parse("2006-01-02", latestFinEnd)
	if err1 != nil || err2 != nil {
		return false
	}
	return fe.Sub(se).Hours()/24 > sharesStaleAfterDays
}

// pick returns the units array for the first tag present with data, in the given
// unit (e.g. "USD", "shares", "USD/shares").
func pick(facts map[string]xbrlConcept, unit string, tags ...string) []factPoint {
	for _, tag := range tags {
		if c, ok := facts[tag]; ok {
			if pts := c.Units[unit]; len(pts) > 0 {
				return pts
			}
		}
	}
	return nil
}

// latestInstant returns the most recent point-in-time fact (max end date, then
// max filed date). Dates are YYYY-MM-DD so lexical comparison is chronological.
func latestInstant(pts []factPoint) (factPoint, bool) {
	var best factPoint
	found := false
	for _, p := range pts {
		if !found || p.End > best.End || (p.End == best.End && p.Filed > best.Filed) {
			best, found = p, true
		}
	}
	return best, found
}

// priorInstant returns the prior-period instant for a balance-sheet concept: the
// most recent point-in-time fact whose end date is at least ~300 days BEFORE
// latestEnd (i.e. the prior fiscal year-end, not a same-year interim quarter). It
// is used to pull a prior-FY balance (equity, assets) for a YoY growth ratio
// without fabricating one — when no such prior instant exists the caller leaves the
// field 0 and the growth ratio reports insufficient. Among qualifying points it
// prefers the newest end date then the latest amendment (matching latestInstant).
func priorInstant(pts []factPoint, latestEnd string) (factPoint, bool) {
	le, err := time.Parse("2006-01-02", latestEnd)
	if err != nil {
		return factPoint{}, false
	}
	cutoff := le.AddDate(0, 0, -300)
	var best factPoint
	found := false
	for _, p := range pts {
		if p.Start != "" || p.End == "" {
			continue // not an instant
		}
		pe, err := time.Parse("2006-01-02", p.End)
		if err != nil || !pe.Before(cutoff) {
			continue
		}
		if !found || p.End > best.End || (p.End == best.End && p.Filed > best.Filed) {
			best, found = p, true
		}
	}
	return best, found
}

// latestAnnual returns the most recent full-year flow fact (period duration
// ~365 days), preferring the newest end date then the latest amendment.
func latestAnnual(pts []factPoint) (factPoint, bool) {
	var best factPoint
	found := false
	for _, p := range pts {
		if p.Start == "" {
			continue
		}
		if d := durationDays(p.Start, p.End); d < 350 || d > 380 {
			continue
		}
		if !found || p.End > best.End || (p.End == best.End && p.Filed > best.Filed) {
			best, found = p, true
		}
	}
	return best, found
}

// annualForEndYear returns the full-year flow fact whose PERIOD ends in a
// specific calendar year (the same ~365-day duration filter as latestAnnual),
// keyed on the END-DATE YEAR — NOT the report-context FY field. This is the
// robust selector for a prior-year value or a same-period cost of revenue.
//
// Why end-year, not the FY field: in SEC companyfacts an annual 10-K embeds its
// 2-3 prior fiscal years as comparative income-statement columns, and SEC
// re-stamps EVERY column with the FILING's fy and the SAME filed date. So
// matching on fiscalYear(p) (the report-context fy) matches ALL comparative
// columns of a 10-K that share the target fy, and the only tie-break (latest
// Filed) cannot separate columns that share a filed date — the SEC array is
// ordered ascending by end-date, so the OLDEST (wrong) comparative column would
// win. Matching on the period's own end-date year is collision-proof: each
// genuine fiscal year has exactly one full-year period ending in it.
//
// Among the matched points it prefers the newest End then the latest Filed
// (End-then-Filed tie-break, exactly like latestAnnual / latestInstant) — so a
// later 10-K's restated value for the year wins over an earlier original.
func annualForEndYear(pts []factPoint, endYr int) (factPoint, bool) {
	var best factPoint
	found := false
	for _, p := range pts {
		if p.Start == "" || endYear(p) != endYr {
			continue
		}
		if d := durationDays(p.Start, p.End); d < 350 || d > 380 {
			continue
		}
		if !found || p.End > best.End || (p.End == best.End && p.Filed > best.Filed) {
			best, found = p, true
		}
	}
	return best, found
}

// fiscalYear returns a fact's fiscal year, falling back to its end-date year
// when the FY field is unset (mirrors fiscalLabel's logic).
func fiscalYear(p factPoint) int {
	y := p.FY
	if y == 0 && len(p.End) >= 4 {
		_, _ = fmt.Sscanf(p.End[:4], "%d", &y)
	}
	return y
}

// endYear parses a fact's End date ("2006-01-02") and returns its calendar
// year, or 0 when End is empty/unparseable. It is the selection key for
// annualForEndYear — robust to the comparative-column FY-restamp collision (see
// annualForEndYear) and consistent for off-calendar fiscal years (revenue and
// COGS of the same FY share an End date → same end-year → they pair correctly).
func endYear(p factPoint) int {
	y := 0
	if len(p.End) >= 4 {
		_, _ = fmt.Sscanf(p.End[:4], "%d", &y)
	}
	return y
}

func durationDays(start, end string) int {
	s, err1 := time.Parse("2006-01-02", start)
	e, err2 := time.Parse("2006-01-02", end)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(e.Sub(s).Hours() / 24)
}

// fiscalLabel renders a fact's fiscal year as "FY2024" (from the FY field, or
// the end-date year as a fallback).
func fiscalLabel(p factPoint) string {
	return fmt.Sprintf("FY%d", fiscalYear(p))
}

// quarterLabel renders a quarterly fact's fiscal period as "Q3 FY2026" (the FP field
// + the fiscal-year label). FP is empty → "Q? FYxxxx" (never fabricated).
func quarterLabel(p factPoint) string {
	fp := p.FP
	if fp == "" {
		fp = "Q?"
	}
	return fp + " " + fiscalLabel(p)
}

// latestQuarterly returns the most recent STANDALONE fiscal-quarter flow fact (period
// duration ~90 days) ending AFTER afterEnd (the latest annual's end), newest end then latest
// amendment. Used to annualize the latest quarter (× 4) for the run-rate / forward P/E. The
// afterEnd guard is essential: a 10-K never tags a standalone Q4, so right after an annual
// the newest standalone quarter (the prior FY's Q3) is OLDER than the annual — annualizing it
// would be stale/misleading. When no standalone quarter is newer than the annual, returns
// false → the run-rate P/E is omitted (insufficient-not-wrong) until the next 10-Q. The
// cumulative year-to-date points (181/272-day) are excluded by the duration window.
func latestQuarterly(pts []factPoint, afterEnd string) (factPoint, bool) {
	var best factPoint
	found := false
	for _, p := range pts {
		if p.Start == "" || p.End <= afterEnd {
			continue
		}
		if d := durationDays(p.Start, p.End); d < 80 || d > 100 {
			continue
		}
		if !found || p.End > best.End || (p.End == best.End && p.Filed > best.Filed) {
			best, found = p, true
		}
	}
	return best, found
}

// latestYTD returns the current fiscal-year-to-date cumulative flow fact: the LONGEST
// non-annual period (duration 80–350 days) ending at the latest quarter-end that falls
// after afterEnd (the latest annual's end). For Q3 it is the 9-month figure (not the
// standalone 3-month quarter); for Q1 it is the single 3-month figure. Returns false when
// no quarterly period is newer than the latest annual (a fresh 10-K is the most recent
// filing → the annual itself is the trailing twelve months).
func latestYTD(pts []factPoint, afterEnd string) (factPoint, bool) {
	// Dates are YYYY-MM-DD so lexical comparison is chronological.
	latestEnd := ""
	for _, p := range pts {
		if p.Start == "" || p.End <= afterEnd {
			continue
		}
		if d := durationDays(p.Start, p.End); d < 80 || d > 350 {
			continue
		}
		if p.End > latestEnd {
			latestEnd = p.End
		}
	}
	if latestEnd == "" {
		return factPoint{}, false
	}
	var best factPoint
	bestDur := 0
	found := false
	for _, p := range pts {
		if p.Start == "" || p.End != latestEnd {
			continue
		}
		d := durationDays(p.Start, p.End)
		if d < 80 || d > 350 {
			continue
		}
		if !found || d > bestDur || (d == bestDur && p.Filed > best.Filed) {
			best, bestDur, found = p, d, true
		}
	}
	if !found {
		return factPoint{}, false
	}
	// Genuine-cumulative guard: a real fiscal-year-to-date STARTS at the FY start (≈ the day
	// after the prior annual's end). Reject a bare standalone quarter masquerading as the YTD
	// — its start sits mid-year — which happens when the current quarter's cumulative figure
	// is absent (so only the 90-day standalone shares the latest end-date). The legitimate Q1
	// case passes (its standalone IS the YTD, starting at the FY start); falling back to the
	// annual otherwise is correct (insufficient-not-wrong, avoids a quarter-over-quarter roll).
	if g := durationDays(afterEnd, best.Start); g < -20 || g > 20 {
		return factPoint{}, false
	}
	return best, true
}

// priorYearYTD returns the same fiscal-year-to-date period one year before cur: the point
// whose duration matches cur's (±20 days) and whose end is ~365 days before cur's (±20
// days). It is the term subtracted to roll the trailing twelve months forward. Returns
// false when no matching prior-year period exists (too little history).
func priorYearYTD(pts []factPoint, cur factPoint) (factPoint, bool) {
	curEnd, err := time.Parse("2006-01-02", cur.End)
	if err != nil {
		return factPoint{}, false
	}
	curDur := durationDays(cur.Start, cur.End)
	target := curEnd.AddDate(-1, 0, 0)
	const tol = 20 * 24 * time.Hour
	var best factPoint
	var bestGap time.Duration
	found := false
	for _, p := range pts {
		if p.Start == "" {
			continue
		}
		d := durationDays(p.Start, p.End)
		if d < curDur-20 || d > curDur+20 {
			continue
		}
		if d >= 350 && curDur < 350 {
			continue // never subtract a full prior-year ANNUAL from an interim YTD (transition filings)
		}
		pe, err := time.Parse("2006-01-02", p.End)
		if err != nil {
			continue
		}
		gap := pe.Sub(target)
		if gap < 0 {
			gap = -gap
		}
		if gap > tol {
			continue
		}
		if !found || gap < bestGap || (gap == bestGap && p.Filed > best.Filed) {
			best, bestGap, found = p, gap, true
		}
	}
	return best, found
}

// trailingTwelveMonths computes the trailing-twelve-month total of a flow concept (revenue /
// net income / diluted EPS) from its XBRL points: the latest full fiscal year, plus the
// current fiscal-year-to-date, minus the prior-year same-period-to-date — i.e. it rolls the
// completed year forward by the quarters reported since. When no quarterly period is newer
// than the latest 10-K, the annual itself IS the trailing twelve months. Returns
// (ttm, periodEnd, ok); ok is false only when there is no annual period to anchor on.
//
// For dollar flows (revenue, net income) the roll is exact. For diluted EPS it sums the
// issuer's own reported cumulative EPS figures (annual + 9-month − prior 9-month), the
// conventional TTM-EPS — slightly approximate across share-count drift but the standard
// basis (validated against Micron: 7.59 + 41.40 − 4.75 = 44.24 → matches the public TTM P/E).
func trailingTwelveMonths(pts []factPoint) (float64, string, bool) {
	annual, ok := latestAnnual(pts)
	if !ok {
		return 0, "", false
	}
	cur, ok := latestYTD(pts, annual.End)
	if !ok {
		return annual.Val, annual.End, true // the 10-K is the most recent filing
	}
	prior, ok := priorYearYTD(pts, cur)
	if !ok {
		return annual.Val, annual.End, true // no prior-year period to roll against
	}
	return annual.Val + cur.Val - prior.Val, cur.End, true
}

// financialsHistoryYears bounds the multi-year trend series to the most recent ~decade.
const financialsHistoryYears = 10

// annualSeriesMerged builds a per-fiscal-year ANNUAL series for a flow concept, MERGING the given
// tags in priority order (tags[0] = highest priority) keyed by the period's END-DATE YEAR — the
// collision-proof key (see annualForEndYear) that survives a 10-K's restamped comparative columns.
// For each end-year the highest-priority tag that reports it wins (so a concept change, e.g.
// revenue's pre/post-ASC-606 tags, is bridged: the new concept covers recent years, the older
// concept fills earlier ones); within one tag, newest End then latest Filed wins (a later 10-K's
// restated value). Only full-year (~365-day) periods qualify. Returns up to maxYears most-recent
// years, OLDEST-FIRST; a year with no annual point is simply absent (never fabricated).
func annualSeriesMerged(gaap map[string]xbrlConcept, unit string, maxYears int, tags ...string) []YearValue {
	years, byYear, offset := annualChosen(gaap, unit, maxYears, tags...)
	out := make([]YearValue, 0, len(years))
	for _, y := range years {
		out = append(out, YearValue{Year: y, FY: y + offset, Val: byYear[y].Val})
	}
	return out
}

// annualChosen is the shared core: it returns the chosen annual flow point per END-DATE YEAR (the
// collision-proof merge — highest-priority tag wins a year, newest End then latest Filed within a
// tag), the most-recent maxYears years OLDEST-FIRST, plus the company-FY DISPLAY offset (from the
// latest year's authoritative fy — see annualSeriesMerged's doc). Reused by fiscalYearEnds to align
// balance-sheet instants to the income-statement fiscal-year-ends.
func annualChosen(gaap map[string]xbrlConcept, unit string, maxYears int, tags ...string) (years []int, byYear map[int]factPoint, fyOffset int) {
	type chosen struct {
		p    factPoint
		rank int // tag index — lower is higher priority
	}
	best := map[int]chosen{}
	for ti, tag := range tags {
		c, ok := gaap[tag]
		if !ok {
			continue
		}
		for _, p := range c.Units[unit] {
			if p.Start == "" {
				continue
			}
			if d := durationDays(p.Start, p.End); d < 350 || d > 380 {
				continue
			}
			y := endYear(p)
			if y == 0 {
				continue
			}
			if cur, exists := best[y]; exists {
				if cur.rank < ti {
					continue // a higher-priority tag already owns this year
				}
				if cur.rank == ti && !(p.End > cur.p.End || (p.End == cur.p.End && p.Filed > cur.p.Filed)) {
					continue
				}
			}
			best[y] = chosen{p: p, rank: ti}
		}
	}
	for y := range best {
		years = append(years, y)
	}
	sort.Ints(years) // oldest first
	if maxYears > 0 && len(years) > maxYears {
		years = years[len(years)-maxYears:] // keep the most recent maxYears
	}
	byYear = make(map[int]factPoint, len(years))
	for _, y := range years {
		byYear[y] = best[y].p
	}
	if len(years) > 0 {
		latest := byYear[years[len(years)-1]]
		fyOffset = fiscalYear(latest) - endYear(latest)
	}
	return years, byYear, fyOffset
}

// yearEnd pairs a fiscal-year-END date with its end-year + company-designated FY label.
type yearEnd struct {
	End  string
	Year int
	FY   int
}

// fiscalYearEnds returns the END date of each of the last maxYears annual flow (income-statement)
// periods — the fiscal-year-ends, OLDEST-FIRST — so balance-sheet instants can be aligned to true
// year-ends (not stray quarter-ends).
func fiscalYearEnds(gaap map[string]xbrlConcept, maxYears int, tags ...string) []yearEnd {
	years, byYear, offset := annualChosen(gaap, "USD", maxYears, tags...)
	out := make([]yearEnd, 0, len(years))
	for _, y := range years {
		out = append(out, yearEnd{End: byYear[y].End, Year: y, FY: y + offset})
	}
	return out
}

// pickInstantsByEnd merges INSTANT (point-in-time, no Start) USD points across tags by end-date —
// the highest-priority tag wins, then the latest amendment (a restated balance). For balance-sheet
// concepts (assets / liabilities / equity), which report a value AT each period-end, not over one.
func pickInstantsByEnd(gaap map[string]xbrlConcept, tags []string) map[string]factPoint {
	byEnd := map[string]factPoint{}
	rank := map[string]int{}
	for ti, tag := range tags {
		c, ok := gaap[tag]
		if !ok {
			continue
		}
		for _, p := range c.Units["USD"] {
			if p.Start != "" || p.End == "" {
				continue // instants only
			}
			if cur, exists := byEnd[p.End]; exists {
				if rank[p.End] < ti {
					continue
				}
				if rank[p.End] == ti && p.Filed <= cur.Filed {
					continue
				}
			}
			byEnd[p.End], rank[p.End] = p, ti
		}
	}
	return byEnd
}

// annualBalanceSeries returns the FISCAL-YEAR-END value of a balance-sheet (instant) concept,
// aligned to the income-statement fiscal-year-ends (so every year is the YEAR-END balance, never a
// stray quarter-end) — the instant reported AT each FY-end date. A year with no matching instant is
// absent (never fabricated). Oldest-first (fyEnds order).
func annualBalanceSeries(gaap map[string]xbrlConcept, fyEnds []yearEnd, tags ...string) []YearValue {
	byEnd := pickInstantsByEnd(gaap, tags)
	out := make([]YearValue, 0, len(fyEnds))
	for _, ye := range fyEnds {
		if p, ok := byEnd[ye.End]; ok {
			out = append(out, YearValue{Year: ye.Year, FY: ye.FY, Val: p.Val})
		}
	}
	return out
}

// fillLiabilities completes the Total-liabilities year series from Assets − Equity for any year a
// tagged us-gaap:Liabilities instant is absent — the SAME fallback the snapshot card uses (real
// reported components at the same fiscal-year-end), so the history row matches the card's coverage
// and the Assets = Liabilities + Equity identity holds for the general filer, not just taggers. A
// tagged value always wins (it is never overwritten). Result stays oldest-first.
func fillLiabilities(liab, assets, equity []YearValue) []YearValue {
	have := make(map[int]bool, len(liab))
	for _, v := range liab {
		have[v.Year] = true
	}
	eqByYear := make(map[int]YearValue, len(equity))
	for _, v := range equity {
		eqByYear[v.Year] = v
	}
	for _, a := range assets {
		if have[a.Year] {
			continue
		}
		if e, ok := eqByYear[a.Year]; ok {
			liab = append(liab, YearValue{Year: a.Year, FY: a.FY, Val: a.Val - e.Val})
		}
	}
	sort.Slice(liab, func(i, j int) bool { return liab[i].Year < liab[j].Year })
	return liab
}

// absYearValues returns the series with each value's MAGNITUDE — XBRL reports capex / buybacks /
// dividends-paid as positive, but a sign-flipped filer is normalized so a derived FCF stays correct.
func absYearValues(s []YearValue) []YearValue {
	out := make([]YearValue, len(s))
	for i, v := range s {
		v.Val = math.Abs(v.Val)
		out[i] = v
	}
	return out
}

// deriveFCF builds the per-fiscal-year FREE CASH FLOW series = operating cash flow − capex, aligned
// by year; a year present in only ONE of the two inputs is DROPPED (never a partial FCF). Capex is
// expected POSITIVE (absYearValues). Oldest-first (follows the ocf order).
func deriveFCF(ocf, capex []YearValue) []YearValue {
	capByYear := make(map[int]float64, len(capex))
	for _, c := range capex {
		capByYear[c.Year] = c.Val
	}
	out := make([]YearValue, 0, len(ocf))
	for _, o := range ocf {
		if capVal, ok := capByYear[o.Year]; ok {
			out = append(out, YearValue{Year: o.Year, FY: o.FY, Val: o.Val - capVal})
		}
	}
	return out
}

// financialsQuartersMax bounds the standalone-quarter series.
const financialsQuartersMax = 8

// Duration windows for the quarterly buckets (day counts, 52/53-week-safe; validated on real
// Micron + Apple companyfacts). A standalone single quarter is ~90d (97d in a 53-week year); the
// 9-month YTD is ~272d (279d); the full year is ~363d (370d). The 6-month YTD (~181d) falls in the
// dead zone between the standalone and 9-month windows and is intentionally never bucketed.
const (
	qStandaloneMin, qStandaloneMax = 85, 100
	qNineMoMin, qNineMoMax         = 265, 285
)

// quarterlySeries builds the last maxQ STANDALONE single-quarter values for a flow concept: the
// directly-reported ~90-day quarters (Q1-Q3) MERGED across the tag priority, plus a DERIVED Q4
// (full fiscal year − the 9-month YTD of the SAME fiscal year, matched by a shared start date).
// Each is labeled by counting back (via end-date distance, gap-robust) from the latest directly-
// reported quarter — whose fp/fy is authoritative because the newest quarter appears ONLY in its
// own 10-Q, never as a later filing's restamped comparative column. Oldest-first. Empty when there
// is no standalone quarter to anchor the labeling (an only-cumulative filer → insufficient-not-wrong).
func quarterlySeries(gaap map[string]xbrlConcept, unit string, maxQ int, tags ...string) []QuarterValue {
	type qp struct {
		start, end, fp, filed string
		val                   float64
		fy, prio              int
	}
	standalone, nineMo, annual := map[string]qp{}, map[string]qp{}, map[string]qp{}
	// Per end-date: a strictly higher-priority TAG wins outright (matching annualSeriesMerged, so
	// the two views agree on which concept a period's value comes from); WITHIN one tag the latest-
	// FILED point wins (a restatement / the un-restamped original of a comparative).
	put := func(m map[string]qp, p qp) {
		if cur, ok := m[p.end]; ok {
			if cur.prio < p.prio {
				return // a higher-priority tag already owns this end-date
			}
			if cur.prio == p.prio && p.filed <= cur.filed {
				return // same tag: keep the latest-filed point
			}
		}
		m[p.end] = p
	}
	for prio, tag := range tags {
		c, ok := gaap[tag]
		if !ok {
			continue
		}
		for _, pt := range c.Units[unit] {
			if pt.Start == "" {
				continue
			}
			p := qp{start: pt.Start, end: pt.End, fp: pt.FP, filed: pt.Filed, val: pt.Val, fy: fiscalYear(pt), prio: prio}
			switch d := durationDays(pt.Start, pt.End); {
			case d >= qStandaloneMin && d <= qStandaloneMax:
				put(standalone, p)
			case d >= qNineMoMin && d <= qNineMoMax:
				put(nineMo, p)
			case d >= 350 && d <= 380:
				put(annual, p)
			}
		}
	}
	type quarter struct {
		fp      string
		val     float64
		fy      int
		derived bool
	}
	byEnd := map[string]quarter{}
	for end, p := range standalone {
		byEnd[end] = quarter{fp: p.fp, val: p.val, fy: p.fy}
	}
	// Derive Q4 = annual − 9-month YTD, the 9-month matched by the SAME fiscal-year start; a real
	// standalone for the same end always wins over a derived one.
	nmByStart := map[string]qp{}
	for _, nm := range nineMo {
		nmByStart[nm.start] = nm
	}
	for aend, ap := range annual {
		if ex, ok := byEnd[aend]; ok && !ex.derived {
			continue
		}
		if nm, ok := nmByStart[ap.start]; ok {
			byEnd[aend] = quarter{val: ap.val - nm.val, fy: ap.fy, derived: true}
		}
	}
	if len(byEnd) == 0 {
		return nil
	}
	ends := make([]string, 0, len(byEnd))
	for end := range byEnd {
		ends = append(ends, end)
	}
	sort.Strings(ends) // oldest first (dates are YYYY-MM-DD)
	// Anchor = the newest STANDALONE quarter carrying a usable "Qn" fp + fy (authoritative).
	anchorEnd, anchorQ, anchorFY := "", 0, 0
	for i := len(ends) - 1; i >= 0; i-- {
		q := byEnd[ends[i]]
		if !q.derived && len(q.fp) == 2 && q.fp[0] == 'Q' && q.fp[1] >= '1' && q.fp[1] <= '4' {
			anchorEnd, anchorQ, anchorFY = ends[i], int(q.fp[1]-'0'), q.fy
			break
		}
	}
	if anchorEnd == "" {
		return nil // only-cumulative filer / no standalone with a quarter fp
	}
	anchorT, _ := time.Parse("2006-01-02", anchorEnd)
	label := func(end string) string {
		t, err := time.Parse("2006-01-02", end)
		if err != nil {
			return ""
		}
		qb := int(math.Round(anchorT.Sub(t).Hours() / 24 / 91.3125)) // quartersBack; negative if newer than anchor
		qi, fy := anchorQ-qb, anchorFY
		for qi < 1 {
			qi += 4
			fy--
		}
		for qi > 4 {
			qi -= 4
			fy++
		}
		return fmt.Sprintf("Q%d FY%d", qi, fy)
	}
	if maxQ > 0 && len(ends) > maxQ {
		ends = ends[len(ends)-maxQ:]
	}
	out := make([]QuarterValue, 0, len(ends))
	for _, end := range ends {
		q := byEnd[end]
		out = append(out, QuarterValue{Label: label(end), End: end, Val: q.val, Derived: q.derived})
	}
	return out
}
