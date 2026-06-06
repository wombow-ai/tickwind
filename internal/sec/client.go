package sec

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	defaultDataBase    = "https://data.sec.gov" // XBRL frames / companyfacts
	defaultArchiveBase = "https://www.sec.gov"  // Archives, company_tickers, daily-index
	maxBody            = 64 << 20               // 64 MiB cap on any single response
)

// Client fetches public-domain SEC EDGAR data. It self-throttles to stay under
// SEC's 10 req/s fair-access limit and sends the required descriptive
// User-Agent (generic agents are 403'd). Safe for concurrent use; requests are
// serialized by the throttle.
type Client struct {
	http        *http.Client
	userAgent   string
	dataBase    string // data.sec.gov
	archiveBase string // www.sec.gov

	mu     sync.Mutex
	last   time.Time
	minGap time.Duration
}

// New returns a Client. userAgent MUST be descriptive and include a contact
// email, e.g. "Tickwind (contact@tickwind.com)".
func New(userAgent string) *Client {
	return &Client{
		http:        &http.Client{Timeout: 30 * time.Second},
		userAgent:   userAgent,
		dataBase:    defaultDataBase,
		archiveBase: defaultArchiveBase,
		minGap:      120 * time.Millisecond, // ≈8 req/s, safely under SEC's 10/s
	}
}

// throttle blocks until at least minGap has elapsed since the previous request.
func (c *Client) throttle(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if wait := c.minGap - time.Since(c.last); wait > 0 {
		t := time.NewTimer(wait)
		defer t.Stop()
		select {
		case <-ctx.Done():
		case <-t.C:
		}
	}
	c.last = time.Now()
}

// get performs a rate-limited GET with the SEC User-Agent and returns the body
// (Go transparently negotiates + decompresses gzip).
func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	c.throttle(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("sec: build request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sec: get %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sec: get %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBody))
}

// Shares returns common shares outstanding per CIK from the dei XBRL frame for
// the given calendar quarter (instantaneous, the cover-page total — one row per
// CIK). Merge two recent quarters at the call site and keep the freshest.
func (c *Client) Shares(ctx context.Context, year, quarter int) (map[int]int64, error) {
	url := fmt.Sprintf(
		"%s/api/xbrl/frames/dei/EntityCommonStockSharesOutstanding/shares/CY%dQ%dI.json",
		c.dataBase, year, quarter,
	)
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []struct {
			CIK int `json:"cik"`
			// val is usually an integer share count, but a few filers report a
			// fractional value — decode as float64 so one odd row can't fail the
			// whole frame, then truncate to a share count.
			Val float64 `json:"val"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("sec: shares decode: %w", err)
	}
	out := make(map[int]int64, len(resp.Data))
	for _, d := range resp.Data {
		v := int64(d.Val)
		if v > out[d.CIK] { // defensive: keep the largest if dupes appear
			out[d.CIK] = v
		}
	}
	return out, nil
}
