package store_test

import (
	"context"
	"testing"

	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/store/memory"
)

func TestSplitRoutesMarketAndUser(t *testing.T) {
	ctx := context.Background()
	market := memory.New()
	user := memory.New()
	s := store.Split{Market: market, User: user}

	// A market-data write lands in Market, not User.
	if err := s.UpsertSecurity(ctx, store.Security{Ticker: "AAPL", Name: "Apple", Market: "US"}); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := market.GetSecurity(ctx, "AAPL"); !ok {
		t.Error("security should be in the Market store")
	}
	if _, ok, _ := user.GetSecurity(ctx, "AAPL"); ok {
		t.Error("security must NOT be in the User store")
	}

	// A per-user write lands in User, not Market.
	const uid = "11111111-1111-1111-1111-111111111111"
	if err := s.AddToWatchlist(ctx, uid, "NVDA"); err != nil {
		t.Fatal(err)
	}
	if got, _ := user.Watchlist(ctx, uid); len(got) != 1 || got[0] != "NVDA" {
		t.Errorf("watchlist should be in the User store; got %v", got)
	}
	if got, _ := market.Watchlist(ctx, uid); len(got) != 0 {
		t.Errorf("watchlist must NOT be in the Market store; got %v", got)
	}

	// Reads route the same way through Split.
	if got, _ := s.Watchlist(ctx, uid); len(got) != 1 || got[0] != "NVDA" {
		t.Errorf("Split.Watchlist = %v; want [NVDA]", got)
	}
	if _, ok, _ := s.GetSecurity(ctx, "AAPL"); !ok {
		t.Error("Split.GetSecurity should find AAPL via Market")
	}
}
