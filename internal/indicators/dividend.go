package indicators

import "github.com/wombow-ai/tickwind/internal/edgar"

// DividendView is a stock's dividend profile — the income-investor lens, surfacing the dividend
// figures that otherwise sit buried among the ~160 indicators. Every number is Go-computed from the
// SEC-filed annual figures (+ the live price for yield); each is DESCRIPTIVE — there is deliberately
// NO blended "dividend-safety grade" (that would read as a rating, the no-advice line). A nil field
// is unavailable (insufficient), never imputed. Period is the fiscal year the annual figures are from.
type DividendView struct {
	Yield       *float64 `json:"yield,omitempty"`        // % — annual dividends / market cap (DividendsPaid / price·shares)
	PayoutRatio *float64 `json:"payout_ratio,omitempty"` // % — dividends / net income
	DPS         *float64 `json:"dps,omitempty"`          // $ — dividends per share
	FCFCoverage *float64 `json:"fcf_coverage,omitempty"` // x — free cash flow / dividends (how many times FCF covers the payout)
	YoYGrowth   *float64 `json:"yoy_growth,omitempty"`   // % — (dividends − prior-FY dividends) / prior-FY dividends
	Period      string   `json:"period,omitempty"`       // the fiscal year of the annual figures (e.g. "FY2024")
}

// HasAny reports whether at least one dividend metric was computable.
func (d DividendView) HasAny() bool {
	return d.Yield != nil || d.PayoutRatio != nil || d.DPS != nil || d.FCFCoverage != nil || d.YoYGrowth != nil
}

// ComputeDividend builds a stock's dividend profile from its SEC fundamentals + the live price.
// Returns ok=false for a NON-PAYER (DividendsPaid <= 0) so the caller can omit the card entirely
// (insufficient-not-wrong — a non-payer has no dividend profile, not a zero one). Reuses the same
// helpers the per-stock dividend indicators use, so the card and the indicators never disagree.
func ComputeDividend(price float64, f edgar.Fundamentals) (DividendView, bool) {
	if f.DividendsPaid <= 0 {
		return DividendView{}, false
	}
	dv := DividendView{Period: f.Period}
	if v, ok := dividendYield(price, f); ok {
		dv.Yield = &v
	}
	if v, ok := payoutRatio(f); ok {
		dv.PayoutRatio = &v
	}
	if v, ok := dps(f); ok {
		dv.DPS = &v
	}
	// FCF coverage = free cash flow / dividends (x). Higher means the payout is more comfortably
	// funded by free cash flow; only meaningful when FCF is computable.
	if cf, ok := fcf(f); ok {
		cov := cf / f.DividendsPaid
		dv.FCFCoverage = &cov
	}
	// YoY dividend growth (needs a positive prior-FY figure; a cut shows as a negative growth).
	if f.DividendsPaidPrior > 0 {
		g := (f.DividendsPaid - f.DividendsPaidPrior) / f.DividendsPaidPrior * 100
		dv.YoYGrowth = &g
	}
	return dv, true
}
