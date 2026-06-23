package indicators

import (
	"sort"

	"github.com/wombow-ai/tickwind/internal/edgar"
)

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

// TickerDividend pairs a ticker with its dividend profile — the unit the market-wide dividend
// leaderboard ranks. (A bare DividendView is ticker-less, for the per-stock card.)
type TickerDividend struct {
	Ticker   string
	Dividend DividendView
}

// DividendRank is one row of the market-wide dividend leaderboard: a ticker and its full Go-computed
// dividend profile (so the UI can show yield + payout + growth + coverage regardless of which view
// ranked it). A DISCLOSED HISTORICAL/AS-FILED statistic, never a rating/advice — the metrics are
// descriptive and Period names the fiscal year they are from. (Mirrors RSRank / FactorRank.)
type DividendRank struct {
	Ticker      string   `json:"ticker"`
	Yield       *float64 `json:"yield,omitempty"`
	PayoutRatio *float64 `json:"payout_ratio,omitempty"`
	DPS         *float64 `json:"dps,omitempty"`
	FCFCoverage *float64 `json:"fcf_coverage,omitempty"`
	YoYGrowth   *float64 `json:"yoy_growth,omitempty"`
	Period      string   `json:"period,omitempty"`
}

// Dividend leaderboard VIEWS (each its own crawlable pSEO surface). Each ranks ONLY the payers whose
// relevant metric is computable (a nil metric → omitted, insufficient-not-wrong):
//
//	highest-yield    — largest trailing dividend yield (Yield, high→low)
//	fastest-growing  — largest YoY dividend growth (YoYGrowth, high→low)
//	best-covered     — payout covered the most times by free cash flow (FCFCoverage, high→low)
//	lowest-payout    — smallest fraction of earnings paid out (PayoutRatio, low→high; positive only)
const (
	DividendViewHighestYield   = "highest-yield"
	DividendViewFastestGrowing = "fastest-growing"
	DividendViewBestCovered    = "best-covered"
	DividendViewLowestPayout   = "lowest-payout"
)

// ValidDividendView reports whether view is a known dividend leaderboard view.
func ValidDividendView(view string) bool {
	switch view {
	case DividendViewHighestYield, DividendViewFastestGrowing, DividendViewBestCovered, DividendViewLowestPayout:
		return true
	}
	return false
}

// RankDividend ranks a tracked-universe population by the chosen VIEW and returns the leaderboard.
// Only payers whose view-relevant metric is computable are included (a nil metric is omitted, never
// imputed). All but lowest-payout sort high→low; lowest-payout sorts low→high and includes only
// POSITIVE payout ratios (a non-positive ratio comes from a loss-maker and is not a meaningful "low
// payout"). Every row carries the full Go-computed profile; ties break by ticker (stable +
// deterministic). An unknown view yields nil. Pure ranking arithmetic — no compute, no I/O.
func RankDividend(pop []TickerDividend, view string) []DividendRank {
	if !ValidDividendView(view) {
		return nil
	}
	type scored struct {
		row DividendRank
		val float64
	}
	rows := make([]scored, 0, len(pop))
	for _, p := range pop {
		if p.Ticker == "" {
			continue
		}
		d := p.Dividend
		var metric *float64
		switch view {
		case DividendViewHighestYield:
			metric = d.Yield
		case DividendViewFastestGrowing:
			metric = d.YoYGrowth
		case DividendViewBestCovered:
			metric = d.FCFCoverage
		case DividendViewLowestPayout:
			metric = d.PayoutRatio
		}
		if metric == nil {
			continue // this view's metric isn't computable for the ticker → omit (insufficient)
		}
		// A non-positive ranking metric is meaningless for these views (and would otherwise sort to the
		// tail): a <=0 payout ratio is a loss-maker, and <=0 FCF coverage means free cash flow did NOT
		// cover the payout — so a cash-burning payer can't belong on a "best covered by FCF" board.
		if *metric <= 0 && (view == DividendViewLowestPayout || view == DividendViewBestCovered) {
			continue
		}
		rows = append(rows, scored{
			row: DividendRank{
				Ticker: p.Ticker, Yield: d.Yield, PayoutRatio: d.PayoutRatio, DPS: d.DPS,
				FCFCoverage: d.FCFCoverage, YoYGrowth: d.YoYGrowth, Period: d.Period,
			},
			val: *metric,
		})
	}
	asc := view == DividendViewLowestPayout
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].val != rows[j].val {
			if asc {
				return rows[i].val < rows[j].val
			}
			return rows[i].val > rows[j].val
		}
		return rows[i].row.Ticker < rows[j].row.Ticker // deterministic tie-break
	})
	out := make([]DividendRank, len(rows))
	for i, s := range rows {
		out[i] = s.row
	}
	return out
}
