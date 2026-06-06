// Package tickertick is a minimal client for the public TickerTick stock-news
// API (https://github.com/hczhu/TickerTick-API). It returns per-ticker
// user-generated and analysis story links. No API key is required.
//
// The API enforces a strict per-IP rate limit (roughly ~10 requests/minute, and
// stricter on bursts). The ingest scheduler fans out across many tickers back to
// back, which trips that limit, so the client self-throttles: it spaces its own
// outbound requests at least minInterval apart (see throttle). That keeps every
// ticker covered each cycle instead of the first few winning and the rest eating
// 429s. The 15-minute scheduler cadence easily absorbs the added pacing.
package tickertick

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

// defaultBaseURL is the public TickerTick feed endpoint.
const defaultBaseURL = "https://api.tickertick.com"

// defaultMinInterval paces outbound requests to stay under TickerTick's rate
// limit. ~8s ≈ 7.5 req/min, comfortably below the ~10/min ceiling with margin
// for the stricter burst behavior observed in production.
const defaultMinInterval = 8 * time.Second

// Client fetches per-ticker story feeds from TickerTick. It is safe for
// concurrent use; requests are serialized to respect the rate limit.
type Client struct {
	http    *http.Client
	baseURL string

	// minInterval is the minimum spacing between outbound requests (0 disables
	// throttling, used in tests). mu guards last across concurrent callers.
	mu          sync.Mutex
	last        time.Time
	minInterval time.Duration
}

// New returns a Client pointed at the public TickerTick API. No key is needed.
func New() *Client {
	return &Client{
		http:        &http.Client{Timeout: 15 * time.Second},
		baseURL:     defaultBaseURL,
		minInterval: defaultMinInterval,
	}
}

// Name identifies the source.
func (c *Client) Name() string { return "tickertick" }

// feedResp mirrors the TickerTick /feed response. Only the fields we map are
// declared; the rest (favicon_url, tags, similar_stories, last_id, …) are
// ignored.
type feedResp struct {
	Stories []struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		URL         string `json:"url"`
		Site        string `json:"site"`
		Time        int64  `json:"time"` // Unix epoch milliseconds.
		Description string `json:"description"`
	} `json:"stories"`
}

// Posts returns up to `limit` recent TickerTick stories for ticker
// (limit <= 0 returns all returned by the API). It prefers user-generated and
// analysis stories; if that narrower query yields nothing it falls back to a
// plain ticker query. Stories without a URL are skipped.
func (c *Client) Posts(ctx context.Context, ticker string, limit int) ([]store.Post, error) {
	// Ask for a few extra so URL-less stories don't shrink us below limit.
	n := limit
	if n <= 0 {
		n = 30
	}

	// Narrower, higher-signal query: stories tagged to the ticker that are
	// user-generated content or analysis articles.
	q := fmt.Sprintf("(and z:%s (or T:ugc T:analysis))", ticker)
	posts, err := c.fetch(ctx, ticker, q, n, limit)
	if err != nil {
		return nil, err
	}
	if len(posts) > 0 {
		return posts, nil
	}

	// Fallback: any stories about the ticker.
	return c.fetch(ctx, ticker, "z:"+ticker, n, limit)
}

// throttle blocks until at least minInterval has elapsed since the previous
// request, keeping us under TickerTick's per-IP rate limit instead of bursting
// and eating 429s. It serializes concurrent callers and returns early if ctx is
// cancelled (so shutdown stays responsive).
func (c *Client) throttle(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.minInterval <= 0 {
		return nil
	}
	if wait := c.minInterval - time.Since(c.last); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	c.last = time.Now()
	return nil
}

// fetch runs one /feed request for query q and maps the stories to posts.
func (c *Client) fetch(ctx context.Context, ticker, q string, n, limit int) ([]store.Post, error) {
	if err := c.throttle(ctx); err != nil {
		return nil, fmt.Errorf("tickertick: throttle %s: %w", ticker, err)
	}

	params := url.Values{}
	params.Set("q", q)
	params.Set("n", fmt.Sprintf("%d", n))
	reqURL := c.baseURL + "/feed?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("tickertick: build request %s: %w", ticker, err)
	}
	req.Header.Set("User-Agent", "Tickwind/0.1 (+https://tickwind.com)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tickertick: feed %s: %w", ticker, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tickertick: feed %s: %s", ticker, resp.Status)
	}

	var body feedResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("tickertick: decode %s: %w", ticker, err)
	}

	out := make([]store.Post, 0, len(body.Stories))
	for _, s := range body.Stories {
		if s.URL == "" {
			continue
		}
		if limit > 0 && len(out) >= limit {
			break
		}
		out = append(out, store.Post{
			Ticker:    ticker,
			ID:        "tickertick:" + s.ID,
			Source:    "tickertick",
			Author:    s.Site, // The publisher/site that posted the story.
			Body:      s.Title,
			URL:       s.URL,
			CreatedAt: time.UnixMilli(s.Time),
		})
	}
	return out, nil
}
