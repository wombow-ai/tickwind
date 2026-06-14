package edgar

import (
	"context"
	"fmt"
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
	GrossProfit       float64 `json:"gross_profit,omitempty"`        // latest-FY gross profit; us-gaap:GrossProfit, else Revenue − cost of revenue
	TotalAssets       float64 `json:"total_assets,omitempty"`        // us-gaap:Assets (latest instant)
	TotalLiabilities  float64 `json:"total_liabilities,omitempty"`   // us-gaap:Liabilities (latest instant), else TotalAssets − Equity
	OperatingCashFlow float64 `json:"operating_cash_flow,omitempty"` // latest-FY cash from operations (can be <0)
	CapEx             float64 `json:"capex,omitempty"`               // latest-FY capital expenditure, stored POSITIVE
	DividendsPaid     float64 `json:"dividends_paid,omitempty"`      // latest-FY common dividends paid, stored POSITIVE; 0 for non-payers
	RevenuePrior      float64 `json:"revenue_prior,omitempty"`       // prior-FY revenue (for YoY growth)
	NetIncomePrior    float64 `json:"net_income_prior,omitempty"`    // prior-FY net income (for YoY growth)

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
	// annualForFY(FY−1), balance-sheet instants via the prior period-end instant).
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
		// Prior-year revenue (same concept, FY = chosenFY−1) for YoY growth.
		if p, ok := annualForFY(revPts, fiscalYear(revPt)-1); ok {
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
		// Prior-year net income (same concept) for YoY growth.
		if p, ok := annualForFY(niPts, fiscalYear(niPt)-1); ok {
			f.NetIncomePrior = p.Val
		}
	}

	// Diluted EPS (annual flow) — drives P/E at the API layer. Keep the concept's
	// full series + chosen point so the prior-FY diluted EPS can be matched
	// (Group 3 eps-growth).
	epsPts := pick(gaap, "USD/shares", "EarningsPerShareDiluted", "EarningsPerShareBasic")
	if p, ok := latestAnnual(epsPts); ok {
		f.EPSDiluted = p.Val
		if pp, ok := annualForFY(epsPts, fiscalYear(p)-1); ok {
			f.EPSDilutedPrior = pp.Val
		}
	}

	// Basic EPS (annual flow) — exposed as fundamental.eps-basic.
	if p, ok := latestAnnual(pick(gaap, "USD/shares", "EarningsPerShareBasic")); ok {
		f.EPSBasic = p.Val
	}

	// Gross profit (annual flow): prefer the reported concept; else derive
	// Revenue − cost of revenue for the SAME fiscal period as Revenue. Keep the
	// reported concept's series so the prior-FY gross profit can be matched
	// (Group 3 gp-growth); the derived path has no clean prior pair, so
	// GrossProfitPrior stays 0 (insufficient) for derived filers.
	gpPts := pick(gaap, "USD", "GrossProfit")
	if p, ok := latestAnnual(gpPts); ok {
		f.GrossProfit = p.Val
		if pp, ok := annualForFY(gpPts, fiscalYear(p)-1); ok {
			f.GrossProfitPrior = pp.Val
		}
	} else if revOK {
		for _, cogsTag := range []string{"CostOfRevenue", "CostOfGoodsAndServicesSold", "CostOfGoodsSold"} {
			if cp, ok := annualForFY(pick(gaap, "USD", cogsTag), fiscalYear(revPt)); ok {
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

	// Common dividends paid (annual flow) — stored positive; 0 for non-payers.
	if p, ok := latestAnnual(pick(gaap, "USD",
		"PaymentsOfDividendsCommonStock", "PaymentsOfDividends")); ok {
		f.DividendsPaid = abs(p.Val)
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
		// chosen FY−1 so the YoY pair is consistent (mirrors RevenuePrior).
		if pp, ok := annualForFY(opPts, fiscalYear(p)-1); ok {
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
			if cp, ok := annualForFY(pick(gaap, "USD", cogsTag), fiscalYear(revPt)); ok {
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

// annualForFY returns the full-year flow fact for a specific fiscal year (the
// same ~365-day duration filter as latestAnnual), preferring the latest
// amendment. Used to pull a prior-year value or a same-period cost of revenue
// for the SAME concept, so a YoY pair or derived margin stays consistent.
func annualForFY(pts []factPoint, fy int) (factPoint, bool) {
	var best factPoint
	found := false
	for _, p := range pts {
		if p.Start == "" || fiscalYear(p) != fy {
			continue
		}
		if d := durationDays(p.Start, p.End); d < 350 || d > 380 {
			continue
		}
		if !found || p.Filed > best.Filed {
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
