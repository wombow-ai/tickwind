// Package bluesky is a minimal client for the Bluesky (AT Protocol) post
// search API. It authenticates with a handle + app password (both free) and
// searches public posts by cashtag. With empty credentials the source is
// disabled and returns no posts.
package bluesky

import (
	"bytes"
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

// Default AT Protocol host for the public Bluesky network. Exposed as
// per-Client overridable fields so tests can point at an httptest server.
const (
	defaultSessionURL = "https://bsky.social/xrpc/com.atproto.server.createSession"
	defaultSearchURL  = "https://bsky.social/xrpc/app.bsky.feed.searchPosts"
)

// Client searches public Bluesky posts. It lazily creates and caches an
// auth session (access JWT), refreshing it once on a 401.
type Client struct {
	http        *http.Client
	handle      string
	appPassword string

	// Base endpoints; default to the real Bluesky hosts. Overridable in tests.
	sessionURL string
	searchURL  string

	mu     sync.Mutex // guards accessJWT
	access string     // cached access JWT; "" until the first session is created
}

// New returns a Client that authenticates as handle using appPassword. If
// either is empty the source is disabled: Posts returns (nil, nil).
func New(handle, appPassword string) *Client {
	return &Client{
		http:        &http.Client{Timeout: 15 * time.Second},
		handle:      handle,
		appPassword: appPassword,
		sessionURL:  defaultSessionURL,
		searchURL:   defaultSearchURL,
	}
}

// Name identifies the source.
func (c *Client) Name() string { return "bluesky" }

// enabled reports whether credentials were supplied.
func (c *Client) enabled() bool { return c.handle != "" && c.appPassword != "" }

type sessionResp struct {
	AccessJWT  string `json:"accessJwt"`
	RefreshJWT string `json:"refreshJwt"`
	DID        string `json:"did"`
	Handle     string `json:"handle"`
}

type searchResp struct {
	Posts []struct {
		URI    string `json:"uri"`
		CID    string `json:"cid"`
		Author struct {
			Handle      string `json:"handle"`
			DisplayName string `json:"displayName"`
		} `json:"author"`
		Record struct {
			Text      string    `json:"text"`
			CreatedAt time.Time `json:"createdAt"`
		} `json:"record"`
		IndexedAt string `json:"indexedAt"`
	} `json:"posts"`
}

// Posts returns up to `limit` recent Bluesky posts mentioning the ticker as a
// cashtag (e.g. "$AAPL"); limit <= 0 returns all posts the API returns. When
// the source is disabled (empty credentials) it returns (nil, nil).
func (c *Client) Posts(ctx context.Context, ticker string, limit int) ([]store.Post, error) {
	if !c.enabled() {
		return nil, nil
	}

	token, err := c.token(ctx, false)
	if err != nil {
		return nil, err
	}

	resp, err := c.search(ctx, token, ticker, limit)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// The cached session may have expired; re-create it once and retry.
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		token, err = c.token(ctx, true)
		if err != nil {
			return nil, err
		}
		resp, err = c.search(ctx, token, ticker, limit)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bluesky: search %s: %s", ticker, resp.Status)
	}

	var body searchResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("bluesky: decode %s: %w", ticker, err)
	}
	out := make([]store.Post, 0, len(body.Posts))
	for _, p := range body.Posts {
		if limit > 0 && len(out) >= limit {
			break
		}
		out = append(out, store.Post{
			Ticker:    ticker,
			ID:        "bluesky:" + p.CID,
			Source:    "bluesky",
			Author:    p.Author.Handle,
			Body:      p.Record.Text,
			URL:       postURL(p.Author.Handle, p.URI),
			CreatedAt: p.Record.CreatedAt,
		})
	}
	return out, nil
}

// token returns a cached access JWT, creating a session if none is cached or
// force is set (used to refresh after a 401).
func (c *Client) token(ctx context.Context, force bool) (string, error) {
	c.mu.Lock()
	cached := c.access
	c.mu.Unlock()
	if cached != "" && !force {
		return cached, nil
	}

	sess, err := c.createSession(ctx)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	c.access = sess.AccessJWT
	c.mu.Unlock()
	return sess.AccessJWT, nil
}

// createSession authenticates with the handle + app password and returns the
// session tokens.
func (c *Client) createSession(ctx context.Context) (sessionResp, error) {
	payload, err := json.Marshal(map[string]string{
		"identifier": c.handle,
		"password":   c.appPassword,
	})
	if err != nil {
		return sessionResp{}, fmt.Errorf("bluesky: encode session request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.sessionURL, bytes.NewReader(payload))
	if err != nil {
		return sessionResp{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Tickwind/0.1 (+https://tickwind.com)")

	resp, err := c.http.Do(req)
	if err != nil {
		return sessionResp{}, fmt.Errorf("bluesky: create session: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return sessionResp{}, fmt.Errorf("bluesky: create session: %s", resp.Status)
	}

	var sess sessionResp
	if err := json.NewDecoder(resp.Body).Decode(&sess); err != nil {
		return sessionResp{}, fmt.Errorf("bluesky: decode session: %w", err)
	}
	if sess.AccessJWT == "" {
		return sessionResp{}, fmt.Errorf("bluesky: create session: empty access token")
	}
	return sess, nil
}

// search issues an authenticated searchPosts request for the ticker cashtag.
// The caller owns the returned response body.
func (c *Client) search(ctx context.Context, token, ticker string, limit int) (*http.Response, error) {
	q := url.Values{}
	q.Set("q", "$"+ticker)
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	endpoint := c.searchURL + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "Tickwind/0.1 (+https://tickwind.com)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bluesky: search %s: %w", ticker, err)
	}
	return resp, nil
}

// postURL builds the public web URL for a post from the author handle and the
// post's AT URI (at://<did>/app.bsky.feed.post/<rkey>); the rkey is the last
// path segment.
func postURL(handle, uri string) string {
	rkey := uri
	if i := strings.LastIndex(uri, "/"); i >= 0 {
		rkey = uri[i+1:]
	}
	return "https://bsky.app/profile/" + handle + "/post/" + rkey
}
