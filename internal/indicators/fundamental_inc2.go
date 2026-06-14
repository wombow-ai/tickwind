package indicators

import "github.com/wombow-ai/tickwind/internal/edgar"

// This file holds the PURE fundamental-ratio math for design §1.2 Groups 1, 2 and
// 4 (income-statement, current-balance-sheet, debt/EV/capital-structure). It is the
// Increment-2 sibling of fundamental.go's Group-0 set and follows the same contract:
// every function returns (value, ok); ok=false signals an insufficient/missing input
// (an absent concept → a 0 field, a zero/negative denominator, or a meaningless
// ratio). The compute layer turns ok=false into Status="insufficient" with a concrete
// reason and NEVER fabricates a number.
//
// Anti-fabrication note: a concept absent from a filer's companyfacts leaves the
// corresponding Fundamentals field 0; each helper treats a 0 numerator-concept that
// would read spuriously (e.g. EBIT inputs, inventory, debt) as insufficient rather
// than emitting a 0. Sign-bearing flows that may legitimately be 0 or negative
// (operating income, pre-tax income, EBIT itself) are gated on their denominator.
//
// Unit convention (set by the compute layer, mirrored here):
//   - Percent (Unit "%"): opm, ebitMargin, preTaxMargin, ebitdaMargin, roce, roic,
//     opGrowth, equityGrowth, assetGrowth, gpGrowth, epsGrowth, rdIntensity,
//     buybackYield, ltDebtRatio, goodwillToEquity.
//   - Plain multiples (Unit "x"): icrTIE, inventoryTurnover, receivablesTurnover,
//     payablesTurnover, currentRatio, quickRatio, cashRatio, fixedAssetTurnover,
//     currentAssetTurnover, wcTurnover, ocfRatio, netGearing, cashToSTDebt, dtnw,
//     tangiblePB, ev, evToSales, evToFCF, evToEBITDA.
//   - Day counts (Unit ""): dio, dso, dpo, ccc.
//   - Per-share USD (Unit ""): epsBasic (stated diluted as a proxy when basic absent).
//
// Point-in-time convention matches roe()/peTTM(): balance-sheet ratios use the latest
// period-end values; turnover ratios use the latest balance (the average-denominator
// variant needs a prior-FY balance we do not faithfully hold yet — documented inline).

// daysPerYear is the day-count basis for DIO/DSO/DPO. The catalog formulas use 365.
const daysPerYear = 365.0

// --- Group 1: income-statement-derived quantities & ratios ---

// ebit returns earnings before interest & taxes (a USD flow that may be negative,
// ok-gated only by data presence). It prefers the reported OperatingIncomeLoss; when
// that concept is absent it derives EBIT = net income + interest expense + income
// tax (the catalog's ebit-margin formula). ok=false only when NEITHER a reported
// operating income NOR the interest+tax inputs are present, so EBIT cannot be formed
// without fabrication. (A genuine 0 EBIT with present inputs is ok.)
func ebit(f edgar.Fundamentals) (float64, bool) {
	if f.OperatingIncomeLoss != 0 {
		return f.OperatingIncomeLoss, true
	}
	// Derived path: requires the interest & tax flows to be present (else NI alone
	// would masquerade as EBIT). Both 0 → the concepts are absent → insufficient.
	if f.InterestExpense == 0 && f.IncomeTaxExpense == 0 {
		return 0, false
	}
	return f.NetIncome + f.InterestExpense + f.IncomeTaxExpense, true
}

// ebitda returns operating EBITDA = operating income + depreciation & amortization
// (a USD flow). ok=false when OperatingIncomeLoss is absent (0) or D&A is absent
// (0) — both concepts are required for a faithful EBITDA, so a missing one is
// insufficient rather than a partial fabrication.
func ebitda(f edgar.Fundamentals) (float64, bool) {
	if f.OperatingIncomeLoss == 0 || f.DepreciationAmort == 0 {
		return 0, false
	}
	return f.OperatingIncomeLoss + f.DepreciationAmort, true
}

// preTaxIncome returns pre-tax income from continuing operations. It prefers the
// reported concept; when absent it derives net income + income tax (the standard
// fallback) provided the tax line is present. ok=false when neither is available.
func preTaxIncome(f edgar.Fundamentals) (float64, bool) {
	if f.PreTaxIncome != 0 {
		return f.PreTaxIncome, true
	}
	if f.IncomeTaxExpense == 0 {
		return 0, false
	}
	return f.NetIncome + f.IncomeTaxExpense, true
}

// opm returns operating margin = operating income / revenue as a PERCENT. ok=false
// when revenue is non-positive or operating income is absent (0, which would read as
// a spurious 0% margin).
func opm(f edgar.Fundamentals) (float64, bool) {
	if f.Revenue <= 0 || f.OperatingIncomeLoss == 0 {
		return 0, false
	}
	return f.OperatingIncomeLoss / f.Revenue * 100, true
}

// ebitMargin returns EBIT / revenue as a PERCENT. ok=false when revenue is
// non-positive or EBIT cannot be formed from the available data.
func ebitMargin(f edgar.Fundamentals) (float64, bool) {
	if f.Revenue <= 0 {
		return 0, false
	}
	e, ok := ebit(f)
	if !ok {
		return 0, false
	}
	return e / f.Revenue * 100, true
}

// preTaxMargin returns pre-tax income / revenue as a PERCENT. ok=false when revenue
// is non-positive or pre-tax income cannot be formed.
func preTaxMargin(f edgar.Fundamentals) (float64, bool) {
	if f.Revenue <= 0 {
		return 0, false
	}
	p, ok := preTaxIncome(f)
	if !ok {
		return 0, false
	}
	return p / f.Revenue * 100, true
}

// ebitdaMargin returns EBITDA / revenue as a PERCENT. ok=false when revenue is
// non-positive or EBITDA cannot be formed (operating income or D&A absent).
func ebitdaMargin(f edgar.Fundamentals) (float64, bool) {
	if f.Revenue <= 0 {
		return 0, false
	}
	e, ok := ebitda(f)
	if !ok {
		return 0, false
	}
	return e / f.Revenue * 100, true
}

// icrTIE returns the interest-coverage ratio (times interest earned) = EBIT /
// interest expense (Unit "x"). ok=false when interest expense is non-positive (no
// debt service to cover, ratio undefined) or EBIT cannot be formed.
func icrTIE(f edgar.Fundamentals) (float64, bool) {
	if f.InterestExpense <= 0 {
		return 0, false
	}
	e, ok := ebit(f)
	if !ok {
		return 0, false
	}
	return e / f.InterestExpense, true
}

// roce returns return on capital employed = EBIT / (total assets − current
// liabilities) as a PERCENT. ok=false when total assets or current liabilities is
// absent, the denominator (capital employed) is non-positive, or EBIT cannot be
// formed. Point-in-time: latest balances.
func roce(f edgar.Fundamentals) (float64, bool) {
	if f.TotalAssets <= 0 || f.LiabilitiesCurrent <= 0 {
		return 0, false
	}
	capitalEmployed := f.TotalAssets - f.LiabilitiesCurrent
	if capitalEmployed <= 0 {
		return 0, false
	}
	e, ok := ebit(f)
	if !ok {
		return 0, false
	}
	return e / capitalEmployed * 100, true
}

// interestBearingDebt returns long-term debt + current debt (the EV / gearing /
// invested-capital debt term). ok=false when BOTH debt concepts are absent (0), so
// a debt figure is never fabricated from a debt-free balance sheet — a genuinely
// debt-free firm reports insufficient for the debt-dependent ratios rather than a 0.
func interestBearingDebt(f edgar.Fundamentals) (float64, bool) {
	if f.LongTermDebt == 0 && f.DebtCurrent == 0 {
		return 0, false
	}
	return f.LongTermDebt + f.DebtCurrent, true
}

// roic returns return on invested capital = NOPAT / invested capital as a PERCENT,
// where NOPAT = EBIT × (1 − tax rate), tax rate = income tax / pre-tax income, and
// invested capital = interest-bearing debt + equity − cash. ok=false when any input
// is missing or the denominator is non-positive. The tax rate is clamped to [0, 1)
// so a negative or >100% effective rate (a tax benefit or a tiny pre-tax base) does
// not invert NOPAT; when pre-tax income is non-positive the rate falls back to 0
// (NOPAT = EBIT, documented).
func roic(f edgar.Fundamentals) (float64, bool) {
	e, ok := ebit(f)
	if !ok {
		return 0, false
	}
	debt, ok := interestBearingDebt(f)
	if !ok {
		return 0, false
	}
	if f.Equity == 0 {
		return 0, false
	}
	invested := debt + f.Equity - f.CashAndEquivalents
	if invested <= 0 {
		return 0, false
	}
	taxRate := 0.0
	if pre, pok := preTaxIncome(f); pok && pre > 0 {
		taxRate = f.IncomeTaxExpense / pre
		if taxRate < 0 {
			taxRate = 0
		}
		if taxRate > 1 {
			taxRate = 1
		}
	}
	nopat := e * (1 - taxRate)
	return nopat / invested * 100, true
}

// opGrowth returns (operating income − prior operating income) / |prior| as a
// PERCENT (the prior abs-normalized so a swing out of an operating loss reads
// positive). ok=false when the prior-FY operating income is 0 (no usable base).
func opGrowth(f edgar.Fundamentals) (float64, bool) {
	if f.OperatingIncomeLossPrior == 0 {
		return 0, false
	}
	base := f.OperatingIncomeLossPrior
	if base < 0 {
		base = -base
	}
	return (f.OperatingIncomeLoss - f.OperatingIncomeLossPrior) / base * 100, true
}

// --- Group 2: current-balance-sheet ratios ---

// currentRatio returns current assets / current liabilities (Unit "x"). ok=false
// when either is non-positive.
func currentRatio(f edgar.Fundamentals) (float64, bool) {
	if f.AssetsCurrent <= 0 || f.LiabilitiesCurrent <= 0 {
		return 0, false
	}
	return f.AssetsCurrent / f.LiabilitiesCurrent, true
}

// quickRatio returns (current assets − inventory) / current liabilities (the acid
// test, Unit "x"). ok=false when current assets or current liabilities is
// non-positive, or inventory is absent (0) — without an inventory figure the quick
// ratio collapses to the current ratio, which would mislabel it, so it is reported
// insufficient.
func quickRatio(f edgar.Fundamentals) (float64, bool) {
	if f.AssetsCurrent <= 0 || f.LiabilitiesCurrent <= 0 || f.InventoryNet == 0 {
		return 0, false
	}
	return (f.AssetsCurrent - f.InventoryNet) / f.LiabilitiesCurrent, true
}

// cashRatio returns cash & equivalents / current liabilities (Unit "x"). ok=false
// when current liabilities is non-positive or cash is absent (0).
func cashRatio(f edgar.Fundamentals) (float64, bool) {
	if f.LiabilitiesCurrent <= 0 || f.CashAndEquivalents == 0 {
		return 0, false
	}
	return f.CashAndEquivalents / f.LiabilitiesCurrent, true
}

// inventoryTurnover returns cost of revenue / inventory (Unit "x"; point-in-time
// ending inventory, not the average). ok=false when inventory is non-positive or
// cost of revenue is absent (0).
func inventoryTurnover(f edgar.Fundamentals) (float64, bool) {
	if f.InventoryNet <= 0 || f.CostOfRevenue == 0 {
		return 0, false
	}
	return f.CostOfRevenue / f.InventoryNet, true
}

// dio returns days inventory outstanding = 365 / inventory turnover (Unit "",
// days). ok=false whenever inventory turnover is insufficient.
func dio(f edgar.Fundamentals) (float64, bool) {
	t, ok := inventoryTurnover(f)
	if !ok || t <= 0 {
		return 0, false
	}
	return daysPerYear / t, true
}

// receivablesTurnover returns revenue / accounts receivable (Unit "x";
// point-in-time ending AR). ok=false when AR is non-positive or revenue is
// non-positive.
func receivablesTurnover(f edgar.Fundamentals) (float64, bool) {
	if f.AccountsReceivable <= 0 || f.Revenue <= 0 {
		return 0, false
	}
	return f.Revenue / f.AccountsReceivable, true
}

// dso returns days sales outstanding = 365 / receivables turnover (Unit "", days).
// ok=false whenever receivables turnover is insufficient.
func dso(f edgar.Fundamentals) (float64, bool) {
	t, ok := receivablesTurnover(f)
	if !ok || t <= 0 {
		return 0, false
	}
	return daysPerYear / t, true
}

// payablesTurnover returns cost of revenue / accounts payable (Unit "x";
// point-in-time ending AP). ok=false when AP is non-positive or cost of revenue is
// absent (0).
func payablesTurnover(f edgar.Fundamentals) (float64, bool) {
	if f.AccountsPayable <= 0 || f.CostOfRevenue == 0 {
		return 0, false
	}
	return f.CostOfRevenue / f.AccountsPayable, true
}

// dpoDays returns days payable outstanding = 365 / payables turnover (Unit "",
// days). ok=false whenever payables turnover is insufficient. (Named dpoDays to
// avoid colliding with the technical DPO oscillator's namespace; the catalog id is
// fundamental.dpo.)
func dpoDays(f edgar.Fundamentals) (float64, bool) {
	t, ok := payablesTurnover(f)
	if !ok || t <= 0 {
		return 0, false
	}
	return daysPerYear / t, true
}

// ccc returns the cash conversion cycle = DIO + DSO − DPO (Unit "", days). ok=false
// when ANY of the three component day-counts is insufficient — a partial CCC would
// silently drop a term, so it is reported insufficient rather than a misleading
// value.
func ccc(f edgar.Fundamentals) (float64, bool) {
	d1, ok1 := dio(f)
	d2, ok2 := dso(f)
	d3, ok3 := dpoDays(f)
	if !ok1 || !ok2 || !ok3 {
		return 0, false
	}
	return d1 + d2 - d3, true
}

// fixedAssetTurnover returns revenue / net property, plant & equipment (Unit "x").
// ok=false when net PP&E is non-positive or revenue is non-positive.
func fixedAssetTurnover(f edgar.Fundamentals) (float64, bool) {
	if f.PropertyPlantNet <= 0 || f.Revenue <= 0 {
		return 0, false
	}
	return f.Revenue / f.PropertyPlantNet, true
}

// currentAssetTurnover returns revenue / current assets (Unit "x"). ok=false when
// current assets is non-positive or revenue is non-positive.
func currentAssetTurnover(f edgar.Fundamentals) (float64, bool) {
	if f.AssetsCurrent <= 0 || f.Revenue <= 0 {
		return 0, false
	}
	return f.Revenue / f.AssetsCurrent, true
}

// wcTurnover returns revenue / working capital (Unit "x"), where working capital =
// current assets − current liabilities. ok=false when current assets or current
// liabilities is non-positive, the working capital is non-positive (a negative-WC
// firm makes the turnover meaningless), or revenue is non-positive.
func wcTurnover(f edgar.Fundamentals) (float64, bool) {
	if f.AssetsCurrent <= 0 || f.LiabilitiesCurrent <= 0 || f.Revenue <= 0 {
		return 0, false
	}
	wc := f.AssetsCurrent - f.LiabilitiesCurrent
	if wc <= 0 {
		return 0, false
	}
	return f.Revenue / wc, true
}

// ocfRatio returns operating cash flow / current liabilities (Unit "x"). ok=false
// when current liabilities is non-positive or operating cash flow is absent (0).
func ocfRatio(f edgar.Fundamentals) (float64, bool) {
	if f.LiabilitiesCurrent <= 0 || f.OperatingCashFlow == 0 {
		return 0, false
	}
	return f.OperatingCashFlow / f.LiabilitiesCurrent, true
}

// --- Group 4: debt / EV / capital-structure ratios ---

// ltDebtRatio returns long-term debt / (long-term debt + equity) as a PERCENT — the
// canonical long-term-debt-to-capitalization ratio (StockCharts/Investopedia). The
// numerator is interest-bearing long-term debt (f.LongTermDebt), NOT total long-term
// liabilities. ok=false when long-term debt is absent (0) or the denominator is
// non-positive.
func ltDebtRatio(f edgar.Fundamentals) (float64, bool) {
	if f.LongTermDebt == 0 {
		return 0, false
	}
	denom := f.LongTermDebt + f.Equity
	if denom <= 0 {
		return 0, false
	}
	return f.LongTermDebt / denom * 100, true
}

// netGearing returns (interest-bearing debt − cash) / equity as a PERCENT. ok=false
// when equity is non-positive or interest-bearing debt cannot be formed (the firm
// is debt-free → the gearing concept is insufficient rather than a fabricated 0).
func netGearing(f edgar.Fundamentals) (float64, bool) {
	if f.Equity <= 0 {
		return 0, false
	}
	debt, ok := interestBearingDebt(f)
	if !ok {
		return 0, false
	}
	return (debt - f.CashAndEquivalents) / f.Equity * 100, true
}

// cashToSTDebt returns cash & equivalents / short-term interest-bearing debt (Unit
// "x"). ok=false when current debt is non-positive (no short-term debt to cover) or
// cash is absent (0).
func cashToSTDebt(f edgar.Fundamentals) (float64, bool) {
	if f.DebtCurrent <= 0 || f.CashAndEquivalents == 0 {
		return 0, false
	}
	return f.CashAndEquivalents / f.DebtCurrent, true
}

// tangibleEquity returns equity − goodwill − intangibles (the tangible net worth /
// tangible book value). ok=false when neither goodwill nor intangibles is present
// (then tangible equity == equity and the dtnw/tbv distinction is meaningless, so a
// plain-equity result would mislabel — report insufficient) or the result is
// non-positive (a firm with intangibles exceeding equity has no meaningful tangible
// book value).
func tangibleEquity(f edgar.Fundamentals) (float64, bool) {
	if f.Goodwill == 0 && f.IntangiblesExGoodwill == 0 {
		return 0, false
	}
	te := f.Equity - f.Goodwill - f.IntangiblesExGoodwill
	if te <= 0 {
		return 0, false
	}
	return te, true
}

// dtnw returns debt to tangible net worth = total liabilities / (equity − goodwill −
// intangibles) (Unit "x"). ok=false when tangible net worth is insufficient.
func dtnw(f edgar.Fundamentals) (float64, bool) {
	te, ok := tangibleEquity(f)
	if !ok {
		return 0, false
	}
	return f.TotalLiabilities / te, true
}

// goodwillToEquity returns goodwill / equity as a PERCENT. ok=false when goodwill is
// absent (0) or equity is non-positive.
func goodwillToEquity(f edgar.Fundamentals) (float64, bool) {
	if f.Goodwill == 0 || f.Equity <= 0 {
		return 0, false
	}
	return f.Goodwill / f.Equity * 100, true
}

// tangiblePB returns the tangible price-to-book = market cap / tangible book value =
// price·shares / (equity − goodwill − intangibles) (Unit "x"). ok=false when price
// or shares is non-positive, or tangible book value is insufficient.
func tangiblePB(price float64, f edgar.Fundamentals) (float64, bool) {
	if price <= 0 || f.Shares <= 0 {
		return 0, false
	}
	te, ok := tangibleEquity(f)
	if !ok {
		return 0, false
	}
	return price * float64(f.Shares) / te, true
}

// ev returns enterprise value = market cap + interest-bearing debt − cash (Unit "x"
// per the catalog output, a USD dollar amount headline; minority interest and
// preferred stock default to 0 as documented). ok=false when market cap cannot be
// formed (no price/shares) or interest-bearing debt cannot be formed (a debt-free
// firm — EV still requires the debt concept to be present to be faithful; a
// genuinely debt-free firm would have EV = mktcap − cash, but without a debt line we
// cannot distinguish "no debt" from "concept absent", so report insufficient).
func ev(price float64, f edgar.Fundamentals) (float64, bool) {
	mc, ok := marketCap(price, f)
	if !ok {
		return 0, false
	}
	debt, ok := interestBearingDebt(f)
	if !ok {
		return 0, false
	}
	return mc + debt - f.CashAndEquivalents, true
}

// evToSales returns EV / revenue (Unit "x"). ok=false when EV cannot be formed or
// revenue is non-positive.
func evToSales(price float64, f edgar.Fundamentals) (float64, bool) {
	if f.Revenue <= 0 {
		return 0, false
	}
	v, ok := ev(price, f)
	if !ok {
		return 0, false
	}
	return v / f.Revenue, true
}

// evToFCF returns EV / free cash flow (Unit "x"), FCF = operating cash flow − capex.
// ok=false when EV cannot be formed, FCF is insufficient (no OCF), or FCF is
// non-positive (a negative-FCF firm makes the multiple meaningless).
func evToFCF(price float64, f edgar.Fundamentals) (float64, bool) {
	v, ok := ev(price, f)
	if !ok {
		return 0, false
	}
	free, ok := fcf(f)
	if !ok || free <= 0 {
		return 0, false
	}
	return v / free, true
}

// evToEBITDA returns EV / EBITDA (Unit "x"), EBITDA = operating income + D&A.
// ok=false when EV cannot be formed, EBITDA is insufficient, or EBITDA is
// non-positive (a negative-EBITDA firm makes the multiple meaningless).
func evToEBITDA(price float64, f edgar.Fundamentals) (float64, bool) {
	v, ok := ev(price, f)
	if !ok {
		return 0, false
	}
	e, ok := ebitda(f)
	if !ok || e <= 0 {
		return 0, false
	}
	return v / e, true
}

// rdIntensity returns R&D expense / revenue as a PERCENT. ok=false when revenue is
// non-positive or R&D is absent (0, a non-R&D filer → insufficient, not a 0%).
func rdIntensity(f edgar.Fundamentals) (float64, bool) {
	if f.Revenue <= 0 || f.ResearchDevelopment == 0 {
		return 0, false
	}
	return f.ResearchDevelopment / f.Revenue * 100, true
}

// buybackYield returns annual buyback amount / market cap as a PERCENT. ok=false
// when there is no buyback (0, a non-repurchaser → insufficient) or market cap
// cannot be formed (no price/shares).
func buybackYield(price float64, f edgar.Fundamentals) (float64, bool) {
	if f.BuybackAmount <= 0 {
		return 0, false
	}
	mc, ok := marketCap(price, f)
	if !ok {
		return 0, false
	}
	return f.BuybackAmount / mc * 100, true
}
