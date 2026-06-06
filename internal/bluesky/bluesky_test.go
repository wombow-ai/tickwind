package bluesky

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// realistic createSession + searchPosts JSON, modeled on the AT Protocol docs.
const (
	sessionJSON = `{
		"accessJwt": "access-token-1",
		"refreshJwt": "refresh-token-1",
		"did": "did:plc:abc123",
		"handle": "tickwind.bsky.social"
	}`

	searchJSON = `{
		"posts": [
			{
				"uri": "at://did:plc:author1/app.bsky.feed.post/3kqp7xyz123",
				"cid": "bafyreigh2akiscaildc",
				"author": {"handle": "trader.bsky.social", "displayName": "A Trader"},
				"record": {"text": "Loading up on $AAPL ahead of earnings", "createdAt": "2026-06-01T12:30:00Z"},
				"indexedAt": "2026-06-01T12:30:05Z"
			},
			{
				"uri": "at://did:plc:author2/app.bsky.feed.post/3kqp8abc456",
				"cid": "bafyreibsecondpost",
				"author": {"handle": "quant.bsky.social", "displayName": "Quant"},
				"record": {"text": "$AAPL looking strong", "createdAt": "2026-06-01T13:00:00Z"},
				"indexedAt": "2026-06-01T13:00:02Z"
			}
		]
	}`
)

// newTestServer returns a server serving createSession + searchPosts and a
// Client pointed at it. The search handler returns 401 until the session has
// been (re)created `unauthorizedUntil` times, so tests can drive the retry.
func newTestServer(t *testing.T, unauthorizedUntil int32) (*httptest.Server, *Client) {
	t.Helper()
	var sessions int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/createSession":
			if r.Method != http.MethodPost {
				t.Errorf("createSession: method = %s, want POST", r.Method)
			}
			atomic.AddInt32(&sessions, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(sessionJSON))
		case "/searchPosts":
			if got := r.Header.Get("Authorization"); got == "" {
				t.Errorf("searchPosts: missing Authorization header")
			}
			if atomic.LoadInt32(&sessions) <= unauthorizedUntil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(searchJSON))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c := New("tickwind.bsky.social", "app-pw-xxxx")
	c.sessionURL = srv.URL + "/createSession"
	c.searchURL = srv.URL + "/searchPosts"
	return srv, c
}

func TestPostsParsesSearchResults(t *testing.T) {
	_, c := newTestServer(t, 0) // never 401

	posts, err := c.Posts(context.Background(), "AAPL", 30)
	if err != nil {
		t.Fatalf("Posts() error = %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("len(posts) = %d, want 2", len(posts))
	}

	got := posts[0]
	want := struct {
		id, source, author, body, url string
		createdAt                     time.Time
	}{
		id:        "bluesky:bafyreigh2akiscaildc",
		source:    "bluesky",
		author:    "trader.bsky.social",
		body:      "Loading up on $AAPL ahead of earnings",
		url:       "https://bsky.app/profile/trader.bsky.social/post/3kqp7xyz123",
		createdAt: time.Date(2026, 6, 1, 12, 30, 0, 0, time.UTC),
	}
	if got.Ticker != "AAPL" {
		t.Errorf("Ticker = %q, want %q", got.Ticker, "AAPL")
	}
	if got.ID != want.id {
		t.Errorf("ID = %q, want %q", got.ID, want.id)
	}
	if got.Source != want.source {
		t.Errorf("Source = %q, want %q", got.Source, want.source)
	}
	if got.Author != want.author {
		t.Errorf("Author = %q, want %q", got.Author, want.author)
	}
	if got.Body != want.body {
		t.Errorf("Body = %q, want %q", got.Body, want.body)
	}
	if got.URL != want.url { // verifies rkey = last path segment of the AT URI
		t.Errorf("URL = %q, want %q", got.URL, want.url)
	}
	if !got.CreatedAt.Equal(want.createdAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, want.createdAt)
	}
}

func TestPostsRespectsLimit(t *testing.T) {
	_, c := newTestServer(t, 0)

	posts, err := c.Posts(context.Background(), "AAPL", 1)
	if err != nil {
		t.Fatalf("Posts() error = %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("len(posts) = %d, want 1", len(posts))
	}
	if posts[0].ID != "bluesky:bafyreigh2akiscaildc" {
		t.Errorf("ID = %q, want first post", posts[0].ID)
	}
}

func TestPostsRetriesOn401(t *testing.T) {
	// First search (sessions==1) returns 401; the client must re-create the
	// session (sessions==2) and retry successfully.
	_, c := newTestServer(t, 1)

	posts, err := c.Posts(context.Background(), "AAPL", 30)
	if err != nil {
		t.Fatalf("Posts() error = %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("len(posts) = %d, want 2 after retry", len(posts))
	}
}

func TestPostsDisabledWithoutCredentials(t *testing.T) {
	for _, tc := range []struct {
		name, handle, pw string
	}{
		{"both empty", "", ""},
		{"no handle", "", "app-pw"},
		{"no password", "tickwind.bsky.social", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := New(tc.handle, tc.pw)
			// Point at an unreachable URL: a disabled client must not call it.
			c.sessionURL = "http://127.0.0.1:0/createSession"
			c.searchURL = "http://127.0.0.1:0/searchPosts"

			posts, err := c.Posts(context.Background(), "AAPL", 30)
			if err != nil {
				t.Fatalf("Posts() error = %v, want nil", err)
			}
			if posts != nil {
				t.Fatalf("Posts() = %v, want nil", posts)
			}
		})
	}
}

func TestName(t *testing.T) {
	if got := New("", "").Name(); got != "bluesky" {
		t.Errorf("Name() = %q, want %q", got, "bluesky")
	}
}
