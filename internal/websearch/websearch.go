// Package websearch is an optional, stdlib-only web-search client for the chat's
// attributed-context tool (search_web). It is DISABLED without an API key (New →
// Enabled()=false), mirroring internal/enrich + internal/billing, so a keyless deployment
// behaves exactly as before. Default provider: Tavily — an LLM-optimized search API that
// returns clean title + url + snippet (free tier). Results are ATTRIBUTED CONTEXT only;
// the chat layer's firewall forbids treating them as fact or quoting numbers from them.
package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const tavilyURL = "https://api.tavily.com/search"

// Result is one attributed web result.
type Result struct {
	Title   string
	URL     string
	Snippet string
}

// Config configures the client. An empty APIKey disables the whole surface.
type Config struct {
	APIKey   string
	Provider string // "tavily" (default)
	BaseURL  string // override for tests; empty → the provider default
}

// Client is a stdlib HTTP web-search client.
type Client struct {
	cfg  Config
	http *http.Client
	base string
}

// New returns a Client. Always non-nil; Enabled() is false until an API key is set.
func New(cfg Config) *Client {
	base := cfg.BaseURL
	if base == "" {
		base = tavilyURL
	}
	if cfg.Provider == "" {
		cfg.Provider = "tavily"
	}
	return &Client{cfg: cfg, http: &http.Client{Timeout: 12 * time.Second}, base: base}
}

// Enabled reports whether a search API key is configured.
func (c *Client) Enabled() bool { return c != nil && c.cfg.APIKey != "" }

// Search runs a web search and returns up to maxResults attributed results. Returns nil
// (no error) when disabled; an error on a transport/decode failure or non-2xx status.
func (c *Client) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	if !c.Enabled() {
		return nil, nil
	}
	if maxResults <= 0 || maxResults > 8 {
		maxResults = 5
	}
	body, _ := json.Marshal(map[string]any{
		"api_key":      c.cfg.APIKey,
		"query":        query,
		"max_results":  maxResults,
		"search_depth": "basic",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("websearch: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		snippet := string(raw)
		if len(snippet) > 300 {
			snippet = snippet[:300]
		}
		return nil, fmt.Errorf("websearch: status %d: %s", resp.StatusCode, snippet)
	}
	var out struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("websearch: decode: %w", err)
	}
	results := make([]Result, 0, len(out.Results))
	for _, r := range out.Results {
		results = append(results, Result{
			Title:   strings.TrimSpace(r.Title),
			URL:     strings.TrimSpace(r.URL),
			Snippet: strings.TrimSpace(r.Content),
		})
	}
	return results, nil
}
