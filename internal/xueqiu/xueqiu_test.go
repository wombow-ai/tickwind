package xueqiu

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// statusJSON is a realistic /statuses/search.json payload for a US symbol. The
// first item exercises HTML stripping + entity unescaping and a relative
// "target"; the second omits "target" (URL must be built from the id) and uses
// "description" as the body fallback (empty "text").
const statusJSON = `{
  "count": 2,
  "list": [
    {
      "id": 123456789,
      "text": "<p>Loading up on <a href=\"/S/AAPL\">$AAPL$</a> &amp; chill &mdash; long &gt; short.</p>",
      "description": "ignored when text present",
      "target": "/9598793634/123456789",
      "created_at": 1700000000000,
      "user": {"id": 9598793634, "screen_name": "价值投资者"}
    },
    {
      "id": 987654321,
      "text": "",
      "description": "<b>second</b> post body",
      "created_at": 1700000600000,
      "user": {"id": 1111, "screen_name": "alice"}
    }
  ]
}`

// newTestServer returns a server that mints a cookie on /hq and serves
// statusJSON on /statuses/search.json. mintHits / searchHits count requests.
func newTestServer(t *testing.T) (*httptest.Server, *int32, *int32) {
	t.Helper()
	var mintHits, searchHits int32
	mux := http.NewServeMux()
	mux.HandleFunc("/hq", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&mintHits, 1)
		http.SetCookie(w, &http.Cookie{Name: "xq_a_token", Value: "tok-abc"})
		http.SetCookie(w, &http.Cookie{Name: "u", Value: "1234567890"})
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>hq</html>"))
	})
	mux.HandleFunc("/statuses/search.json", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&searchHits, 1)
		// The mint must have happened first and supplied the cookie.
		if _, err := r.Cookie("xq_a_token"); err != nil {
			http.Error(w, `{"error_code":"400016"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json;charset=UTF-8")
		_, _ = w.Write([]byte(statusJSON))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &mintHits, &searchHits
}

func TestPosts(t *testing.T) {
	srv, mintHits, searchHits := newTestServer(t)

	c := New()
	c.baseURL = srv.URL

	posts, err := c.Posts(context.Background(), "AAPL", 0)
	if err != nil {
		t.Fatalf("Posts: %v", err)
	}
	if got := atomic.LoadInt32(mintHits); got != 1 {
		t.Errorf("mint hits = %d, want 1", got)
	}
	if got := atomic.LoadInt32(searchHits); got != 1 {
		t.Errorf("search hits = %d, want 1", got)
	}
	if len(posts) != 2 {
		t.Fatalf("len(posts) = %d, want 2", len(posts))
	}

	p := posts[0]
	if p.ID != "xueqiu:123456789" {
		t.Errorf("ID = %q, want xueqiu:123456789", p.ID)
	}
	if p.Source != "xueqiu" {
		t.Errorf("Source = %q, want xueqiu", p.Source)
	}
	if p.Ticker != "AAPL" {
		t.Errorf("Ticker = %q, want AAPL", p.Ticker)
	}
	if p.Author != "价值投资者" {
		t.Errorf("Author = %q, want 价值投资者", p.Author)
	}
	// HTML tags stripped, entities unescaped, whitespace trimmed.
	wantBody := "Loading up on $AAPL$ & chill — long > short."
	if p.Body != wantBody {
		t.Errorf("Body = %q, want %q", p.Body, wantBody)
	}
	if p.URL != srv.URL+"/9598793634/123456789" {
		t.Errorf("URL = %q, want %s/9598793634/123456789", p.URL, srv.URL)
	}
	if want := time.UnixMilli(1700000000000); !p.CreatedAt.Equal(want) {
		t.Errorf("CreatedAt = %v, want %v", p.CreatedAt, want)
	}

	// Second post: no "target" -> URL built from id; "description" fallback.
	p2 := posts[1]
	if p2.ID != "xueqiu:987654321" {
		t.Errorf("ID[1] = %q, want xueqiu:987654321", p2.ID)
	}
	if p2.URL != srv.URL+"/statuses/show/987654321" {
		t.Errorf("URL[1] = %q, want %s/statuses/show/987654321", p2.URL, srv.URL)
	}
	if p2.Body != "second post body" {
		t.Errorf("Body[1] = %q, want %q", p2.Body, "second post body")
	}
}

// TestPostsLimit verifies the caller's limit is respected.
func TestPostsLimit(t *testing.T) {
	srv, _, _ := newTestServer(t)
	c := New()
	c.baseURL = srv.URL

	posts, err := c.Posts(context.Background(), "AAPL", 1)
	if err != nil {
		t.Fatalf("Posts: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("len(posts) = %d, want 1 (limit)", len(posts))
	}
	if posts[0].ID != "xueqiu:123456789" {
		t.Errorf("ID = %q, want xueqiu:123456789", posts[0].ID)
	}
}

// TestCookieReminted verifies that a 400 (stale/expired cookie) triggers one
// re-mint + retry, after which Posts succeeds.
func TestCookieReminted(t *testing.T) {
	var mintHits, searchHits int32
	mux := http.NewServeMux()
	mux.HandleFunc("/hq", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&mintHits, 1)
		// First mint sets a "stale" token the search rejects; second mint
		// sets a "good" token the search accepts.
		val := "stale"
		if n >= 2 {
			val = "good"
		}
		http.SetCookie(w, &http.Cookie{Name: "xq_a_token", Value: val})
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/statuses/search.json", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&searchHits, 1)
		ck, err := r.Cookie("xq_a_token")
		if err != nil || ck.Value != "good" {
			http.Error(w, `{"error_code":"400016"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(statusJSON))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New()
	c.baseURL = srv.URL

	posts, err := c.Posts(context.Background(), "AAPL", 0)
	if err != nil {
		t.Fatalf("Posts after re-mint: %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("len(posts) = %d, want 2", len(posts))
	}
	if got := atomic.LoadInt32(&mintHits); got != 2 {
		t.Errorf("mint hits = %d, want 2 (initial + re-mint)", got)
	}
	if got := atomic.LoadInt32(&searchHits); got != 2 {
		t.Errorf("search hits = %d, want 2 (reject + retry)", got)
	}
}

// TestEmptyBodyIsNoPosts verifies the soft-block case (200 + empty body) is
// treated as zero posts, not a decode error.
func TestEmptyBodyIsNoPosts(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/hq", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "xq_a_token", Value: "tok"})
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/statuses/search.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json;charset=UTF-8")
		w.WriteHeader(http.StatusOK) // empty body
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New()
	c.baseURL = srv.URL

	posts, err := c.Posts(context.Background(), "AAPL", 0)
	if err != nil {
		t.Fatalf("Posts: %v", err)
	}
	if len(posts) != 0 {
		t.Errorf("len(posts) = %d, want 0", len(posts))
	}
}

func TestName(t *testing.T) {
	if got := New().Name(); got != "xueqiu" {
		t.Errorf("Name() = %q, want xueqiu", got)
	}
}

func TestCleanHTML(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"<p>hi</p>", "hi"},
		{"a &amp; b", "a & b"},
		{"  <b>x</b>  ", "x"},
		{"<a href=\"/S/AAPL\">$AAPL$</a> up", "$AAPL$ up"},
		{"plain", "plain"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := cleanHTML(tt.in); got != tt.want {
			t.Errorf("cleanHTML(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
