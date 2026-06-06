// Package alphavantage is a client for the Alpha Vantage NEWS_SENTIMENT API,
// producing a per-ticker news-sentiment store.Signal. The free tier allows only
// 25 requests/day, so the client self-budgets (an internal daily cap) and
// caches: one bulk call covers many tickers and is refreshed at most every
// refreshInterval; between refreshes (or once the budget is spent) it serves the
// cached snapshot. It satisfies the ingest SignalSource shape. An empty API key
// disables the source (Signals is a no-op).
package alphavantage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

// defaultBaseURL is the Alpha Vantage API host.
const defaultBaseURL = "https://www.alphavantage.co"

// maxTickers caps how many requested tickers we aggregate sentiment for in one
// pass. This is an in-memory bound only: there is no API-side ticker filter
// (see fetch), so it just protects against an unbounded watchlist.
const maxTickers = 250

// Client fetches and aggregates per-ticker news sentiment from Alpha Vantage.
// It is safe for concurrent use.
type Client struct {
	http    *http.Client
	baseURL string
	apiKey  string

	refreshInterval time.Duration // min spacing between live fetches
	dailyCap        int           // max requests per UTC day (free tier is 25)
	now             func() time.Time

	mu         sync.Mutex
	cache      []store.Signal
	lastFetch  time.Time
	callsToday int
	day        string // UTC date that callsToday is counted against ("2006-01-02")
}

// New returns a Client for the given API key. An empty key disables the source.
// Defaults respect the free tier: at most dailyCap (20, leaving headroom under
// 25) live requests per UTC day, no more often than every 90 minutes.
func New(apiKey string) *Client {
	return &Client{
		http:            &http.Client{Timeout: 15 * time.Second},
		baseURL:         defaultBaseURL,
		apiKey:          apiKey,
		refreshInterval: 90 * time.Minute,
		dailyCap:        20,
		now:             time.Now,
	}
}

// Name identifies the source.
func (c *Client) Name() string { return "alphavantage" }

// feedResp mirrors the NEWS_SENTIMENT response. Throttle/limit replies omit
// "feed" and carry Information or Note; malformed requests carry "Error Message".
type feedResp struct {
	Feed []struct {
		TickerSentiment []struct {
			Ticker         string `json:"ticker"`
			RelevanceScore string `json:"relevance_score"`
			SentimentScore string `json:"ticker_sentiment_score"`
		} `json:"ticker_sentiment"`
	} `json:"feed"`
	Information  string `json:"Information"`
	Note         string `json:"Note"`
	ErrorMessage string `json:"Error Message"`
}

// Signals returns per-ticker sentiment for the requested tickers. It fetches at
// most once per refreshInterval and never more than dailyCap times per UTC day;
// otherwise it returns the cached snapshot. A rate-limit reply marks the day's
// budget spent. Returns nil when the source is disabled (no API key).
func (c *Client) Signals(ctx context.Context, tickers []string) ([]store.Signal, error) {
	if c.apiKey == "" {
		return nil, nil // disabled
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now().UTC()
	if today := now.Format("2006-01-02"); today != c.day {
		c.day = today
		c.callsToday = 0
	}

	// Serve from cache while fresh, or once the daily budget is exhausted.
	if !c.lastFetch.IsZero() && now.Sub(c.lastFetch) < c.refreshInterval {
		return c.cache, nil
	}
	if c.callsToday >= c.dailyCap {
		return c.cache, nil
	}

	// Spend one request (count it regardless of outcome so failures can't bust
	// the budget by retrying every cycle).
	body, err := c.fetch(ctx)
	c.callsToday++
	c.lastFetch = now
	if err != nil {
		return nil, err
	}
	switch {
	case body.Information != "" || body.Note != "":
		c.callsToday = c.dailyCap // throttled: stop trying for the rest of the day
		return c.cache, nil
	case body.ErrorMessage != "":
		return nil, fmt.Errorf("alphavantage: api error: %s", body.ErrorMessage)
	}
	c.cache = aggregate(tickers, body, now)
	return c.cache, nil
}

// fetch performs one NEWS_SENTIMENT request for the latest market-wide news.
// It deliberately does NOT set the `tickers` filter: Alpha Vantage treats a
// multi-ticker filter as an AND (only articles mentioning *all* of them), which
// returns an empty feed for any real watchlist. Instead we pull the latest
// articles and extract per-ticker sentiment from each article's
// ticker_sentiment list (see aggregate).
func (c *Client) fetch(ctx context.Context) (*feedResp, error) {
	params := url.Values{}
	params.Set("function", "NEWS_SENTIMENT")
	params.Set("limit", "1000")
	params.Set("sort", "LATEST")
	params.Set("apikey", c.apiKey)
	reqURL := c.baseURL + "/query?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("alphavantage: build request: %w", err)
	}
	req.Header.Set("User-Agent", "Tickwind/0.1 (+https://tickwind.com)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("alphavantage: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("alphavantage: request: %s", resp.Status)
	}

	var body feedResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("alphavantage: decode: %w", err)
	}
	return &body, nil
}

// aggregate rolls the article feed into one sentiment Signal per requested
// ticker (relevance-weighted average score). Tickers with no coverage are
// omitted.
func aggregate(tickers []string, body *feedResp, now time.Time) []store.Signal {
	type acc struct {
		weighted float64 // Σ relevance×score
		relSum   float64 // Σ relevance
		count    int
	}
	want := make(map[string]*acc)
	for _, t := range dedupeUpper(tickers, maxTickers) {
		want[t] = &acc{}
	}

	for _, item := range body.Feed {
		for _, ts := range item.TickerSentiment {
			a, ok := want[strings.ToUpper(strings.TrimSpace(ts.Ticker))]
			if !ok {
				continue
			}
			rel, err1 := strconv.ParseFloat(ts.RelevanceScore, 64)
			score, err2 := strconv.ParseFloat(ts.SentimentScore, 64)
			if err1 != nil || err2 != nil {
				continue
			}
			a.weighted += rel * score
			a.relSum += rel
			a.count++
		}
	}

	out := make([]store.Signal, 0, len(want))
	for tk, a := range want {
		if a.count == 0 || a.relSum == 0 {
			continue
		}
		score := a.weighted / a.relSum
		out = append(out, store.Signal{
			Ticker:     tk,
			Source:     "alphavantage",
			Kind:       "sentiment",
			Score:      score,
			Label:      label(score),
			SampleSize: a.count,
			UpdatedAt:  now,
		})
	}
	return out
}

// label maps a sentiment score to Alpha Vantage's documented bands.
func label(score float64) string {
	switch {
	case score <= -0.35:
		return "Bearish"
	case score <= -0.15:
		return "Somewhat-Bearish"
	case score < 0.15:
		return "Neutral"
	case score < 0.35:
		return "Somewhat-Bullish"
	default:
		return "Bullish"
	}
}

// dedupeUpper uppercases, trims, de-duplicates and caps a ticker list.
func dedupeUpper(tickers []string, max int) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(tickers))
	for _, t := range tickers {
		t = strings.ToUpper(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
		if len(out) >= max {
			break
		}
	}
	return out
}
