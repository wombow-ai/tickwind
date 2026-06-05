// Package alpaca is a minimal client for the Alpaca Market Data API. It fetches
// the latest trade, which includes pre-market, after-hours and overnight
// prints, so Tickwind can show an all-session price. Market data works with an
// unfunded paper account, so no real money is ever at risk.
package alpaca

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

// DefaultDataURL is the Alpaca market-data base URL.
const DefaultDataURL = "https://data.alpaca.markets"

// Client fetches market data from Alpaca.
type Client struct {
	http    *http.Client
	keyID   string
	secret  string
	dataURL string
	feed    string
	loc     *time.Location
}

// New returns a Client. Empty dataURL falls back to DefaultDataURL; empty feed
// falls back to "iex" (the free feed).
func New(keyID, secret, dataURL, feed string) *Client {
	if dataURL == "" {
		dataURL = DefaultDataURL
	}
	if feed == "" {
		feed = "iex"
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		loc = time.UTC
	}
	return &Client{
		http:    &http.Client{Timeout: 10 * time.Second},
		keyID:   keyID,
		secret:  secret,
		dataURL: dataURL,
		feed:    feed,
		loc:     loc,
	}
}

type latestTradeResp struct {
	Symbol string `json:"symbol"`
	Trade  struct {
		Timestamp time.Time `json:"t"`
		Price     float64   `json:"p"`
	} `json:"trade"`
}

// LatestQuote returns the most recent trade for ticker as a store.Quote,
// including extended-hours and overnight prints.
func (c *Client) LatestQuote(ctx context.Context, ticker string) (store.Quote, error) {
	url := fmt.Sprintf("%s/v2/stocks/%s/trades/latest?feed=%s", c.dataURL, ticker, c.feed)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return store.Quote{}, err
	}
	req.Header.Set("APCA-API-KEY-ID", c.keyID)
	req.Header.Set("APCA-API-SECRET-KEY", c.secret)

	resp, err := c.http.Do(req)
	if err != nil {
		return store.Quote{}, fmt.Errorf("alpaca: get latest trade %s: %w", ticker, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return store.Quote{}, fmt.Errorf("alpaca: latest trade %s: %s", ticker, resp.Status)
	}

	var body latestTradeResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return store.Quote{}, fmt.Errorf("alpaca: decode trade %s: %w", ticker, err)
	}
	return store.Quote{
		Ticker:  ticker,
		Price:   body.Trade.Price,
		Session: c.sessionAt(body.Trade.Timestamp),
		Source:  "alpaca",
		At:      body.Trade.Timestamp,
	}, nil
}

// sessionAt classifies a US-equity trading session for a timestamp, evaluated
// in America/New_York. Holidays are not accounted for (best-effort, for display
// only): pre 04:00–09:30, regular 09:30–16:00, post 16:00–20:00, otherwise
// overnight; weekends are "closed".
func (c *Client) sessionAt(t time.Time) string {
	if t.IsZero() {
		return "closed"
	}
	et := t.In(c.loc)
	if wd := et.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return "closed"
	}
	mins := et.Hour()*60 + et.Minute()
	switch {
	case mins >= 4*60 && mins < 9*60+30:
		return "pre"
	case mins >= 9*60+30 && mins < 16*60:
		return "regular"
	case mins >= 16*60 && mins < 20*60:
		return "post"
	default:
		return "overnight"
	}
}
