package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/store/memory"
)

// TestSplitPruneRoutesToMarket verifies Pruner calls hit only the durable Market
// store; the User store is left untouched.
func TestSplitPruneRoutesToMarket(t *testing.T) {
	ctx := context.Background()
	market := memory.New()
	user := memory.New()
	s := store.Split{Market: market, User: user}
	now := time.Now().UTC()
	old := now.AddDate(0, 0, -100)

	// Same old news in both backends directly (bypassing the Split's own routing).
	for _, m := range []*memory.Store{market, user} {
		if err := m.SaveNews(ctx, "T", []store.News{{Ticker: "T", ID: "old", Published: old}}); err != nil {
			t.Fatal(err)
		}
	}

	n, err := s.PruneNews(ctx, now.AddDate(0, 0, -60), now.AddDate(0, 0, -120))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("pruned %d rows, want 1 (Market only)", n)
	}
	if mn, _ := market.ListNews(ctx, "T", 10); len(mn) != 0 {
		t.Errorf("Market news = %d, want 0 (pruned)", len(mn))
	}
	if un, _ := user.ListNews(ctx, "T", 10); len(un) != 1 {
		t.Errorf("User news = %d, want 1 (untouched)", len(un))
	}
}
