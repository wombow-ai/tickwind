package ratecut

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

// polymarketDefaultBaseURL is the public, keyless Polymarket Gamma API host.
const polymarketDefaultBaseURL = "https://gamma-api.polymarket.com"

// polymarketEventSlug is the macro Fed rate-cut event we read by default:
// "How many Fed rate cuts in 2026?" — a clean, mutually-exclusive multi-bucket
// (negRisk) market whose Yes prices sum to ~1, mapping directly to a count-of-
// cuts distribution. Targeting a fixed macro slug deliberately avoids election
// and other politically contentious markets.
const polymarketEventSlug = "how-many-fed-rate-cuts-in-2026"

// PolymarketClient reads a Fed rate-cut distribution from the Polymarket Gamma
// API. No API key is required for these public market-data endpoints.
type PolymarketClient struct {
	http    *http.Client
	baseURL string
	slug    string
	now     func() time.Time
}

// NewPolymarket returns a PolymarketClient pointed at the public Gamma API,
// reading the default macro rate-cut event.
func NewPolymarket() *PolymarketClient {
	return &PolymarketClient{
		http:    &http.Client{Timeout: 20 * time.Second},
		baseURL: polymarketDefaultBaseURL,
		slug:    polymarketEventSlug,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// polymarketEvent mirrors one event from GET /events?slug=…. Each nested market
// is a binary Yes/No sub-question; for a negRisk multi-outcome event the Yes
// price of each is the probability of that bucket.
type polymarketEvent struct {
	Title   string             `json:"title"`
	Slug    string             `json:"slug"`
	Closed  bool               `json:"closed"`
	Markets []polymarketMarket `json:"markets"`
}

type polymarketMarket struct {
	Question       string `json:"question"`
	GroupItemTitle string `json:"groupItemTitle"` // bucket label, e.g. "1 (25 bps)"
	Closed         bool   `json:"closed"`
	Active         bool   `json:"active"`
	// Outcomes and OutcomePrices are JSON-encoded string arrays, e.g.
	// outcomes: "[\"Yes\", \"No\"]", outcomePrices: "[\"0.155\", \"0.845\"]".
	Outcomes      string `json:"outcomes"`
	OutcomePrices string `json:"outcomePrices"`
}

// Fetch returns the normalized rate-cut distribution for the configured macro
// event. Each Outcome is one bucket (the market's groupItemTitle) with its Yes
// probability. Closed/inactive sub-markets and any with an unparseable price are
// skipped. An event that yields no usable outcomes is an error (prefer a stale
// cache entry over an empty one).
func (c *PolymarketClient) Fetch(ctx context.Context) (Market, error) {
	q := url.Values{}
	q.Set("slug", c.slug)
	var events []polymarketEvent
	if err := c.getJSON(ctx, "/events?"+q.Encode(), &events); err != nil {
		return Market{}, err
	}
	if len(events) == 0 {
		return Market{}, fmt.Errorf("polymarket: event %q not found", c.slug)
	}
	ev := events[0]

	outcomes := make([]Outcome, 0, len(ev.Markets))
	for _, m := range ev.Markets {
		if m.Closed {
			continue
		}
		prob, ok := polymarketYesProbability(m)
		if !ok {
			continue
		}
		label := strings.TrimSpace(m.GroupItemTitle)
		if label == "" {
			label = strings.TrimSpace(m.Question)
		}
		if label == "" {
			continue
		}
		outcomes = append(outcomes, Outcome{Label: label, Probability: prob})
	}
	if len(outcomes) == 0 {
		return Market{}, fmt.Errorf("polymarket: event %q had no priced open markets", c.slug)
	}

	question := strings.TrimSpace(ev.Title)
	if question == "" {
		question = "How many Fed rate cuts?"
	}
	return Market{
		Source:   c.Source(),
		Question: question,
		AsOf:     c.now().Format(time.RFC3339),
		Outcomes: sortOutcomes(outcomes),
		URL:      "https://polymarket.com/event/" + ev.Slug,
	}, nil
}

// Source identifies the provider.
func (c *PolymarketClient) Source() string { return "polymarket" }

// getJSON performs a GET against baseURL+path and decodes the JSON body.
func (c *PolymarketClient) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("polymarket: build request %s: %w", path, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("polymarket: fetch %s: %w", path, err)
	}
	defer func() {
		// Drain any unread bytes so the keep-alive connection can be reused
		// cleanly (an undrained body desyncs the next response on the pool).
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("polymarket: fetch %s: %s", path, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("polymarket: read %s: %w", path, err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("polymarket: decode %s: %w", path, err)
	}
	return nil
}

// polymarketYesProbability extracts the "Yes" leg probability (0–1) from a
// binary sub-market by pairing the decoded outcomes/outcomePrices arrays. It
// returns ok=false when the arrays are malformed, mismatched, or carry no "Yes".
func polymarketYesProbability(m polymarketMarket) (float64, bool) {
	labels, ok := decodeStringArray(m.Outcomes)
	if !ok {
		return 0, false
	}
	prices, ok := decodeFloatArray(m.OutcomePrices)
	if !ok {
		return 0, false
	}
	if len(labels) != len(prices) || len(labels) == 0 {
		return 0, false
	}
	for i, l := range labels {
		if strings.EqualFold(strings.TrimSpace(l), "Yes") {
			return clamp01(prices[i]), true
		}
	}
	return 0, false
}

// decodeStringArray decodes a JSON-string-encoded array of strings, e.g.
// `["Yes", "No"]`.
func decodeStringArray(s string) ([]string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, false
	}
	return out, true
}

// decodeFloatArray decodes a JSON-string-encoded array of stringified floats,
// e.g. `["0.155", "0.845"]`, into float64s. Gamma encodes the numbers as
// strings, so the array element type is string.
func decodeFloatArray(s string) ([]float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	var raw []string
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return nil, false
	}
	out := make([]float64, 0, len(raw))
	for _, r := range raw {
		v, ok := parseDollars(r)
		if !ok {
			return nil, false
		}
		out = append(out, v)
	}
	return out, true
}
