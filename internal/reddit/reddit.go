// Package reddit is a client for Reddit's OAuth search API. Reddit's public
// (unauthenticated) JSON endpoints return 403 from datacenter IPs, so this
// uses a "script"-type OAuth app: it exchanges a bot account's username +
// password (grant_type=password) for a bearer token and queries
// oauth.reddit.com. The source is disabled (Posts returns nil) when any
// credential is missing, so the app runs fine without Reddit configured.
//
// Live operation needs REDDIT_CLIENT_ID, REDDIT_SECRET, REDDIT_USERNAME and
// REDDIT_PASSWORD: create a "script" app at https://www.reddit.com/prefs/apps
// and use a dedicated bot account.
package reddit

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

// Default Reddit hosts. Overridable per-Client for tests (see tokenURL/apiBase).
const (
	defaultTokenURL = "https://www.reddit.com/api/v1/access_token"
	defaultAPIBase  = "https://oauth.reddit.com"
)

// bodyLimit caps how much of a post's selftext we append to the body.
const bodyLimit = 280

// subreddits are the finance communities searched for ticker mentions.
var subreddits = []string{"wallstreetbets", "stocks", "investing", "options"}

// Client fetches posts from Reddit's OAuth search API. It caches a bearer
// token behind a mutex and refreshes it shortly before expiry.
type Client struct {
	http      *http.Client
	clientID  string
	secret    string
	username  string
	password  string
	userAgent string

	// Overridable endpoints (default to the real Reddit hosts) so tests can
	// point the client at an httptest server.
	tokenURL string
	apiBase  string

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// New returns a Client authenticated as a Reddit "script" app. If any of
// clientID, secret, username or password is empty the source is disabled and
// Posts returns (nil, nil), letting the app run without Reddit credentials.
func New(clientID, secret, username, password string) *Client {
	ua := "tickwind:com.tickwind.ingest:0.1 (by /u/" + username + ")"
	if username == "" {
		ua = "tickwind:com.tickwind.ingest:0.1"
	}
	return &Client{
		http:      &http.Client{Timeout: 15 * time.Second},
		clientID:  clientID,
		secret:    secret,
		username:  username,
		password:  password,
		userAgent: ua,
		tokenURL:  defaultTokenURL,
		apiBase:   defaultAPIBase,
	}
}

// Name identifies the source.
func (c *Client) Name() string { return "reddit" }

// enabled reports whether all four credentials are present.
func (c *Client) enabled() bool {
	return c.clientID != "" && c.secret != "" && c.username != "" && c.password != ""
}

type tokenResp struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	Scope       string `json:"scope"`
}

// accessToken returns a valid bearer token, fetching (or refreshing) one via
// the password grant when the cached token is missing or expired.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExp) {
		return c.token, nil
	}

	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("username", c.username)
	form.Set("password", c.password)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(c.clientID, c.secret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("reddit: token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("reddit: token: %s", resp.Status)
	}

	var tok tokenResp
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", fmt.Errorf("reddit: decode token: %w", err)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("reddit: token: empty access_token")
	}

	c.token = tok.AccessToken
	// Refresh a minute early to avoid using a token that expires mid-request.
	c.tokenExp = time.Now().Add(time.Duration(tok.ExpiresIn-60) * time.Second)
	return c.token, nil
}

type searchResp struct {
	Data struct {
		Children []struct {
			Data struct {
				ID        string  `json:"id"`
				Author    string  `json:"author"`
				Title     string  `json:"title"`
				Selftext  string  `json:"selftext"`
				Permalink string  `json:"permalink"`
				CreatedAt float64 `json:"created_utc"`
				Subreddit string  `json:"subreddit"`
			} `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

// Posts returns up to `limit` recent Reddit posts mentioning ticker across the
// finance subreddits, newest first, deduped by post id. Returns (nil, nil)
// when the source is disabled (missing credentials). A per-subreddit failure
// is skipped so one bad subreddit does not drop the rest.
func (c *Client) Posts(ctx context.Context, ticker string, limit int) ([]store.Post, error) {
	if !c.enabled() {
		return nil, nil
	}

	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]store.Post, 0, limit)
	seen := make(map[string]bool)
	for _, sub := range subreddits {
		if limit > 0 && len(out) >= limit {
			break
		}
		posts, err := c.searchSub(ctx, token, sub, ticker, limit)
		if err != nil {
			// Tolerate a single subreddit failing; keep merging the others.
			continue
		}
		for _, p := range posts {
			if limit > 0 && len(out) >= limit {
				break
			}
			rawID := strings.TrimPrefix(p.ID, "reddit:")
			if seen[rawID] {
				continue
			}
			seen[rawID] = true
			out = append(out, p)
		}
	}
	return out, nil
}

// searchSub queries one subreddit's OAuth search endpoint and maps results to
// store.Post values.
func (c *Client) searchSub(ctx context.Context, token, sub, ticker string, limit int) ([]store.Post, error) {
	q := url.Values{}
	q.Set("q", ticker)
	q.Set("restrict_sr", "1")
	q.Set("sort", "new")
	q.Set("t", "week")
	q.Set("type", "link")
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	endpoint := c.apiBase + "/r/" + sub + "/search?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "bearer "+token)
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reddit: search r/%s %s: %w", sub, ticker, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reddit: search r/%s %s: %s", sub, ticker, resp.Status)
	}

	var body searchResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("reddit: decode r/%s %s: %w", sub, ticker, err)
	}
	out := make([]store.Post, 0, len(body.Data.Children))
	for _, ch := range body.Data.Children {
		d := ch.Data
		out = append(out, store.Post{
			Ticker:    ticker,
			ID:        "reddit:" + d.ID,
			Source:    "reddit",
			Author:    d.Author,
			Body:      buildBody(d.Title, d.Selftext),
			URL:       "https://www.reddit.com" + d.Permalink,
			CreatedAt: time.Unix(int64(d.CreatedAt), 0).UTC(),
		})
	}
	return out, nil
}

// buildBody joins a post's title with its selftext (truncated to bodyLimit
// runes) when the selftext is non-empty.
func buildBody(title, selftext string) string {
	if selftext == "" {
		return title
	}
	if r := []rune(selftext); len(r) > bodyLimit {
		selftext = string(r[:bodyLimit])
	}
	return title + "\n\n" + selftext
}
