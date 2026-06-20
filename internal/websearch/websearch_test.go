package websearch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDisabledWithoutKey: no API key → Enabled()=false and Search is a no-op (no error, nil).
func TestDisabledWithoutKey(t *testing.T) {
	c := New(Config{})
	if c.Enabled() {
		t.Fatal("Enabled() true with no API key")
	}
	res, err := c.Search(context.Background(), "anything", 5)
	if err != nil || res != nil {
		t.Fatalf("disabled Search = (%v, %v); want (nil, nil)", res, err)
	}
}

// TestSearchParse: a Tavily-shaped response is parsed into attributed results, fields trimmed.
func TestSearchParse(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = io.WriteString(w, `{"results":[
			{"title":" Apple unveils X ","url":"https://www.reuters.com/tech/apple","content":" Apple announced a new product. "},
			{"title":"Q3 outlook","url":"https://bloomberg.com/x","content":"Analysts weigh in."}
		]}`)
	}))
	defer srv.Close()

	c := New(Config{APIKey: "k", BaseURL: srv.URL})
	if !c.Enabled() {
		t.Fatal("Enabled() false with an API key set")
	}
	res, err := c.Search(context.Background(), "apple news", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("want 2 results, got %d", len(res))
	}
	if res[0].Title != "Apple unveils X" || res[0].Snippet != "Apple announced a new product." {
		t.Fatalf("result not trimmed/parsed: %+v", res[0])
	}
	if res[0].URL != "https://www.reuters.com/tech/apple" {
		t.Fatalf("url = %q", res[0].URL)
	}
	// the request must carry the api key + query + a bounded max_results.
	if gotBody["api_key"] != "k" || gotBody["query"] != "apple news" {
		t.Fatalf("request body missing key/query: %v", gotBody)
	}
}

// TestSearchNon2xx: a non-2xx upstream status is surfaced as an error (never silently empty).
func TestSearchNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"bad key"}`)
	}))
	defer srv.Close()

	c := New(Config{APIKey: "k", BaseURL: srv.URL})
	_, err := c.Search(context.Background(), "q", 5)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("want a 401 error, got %v", err)
	}
}
