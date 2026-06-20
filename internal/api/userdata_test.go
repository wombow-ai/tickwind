package api

import (
	"context"
	"strings"
	"testing"

	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/store/memory"
)

// TestChatUserDataPrivacyIsolation is the core privacy guard: the user-data tools must
// return ONLY the authenticated user's own data, never another user's.
func TestChatUserDataPrivacyIsolation(t *testing.T) {
	st := memory.New()
	ctx := context.Background()

	// User A.
	_ = st.AddToWatchlist(ctx, "userA", "AAPL")
	_ = st.SaveHolding(ctx, store.Holding{ID: "h1", UserID: "userA", Ticker: "AAPL", Shares: 10, AvgCost: 100})
	_ = st.SaveNote(ctx, store.Note{ID: "n1", UserID: "userA", Ticker: "AAPL", Body: "A's secret thesis"})
	// User B.
	_ = st.AddToWatchlist(ctx, "userB", "TSLA")
	_ = st.SaveHolding(ctx, store.Holding{ID: "h2", UserID: "userB", Ticker: "TSLA", Shares: 5, AvgCost: 200})
	_ = st.SaveNote(ctx, store.Note{ID: "n2", UserID: "userB", Ticker: "TSLA", Body: "B's private note"})

	ud := NewChatUserData(st)

	wl := ud.Watchlist(ctx, "userA", "en")
	if !strings.Contains(wl, "AAPL") || strings.Contains(wl, "TSLA") {
		t.Fatalf("watchlist leaked across users: %q", wl)
	}
	hd := ud.Holdings(ctx, "userA", "en")
	if !strings.Contains(hd, "AAPL") || strings.Contains(hd, "TSLA") {
		t.Fatalf("holdings leaked across users: %q", hd)
	}
	nt := ud.Notes(ctx, "userA", "", "en")
	if !strings.Contains(nt, "A's secret thesis") || strings.Contains(nt, "B's private note") {
		t.Fatalf("notes leaked across users: %q", nt)
	}

	// Holdings P&L is Go-computed (no quote here → no-price branch, but still A-only).
	if strings.Contains(hd, "B's") {
		t.Fatal("holdings must never reference another user")
	}
	// Empty user sees nothing.
	if w := ud.Watchlist(ctx, "nobody", "en"); strings.Contains(w, "AAPL") || strings.Contains(w, "TSLA") {
		t.Fatalf("unknown user got data: %q", w)
	}
}

// TestChatUserDataHoldingsEmitsAbsolutePnL guards the anti-hallucination fix: the
// Holdings string fed to the chat model must carry the Go-computed absolute
// unrealized P&L ($), not just gain% + price — otherwise the model mislabels the
// share price as the unrealized $ (observed in the live E2E). Numbers stay Go-owned.
func TestChatUserDataHoldingsEmitsAbsolutePnL(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	if err := st.SaveHolding(ctx, store.Holding{ID: "h1", UserID: "u1", Ticker: "AAPL", Shares: 10, AvgCost: 150}); err != nil {
		t.Fatalf("SaveHolding: %v", err)
	}
	if err := st.UpsertQuote(ctx, store.Quote{Ticker: "AAPL", Price: 300, PrevClose: 290}); err != nil {
		t.Fatalf("UpsertQuote: %v", err)
	}
	out := NewChatUserData(st).Holdings(ctx, "u1", "en")
	// gain = (300-150)*10 = 1500; gainPct = +100.0%; value = 3000.
	if !strings.Contains(out, "P&L $+1500") {
		t.Errorf("holdings missing Go-computed absolute P&L: %q", out)
	}
	if !strings.Contains(out, "+100.0%") {
		t.Errorf("holdings missing gain%%: %q", out)
	}
	if !strings.Contains(out, "$300.00") {
		t.Errorf("holdings missing current price: %q", out)
	}
}
