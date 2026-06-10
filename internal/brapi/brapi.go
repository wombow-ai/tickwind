// Package brapi is a minimal client for brapi.dev, a free Brazilian-market
// (B3 / Bovespa) quote API. It needs a free API token (BRAPI_API_KEY); without
// one the client reports Enabled()==false and the Brazil adapter stays dark.
// Quotes are delayed/EOD-ish — an owner-authorized convenience source for the
// handful of B3 names followed, attributed honestly ("brapi"), never resold.
package brapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const baseURL = "https://brapi.dev/api/quote/"

// Client fetches B3 quotes from brapi.dev.
type Client struct {
	hc    *http.Client
	token string
	base  string // overridable in tests
}

// New builds a client from the API token (empty token → disabled).
func New(token string) *Client {
	return &Client{
		hc:    &http.Client{Timeout: 12 * time.Second},
		token: strings.TrimSpace(token),
		base:  baseURL,
	}
}

// Enabled reports whether a token is configured.
func (c *Client) Enabled() bool { return c.token != "" }

// Quote is one B3 security's latest snapshot.
type Quote struct {
	Symbol    string // bare B3 code, e.g. "PETR4"
	Name      string
	Currency  string // "BRL"
	Price     float64
	PrevClose float64
	At        time.Time
}

// Quote fetches a single B3 quote by bare code (e.g. "PETR4"). ok=false when
// the symbol has no result (unknown / not listed).
func (c *Client) Quote(ctx context.Context, symbol string) (Quote, bool, error) {
	if !c.Enabled() {
		return Quote{}, false, fmt.Errorf("brapi: no token configured")
	}
	u := c.base + url.PathEscape(symbol) + "?token=" + url.QueryEscape(c.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Quote{}, false, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Tickwind/0.1 (contact@tickwind.com)")
	resp, err := c.hc.Do(req)
	if err != nil {
		return Quote{}, false, fmt.Errorf("brapi quote %s: %w", symbol, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Quote{}, false, err
	}
	if resp.StatusCode != http.StatusOK {
		// 404 = unknown symbol (not an error worth surfacing); others bubble up.
		if resp.StatusCode == http.StatusNotFound {
			return Quote{}, false, nil
		}
		return Quote{}, false, fmt.Errorf("brapi quote %s: %s", symbol, resp.Status)
	}
	return parseQuote(body)
}

// resp mirrors the brapi /quote payload (just the fields we use).
type resp struct {
	Results []struct {
		Symbol                     string  `json:"symbol"`
		ShortName                  string  `json:"shortName"`
		LongName                   string  `json:"longName"`
		Currency                   string  `json:"currency"`
		RegularMarketPrice         float64 `json:"regularMarketPrice"`
		RegularMarketPreviousClose float64 `json:"regularMarketPreviousClose"`
		RegularMarketTime          string  `json:"regularMarketTime"`
	} `json:"results"`
	Error   bool   `json:"error"`
	Message string `json:"message"`
}

// parseQuote extracts the first result from a /quote response body.
func parseQuote(body []byte) (Quote, bool, error) {
	var r resp
	if err := json.Unmarshal(body, &r); err != nil {
		return Quote{}, false, fmt.Errorf("brapi: parse: %w", err)
	}
	if r.Error {
		return Quote{}, false, fmt.Errorf("brapi: %s", r.Message)
	}
	if len(r.Results) == 0 {
		return Quote{}, false, nil
	}
	x := r.Results[0]
	if x.RegularMarketPrice <= 0 {
		return Quote{}, false, nil
	}
	name := strings.TrimSpace(x.LongName)
	if name == "" {
		name = strings.TrimSpace(x.ShortName)
	}
	at := time.Now().UTC()
	if x.RegularMarketTime != "" {
		if parsed, err := time.Parse(time.RFC3339, x.RegularMarketTime); err == nil {
			at = parsed.UTC()
		}
	}
	return Quote{
		Symbol:    x.Symbol,
		Name:      name,
		Currency:  x.Currency,
		Price:     x.RegularMarketPrice,
		PrevClose: x.RegularMarketPreviousClose,
		At:        at,
	}, true, nil
}
