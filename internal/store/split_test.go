package store_test

import (
	"context"
	"encoding/json"
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

	// Fear & Greed history is public market data → it must route to Market.
	if err := s.SaveFearGreed(ctx, "2026-06-14", 55); err != nil {
		t.Fatal(err)
	}
	if got, _ := market.FearGreedHistory(ctx, 0); len(got) != 1 || got[0].Score != 55 {
		t.Errorf("fear&greed should be in the Market store; got %v", got)
	}
	if got, _ := user.FearGreedHistory(ctx, 0); len(got) != 0 {
		t.Errorf("fear&greed must NOT be in the User store; got %v", got)
	}
	if got, _ := s.FearGreedHistory(ctx, 0); len(got) != 1 || got[0].Date != "2026-06-14" {
		t.Errorf("Split.FearGreedHistory = %v; want one 2026-06-14 point via Market", got)
	}

	// Per-user prefs are cheap-to-rebuild UI state → they must route to User.
	blob := json.RawMessage(`{"indicators":{"ids":["technical.rsi"]}}`)
	if err := s.PutPrefs(ctx, uid, blob); err != nil {
		t.Fatal(err)
	}
	if got, ok, _ := user.GetPrefs(ctx, uid); !ok || string(got) != string(blob) {
		t.Errorf("prefs should be in the User store; got (%s, %v)", got, ok)
	}
	if _, ok, _ := market.GetPrefs(ctx, uid); ok {
		t.Error("prefs must NOT be in the Market store")
	}
	if got, ok, _ := s.GetPrefs(ctx, uid); !ok || string(got) != string(blob) {
		t.Errorf("Split.GetPrefs = (%s, %v); want the blob via User", got, ok)
	}

	// The deep-research MONTHLY quota counter is per-user state → it must route to
	// the cheap-to-rebuild User store, never the durable Market store.
	const period = "2026-06" // ET-month period key
	if err := s.IncrDeepQuotaUsed(ctx, uid, period); err != nil {
		t.Fatal(err)
	}
	if used, _ := user.GetDeepQuotaUsed(ctx, uid, period); used != 1 {
		t.Errorf("deep quota should be in the User store; got %d", used)
	}
	if used, _ := market.GetDeepQuotaUsed(ctx, uid, period); used != 0 {
		t.Errorf("deep quota must NOT be in the Market store; got %d", used)
	}
	if used, _ := s.GetDeepQuotaUsed(ctx, uid, period); used != 1 {
		t.Errorf("Split.GetDeepQuotaUsed = %d; want 1 via User", used)
	}
}
