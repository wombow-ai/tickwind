package tickertick

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// stories JSON modelled on a real api.tickertick.com/feed response. The third
// story has an empty url and must be skipped.
const sampleFeed = `{
  "stories": [
    {
      "id": "3309673749654575630",
      "title": "Apple Loop: iPhone 18 Pro Specs",
      "url": "https://www.forbes.com/sites/ewanspence/2026/06/05/apple-news/",
      "site": "forbes.com",
      "time": 1780701776000,
      "favicon_url": "https://static.tickertick.com/website_icons/forbes.com.ico",
      "tags": ["aapl"],
      "description": "This week's Apple headlines.",
      "tickers": ["aapl"]
    },
    {
      "id": "-3761081248663816335",
      "title": "Why June 8 Could Be a Huge Day for Apple Stock",
      "url": "https://www.fool.com/investing/2026/06/05/why-june-8/",
      "site": "fool.com",
      "time": 1780682280000,
      "tags": ["aapl"],
      "tickers": ["aapl"]
    },
    {
      "id": "no-url-story",
      "title": "This one has no link and should be skipped",
      "url": "",
      "site": "example.com",
      "time": 1780668000000,
      "tickers": ["aapl"]
    }
  ],
  "last_id": "-3761081248663816335"
}`

func TestPostsParsesFeed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/feed" {
			t.Errorf("path = %q, want /feed", got)
		}
		if got := r.URL.Query().Get("q"); got == "" {
			t.Error("missing q query parameter")
		}
		if got := r.Header.Get("User-Agent"); got != "Tickwind/0.1 (+https://tickwind.com)" {
			t.Errorf("User-Agent = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sampleFeed))
	}))
	defer srv.Close()

	c := New()
	c.baseURL = srv.URL

	posts, err := c.Posts(context.Background(), "aapl", 30)
	if err != nil {
		t.Fatalf("Posts: %v", err)
	}

	// Three stories in, one URL-less → two posts out.
	if len(posts) != 2 {
		t.Fatalf("got %d posts, want 2", len(posts))
	}

	p := posts[0]
	if p.ID != "tickertick:3309673749654575630" {
		t.Errorf("ID = %q", p.ID)
	}
	if p.Source != "tickertick" {
		t.Errorf("Source = %q, want tickertick", p.Source)
	}
	if p.Ticker != "aapl" {
		t.Errorf("Ticker = %q, want aapl", p.Ticker)
	}
	if p.Author != "forbes.com" { // Author = site.
		t.Errorf("Author = %q, want forbes.com", p.Author)
	}
	if p.Body != "Apple Loop: iPhone 18 Pro Specs" { // Body = title.
		t.Errorf("Body = %q", p.Body)
	}
	if p.URL != "https://www.forbes.com/sites/ewanspence/2026/06/05/apple-news/" {
		t.Errorf("URL = %q", p.URL)
	}
	if want := time.UnixMilli(1780701776000); !p.CreatedAt.Equal(want) {
		t.Errorf("CreatedAt = %v, want %v", p.CreatedAt, want)
	}

	// Second story (no description) maps fine too.
	if posts[1].ID != "tickertick:-3761081248663816335" {
		t.Errorf("posts[1].ID = %q", posts[1].ID)
	}
}

func TestPostsRespectsLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleFeed))
	}))
	defer srv.Close()

	c := New()
	c.baseURL = srv.URL

	posts, err := c.Posts(context.Background(), "aapl", 1)
	if err != nil {
		t.Fatalf("Posts: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("got %d posts, want 1 (limit)", len(posts))
	}
	if posts[0].ID != "tickertick:3309673749654575630" {
		t.Errorf("ID = %q", posts[0].ID)
	}
}

func TestPostsFallsBackWhenEmpty(t *testing.T) {
	// First call (the narrow ugc/analysis query) returns no stories; the client
	// should retry with the plain ticker query, which returns one.
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(`{"stories":[],"last_id":""}`))
			return
		}
		_, _ = w.Write([]byte(sampleFeed))
	}))
	defer srv.Close()

	c := New()
	c.baseURL = srv.URL

	posts, err := c.Posts(context.Background(), "aapl", 30)
	if err != nil {
		t.Fatalf("Posts: %v", err)
	}
	if calls != 2 {
		t.Errorf("server calls = %d, want 2 (narrow + fallback)", calls)
	}
	if len(posts) != 2 {
		t.Fatalf("got %d posts, want 2", len(posts))
	}
}

func TestPostsErrorOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := New()
	c.baseURL = srv.URL

	if _, err := c.Posts(context.Background(), "aapl", 5); err == nil {
		t.Fatal("expected error on 429, got nil")
	}
}
