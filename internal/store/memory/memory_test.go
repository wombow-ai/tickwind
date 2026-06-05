package memory

import (
	"context"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

func TestWatchlist(t *testing.T) {
	s := New()
	ctx := context.Background()

	if wl, err := s.Watchlist(ctx); err != nil || len(wl) != 0 {
		t.Fatalf("new watchlist = %v, %v; want empty", wl, err)
	}

	// Adds normalize case, ignore blanks, and dedupe; order is preserved.
	for _, tk := range []string{"aapl", "NVDA", "AAPL", "   ", "tsla"} {
		if err := s.AddToWatchlist(ctx, tk); err != nil {
			t.Fatalf("add %q: %v", tk, err)
		}
	}
	if got, _ := s.Watchlist(ctx); !equal(got, []string{"AAPL", "NVDA", "TSLA"}) {
		t.Fatalf("watchlist = %v", got)
	}

	if err := s.RemoveFromWatchlist(ctx, "nvda"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got, _ := s.Watchlist(ctx); !equal(got, []string{"AAPL", "TSLA"}) {
		t.Fatalf("after remove = %v", got)
	}

	// The returned slice must be a copy, not the store's backing array.
	got, _ := s.Watchlist(ctx)
	got[0] = "MUTATED"
	if again, _ := s.Watchlist(ctx); again[0] != "AAPL" {
		t.Fatalf("watchlist not copied: %v", again)
	}
}

func TestFilingsDedupeOrderLimit(t *testing.T) {
	s := New()
	ctx := context.Background()
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	mustSaveFilings(t, s, "AAPL", []store.Filing{
		{Ticker: "AAPL", AccessionNo: "a", FiledAt: jan},
		{Ticker: "AAPL", AccessionNo: "b", FiledAt: feb},
	})
	mustSaveFilings(t, s, "AAPL", []store.Filing{{Ticker: "AAPL", AccessionNo: "a", FiledAt: jan}})

	got, err := s.ListFilings(ctx, "aapl", 0) // case-insensitive lookup
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d; want 2 (deduped by accession)", len(got))
	}
	if got[0].AccessionNo != "b" {
		t.Fatalf("first = %q; want newest first (b)", got[0].AccessionNo)
	}

	if lim, _ := s.ListFilings(ctx, "AAPL", 1); len(lim) != 1 || lim[0].AccessionNo != "b" {
		t.Fatalf("limit=1 = %v", lim)
	}
}

func TestQuoteLatestWins(t *testing.T) {
	s := New()
	ctx := context.Background()

	if _, ok, _ := s.GetQuote(ctx, "AAPL"); ok {
		t.Fatal("want no quote initially")
	}
	mustUpsertQuote(t, s, store.Quote{Ticker: "AAPL", Price: 1})
	mustUpsertQuote(t, s, store.Quote{Ticker: "AAPL", Price: 2})
	if q, ok, _ := s.GetQuote(ctx, "aapl"); !ok || q.Price != 2 {
		t.Fatalf("quote = %v, ok=%v; want latest 2", q, ok)
	}
}

func mustSaveFilings(t *testing.T, s *Store, ticker string, f []store.Filing) {
	t.Helper()
	if err := s.SaveFilings(context.Background(), ticker, f); err != nil {
		t.Fatalf("save filings: %v", err)
	}
}

func mustUpsertQuote(t *testing.T, s *Store, q store.Quote) {
	t.Helper()
	if err := s.UpsertQuote(context.Background(), q); err != nil {
		t.Fatalf("upsert quote: %v", err)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
