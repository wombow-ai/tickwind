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
}

// HasData reports whether any meaningful figure was extracted.
func (f Fundamentals) HasData() bool {
	return f.Shares > 0 || f.Revenue != 0 || f.NetIncome != 0 || f.EPSDiluted != 0
}

type factsResp struct {
	CIK        int    `json:"cik"`
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

	// Shares outstanding (point-in-time): dei is canonical, us-gaap as fallback.
	if p, ok := latestInstant(pick(dei, "shares", "EntityCommonStockSharesOutstanding")); ok {
		f.Shares = int64(p.Val)
	} else if p, ok := latestInstant(pick(gaap, "shares", "CommonStockSharesOutstanding")); ok {
		f.Shares = int64(p.Val)
	}
	// Fallback for multi-class / oddly-tagged issuers (e.g. MSTR) that omit a
	// point-in-time cover-page count: the latest weighted-average share count
	// (a clean single total — it's also the EPS denominator).
	if f.Shares == 0 {
		if p, ok := latestInstant(pick(gaap, "shares",
			"WeightedAverageNumberOfSharesOutstandingBasic",
			"WeightedAverageNumberOfDilutedSharesOutstanding")); ok && p.Val > 0 {
			f.Shares = int64(p.Val)
		}
	}

	// Stockholders' equity (point-in-time) — for P/B at the API layer.
	if p, ok := latestInstant(pick(gaap, "USD",
		"StockholdersEquity",
		"StockholdersEquityIncludingPortionAttributableToNoncontrollingInterest")); ok {
		f.Equity = p.Val
	}

	// Revenue (annual flow).
	if p, ok := latestAnnual(pick(gaap, "USD",
		"RevenueFromContractWithCustomerExcludingAssessedTax",
		"Revenues",
		"RevenueFromContractWithCustomerIncludingAssessedTax",
		"SalesRevenueNet")); ok {
		f.Revenue, f.Period, f.AsOf = p.Val, fiscalLabel(p), p.End
	}

	// Net income (annual flow) — can be negative (loss).
	if p, ok := latestAnnual(pick(gaap, "USD", "NetIncomeLoss", "ProfitLoss")); ok {
		f.NetIncome = p.Val
		if f.Period == "" {
			f.Period, f.AsOf = fiscalLabel(p), p.End
		}
	}

	// Diluted EPS (annual flow) — drives P/E at the API layer.
	if p, ok := latestAnnual(pick(gaap, "USD/shares",
		"EarningsPerShareDiluted", "EarningsPerShareBasic")); ok {
		f.EPSDiluted = p.Val
	}

	return f
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
	y := p.FY
	if y == 0 && len(p.End) >= 4 {
		_, _ = fmt.Sscanf(p.End[:4], "%d", &y)
	}
	return fmt.Sprintf("FY%d", y)
}
