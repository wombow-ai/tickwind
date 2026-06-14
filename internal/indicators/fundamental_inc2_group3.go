package indicators

import "github.com/wombow-ai/tickwind/internal/edgar"

// This file holds the PURE Group-3 (design §1.2) growth/per-share ratios that can be
// computed FAITHFULLY from companyfacts prior-FY values: the prior diluted EPS,
// gross profit, equity and total assets the extractor pulls via annualForFY(FY−1) /
// priorInstant. Each returns (value, ok); ok=false when the prior-period value is
// absent (0) — NEVER a fabricated prior. Growth rates are PERCENT (Unit "%"); basic
// EPS is a stated per-share USD amount (Unit "").
//
// The Group-3 ratios whose prior value cannot be pulled faithfully are intentionally
// NOT registered (so they are simply absent from the response, never faked): in
// particular gp-growth for filers whose gross profit is DERIVED (Revenue − COGS) has
// no clean prior pair, so GrossProfitPrior is 0 and gp-growth reports insufficient
// for them — correct, not invented.

// epsGrowth returns (EPS − priorEPS) / |priorEPS| as a PERCENT (the prior
// abs-normalized so a swing out of a loss reads as positive growth, mirroring
// earningsGrowthYoY). ok=false when the prior-FY diluted EPS is 0 (no usable base).
func epsGrowth(f edgar.Fundamentals) (float64, bool) {
	if f.EPSDilutedPrior == 0 {
		return 0, false
	}
	base := f.EPSDilutedPrior
	if base < 0 {
		base = -base
	}
	return (f.EPSDiluted - f.EPSDilutedPrior) / base * 100, true
}

// equityGrowth returns (equity − priorEquity) / priorEquity as a PERCENT. ok=false
// when the prior period-end equity is non-positive (no usable base — a prior
// negative-equity firm makes the percent growth meaningless).
func equityGrowth(f edgar.Fundamentals) (float64, bool) {
	if f.EquityPrior <= 0 {
		return 0, false
	}
	return (f.Equity - f.EquityPrior) / f.EquityPrior * 100, true
}

// assetGrowth returns (assets − priorAssets) / priorAssets as a PERCENT. ok=false
// when the prior period-end total assets is non-positive (no usable base).
func assetGrowth(f edgar.Fundamentals) (float64, bool) {
	if f.TotalAssetsPrior <= 0 {
		return 0, false
	}
	return (f.TotalAssets - f.TotalAssetsPrior) / f.TotalAssetsPrior * 100, true
}

// gpGrowth returns (gross profit − priorGP) / priorGP as a PERCENT. ok=false when
// the prior-FY gross profit is non-positive (no usable base; also 0 for filers
// whose gross profit is DERIVED, which have no faithful prior pair).
func gpGrowth(f edgar.Fundamentals) (float64, bool) {
	if f.GrossProfitPrior <= 0 {
		return 0, false
	}
	return (f.GrossProfit - f.GrossProfitPrior) / f.GrossProfitPrior * 100, true
}

// epsBasic returns the latest-FY basic EPS exactly as extracted (a USD per-share
// amount, Unit ""). It may be negative (a loss). ok=false only when the concept was
// absent (EPSBasic == 0), which the extractor leaves zero.
func epsBasic(f edgar.Fundamentals) (float64, bool) {
	if f.EPSBasic == 0 {
		return 0, false
	}
	return f.EPSBasic, true
}
