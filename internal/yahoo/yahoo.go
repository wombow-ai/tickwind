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

// seriesResp decodes the chart's intraday close series + timestamps, used by
// ExtendedQuote to read the freshest pre/post-market print (the meta block's
// regularMarketPrice freezes at the 16:00 ET close, so it can't see extended
// hours — but the includePrePost minute series keeps printing).
type seriesResp struct {
	Chart struct {
		Result []struct {
			Meta struct {
				ChartPreviousClose float64 `json:"chartPreviousClose"`
				PreviousClose      float64 `json:"previousClose"`
			} `json:"meta"`
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Close []*float64 `json:"close"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
	} `json:"chart"`
}

// ExtendedQuote returns the freshest consolidated last trade for symbol —
// INCLUDING pre- and post-market — by reading Yahoo's 1-minute chart with
// extended hours enabled. price/at are the last printed close and its time
// (which can be a pre/post print that free Alpaca IEX and Finnhub's free
// /quote both miss); prevClose is the prior regular-session close. ok=false
// when Yahoo has no usable series.
//
// This is the pre/post-aware backstop for thin US names: free IEX has sparse
// extended-hours prints and Finnhub's free /quote freezes at the regular
// close, so without this an after-hours mover shows a stale close.
func (c *Client) ExtendedQuote(ctx context.Context, symbol string) (price, prevClose float64, at time.Time, ok bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+symbol+"?includePrePost=true&interval=1m&range=1d", nil)
	if err != nil {
		return 0, 0, time.Time{}, false, err
	}
	req.Header.Set("User-Agent", c.ua)
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, 0, time.Time{}, false, fmt.Errorf("yahoo extended %s: %w", symbol, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, time.Time{}, false, fmt.Errorf("yahoo extended %s: status %d", symbol, resp.StatusCode)
	}
	var cr seriesResp
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return 0, 0, time.Time{}, false, fmt.Errorf("yahoo extended %s: decode: %w", symbol, err)
	}
	if len(cr.Chart.Result) == 0 || len(cr.Chart.Result[0].Indicators.Quote) == 0 {
		return 0, 0, time.Time{}, false, nil
	}
	r := cr.Chart.Result[0]
	prev := r.Meta.ChartPreviousClose
	if prev == 0 {
		prev = r.Meta.PreviousClose
	}
	closes := r.Indicators.Quote[0].Close
	// Walk back to the last non-null close — the freshest print, in whichever
	// session is currently active (pre / regular / post).
	for i := len(closes) - 1; i >= 0; i-- {
		if closes[i] == nil || *closes[i] <= 0 {
			continue
		}
		at = time.Now().UTC()
		if i < len(r.Timestamp) {
			at = time.Unix(r.Timestamp[i], 0).UTC()
		}
		return *closes[i], prev, at, true, nil
	}
	return 0, 0, time.Time{}, false, nil
}

// Consolidated adapts a *Client to the pre/post-aware freshness-fallback shape
// ingest.ConsolidatedQuoter expects (its method is named Quote, which the HK
// snapshot Quote already occupies — hence this thin wrapper around
// ExtendedQuote).
type Consolidated struct{ *Client }

// Quote returns the freshest extended-hours print, satisfying the consolidated
// quoter interface used by the price freshness fallback.
func (c Consolidated) Quote(ctx context.Context, symbol string) (price, prevClose float64, at time.Time, ok bool, err error) {
	return c.Client.ExtendedQuote(ctx, symbol)
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
