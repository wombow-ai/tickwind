// Package xueqiu is a client for 雪球 (Xueqiu), a large Chinese investor
// social network. It surfaces user discussion ("statuses") about a symbol,
// which is a useful complement to English-language sources for US tickers —
// Xueqiu stores US names as bare symbols (AAPL, NVDA, TSLA), so the same
// ticker the rest of Tickwind uses works directly.
//
// UNOFFICIAL / ToS-GRAY: Xueqiu publishes no documented public API. This hits
// the same JSON endpoint the website's own front-end calls
// (/statuses/search.json), which is protected by a light anti-bot layer: the
// first request must carry a cookie token that the site mints when you load a
// page. We mint it lazily by GETting the quotes landing page (/hq) with a
// browser User-Agent and reusing the Set-Cookie values (xq_a_token, u, …) via
// a cookie jar. The token rotates and the endpoint may change or rate-limit /
// soft-block (return an empty body) requests from datacenter IPs; callers must
// treat errors and empty results as normal, transient outcomes. This mirrors
// the patterns proven by community libraries such as github.com/1dot75cm/xueqiu
// and github.com/uname-yang/pysnowball.
package xueqiu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

// Default Xueqiu host. Overridable per-Client for tests (see baseURL).
const defaultBaseURL = "https://xueqiu.com"

// browserUA is a normal desktop-Chrome User-Agent. Xueqiu's anti-bot layer
// refuses obviously non-browser clients, so we present as a browser on every
// request (cookie mint and search alike).
const browserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// searchCount is the page size requested from Xueqiu. We over-fetch a little
// so a small caller limit still has posts to choose from after any server-side
// filtering, then trim to the caller's limit in Posts.
const searchCount = 20

// tagRE matches a single HTML tag (Xueqiu post bodies are HTML fragments such
// as "<p>买入 <a href=...>$AAPL$</a></p>"). It is deliberately simple: Xueqiu
// returns server-rendered markup, not arbitrary user HTML, so a tag stripper
// plus entity unescaping is enough to recover readable text.
var tagRE = regexp.MustCompile(`<[^>]*>`)

// Client fetches investor discussion from Xueqiu's status-search endpoint. It
// mints and caches an anti-bot cookie lazily on first use, behind a mutex, and
// re-mints once if a request is rejected (HTTP 400/403).
type Client struct {
	http *http.Client

	// baseURL is the Xueqiu origin (default defaultBaseURL); overridable so
	// tests can point the client at an httptest server.
	baseURL string

	mu        sync.Mutex
	hasCookie bool
}

// New returns a Client. It is keyless: the anti-bot cookie is minted lazily on
// the first Posts call (and re-minted once on a 400/403), so constructing a
// Client performs no network I/O.
func New() *Client {
	// A cookie jar carries Xueqiu's Set-Cookie values (xq_a_token, u, …)
	// automatically on subsequent same-origin requests. cookiejar.New never
	// returns a non-nil error for nil options, so the error is unreachable.
	jar, _ := cookiejar.New(nil)
	return &Client{
		http:    &http.Client{Timeout: 15 * time.Second, Jar: jar},
		baseURL: defaultBaseURL,
	}
}

// Name identifies the source.
func (c *Client) Name() string { return "xueqiu" }

// statusResp is the subset of /statuses/search.json we consume.
type statusResp struct {
	List []status `json:"list"`
}

// status is one Xueqiu post. Both "text" and "description" carry the body in
// HTML; "text" is the full post and is preferred, with "description" as a
// fallback. "created_at" is epoch milliseconds. "target" is the site-relative
// permalink (e.g. "/9598793634/123456789").
type status struct {
	ID          int64  `json:"id"`
	Text        string `json:"text"`
	Description string `json:"description"`
	Target      string `json:"target"`
	CreatedAt   int64  `json:"created_at"`
	User        struct {
		ScreenName string `json:"screen_name"`
	} `json:"user"`
}

// Posts returns up to `limit` recent Xueqiu statuses mentioning ticker
// (limit <= 0 returns all the endpoint returned). An empty result is normal:
// the endpoint soft-blocks (returns an empty body) for some clients/IPs.
//
// ENDPOINT: GET {base}/statuses/search.json
//
//	?q=&symbol=<TICKER>&count=<n>&page=1&sort=&source=all&comment=0&hl=0
//
// chosen over the alternative /query/v1/symbol/search/status.json — at the time
// of writing only /statuses/search.json returns JSON for US symbols; the
// /query/... path serves an anti-bot challenge page. It carries the minted
// cookie plus a browser User-Agent and Referer of the Xueqiu origin.
func (c *Client) Posts(ctx context.Context, ticker string, limit int) ([]store.Post, error) {
	body, err := c.search(ctx, ticker)
	if err != nil {
		// One transient cause is a stale/expired cookie. Re-mint once and
		// retry before giving up.
		if isAntiBot(err) {
			c.invalidateCookie()
			body, err = c.search(ctx, ticker)
		}
		if err != nil {
			return nil, err
		}
	}

	var parsed statusResp
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("xueqiu: decode %s: %w", ticker, err)
	}

	out := make([]store.Post, 0, len(parsed.List))
	for _, s := range parsed.List {
		if limit > 0 && len(out) >= limit {
			break
		}
		out = append(out, store.Post{
			Ticker:    ticker,
			ID:        "xueqiu:" + strconv.FormatInt(s.ID, 10),
			Source:    "xueqiu",
			Author:    s.User.ScreenName,
			Body:      cleanHTML(pick(s.Text, s.Description)),
			URL:       c.postURL(s),
			CreatedAt: time.UnixMilli(s.CreatedAt),
		})
	}
	return out, nil
}

// search ensures a cookie is present, then performs one status-search request
// and returns the raw response body. A non-2xx status is reported as an error;
// 400/403 are wrapped as anti-bot errors so Posts can re-mint and retry.
func (c *Client) search(ctx context.Context, ticker string) ([]byte, error) {
	if err := c.ensureCookie(ctx); err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("q", "")
	q.Set("symbol", ticker)
	q.Set("count", strconv.Itoa(searchCount))
	q.Set("page", "1")
	q.Set("sort", "")
	q.Set("source", "all")
	q.Set("comment", "0")
	q.Set("hl", "0")
	endpoint := c.baseURL + "/statuses/search.json?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("xueqiu: build search %s: %w", ticker, err)
	}
	c.setBrowserHeaders(req)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xueqiu: search %s: %w", ticker, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusForbidden {
		return nil, antiBotErr{fmt.Errorf("xueqiu: search %s: %s", ticker, resp.Status)}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xueqiu: search %s: %s", ticker, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("xueqiu: read %s: %w", ticker, err)
	}
	// A soft-block returns 200 with an empty body. Treat it as "no posts"
	// rather than a JSON decode error.
	if len(strings.TrimSpace(string(body))) == 0 {
		return []byte(`{"list":[]}`), nil
	}
	return body, nil
}

// ensureCookie mints the anti-bot cookie once (guarded by the mutex). Minting
// is a GET of the quotes landing page (/hq): the plain homepage ("/") does not
// set xq_a_token, but /hq returns the full Set-Cookie set the search endpoint
// needs. The cookie is stored in the client's cookie jar, so callers never see
// it directly.
func (c *Client) ensureCookie(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.hasCookie {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/hq", nil)
	if err != nil {
		return fmt.Errorf("xueqiu: build token request: %w", err)
	}
	c.setBrowserHeaders(req)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("xueqiu: mint token: %w", err)
	}
	defer resp.Body.Close()
	// Drain so the connection can be reused; the body itself is irrelevant —
	// we only want the Set-Cookie headers (already applied to the jar).
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("xueqiu: mint token: %s", resp.Status)
	}

	c.hasCookie = true
	return nil
}

// invalidateCookie forces the next ensureCookie to re-mint. Cookie values
// already in the jar are harmless to leave (the fresh mint overwrites them).
func (c *Client) invalidateCookie() {
	c.mu.Lock()
	c.hasCookie = false
	c.mu.Unlock()
}

// setBrowserHeaders applies the User-Agent and Referer Xueqiu expects on both
// the token-mint and search requests.
func (c *Client) setBrowserHeaders(req *http.Request) {
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Referer", c.baseURL+"/")
}

// postURL builds the permalink for a status, preferring the API-supplied
// relative "target" and falling back to a path built from the id.
func (c *Client) postURL(s status) string {
	if s.Target != "" {
		return c.baseURL + s.Target
	}
	return c.baseURL + "/statuses/show/" + strconv.FormatInt(s.ID, 10)
}

// pick returns the first non-empty string.
func pick(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// cleanHTML turns a Xueqiu HTML body fragment into plain text: strip tags,
// unescape HTML entities, and collapse surrounding whitespace.
func cleanHTML(s string) string {
	s = tagRE.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(s)
}

// antiBotErr marks errors caused by Xueqiu's anti-bot layer (HTTP 400/403),
// which Posts retries once after re-minting the cookie.
type antiBotErr struct{ err error }

func (e antiBotErr) Error() string { return e.err.Error() }
func (e antiBotErr) Unwrap() error { return e.err }

// isAntiBot reports whether err is (or wraps) an antiBotErr.
func isAntiBot(err error) bool {
	var a antiBotErr
	return errors.As(err, &a)
}
