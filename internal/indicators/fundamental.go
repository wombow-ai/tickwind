package indicators

import "github.com/wombow-ai/tickwind/internal/edgar"

// This file holds the PURE fundamental-ratio math over edgar.Fundamentals plus a
// live price. Every function returns (value, ok) where ok=false signals an
// insufficient/missing input (a zero or negative denominator, an absent concept,
// or — for P/E — non-positive earnings). The compute layer turns ok=false into a
// StockIndicator with Status="insufficient" and a concrete reason; it NEVER
// fabricates a number.
//
// Unit convention (see the shared contract): margins, ROE, growth rates,
// dividend yield, and the debt-to-asset ratio are returned as PERCENT values
// (e.g. a 0.42 ratio is returned as 42.0, to be rendered with Unit "%"). P/E and
// P/B are plain multiples returned as-is (Unit "x"). FCF is a USD dollar amount
// (Unit ""). The callers in compute.go set the Unit string accordingly.
//
// Dataset-faithfulness notes (the catalog formulas are authoritative; where the
// available data forces an approximation it is documented here, not silently
// changed):
//   - pe-ttm: the catalog formula is "market cap / trailing 4-quarter net income".
//     Computed as price / TRAILING-12-MONTH diluted EPS (edgar.Fundamentals'
//     EPSDilutedTTM — annual + current-FY-to-date − prior-year-to-date), the same
//     numerator the public fundamentals card's pe_ttm uses, so the two never disagree.
//     Falls back to the latest-FY diluted EPS only when no TTM is computable (a filer
//     with too thin a quarterly history) — degrades to annual, never to insufficient.
//   - roe: the catalog formula uses AVERAGE equity attributable to parent;
//     Fundamentals carries only the latest period-end equity, so this uses the
//     latest equity (a point-in-time ROE), per the shared contract.

// peTTM returns price / TRAILING-12-MONTH diluted EPS (matching the public card's pe_ttm),
// falling back to the latest annual diluted EPS only when no TTM is computable. ok=false when the
// chosen EPS is non-positive (a loss or zero), for which P/E is not meaningful. Plain multiple ("x").
func peTTM(price float64, f edgar.Fundamentals) (float64, bool) {
	eps := f.EPSDilutedTTM
	if eps == 0 { // no TTM (thin quarterly history) → fall back to the latest annual diluted EPS
		eps = f.EPSDiluted
	}
	if price <= 0 || eps <= 0 {
		return 0, false
	}
	return price / eps, true
}

// pb returns market cap / equity = price·shares / equity. ok=false when price,
// shares, or equity is non-positive. Plain multiple (Unit "x").
func pb(price float64, f edgar.Fundamentals) (float64, bool) {
	if price <= 0 || f.Shares <= 0 || f.Equity <= 0 {
		return 0, false
	}
	return price * float64(f.Shares) / f.Equity, true
}

// roe returns net income / equity as a PERCENT. ok=false when equity is
// non-positive (a negative-equity firm makes the ratio meaningless). Net income
// may be negative → a negative ROE percent.
func roe(f edgar.Fundamentals) (float64, bool) {
	if f.Equity <= 0 {
		return 0, false
	}
	return f.NetIncome / f.Equity * 100, true
}

// npm returns net income / revenue as a PERCENT (net margin). ok=false when
// revenue is non-positive.
func npm(f edgar.Fundamentals) (float64, bool) {
	if f.Revenue <= 0 {
		return 0, false
	}
	return f.NetIncome / f.Revenue * 100, true
}

// gpm returns gross profit / revenue as a PERCENT (gross margin). ok=false when
// revenue is non-positive or gross profit is absent (0, which would make the
// margin spuriously 0%).
func gpm(f edgar.Fundamentals) (float64, bool) {
	if f.Revenue <= 0 || f.GrossProfit == 0 {
		return 0, false
	}
	return f.GrossProfit / f.Revenue * 100, true
}

// revenueGrowthYoY returns (revenue − priorRevenue) / priorRevenue as a PERCENT.
// ok=false when prior-year revenue is non-positive (no usable base).
func revenueGrowthYoY(f edgar.Fundamentals) (float64, bool) {
	if f.RevenuePrior <= 0 {
		return 0, false
	}
	return (f.Revenue - f.RevenuePrior) / f.RevenuePrior * 100, true
}

// earningsGrowthYoY returns (netIncome − priorNetIncome) / |priorNetIncome| as a
// PERCENT, per the catalog formula (the prior period is abs-normalized so a swing
// out of a loss reads as positive growth). ok=false when prior net income is 0
// (no base to grow from).
func earningsGrowthYoY(f edgar.Fundamentals) (float64, bool) {
	if f.NetIncomePrior == 0 {
		return 0, false
	}
	base := f.NetIncomePrior
	if base < 0 {
		base = -base
	}
	return (f.NetIncome - f.NetIncomePrior) / base * 100, true
}

// fcf returns operating cash flow − capex as a USD dollar amount (Unit ""). It is
// ok whenever operating cash flow is present (non-zero); capex defaults to 0 when
// absent, which yields FCF = OCF. ok=false only when operating cash flow is 0
// (the concept was absent).
func fcf(f edgar.Fundamentals) (float64, bool) {
	if f.OperatingCashFlow == 0 {
		return 0, false
	}
	return f.OperatingCashFlow - f.CapEx, true
}

// dividendYield returns total common dividends paid / market cap as a PERCENT
// (a market-cap dividend yield: DividendsPaid / (price·shares)). ok=false when
// there are no dividends (a non-payer), or no price/shares to size the market cap.
func dividendYield(price float64, f edgar.Fundamentals) (float64, bool) {
	if f.DividendsPaid <= 0 || price <= 0 || f.Shares <= 0 {
		return 0, false
	}
	marketCap := price * float64(f.Shares)
	return f.DividendsPaid / marketCap * 100, true
}

// debtToAsset returns total liabilities / total assets as a PERCENT. ok=false
// when total assets is non-positive (no base).
func debtToAsset(f edgar.Fundamentals) (float64, bool) {
	if f.TotalAssets <= 0 {
		return 0, false
	}
	return f.TotalLiabilities / f.TotalAssets * 100, true
}

// --- Group 0 ratios (no new XBRL: existing Fundamentals fields + price) ---
//
// These follow the design-doc §1.2 Group-0 formula table exactly. Every function
// is a deterministic latest scalar over edgar.Fundamentals (+ price) and returns
// (value, ok); ok=false on a missing input or a zero/negative denominator, so the
// compute layer reports insufficient rather than fabricating a number. The
// point-in-time convention matches roe()/peTTM(): balance-sheet ratios use the
// latest period-end values (not an average across two fiscal years), and per-share
// items use the latest share count, documented here where the catalog formula's
// canonical denominator (e.g. AVERAGE total assets) is approximated by the latest
// value we hold.
//
// Unit convention (set by the compute layer, mirrored here for clarity):
//   - USD dollar amounts (Unit ""): marketCap, capex, dps, cfps, fcfps, bvps, sps.
//   - Plain multiples (Unit "x"): ps, debtToEquity, equityMultiplier, ocfToNI,
//     ocfCFO, fcfConversion, pcf, tobinsQ, peLYR.
//   - Percent (Unit "%"): roa, gpToAssets, assetTurnover, capexToSales, fcfYield,
//     payoutRatio, retentionRatio, sgr, accruals.

// marketCap returns price · shares (a USD dollar amount, Unit ""). ok=false when
// price or shares is non-positive (no live quote or no share count).
func marketCap(price float64, f edgar.Fundamentals) (float64, bool) {
	if price <= 0 || f.Shares <= 0 {
		return 0, false
	}
	return price * float64(f.Shares), true
}

// epsDiluted returns the latest-FY diluted EPS exactly as extracted (a USD
// per-share amount, Unit ""). It may be negative (a loss). ok=false only when the
// concept was absent (EPSDiluted == 0), which the extractor leaves zero.
func epsDiluted(f edgar.Fundamentals) (float64, bool) {
	if f.EPSDiluted == 0 {
		return 0, false
	}
	return f.EPSDiluted, true
}

// bvps returns book value per share = equity / shares (a USD per-share amount,
// Unit ""). ok=false when equity is non-positive (a negative-equity firm has no
// meaningful book value per share) or shares is non-positive.
func bvps(f edgar.Fundamentals) (float64, bool) {
	if f.Equity <= 0 || f.Shares <= 0 {
		return 0, false
	}
	return f.Equity / float64(f.Shares), true
}

// sps returns sales (revenue) per share = revenue / shares (a USD per-share
// amount, Unit ""). ok=false when revenue is non-positive or shares is
// non-positive.
func sps(f edgar.Fundamentals) (float64, bool) {
	if f.Revenue <= 0 || f.Shares <= 0 {
		return 0, false
	}
	return f.Revenue / float64(f.Shares), true
}

// ps returns the price-to-sales multiple = market cap / revenue = price·shares /
// revenue (Unit "x"). ok=false when price, shares, or revenue is non-positive.
func ps(price float64, f edgar.Fundamentals) (float64, bool) {
	if price <= 0 || f.Shares <= 0 || f.Revenue <= 0 {
		return 0, false
	}
	return price * float64(f.Shares) / f.Revenue, true
}

// debtToEquity returns total liabilities / equity (the D/E multiple, Unit "x").
// ok=false when equity is non-positive (a negative-equity firm makes the ratio
// meaningless).
func debtToEquity(f edgar.Fundamentals) (float64, bool) {
	if f.Equity <= 0 {
		return 0, false
	}
	return f.TotalLiabilities / f.Equity, true
}

// equityMultiplier returns total assets / equity (the leverage multiple, Unit
// "x"). ok=false when equity is non-positive or total assets is non-positive.
func equityMultiplier(f edgar.Fundamentals) (float64, bool) {
	if f.Equity <= 0 || f.TotalAssets <= 0 {
		return 0, false
	}
	return f.TotalAssets / f.Equity, true
}

// roa returns return on assets = net income / total assets as a PERCENT. Net
// income may be negative → a negative ROA percent. ok=false when total assets is
// non-positive. Point-in-time: the catalog's canonical denominator is AVERAGE
// total assets, but Fundamentals carries only the latest period-end assets, so
// this uses the latest assets (as roe() uses the latest equity).
func roa(f edgar.Fundamentals) (float64, bool) {
	if f.TotalAssets <= 0 {
		return 0, false
	}
	return f.NetIncome / f.TotalAssets * 100, true
}

// gpToAssets returns gross-profitability = gross profit / total assets as a
// PERCENT (the Novy-Marx ratio). ok=false when total assets is non-positive or
// gross profit is absent (0, which would read as a spurious 0%).
func gpToAssets(f edgar.Fundamentals) (float64, bool) {
	if f.TotalAssets <= 0 || f.GrossProfit == 0 {
		return 0, false
	}
	return f.GrossProfit / f.TotalAssets * 100, true
}

// assetTurnover returns total-asset turnover = revenue / total assets (a turnover
// multiple, Unit "x"). ok=false when revenue is non-positive or total assets is
// non-positive. Point-in-time: latest assets (the average-denominator variant
// needs a prior-FY balance we do not yet hold).
func assetTurnover(f edgar.Fundamentals) (float64, bool) {
	if f.Revenue <= 0 || f.TotalAssets <= 0 {
		return 0, false
	}
	return f.Revenue / f.TotalAssets, true
}

// ocfToNI returns the operating-cash-flow-to-net-income ratio = operating cash
// flow / net income (Unit "x"). ok=false when net income is non-positive (a
// loss-maker makes the quality ratio meaningless) or operating cash flow is
// absent (0).
func ocfToNI(f edgar.Fundamentals) (float64, bool) {
	if f.NetIncome <= 0 || f.OperatingCashFlow == 0 {
		return 0, false
	}
	return f.OperatingCashFlow / f.NetIncome, true
}

// ocfCFO returns operating cash flow as stated (a USD dollar amount, Unit ""). It
// may be negative. ok=false only when the concept was absent (OperatingCashFlow
// == 0). Despite the "x" siblings, this is a stated dollar flow, so the compute
// layer sets Unit "".
func ocfCFO(f edgar.Fundamentals) (float64, bool) {
	if f.OperatingCashFlow == 0 {
		return 0, false
	}
	return f.OperatingCashFlow, true
}

// fcfConversion returns free cash flow / net income (Unit "x"), where FCF =
// operating cash flow − capex. ok=false when net income is non-positive or
// operating cash flow is absent (0, which fcf() reports insufficient).
func fcfConversion(f edgar.Fundamentals) (float64, bool) {
	if f.NetIncome <= 0 {
		return 0, false
	}
	v, ok := fcf(f)
	if !ok {
		return 0, false
	}
	return v / f.NetIncome, true
}

// capex returns capital expenditure as a USD dollar amount (Unit ""), stored
// positive by the extractor. ok=false when capex is absent (0).
func capex(f edgar.Fundamentals) (float64, bool) {
	if f.CapEx == 0 {
		return 0, false
	}
	return f.CapEx, true
}

// capexToSales returns capex / revenue as a PERCENT (capital intensity). ok=false
// when revenue is non-positive or capex is absent (0).
func capexToSales(f edgar.Fundamentals) (float64, bool) {
	if f.Revenue <= 0 || f.CapEx == 0 {
		return 0, false
	}
	return f.CapEx / f.Revenue * 100, true
}

// fcfYield returns free cash flow / market cap as a PERCENT, where FCF =
// operating cash flow − capex and market cap = price·shares. ok=false when price
// or shares is non-positive, or operating cash flow is absent (0). FCF may be
// negative → a negative yield.
func fcfYield(price float64, f edgar.Fundamentals) (float64, bool) {
	if price <= 0 || f.Shares <= 0 {
		return 0, false
	}
	v, ok := fcf(f)
	if !ok {
		return 0, false
	}
	return v / (price * float64(f.Shares)) * 100, true
}

// pcf returns the price-to-cash-flow multiple = market cap / operating cash flow
// = price·shares / operating cash flow (Unit "x"). ok=false when price or shares
// is non-positive, or operating cash flow is non-positive (a negative-OCF firm
// makes the multiple meaningless).
func pcf(price float64, f edgar.Fundamentals) (float64, bool) {
	if price <= 0 || f.Shares <= 0 || f.OperatingCashFlow <= 0 {
		return 0, false
	}
	return price * float64(f.Shares) / f.OperatingCashFlow, true
}

// cfps returns cash flow per share = operating cash flow / shares (a USD
// per-share amount, Unit ""). It may be negative. ok=false when shares is
// non-positive or operating cash flow is absent (0).
func cfps(f edgar.Fundamentals) (float64, bool) {
	if f.Shares <= 0 || f.OperatingCashFlow == 0 {
		return 0, false
	}
	return f.OperatingCashFlow / float64(f.Shares), true
}

// fcfps returns free cash flow per share = (operating cash flow − capex) / shares
// (a USD per-share amount, Unit ""). It may be negative. ok=false when shares is
// non-positive or operating cash flow is absent (0, which fcf() reports
// insufficient).
func fcfps(f edgar.Fundamentals) (float64, bool) {
	if f.Shares <= 0 {
		return 0, false
	}
	v, ok := fcf(f)
	if !ok {
		return 0, false
	}
	return v / float64(f.Shares), true
}

// payoutRatio returns dividends paid / net income as a PERCENT. ok=false when net
// income is non-positive (a payout out of a loss is not a meaningful ratio) or
// there are no dividends (DividendsPaid == 0, a non-payer).
func payoutRatio(f edgar.Fundamentals) (float64, bool) {
	if f.NetIncome <= 0 || f.DividendsPaid <= 0 {
		return 0, false
	}
	return f.DividendsPaid / f.NetIncome * 100, true
}

// retentionRatio returns 1 − payout as a PERCENT (the plowback ratio). ok=false
// whenever the payout ratio is insufficient (no positive net income or no
// dividends), so a non-payer is reported insufficient rather than a fabricated
// 100%.
func retentionRatio(f edgar.Fundamentals) (float64, bool) {
	p, ok := payoutRatio(f)
	if !ok {
		return 0, false
	}
	return 100 - p, true
}

// dps returns dividends per share = dividends paid / shares (a USD per-share
// amount, Unit ""). ok=false when shares is non-positive or there are no
// dividends (a non-payer).
func dps(f edgar.Fundamentals) (float64, bool) {
	if f.Shares <= 0 || f.DividendsPaid <= 0 {
		return 0, false
	}
	return f.DividendsPaid / float64(f.Shares), true
}

// sgr returns the sustainable growth rate = ROE × retention as a PERCENT, where
// ROE is the fraction net income / equity and retention is 1 − payout (a
// fraction). Computed from roe() (a percent) × the retention fraction, which
// yields the percent SGR directly. ok=false when ROE is insufficient
// (non-positive equity) or retention is insufficient (no positive net income or
// no dividends).
func sgr(f edgar.Fundamentals) (float64, bool) {
	r, ok := roe(f)
	if !ok {
		return 0, false
	}
	ret, ok := retentionRatio(f)
	if !ok {
		return 0, false
	}
	return r * (ret / 100), true
}

// accruals returns the balance-sheet accruals ratio = (net income − operating
// cash flow) / total assets as a PERCENT (Sloan accruals; a higher value flags
// lower earnings quality). ok=false when total assets is non-positive or
// operating cash flow is absent (0, which would make accruals == ROA spuriously).
func accruals(f edgar.Fundamentals) (float64, bool) {
	if f.TotalAssets <= 0 || f.OperatingCashFlow == 0 {
		return 0, false
	}
	return (f.NetIncome - f.OperatingCashFlow) / f.TotalAssets * 100, true
}

// tobinsQ returns Tobin's Q ≈ (market cap + total liabilities) / total assets
// (Unit "x"), using market cap as a proxy for the market value of equity. ok=false
// when price or shares is non-positive (no market cap) or total assets is
// non-positive.
func tobinsQ(price float64, f edgar.Fundamentals) (float64, bool) {
	if price <= 0 || f.Shares <= 0 || f.TotalAssets <= 0 {
		return 0, false
	}
	return (price*float64(f.Shares) + f.TotalLiabilities) / f.TotalAssets, true
}

// peLYR returns the last-year (prior-FY) P/E = market cap / prior-year net income
// = price·shares / NetIncomePrior (Unit "x"). ok=false when price or shares is
// non-positive, or prior-year net income is non-positive (a prior loss makes the
// multiple meaningless).
func peLYR(price float64, f edgar.Fundamentals) (float64, bool) {
	if price <= 0 || f.Shares <= 0 || f.NetIncomePrior <= 0 {
		return 0, false
	}
	return price * float64(f.Shares) / f.NetIncomePrior, true
}
