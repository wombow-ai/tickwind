package reddit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestPostsDisabledWithoutCreds verifies the source is a graceful no-op when
// any credential is missing.
func TestPostsDisabledWithoutCreds(t *testing.T) {
	cases := []struct {
		name                           string
		id, secret, username, password string
	}{
		{"all empty", "", "", "", ""},
		{"missing id", "", "s", "u", "p"},
		{"missing secret", "id", "", "u", "p"},
		{"missing username", "id", "s", "", "p"},
		{"missing password", "id", "s", "u", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New(tc.id, tc.secret, tc.username, tc.password)
			posts, err := c.Posts(context.Background(), "AAPL", 10)
			if err != nil {
				t.Fatalf("Posts() err = %v, want nil", err)
			}
			if posts != nil {
				t.Fatalf("Posts() = %v, want nil", posts)
			}
		})
	}
}

// TestPostsAuthenticatesAndParses spins up an httptest server that serves both
// the token endpoint and per-subreddit search, then verifies the client
// authenticates, parses children into store.Post, and dedupes across subs.
func TestPostsAuthenticatesAndParses(t *testing.T) {
	const wantTicker = "AAPL"

	// A post present in two subreddits (same id) to exercise dedupe.
	const dupSearch = `{"data":{"children":[
		{"data":{"id":"dup1","author":"shared","title":"Shared post","selftext":"","permalink":"/r/x/comments/dup1/shared/","created_utc":1700000000.0,"subreddit":"x"}}
	]}}`

	var tokenCalls int
	var sawBasicAuth, sawSearchAuth bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); !strings.Contains(ua, "tickwind") {
			t.Errorf("missing/short User-Agent: %q", ua)
		}
		switch {
		case r.URL.Path == "/api/v1/access_token":
			tokenCalls++
			if u, p, ok := r.BasicAuth(); ok && u == "cid" && p == "csecret" {
				sawBasicAuth = true
			}
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse token form: %v", err)
			}
			if r.PostForm.Get("grant_type") != "password" ||
				r.PostForm.Get("username") != "bot" ||
				r.PostForm.Get("password") != "pw" {
				t.Errorf("bad token form: %v", r.PostForm)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"access_token":"TOK123","token_type":"bearer","expires_in":3600,"scope":"*"}`))

		case strings.HasPrefix(r.URL.Path, "/r/") && strings.HasSuffix(r.URL.Path, "/search"):
			if r.Header.Get("Authorization") == "bearer TOK123" {
				sawSearchAuth = true
			}
			if got := r.URL.Query().Get("q"); got != wantTicker {
				t.Errorf("search q = %q, want %q", got, wantTicker)
			}
			w.Header().Set("Content-Type", "application/json")
			sub := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/r/"), "/search")
			switch sub {
			case "wallstreetbets":
				// One real post with selftext to exercise body building.
				w.Write([]byte(`{"data":{"children":[
					{"data":{"id":"abc","author":"alice","title":"AAPL to the moon","selftext":"long thesis here","permalink":"/r/wallstreetbets/comments/abc/aapl/","created_utc":1700000123.0,"subreddit":"wallstreetbets"}},
					{"data":{"id":"dup1","author":"shared","title":"Shared post","selftext":"","permalink":"/r/wallstreetbets/comments/dup1/shared/","created_utc":1700000000.0,"subreddit":"wallstreetbets"}}
				]}}`))
			default:
				// Other subreddits return the duplicate so dedupe is tested.
				w.Write([]byte(dupSearch))
			}

		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New("cid", "csecret", "bot", "pw")
	c.tokenURL = srv.URL + "/api/v1/access_token"
	c.apiBase = srv.URL

	posts, err := c.Posts(context.Background(), wantTicker, 50)
	if err != nil {
		t.Fatalf("Posts() err = %v", err)
	}
	if !sawBasicAuth {
		t.Error("token request did not use HTTP Basic auth with clientID:secret")
	}
	if !sawSearchAuth {
		t.Error("search request did not send 'Authorization: bearer <token>'")
	}

	// Two unique posts: "abc" and "dup1" (dup1 appears in every sub).
	if len(posts) != 2 {
		t.Fatalf("got %d posts, want 2 (deduped): %+v", len(posts), posts)
	}

	// Verify the first/full post's field mapping.
	p := posts[0]
	if p.ID != "reddit:abc" {
		t.Errorf("ID = %q, want reddit:abc", p.ID)
	}
	if p.Source != "reddit" {
		t.Errorf("Source = %q, want reddit", p.Source)
	}
	if p.Ticker != wantTicker {
		t.Errorf("Ticker = %q, want %q", p.Ticker, wantTicker)
	}
	if p.Author != "alice" {
		t.Errorf("Author = %q, want alice", p.Author)
	}
	if !strings.HasPrefix(p.Body, "AAPL to the moon") || !strings.Contains(p.Body, "long thesis here") {
		t.Errorf("Body = %q, want title + selftext", p.Body)
	}
	if p.URL != "https://www.reddit.com/r/wallstreetbets/comments/abc/aapl/" {
		t.Errorf("URL = %q", p.URL)
	}
	if want := time.Unix(1700000123, 0).UTC(); !p.CreatedAt.Equal(want) {
		t.Errorf("CreatedAt = %v, want %v", p.CreatedAt, want)
	}

	// Dedupe must not call the token endpoint more than once for one Posts().
	if tokenCalls != 1 {
		t.Errorf("token endpoint called %d times, want 1", tokenCalls)
	}
}
