// Package opportunity ranks the Opportunity board: small-cap US stocks where
// insiders have been buying on the open market (SEC Form 4, code P). It is pure
// (no I/O) — given recent insider buys, shares-outstanding and prices it returns
// a ranked, gated board, so it is fully unit-testable. The board is presented as
// observed, sourced facts (who bought, how much, link to the filing), never as a
// recommendation.
package opportunity

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

// Gates (anti-pump): a real small-cap with meaningful insider conviction.
const (
	MinMarketCap = 300_000_000   // $300M — excludes nano/micro pump targets
	MaxMarketCap = 2_500_000_000 // $2.5B — small-cap ceiling
	MinBuyValue  = 25_000        // $25k per filing — drops token buys
	maxTopBuyers = 4
)

// Stock is one row of the Opportunity board.
type Stock struct {
	Ticker      string    `json:"ticker"`
	CIK         int       `json:"cik"`
	Company     string    `json:"company"`
	Price       float64   `json:"price"`
	MarketCap   float64   `json:"market_cap"`
	Rank        int       `json:"rank"`
	Buyers      int       `json:"buyers"`    // distinct insiders who bought in the window
	BuyValue    float64   `json:"buy_value"` // total $ value of those buys
	BuyCount    int       `json:"buy_count"` // number of buy filings
	LastBuyDate time.Time `json:"last_buy_date"`
	Explainer   string    `json:"explainer"` // "3 insiders bought $1.2M in the last 30 days"
	TopBuyers   []Buyer   `json:"top_buyers"`
	FilingURL   string    `json:"filing_url"` // most recent Form 4 — the trust anchor
	UpdatedAt   time.Time `json:"updated_at"`
}

// Buyer is one insider's buy, for the evidence drawer.
type Buyer struct {
	Name  string    `json:"name"`
	Title string    `json:"title"`
	Date  time.Time `json:"date"`
	Value float64   `json:"value"`
}

// Recompute groups recent insider buys by ticker, gates to the small-cap band
// (market cap = shares × price, both required), and ranks by conviction:
// distinct buyers first, then total dollar value. Buzz/momentum is intentionally
// NOT a ranking input here — insider money is the signal.
func Recompute(now time.Time, buys []store.InsiderBuy, shares map[int]int64, prices map[string]float64) []Stock {
	type agg struct {
		cik       int
		company   string
		owners    map[string]struct{}
		value     float64
		count     int
		last      time.Time
		filingURL string
		buyers    []Buyer
	}
	byTicker := map[string]*agg{}
	for _, b := range buys {
		if b.Ticker == "" || b.Value < MinBuyValue {
			continue
		}
		a := byTicker[b.Ticker]
		if a == nil {
			a = &agg{cik: b.CIK, owners: map[string]struct{}{}}
			byTicker[b.Ticker] = a
		}
		if b.Company != "" {
			a.company = b.Company
		}
		if b.CIK != 0 {
			a.cik = b.CIK
		}
		a.owners[strings.ToLower(strings.TrimSpace(b.OwnerName))] = struct{}{}
		a.value += b.Value
		a.count++
		title := b.Title
		if title == "" && b.IsDirector {
			title = "Director"
		}
		a.buyers = append(a.buyers, Buyer{Name: b.OwnerName, Title: title, Date: b.FiledDate, Value: b.Value})
		if b.FiledDate.After(a.last) {
			a.last = b.FiledDate
			a.filingURL = b.FilingURL
		}
	}

	out := make([]Stock, 0, len(byTicker))
	for ticker, a := range byTicker {
		price := prices[ticker]
		sh := shares[a.cik]
		if price <= 0 || sh <= 0 {
			continue // no honest market cap → not board-worthy
		}
		mktcap := price * float64(sh)
		if mktcap < MinMarketCap || mktcap > MaxMarketCap {
			continue // small-cap gate
		}
		sort.SliceStable(a.buyers, func(i, j int) bool { return a.buyers[i].Value > a.buyers[j].Value })
		top := a.buyers
		if len(top) > maxTopBuyers {
			top = top[:maxTopBuyers]
		}
		out = append(out, Stock{
			Ticker: ticker, CIK: a.cik, Company: a.company, Price: price, MarketCap: mktcap,
			Buyers: len(a.owners), BuyValue: a.value, BuyCount: a.count, LastBuyDate: a.last,
			Explainer: explainer(len(a.owners), a.value), TopBuyers: top, FilingURL: a.filingURL,
			UpdatedAt: now,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Buyers != out[j].Buyers {
			return out[i].Buyers > out[j].Buyers
		}
		return out[i].BuyValue > out[j].BuyValue
	})
	for i := range out {
		out[i].Rank = i + 1
	}
	return out
}

func explainer(buyers int, value float64) string {
	who := "insider"
	if buyers != 1 {
		who = "insiders"
	}
	return fmt.Sprintf("%d %s bought %s on the open market in the last 30 days", buyers, who, money(value))
}

func money(v float64) string {
	switch {
	case v >= 1e6:
		return fmt.Sprintf("$%.1fM", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("$%.0fK", v/1e3)
	default:
		return fmt.Sprintf("$%.0f", v)
	}
}
