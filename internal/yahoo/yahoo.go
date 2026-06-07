// Package yahoo is a minimal client for Yahoo Finance's public chart endpoint,
// used for DELAYED Hong Kong equity quotes.
//
// NOTE: this is an explicitly owner-authorized "gray" source. HK exchange quotes
// are vendor-licence-gated, so unlike the US/TW data (redistribution-clean public
// APIs), Yahoo's feed is delayed and its terms are restrictive. It is used only
// for the handful of HK tickers the owner follows, and is easy to rip out if a
// licensed feed is ever added.
package yahoo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const chartURL = "https://query1.finance.yahoo.com/v8/finance/chart/"

// Client fetches delayed quotes from Yahoo's chart endpoint.
type Client struct {
	http *http.Client
	ua   string
	base string // chart endpoint base (overridable in tests)
}

// New builds a Yahoo client. A browser-like User-Agent is required — Yahoo
// rate-limits (HTTP 429) requests without one.
func New() *Client {
	return &Client{
		http: &http.Client{Timeout: 12 * time.Second},
		ua:   "Mozilla/5.0 (compatible; Tickwind/0.1; +https://tickwind.com)",
		base: chartURL,
	}
}

// Quote is a delayed snapshot parsed from the chart endpoint's meta block.
type Quote struct {
	Price       float64
	PrevClose   float64
	Currency    string
	Name        string
	MarketState string // REGULAR | PRE | POST | CLOSED | "" (absent)
	At          time.Time
}

type chartResp struct {
	Chart struct {
		Result []struct {
			Meta struct {
				RegularMarketPrice float64 `json:"regularMarketPrice"`
				ChartPreviousClose float64 `json:"chartPreviousClose"`
				PreviousClose      float64 `json:"previousClose"`
				Currency           string  `json:"currency"`
				ShortName          string  `json:"shortName"`
				LongName           string  `json:"longName"`
				MarketState        string  `json:"marketState"`
				RegularMarketTime  int64   `json:"regularMarketTime"`
			} `json:"meta"`
		} `json:"result"`
	} `json:"chart"`
}

// Quote fetches a delayed quote for a Yahoo symbol (e.g. "0700.HK"). ok=false
// when the symbol has no price (unknown / never traded).
func (c *Client) Quote(ctx context.Context, symbol string) (Quote, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+symbol+"?interval=1d&range=1d", nil)
	if err != nil {
		return Quote{}, false, err
	}
	req.Header.Set("User-Agent", c.ua)
	resp, err := c.http.Do(req)
	if err != nil {
		return Quote{}, false, fmt.Errorf("yahoo quote %s: %w", symbol, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Quote{}, false, fmt.Errorf("yahoo quote %s: status %d", symbol, resp.StatusCode)
	}
	var cr chartResp
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return Quote{}, false, fmt.Errorf("yahoo quote %s: decode: %w", symbol, err)
	}
	if len(cr.Chart.Result) == 0 {
		return Quote{}, false, nil
	}
	m := cr.Chart.Result[0].Meta
	if m.RegularMarketPrice == 0 {
		return Quote{}, false, nil
	}
	prev := m.ChartPreviousClose
	if prev == 0 {
		prev = m.PreviousClose
	}
	name := strings.TrimSpace(m.ShortName)
	if name == "" {
		name = strings.TrimSpace(m.LongName)
	}
	at := time.Now().UTC()
	if m.RegularMarketTime > 0 {
		at = time.Unix(m.RegularMarketTime, 0).UTC()
	}
	return Quote{
		Price:       m.RegularMarketPrice,
		PrevClose:   prev,
		Currency:    m.Currency,
		Name:        name,
		MarketState: m.MarketState,
		At:          at,
	}, true, nil
}
