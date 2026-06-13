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
//     edgar.Fundamentals exposes the latest-FY diluted EPS, not a TTM net-income
//     series, and per the shared contract this is computed as price / EPSDiluted
//     (mathematically equal to market cap / (EPS·shares) when EPS is the same FY
//     denominator). It is an annual (FY) P/E, not a strict trailing-4-quarter one.
//   - roe: the catalog formula uses AVERAGE equity attributable to parent;
//     Fundamentals carries only the latest period-end equity, so this uses the
//     latest equity (a point-in-time ROE), per the shared contract.

// peTTM returns price / diluted EPS. ok=false when EPS is non-positive (a loss
// or zero), for which P/E is not meaningful. Plain multiple (Unit "x").
func peTTM(price float64, f edgar.Fundamentals) (float64, bool) {
	if price <= 0 || f.EPSDiluted <= 0 {
		return 0, false
	}
	return price / f.EPSDiluted, true
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
