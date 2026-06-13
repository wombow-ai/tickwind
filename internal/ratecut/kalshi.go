package ratecut

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// kalshiDefaultBaseURL is the public, keyless Kalshi trade API v2 host.
const kalshiDefaultBaseURL = "https://api.elections.kalshi.com/trade-api/v2"

// maxResponseBytes caps how much of a response body we buffer; the Fed event
// ladders and rate-cut events are a few KB, so this is a generous safety bound.
const maxResponseBytes = 4 << 20 // 4 MiB

// kalshiSeriesTicker is the Fed-funds-rate decision series. Its events are one
// per FOMC meeting (event_ticker like KXFED-26OCT); each event's markets are the
// "rate above X%" threshold ladder used to read the implied post-meeting level.
const kalshiSeriesTicker = "KXFED"

const userAgent = "Tickwind/0.1 (+https://tickwind.com)"

// KalshiClient reads the implied federal-funds-rate distribution for the next
// FOMC meeting from Kalshi's KXFED series. No API key is required for these
// public market-data endpoints.
type KalshiClient struct {
	http    *http.Client
	baseURL string
	series  string
	now     func() time.Time // injectable clock for deterministic event selection
}

// NewKalshi returns a KalshiClient pointed at the public Kalshi trade API.
func NewKalshi() *KalshiClient {
	return &KalshiClient{
		http:    &http.Client{Timeout: 20 * time.Second},
		baseURL: kalshiDefaultBaseURL,
		series:  kalshiSeriesTicker,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// Source identifies the provider.
func (c *KalshiClient) Source() string { return "kalshi" }

// kalshiEventsResp mirrors GET /events?series_ticker=…&status=open.
type kalshiEventsResp struct {
	Events []kalshiEvent `json:"events"`
}

type kalshiEvent struct {
	EventTicker  string `json:"event_ticker"`
	SeriesTicker string `json:"series_ticker"`
	Title        string `json:"title"`
	StrikeDate   string `json:"strike_date"` // RFC3339, e.g. 2026-10-28T18:00:00Z
}

// kalshiEventResp mirrors GET /events/{event_ticker}?with_nested_markets=true.
type kalshiEventResp struct {
	Event struct {
		EventTicker string         `json:"event_ticker"`
		Title       string         `json:"title"`
		Markets     []kalshiMarket `json:"markets"`
	} `json:"event"`
}

// kalshiMarket is one threshold market within a Fed event. Prices arrive as
// decimal-dollar strings ("0.8900" = 89% = $0.89), already in 0–1 probability
// units. floor_strike is the rate threshold; strike_type is "greater"/"less".
type kalshiMarket struct {
	Ticker          string  `json:"ticker"`
	Status          string  `json:"status"`
	StrikeType      string  `json:"strike_type"`  // "greater" | "less" | "between"
	FloorStrike     float64 `json:"floor_strike"` // e.g. 3.25 (percent)
	CapStrike       float64 `json:"cap_strike"`
	Subtitle        string  `json:"subtitle"`      // e.g. "3.25%"
	YesSubTitle     string  `json:"yes_sub_title"` // e.g. "Above 3.25%"
	YesBidDollars   string  `json:"yes_bid_dollars"`
	YesAskDollars   string  `json:"yes_ask_dollars"`
	LastPriceDollar string  `json:"last_price_dollars"`
}

// Fetch returns the implied post-meeting federal-funds-rate distribution for the
// soonest-upcoming FOMC meeting as a normalized Market. Each Outcome is one
// rate-threshold market; its probability is the mid of yes_bid/yes_ask (falling
// back to last price). Markets that are not active or have no usable price are
// skipped. An empty result after filtering is treated as an error so a stale
// cache entry is preferred over an empty one.
func (c *KalshiClient) Fetch(ctx context.Context) (Market, error) {
	ev, err := c.nextEvent(ctx)
	if err != nil {
		return Market{}, err
	}
	full, err := c.eventMarkets(ctx, ev.EventTicker)
	if err != nil {
		return Market{}, err
	}

	asOf := c.now().Format(time.RFC3339)
	outcomes := make([]Outcome, 0, len(full.Event.Markets))
	for _, m := range full.Event.Markets {
		if !strings.EqualFold(m.Status, "active") {
			continue
		}
		prob, ok := kalshiProbability(m)
		if !ok {
			continue
		}
		outcomes = append(outcomes, Outcome{Label: kalshiLabel(m), Probability: prob})
	}
	if len(outcomes) == 0 {
		return Market{}, fmt.Errorf("kalshi: event %s had no priced active markets", ev.EventTicker)
	}

	question := strings.TrimSpace(full.Event.Title)
	if question == "" {
		question = strings.TrimSpace(ev.Title)
	}
	if question == "" {
		question = "Federal funds rate after the next FOMC meeting"
	}
	return Market{
		Source:   c.Source(),
		Question: question,
		AsOf:     asOf,
		Outcomes: sortOutcomes(outcomes),
		URL:      "https://kalshi.com/markets/" + strings.ToLower(c.series),
	}, nil
}

// nextEvent picks the open FOMC event with the soonest strike_date at or after
// now (Kalshi returns the events unsorted and includes meetings a year out).
func (c *KalshiClient) nextEvent(ctx context.Context) (kalshiEvent, error) {
	q := url.Values{}
	q.Set("series_ticker", c.series)
	q.Set("status", "open")
	q.Set("limit", "50")
	var resp kalshiEventsResp
	if err := c.getJSON(ctx, "/events?"+q.Encode(), &resp); err != nil {
		return kalshiEvent{}, err
	}
	now := c.now()
	var best kalshiEvent
	var bestTime time.Time
	for _, e := range resp.Events {
		t, err := time.Parse(time.RFC3339, e.StrikeDate)
		if err != nil {
			continue // skip events with an unparseable strike date
		}
		if t.Before(now) {
			continue // meeting already settled / in the past
		}
		if best.EventTicker == "" || t.Before(bestTime) {
			best, bestTime = e, t
		}
	}
	if best.EventTicker == "" {
		return kalshiEvent{}, fmt.Errorf("kalshi: no upcoming open %s event found", c.series)
	}
	return best, nil
}

// eventMarkets fetches one event with its nested markets.
func (c *KalshiClient) eventMarkets(ctx context.Context, eventTicker string) (kalshiEventResp, error) {
	var resp kalshiEventResp
	path := "/events/" + url.PathEscape(eventTicker) + "?with_nested_markets=true"
	if err := c.getJSON(ctx, path, &resp); err != nil {
		return kalshiEventResp{}, err
	}
	return resp, nil
}

// getJSON performs a GET against baseURL+path and decodes the JSON body.
func (c *KalshiClient) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("kalshi: build request %s: %w", path, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("kalshi: fetch %s: %w", path, err)
	}
	defer func() {
		// Drain any unread bytes so the keep-alive connection can be reused
		// cleanly (an undrained body desyncs the next response on the pool).
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("kalshi: fetch %s: %s", path, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("kalshi: read %s: %w", path, err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("kalshi: decode %s: %w", path, err)
	}
	return nil
}

// kalshiProbability derives a 0–1 probability for a market. It prefers the mid
// of a two-sided yes_bid/yes_ask quote (the market's best estimate); if only one
// side is present it uses that, and finally falls back to last_price. It returns
// ok=false when no usable, in-range price exists.
func kalshiProbability(m kalshiMarket) (float64, bool) {
	bid, bidOK := parseDollars(m.YesBidDollars)
	ask, askOK := parseDollars(m.YesAskDollars)
	switch {
	case bidOK && askOK && ask >= bid:
		return clamp01((bid + ask) / 2), true
	case bidOK:
		return clamp01(bid), true
	case askOK:
		return clamp01(ask), true
	}
	if last, ok := parseDollars(m.LastPriceDollar); ok {
		return clamp01(last), true
	}
	return 0, false
}

// kalshiLabel builds a readable threshold label from the market's strike fields,
// preferring the API's yes_sub_title / subtitle and falling back to a derived
// "Above/Below X%" form.
func kalshiLabel(m kalshiMarket) string {
	if s := strings.TrimSpace(m.YesSubTitle); s != "" {
		return s
	}
	if s := strings.TrimSpace(m.Subtitle); s != "" {
		return s
	}
	pct := strconv.FormatFloat(m.FloorStrike, 'f', -1, 64) + "%"
	switch strings.ToLower(strings.TrimSpace(m.StrikeType)) {
	case "less":
		return "Below " + pct
	case "greater":
		return "Above " + pct
	default:
		return pct
	}
}

// parseDollars parses a Kalshi decimal-dollar string ("0.8900") into a float in
// 0–1 probability units. Empty/zero-ish strings that fail to parse return ok=false.
func parseDollars(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// clamp01 bounds a probability into [0,1] (guards against odd quotes).
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
