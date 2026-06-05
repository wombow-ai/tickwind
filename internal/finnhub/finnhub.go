// Package finnhub is a minimal client for the Finnhub company-news API. A free
// API token is required (https://finnhub.io). News ingestion is optional in
// Tickwind — without a token, it is simply skipped.
package finnhub

import (
	"context"
	"encoding/json"
	"fmt"
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
