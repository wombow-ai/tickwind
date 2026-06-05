// Package reddit is a minimal client for Reddit's public search JSON. No API
// key is required (read-only public endpoint, rate-limited).
package reddit

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

// searchURL searches a set of finance subreddits for ticker mentions.
const searchURL = "https://www.reddit.com/r/stocks+wallstreetbets+investing+StockMarket/search.json"

// Client fetches public posts from Reddit.
type Client struct {
	http *http.Client
}

// New returns a Client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 15 * time.Second}}
}

// Name identifies the source.
func (c *Client) Name() string { return "reddit" }

type searchResp struct {
	Data struct {
		Children []struct {
			Data struct {
				ID        string  `json:"id"`
				Title     string  `json:"title"`
				Author    string  `json:"author"`
				Permalink string  `json:"permalink"`
				CreatedAt float64 `json:"created_utc"`
			} `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

// Posts returns up to `limit` recent Reddit posts in finance subreddits that
// mention ticker, newest first.
func (c *Client) Posts(ctx context.Context, ticker string, limit int) ([]store.Post, error) {
	q := url.Values{}
	q.Set("q", ticker)
	q.Set("restrict_sr", "1")
	q.Set("sort", "new")
	q.Set("type", "link")
	q.Set("limit", strconv.Itoa(limit))
	endpoint := searchURL + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	// Reddit requires a descriptive User-Agent or it returns 429.
	req.Header.Set("User-Agent", "Tickwind/0.1 (+https://tickwind.com)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reddit: search %s: %w", ticker, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reddit: search %s: %s", ticker, resp.Status)
	}

	var body searchResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("reddit: decode %s: %w", ticker, err)
	}
	out := make([]store.Post, 0, len(body.Data.Children))
	for _, ch := range body.Data.Children {
		d := ch.Data
		out = append(out, store.Post{
			Ticker:    ticker,
			ID:        "reddit:" + d.ID,
			Source:    "reddit",
			Author:    d.Author,
			Body:      d.Title,
			URL:       "https://www.reddit.com" + d.Permalink,
			CreatedAt: time.Unix(int64(d.CreatedAt), 0).UTC(),
		})
	}
	return out, nil
}
