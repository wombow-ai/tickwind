package ingest

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/guru"
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/substack"
)

// fakeGuruFeeds returns canned posts per feed URL.
type fakeGuruFeeds struct{ byURL map[string][]substack.Post }

func (f fakeGuruFeeds) Posts(_ context.Context, url string) ([]substack.Post, error) {
	return f.byURL[url], nil
}

// fakeGuruStore records SaveSocial calls.
type fakeGuruStore struct{ saved map[string][]store.Post }

func (s *fakeGuruStore) SaveSocial(_ context.Context, ticker string, posts []store.Post) error {
	if s.saved == nil {
		s.saved = map[string][]store.Post{}
	}
	s.saved[ticker] = append(s.saved[ticker], posts...)
	return nil
}

func TestGuruIngestorRefresh(t *testing.T) {
	feeds := []substack.Feed{{Name: "Serenity", URL: "u1"}}
	client := fakeGuruFeeds{byURL: map[string][]substack.Post{
		"u1": {
			{
				Title:     "Sivers thesis",
				URL:       "https://x/p/sivers",
				Published: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
				Tickers:   []string{"SIVE", "POET"},
			},
			{Title: "No tickers here", URL: "https://x/p/none", Tickers: nil}, // kept on rail, no fan-out
		},
	}}
	st := &fakeGuruStore{}
	cache := guru.NewCache()
	ing := NewGuruIngestor(client, feeds, cache, st, 60, time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)))

	ing.refresh(context.Background())

	// Rail: BOTH posts (tickers optional now) — the dated, anchored one sorts first.
	if rail := cache.Get(); len(rail) != 2 || rail[0].Title != "Sivers thesis" || rail[0].Author != "Serenity" {
		t.Fatalf("rail=%v want 2 items, Sivers/Serenity first", rail)
	}
	// Discussion: only the ticker-anchored post fans out (the untagged one cannot).
	if len(st.saved["SIVE"]) != 1 || len(st.saved["POET"]) != 1 {
		t.Fatalf("saved=%v want SIVE+POET", st.saved)
	}
	if len(st.saved) != 2 {
		t.Fatalf("saved keys=%v want exactly SIVE+POET (untagged post must not fan out)", st.saved)
	}
	p := st.saved["SIVE"][0]
	if p.Source != guruSource || p.Author != "Serenity" || p.Ticker != "SIVE" || p.Body != "Sivers thesis" {
		t.Errorf("bad post: %+v", p)
	}
	if !strings.HasPrefix(p.ID, "substack:") {
		t.Errorf("id=%q want substack: prefix", p.ID)
	}
	// Same post → same id across tickers (dedupe key).
	if st.saved["POET"][0].ID != p.ID {
		t.Errorf("id differs across tickers: %q vs %q", st.saved["POET"][0].ID, p.ID)
	}
}

// A nil store disables Discussion fan-out but still builds the rail.
func TestGuruIngestorNilStore(t *testing.T) {
	feeds := []substack.Feed{{Name: "Serenity", URL: "u1"}}
	client := fakeGuruFeeds{byURL: map[string][]substack.Post{
		"u1": {{Title: "x", URL: "https://x/p/1", Tickers: []string{"AAA"}}},
	}}
	cache := guru.NewCache()
	ing := NewGuruIngestor(client, feeds, cache, nil, 60, time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ing.refresh(context.Background()) // must not panic
	if len(cache.Get()) != 1 {
		t.Fatalf("rail=%d want 1", len(cache.Get()))
	}
}
