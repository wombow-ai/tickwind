package openfigi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMapResolvesAndCaches(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		// Aligned with the request: first CUSIP maps, second is a no-match warning.
		_, _ = w.Write([]byte(`[{"data":[{"ticker":"AAPL","exchCode":"US"}]},{"warning":"No identifier found."}]`))
	}))
	defer srv.Close()

	c := New("")
	c.url = srv.URL
	got, err := c.Map(context.Background(), []string{"037833100", "11111AAA1"})
	if err != nil {
		t.Fatal(err)
	}
	if got["037833100"] != "AAPL" {
		t.Errorf("AAPL mapping = %q, want AAPL", got["037833100"])
	}
	if _, ok := got["11111AAA1"]; ok {
		t.Error("unmappable CUSIP should be absent from the result")
	}

	// A repeat lookup is served from cache (misses cached too) — no new HTTP call.
	got2, err := c.Map(context.Background(), []string{"037833100", "11111AAA1"})
	if err != nil {
		t.Fatal(err)
	}
	if got2["037833100"] != "AAPL" {
		t.Error("cached mapping lost")
	}
	if calls != 1 {
		t.Errorf("want 1 HTTP call (second served from cache), got %d", calls)
	}
}
