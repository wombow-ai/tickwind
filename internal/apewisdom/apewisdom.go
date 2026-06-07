// Package apewisdom is a client for the free, keyless ApeWisdom API
// (https://apewisdom.io), which ranks stocks by Reddit / WallStreetBets mention
// volume. It produces a per-ticker "buzz" store.Signal (mentions, rank, upvotes
// and their 24h-ago values) rather than a feed of posts, satisfying the ingest
// SignalSource shape. Tickers absent from the leaderboard simply have no buzz
// signal and are omitted. No API key is required.
package apewisdom

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

// defaultBaseURL is the public ApeWisdom API host.
const defaultBaseURL = "https://apewisdom.io"

// maxPages caps how many leaderboard pages (≈100 stocks each) we scan to locate
// the requested tickers. The common case (popular tickers) resolves on page 1;
// extra pages only get fetched when a requested ticker is further down.
const maxPages = 3

// Client fetches mention-momentum rankings from ApeWisdom.
type Client struct {
	http    *http.Client
	baseURL string
}

// New returns a Client pointed at the public ApeWisdom API. No key is required.
func New() *Client {
	return &Client{
		http:    &http.Client{Timeout: 15 * time.Second},
		baseURL: defaultBaseURL,
	}
}

// Name identifies the source.
func (c *Client) Name() string { return "apewisdom" }

// pageResp mirrors one ApeWisdom leaderboard page. Only the fields we map are
// declared; rank_24h_ago / mentions_24h_ago may be null for new entries, which
// JSON decodes to 0 (treated as "no prior data").
type pageResp struct {
	Pages   int `json:"pages"`
	Results []struct {
		Rank           int    `json:"rank"`
		Ticker         string `json:"ticker"`
		Name           string `json:"name"`
		Mentions       int    `json:"mentions"`
		Upvotes        int    `json:"upvotes"`
		Rank24hAgo     int    `json:"rank_24h_ago"`
		Mentions24hAgo int    `json:"mentions_24h_ago"`
	} `json:"results"`
}

// Signals returns a "buzz" Signal for each requested ticker currently on the
// ApeWisdom leaderboard. It scans up to maxPages pages, stopping as soon as
// every requested ticker is found (or the leaderboard ends). Tickers not on the
// leaderboard are omitted. An empty request set yields an empty (non-nil) slice.
func (c *Client) Signals(ctx context.Context, tickers []string) ([]store.Signal, error) {
	want := make(map[string]struct{})
	for _, t := range tickers {
		if t = strings.ToUpper(strings.TrimSpace(t)); t != "" {
			want[t] = struct{}{}
		}
	}
	out := make([]store.Signal, 0, len(want))
	if len(want) == 0 {
		return out, nil
	}

	now := time.Now().UTC()
	found := make(map[string]struct{})
	for page := 1; page <= maxPages; page++ {
		resp, err := c.fetchPage(ctx, "all-stocks", page)
		if err != nil {
			if page == 1 {
				return nil, err
			}
			break // partial coverage from earlier pages beats failing outright
		}
		for _, r := range resp.Results {
			tk := strings.ToUpper(strings.TrimSpace(r.Ticker))
			if _, ok := want[tk]; !ok {
				continue
			}
			if _, done := found[tk]; done {
				continue
			}
			found[tk] = struct{}{}
			out = append(out, store.Signal{
				Ticker:       tk,
				Source:       "apewisdom",
				Kind:         "buzz",
				Mentions:     r.Mentions,
				MentionsPrev: r.Mentions24hAgo,
				Rank:         r.Rank,
				RankPrev:     r.Rank24hAgo,
				Upvotes:      r.Upvotes,
				UpdatedAt:    now,
			})
		}
		if len(found) == len(want) || page >= resp.Pages {
			break
		}
	}
	return out, nil
}

// Leaderboard returns the top-`limit` most-mentioned stocks from ApeWisdom's
// all-stocks board (ranked by 24h mention volume), unfiltered by any watchlist.
// The returned HotStocks carry the raw inputs (Ticker, Name, Mentions,
// MentionsPrev, Upvotes); Change, Heat and Rank are left for the caller's heat
// scoring. limit <= 0 defaults to one page (top 100).
func (c *Client) Leaderboard(ctx context.Context, limit int) ([]store.HotStock, error) {
	if limit <= 0 {
		limit = 100
	}
	out := make([]store.HotStock, 0, limit)
	for page := 1; page <= maxPages && len(out) < limit; page++ {
		resp, err := c.fetchPage(ctx, "all-stocks", page)
		if err != nil {
			if page == 1 {
				return nil, err
			}
			break // keep what earlier pages gave us
		}
		for _, r := range resp.Results {
			tk := strings.ToUpper(strings.TrimSpace(r.Ticker))
			if tk == "" {
				continue
			}
			out = append(out, store.HotStock{
				Ticker:       tk,
				Name:         html.UnescapeString(r.Name), // ApeWisdom HTML-encodes names (S&amp;P → S&P)
				Mentions:     r.Mentions,
				MentionsPrev: r.Mentions24hAgo,
				Upvotes:      r.Upvotes,
			})
			if len(out) >= limit {
				break
			}
		}
		if page >= resp.Pages {
			break
		}
	}
	return out, nil
}

// WallStreetBets returns the top-`limit` most-mentioned tickers on
// r/wallstreetbets specifically (vs Leaderboard's all-subreddits board), for the
// "WSB Trending" view. Same HotStock shape (Ticker, Name, Mentions,
// MentionsPrev, Upvotes); ranking/Change are the caller's. limit <= 0 → top 100.
func (c *Client) WallStreetBets(ctx context.Context, limit int) ([]store.HotStock, error) {
	if limit <= 0 {
		limit = 100
	}
	resp, err := c.fetchPage(ctx, "wallstreetbets", 1) // one page (top 100) is plenty
	if err != nil {
		return nil, err
	}
	out := make([]store.HotStock, 0, limit)
	for _, r := range resp.Results {
		tk := strings.ToUpper(strings.TrimSpace(r.Ticker))
		if tk == "" {
			continue
		}
		out = append(out, store.HotStock{
			Ticker:       tk,
			Name:         html.UnescapeString(r.Name),
			Rank:         r.Rank,
			RankPrev:     r.Rank24hAgo,
			Mentions:     r.Mentions,
			MentionsPrev: r.Mentions24hAgo,
			Upvotes:      r.Upvotes,
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// fetchPage retrieves one page of the given ApeWisdom filter (e.g. "all-stocks"
// or "wallstreetbets").
func (c *Client) fetchPage(ctx context.Context, filter string, page int) (*pageResp, error) {
	url := c.baseURL + "/api/v1.0/filter/" + filter + "/page/" + strconv.Itoa(page)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("apewisdom: build request page %d: %w", page, err)
	}
	req.Header.Set("User-Agent", "Tickwind/0.1 (+https://tickwind.com)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("apewisdom: fetch page %d: %w", page, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("apewisdom: fetch page %d: %s", page, resp.Status)
	}

	var body pageResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("apewisdom: decode page %d: %w", page, err)
	}
	return &body, nil
}
