package apewisdom

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wombow-ai/tickwind/internal/store"
)

func TestSignalsParsesAndMaps(t *testing.T) {
	const body = `{"pages":1,"current_page":1,"results":[
		{"rank":1,"ticker":"TSLA","name":"Tesla","mentions":200,"upvotes":1000,"rank_24h_ago":3,"mentions_24h_ago":150},
		{"rank":2,"ticker":"AAPL","name":"Apple","mentions":120,"upvotes":640,"rank_24h_ago":2,"mentions_24h_ago":130},
		{"rank":3,"ticker":"NVDA","name":"Nvidia","mentions":90,"upvotes":300,"rank_24h_ago":1,"mentions_24h_ago":210}
	]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "Tickwind/0.1 (+https://tickwind.com)" {
			t.Errorf("User-Agent = %q", got)
		}
		if !strings.HasSuffix(r.URL.Path, "/page/1") {
			t.Errorf("path = %q, want .../page/1", r.URL.Path)
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := New()
	c.baseURL = srv.URL

	sigs, err := c.Signals(context.Background(), []string{"aapl", "TSLA", "ZZZZ"})
	if err != nil {
		t.Fatalf("Signals: %v", err)
	}
	if len(sigs) != 2 {
		t.Fatalf("got %d signals, want 2 (AAPL,TSLA; ZZZZ omitted)", len(sigs))
	}

	by := map[string]store.Signal{}
	for _, s := range sigs {
		by[s.Ticker] = s
	}
	tsla, ok := by["TSLA"]
	if !ok {
		t.Fatal("missing TSLA signal")
	}
	if tsla.Source != "apewisdom" || tsla.Kind != "buzz" {
		t.Errorf("TSLA source/kind = %q/%q, want apewisdom/buzz", tsla.Source, tsla.Kind)
	}
	if tsla.Mentions != 200 || tsla.MentionsPrev != 150 || tsla.Rank != 1 || tsla.RankPrev != 3 || tsla.Upvotes != 1000 {
		t.Errorf("TSLA fields = %+v", tsla)
	}
	if tsla.UpdatedAt.IsZero() {
		t.Error("TSLA UpdatedAt is zero")
	}
	if _, ok := by["ZZZZ"]; ok {
		t.Error("ZZZZ is not on the leaderboard and should be omitted")
	}
}

func TestSignalsScansMultiplePages(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, "/page/1"):
			_, _ = w.Write([]byte(`{"pages":2,"results":[{"rank":1,"ticker":"TSLA","mentions":10,"upvotes":5,"rank_24h_ago":1,"mentions_24h_ago":8}]}`))
		case strings.HasSuffix(r.URL.Path, "/page/2"):
			_, _ = w.Write([]byte(`{"pages":2,"results":[{"rank":101,"ticker":"AAPL","mentions":3,"upvotes":2,"rank_24h_ago":99,"mentions_24h_ago":4}]}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := New()
	c.baseURL = srv.URL

	sigs, err := c.Signals(context.Background(), []string{"AAPL"})
	if err != nil {
		t.Fatalf("Signals: %v", err)
	}
	if len(sigs) != 1 || sigs[0].Ticker != "AAPL" {
		t.Fatalf("got %+v, want exactly one AAPL signal", sigs)
	}
	if sigs[0].Mentions != 3 || sigs[0].Rank != 101 {
		t.Errorf("AAPL fields = %+v", sigs[0])
	}
	if len(paths) != 2 {
		t.Errorf("server hits = %d (%v), want 2 (page1 then page2)", len(paths), paths)
	}
}

func TestSignalsStopsWhenAllFound(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		// Claims 5 pages, but everything wanted is on page 1.
		_, _ = w.Write([]byte(`{"pages":5,"results":[{"rank":1,"ticker":"AAPL","mentions":10,"upvotes":5}]}`))
	}))
	defer srv.Close()

	c := New()
	c.baseURL = srv.URL

	sigs, err := c.Signals(context.Background(), []string{"AAPL"})
	if err != nil {
		t.Fatalf("Signals: %v", err)
	}
	if len(sigs) != 1 {
		t.Fatalf("got %d signals, want 1", len(sigs))
	}
	if hits != 1 {
		t.Errorf("server hits = %d, want 1 (stop scanning once all found)", hits)
	}
}

func TestSignalsErrorOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New()
	c.baseURL = srv.URL

	if _, err := c.Signals(context.Background(), []string{"AAPL"}); err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}

func TestSignalsEmptyRequestMakesNoCall(t *testing.T) {
	c := New() // baseURL is the real host; an empty request must not call out.
	sigs, err := c.Signals(context.Background(), nil)
	if err != nil {
		t.Fatalf("Signals: %v", err)
	}
	if sigs == nil || len(sigs) != 0 {
		t.Fatalf("want empty non-nil slice, got %#v", sigs)
	}
}
