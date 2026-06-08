// Package finnhub is a minimal client for the Finnhub company-news API. A free
// API token is required (https://finnhub.io). News ingestion is optional in
// Tickwind — without a token, it is simply skipped.
package finnhub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

const baseURL = "https://finnhub.io/api/v1"

// Client fetches data from Finnhub.
type Client struct {
	http  *http.Client
	token string
}

// New returns a Client that authenticates with the given API token.
func New(token string) *Client {
	return &Client{http: &http.Client{Timeout: 15 * time.Second}, token: token}
}

type newsItem struct {
	ID       int64  `json:"id"`
	Datetime int64  `json:"datetime"` // unix seconds
	Headline string `json:"headline"`
	Summary  string `json:"summary"`
	Source   string `json:"source"`
	URL      string `json:"url"`
}

// CompanyNews returns company news for ticker over the last `days` days.
func (c *Client) CompanyNews(ctx context.Context, ticker string, days int) ([]store.News, error) {
	now := time.Now().UTC()
	q := url.Values{}
	q.Set("symbol", ticker)
	q.Set("from", now.AddDate(0, 0, -days).Format("2006-01-02"))
	q.Set("to", now.Format("2006-01-02"))
	q.Set("token", c.token)
	endpoint := baseURL + "/company-news?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("finnhub: company-news %s: %w", ticker, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("finnhub: company-news %s: %s", ticker, resp.Status)
	}

	var items []newsItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("finnhub: decode news %s: %w", ticker, err)
	}
	out := make([]store.News, 0, len(items))
	for _, it := range items {
		out = append(out, store.News{
			Ticker:    ticker,
			ID:        strconv.FormatInt(it.ID, 10),
			Headline:  it.Headline,
			Summary:   it.Summary,
			Source:    it.Source,
			URL:       it.URL,
			Published: time.Unix(it.Datetime, 0).UTC(),
		})
	}
	return out, nil
}

type earningsResp struct {
	EarningsCalendar []earningRow `json:"earningsCalendar"`
}

type earningRow struct {
	Date            string   `json:"date"` // YYYY-MM-DD
	Symbol          string   `json:"symbol"`
	Hour            string   `json:"hour"` // bmo | amc | dmh | ""
	EPSEstimate     *float64 `json:"epsEstimate"`
	EPSActual       *float64 `json:"epsActual"`
	RevenueEstimate *float64 `json:"revenueEstimate"`
	RevenueActual   *float64 `json:"revenueActual"`
}

// EarningsCalendar returns scheduled/reported earnings between from and to
// (inclusive). Used by the earnings-calendar feature.
func (c *Client) EarningsCalendar(ctx context.Context, from, to time.Time) ([]store.Earning, error) {
	q := url.Values{}
	q.Set("from", from.Format("2006-01-02"))
	q.Set("to", to.Format("2006-01-02"))
	q.Set("token", c.token)
	endpoint := baseURL + "/calendar/earnings?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("finnhub: earnings calendar: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("finnhub: earnings calendar: %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("finnhub: read earnings: %w", err)
	}
	return parseEarningsCalendar(data)
}

// parseEarningsCalendar parses the Finnhub earnings-calendar payload, dropping
// rows missing a symbol or a parseable date. Pure (testable without network).
func parseEarningsCalendar(data []byte) ([]store.Earning, error) {
	var body earningsResp
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, fmt.Errorf("finnhub: decode earnings: %w", err)
	}
	out := make([]store.Earning, 0, len(body.EarningsCalendar))
	for _, r := range body.EarningsCalendar {
		if r.Symbol == "" || r.Date == "" {
			continue
		}
		d, err := time.Parse("2006-01-02", r.Date)
		if err != nil {
			continue
		}
		out = append(out, store.Earning{
			Ticker:          r.Symbol,
			Date:            d,
			Hour:            r.Hour,
			EPSEstimate:     r.EPSEstimate,
			EPSActual:       r.EPSActual,
			RevenueEstimate: r.RevenueEstimate,
			RevenueActual:   r.RevenueActual,
		})
	}
	return out, nil
}
