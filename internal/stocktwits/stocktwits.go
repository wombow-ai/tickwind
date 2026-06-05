// Package stocktwits is a minimal client for the public StockTwits symbol
// streams API. No API key is required (public, rate-limited).
package stocktwits

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

const baseURL = "https://api.stocktwits.com/api/2"

// Client fetches public message streams from StockTwits.
type Client struct {
	http *http.Client
}

// New returns a Client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 15 * time.Second}}
}

type streamResp struct {
	Messages []struct {
		ID        int64     `json:"id"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"created_at"`
		User      struct {
			Username string `json:"username"`
		} `json:"user"`
	} `json:"messages"`
}

// SymbolStream returns up to `limit` recent StockTwits messages for ticker
// (limit <= 0 returns all returned by the API).
func (c *Client) SymbolStream(ctx context.Context, ticker string, limit int) ([]store.Post, error) {
	url := fmt.Sprintf("%s/streams/symbol/%s.json", baseURL, ticker)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Tickwind/0.1 (+https://tickwind.com)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stocktwits: stream %s: %w", ticker, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stocktwits: stream %s: %s", ticker, resp.Status)
	}

	var body streamResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("stocktwits: decode %s: %w", ticker, err)
	}
	out := make([]store.Post, 0, len(body.Messages))
	for _, m := range body.Messages {
		if limit > 0 && len(out) >= limit {
			break
		}
		out = append(out, store.Post{
			Ticker:    ticker,
			ID:        "stocktwits:" + strconv.FormatInt(m.ID, 10),
			Source:    "stocktwits",
			Author:    m.User.Username,
			Body:      m.Body,
			URL:       "https://stocktwits.com/symbol/" + ticker,
			CreatedAt: m.CreatedAt,
		})
	}
	return out, nil
}
