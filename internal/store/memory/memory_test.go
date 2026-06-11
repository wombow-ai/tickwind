package memory

import (
	"context"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

func TestAlertsCRUD(t *testing.T) {
	s := New()
	ctx := context.Background()
	a := store.Alert{ID: "a1", UserID: "u1", Ticker: "AAPL", Kind: "price_above", Threshold: 200, Active: true, CreatedAt: time.Now()}
	if err := s.SaveAlert(ctx, a); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ListAlerts(ctx, "u2"); len(got) != 0 {
		t.Errorf("u2 sees %d alerts, want 0 (scoped per user)", len(got))
	}
	got, err := s.ListAlerts(ctx, "u1")
	if err != nil || len(got) != 1 || got[0].Ticker != "AAPL" || got[0].Threshold != 200 {
		t.Fatalf("ListAlerts(u1) = %+v, err %v", got, err)
	}
	if ok, _ := s.DeleteAlert(ctx, "u2", "a1"); ok {
		t.Error("u2 deleted u1's alert (ownership not enforced)")
	}
	if ok, _ := s.DeleteAlert(ctx, "u1", "a1"); !ok {
		t.Error("owner delete returned false")
	}
	if got, _ := s.ListAlerts(ctx, "u1"); len(got) != 0 {
		t.Errorf("after delete: %d alerts, want 0", len(got))
	}
}

func TestHoldingsCRUD(t *testing.T) {
	s := New()
	ctx := context.Background()
	now := time.Now()
	mk := func(id, ticker string, shares, cost float64) store.Holding {
		return store.Holding{ID: id, UserID: "u1", Ticker: ticker, Shares: shares, AvgCost: cost, CreatedAt: now, UpdatedAt: now}
	}
	if err := s.SaveHolding(ctx, mk("h1", "AAPL", 10, 150)); err != nil {
		t.Fatal(err)
	}
	// Re-saving the same ticker upserts (still one row, updated shares).
	if err := s.SaveHolding(ctx, mk("h2", "AAPL", 25, 160)); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListHoldings(ctx, "u1")
	if err != nil || len(got) != 1 || got[0].Shares != 25 || got[0].Ticker != "AAPL" {
		t.Fatalf("after upsert ListHoldings = %+v (err %v), want 1 AAPL row w/ 25 shares", got, err)
	}
	aaplID := got[0].ID
	// A different ticker is a separate row; scoping is per user.
	if err := s.SaveHolding(ctx, mk("h3", "MSFT", 5, 400)); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ListHoldings(ctx, "u1"); len(got) != 2 {
		t.Fatalf("after MSFT: %d rows, want 2", len(got))
	}
	if got, _ := s.ListHoldings(ctx, "u2"); len(got) != 0 {
		t.Errorf("u2 sees %d holdings, want 0 (per-user)", len(got))
	}
	if ok, _ := s.DeleteHolding(ctx, "u2", aaplID); ok {
		t.Error("u2 deleted u1's holding (ownership not enforced)")
	}
	if ok, _ := s.DeleteHolding(ctx, "u1", aaplID); !ok {
		t.Error("owner delete returned false")
	}
	if got, _ := s.ListHoldings(ctx, "u1"); len(got) != 1 {
		t.Errorf("after delete: %d rows, want 1", len(got))
	}
}

func TestEarningsCRUD(t *testing.T) {
	s := New()
	ctx := context.Background()
	d := func(v string) time.Time { tm, _ := time.Parse("2006-01-02", v); return tm }
	if err := s.SaveEarnings(ctx, []store.Earning{
		{Ticker: "AAPL", Date: d("2026-06-12"), Hour: "amc"},
		{Ticker: "MSFT", Date: d("2026-06-20"), Hour: "bmo"},
		{Ticker: "AAPL", Date: d("2026-09-15"), Hour: "amc"},
	}); err != nil {
		t.Fatal(err)
	}
	// Upsert by (ticker,date): re-saving AAPL 06-12 updates, not duplicates.
	if err := s.SaveEarnings(ctx, []store.Earning{{Ticker: "AAPL", Date: d("2026-06-12"), Hour: "bmo"}}); err != nil {
		t.Fatal(err)
	}
	jun, _ := s.ListEarnings(ctx, d("2026-06-01"), d("2026-06-30"))
	if len(jun) != 2 { // AAPL 06-12 + MSFT 06-20 (09-15 out of range)
		t.Fatalf("ListEarnings(June) = %d, want 2: %+v", len(jun), jun)
	}
	if jun[0].Date.After(jun[1].Date) {
		t.Error("ListEarnings should be ascending by date")
	}
	for _, e := range jun {
		if e.Ticker == "AAPL" && e.Hour != "bmo" {
			t.Errorf("AAPL hour = %q, want bmo (upsert applied)", e.Hour)
		}
	}
	aapl, _ := s.ListEarningsForTicker(ctx, "aapl", 10)
	if len(aapl) != 2 { // 06-12 + 09-15
		t.Fatalf("AAPL earnings = %d, want 2", len(aapl))
	}
}

func TestAlertsActiveAndTrigger(t *testing.T) {
	s := New()
	ctx := context.Background()
	_ = s.SaveAlert(ctx, store.Alert{ID: "x1", UserID: "u1", Ticker: "AAPL", Kind: "price_above", Threshold: 200, Active: true, CreatedAt: time.Now()})
	_ = s.SaveAlert(ctx, store.Alert{ID: "x2", UserID: "u2", Ticker: "MSTR", Kind: "price_below", Threshold: 100, Active: true, CreatedAt: time.Now()})

	// ListActiveAlerts spans all users.
	if active, err := s.ListActiveAlerts(ctx); err != nil || len(active) != 2 {
		t.Fatalf("ListActiveAlerts = %d, %v; want 2", len(active), err)
	}
	// Triggering drops it from the active set + stamps TriggeredAt.
	if err := s.MarkAlertTriggered(ctx, "x1", time.Now()); err != nil {
		t.Fatal(err)
	}
	active, _ := s.ListActiveAlerts(ctx)
	if len(active) != 1 || active[0].ID != "x2" {
		t.Fatalf("after trigger, active = %+v; want only x2", active)
	}
	if got, _ := s.ListAlerts(ctx, "u1"); len(got) != 1 || got[0].TriggeredAt.IsZero() {
		t.Errorf("x1 TriggeredAt not set: %+v", got)
	}
}

func TestWatchlist(t *testing.T) {
	s := New()
	ctx := context.Background()
	const u = "user-1"

	if wl, err := s.Watchlist(ctx, u); err != nil || len(wl) != 0 {
		t.Fatalf("new watchlist = %v, %v; want empty", wl, err)
	}

	// Adds normalize case, ignore blanks, and dedupe; order is preserved.
	for _, tk := range []string{"aapl", "NVDA", "AAPL", "   ", "tsla"} {
		if err := s.AddToWatchlist(ctx, u, tk); err != nil {
			t.Fatalf("add %q: %v", tk, err)
		}
	}
	if got, _ := s.Watchlist(ctx, u); !equal(got, []string{"AAPL", "NVDA", "TSLA"}) {
		t.Fatalf("watchlist = %v", got)
	}

	if err := s.RemoveFromWatchlist(ctx, u, "nvda"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got, _ := s.Watchlist(ctx, u); !equal(got, []string{"AAPL", "TSLA"}) {
		t.Fatalf("after remove = %v", got)
	}

	// The returned slice must be a copy, not the store's backing array.
	got, _ := s.Watchlist(ctx, u)
	got[0] = "MUTATED"
	if again, _ := s.Watchlist(ctx, u); again[0] != "AAPL" {
		t.Fatalf("watchlist not copied: %v", again)
	}
}

func TestWatchlistPerUserAndUnion(t *testing.T) {
	s := New()
	ctx := context.Background()
	_ = s.AddToWatchlist(ctx, "u1", "AAPL")
	_ = s.AddToWatchlist(ctx, "u1", "NVDA")
	_ = s.AddToWatchlist(ctx, "u2", "NVDA")
	_ = s.AddToWatchlist(ctx, "u2", "TSLA")

	if got, _ := s.Watchlist(ctx, "u1"); !equal(got, []string{"AAPL", "NVDA"}) {
		t.Fatalf("u1 = %v", got)
	}
	if got, _ := s.Watchlist(ctx, "u2"); !equal(got, []string{"NVDA", "TSLA"}) {
		t.Fatalf("u2 = %v", got)
	}
	// Union is deduped (AAPL, NVDA, TSLA).
	if all, _ := s.AllWatchlistTickers(ctx); len(all) != 3 {
		t.Fatalf("union = %v; want 3 distinct", all)
	}
}

func TestClipsPerUser(t *testing.T) {
	s := New()
	ctx := context.Background()
	_ = s.SaveClip(ctx, store.Clip{ID: "c1", UserID: "u1", Ticker: "AAPL", Title: "a", URL: "x"})
	_ = s.SaveClip(ctx, store.Clip{ID: "c2", UserID: "u1", Ticker: "NVDA", Title: "b", URL: "y"})
	_ = s.SaveClip(ctx, store.Clip{ID: "c3", UserID: "u2", Ticker: "AAPL", Title: "c", URL: "z"})

	if got, _ := s.ListClips(ctx, "u1", "AAPL", 0); len(got) != 1 || got[0].ID != "c1" {
		t.Fatalf("u1 AAPL clips = %v", got)
	}
	if got, _ := s.ListClips(ctx, "u2", "AAPL", 0); len(got) != 1 || got[0].ID != "c3" {
		t.Fatalf("u2 AAPL clips = %v", got)
	}
	if got, _ := s.ListClips(ctx, "u1", "NVDA", 0); len(got) != 1 {
		t.Fatalf("u1 NVDA clips = %v", got)
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

func TestSeenForm4(t *testing.T) {
	s := New()
	ctx := context.Background()
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	if err := s.MarkForm4Seen(ctx, []string{"acc-old"}, old); err != nil {
		t.Fatalf("mark old: %v", err)
	}
	if err := s.MarkForm4Seen(ctx, []string{"acc-a", "acc-b", "", "acc-a"}, recent); err != nil {
		t.Fatalf("mark recent: %v", err)
	}

	// Only accessions on/after `since` come back (the empty one is dropped, the
	// duplicate is collapsed by the map).
	got, err := s.SeenForm4Since(ctx, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("seen since: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %v, want [acc-a acc-b]", got)
	}
	set := map[string]bool{got[0]: true, got[1]: true}
	if !set["acc-a"] || !set["acc-b"] || set["acc-old"] || set[""] {
		t.Errorf("unexpected seen set: %v", got)
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

func TestUpdateComment(t *testing.T) {
	s := New()
	ctx := context.Background()
	c := store.Comment{ID: "c1", UserID: "u1", Author: "alice", Ticker: "AAPL", Body: "first", CreatedAt: time.Now()}
	if err := s.SaveComment(ctx, c); err != nil {
		t.Fatal(err)
	}

	// Author edits → ok, body updated, EditedAt set, mentions replaced.
	got, ok, err := s.UpdateComment(ctx, "c1", "u1", "edited body $NVDA", []string{"NVDA"})
	if err != nil || !ok {
		t.Fatalf("UpdateComment author: ok=%v err=%v", ok, err)
	}
	if got.Body != "edited body $NVDA" || got.EditedAt == nil {
		t.Fatalf("got body=%q editedAt=%v, want edited + non-nil", got.Body, got.EditedAt)
	}
	// The mention fans the comment out into NVDA's list too.
	if nv, _ := s.ListComments(ctx, "NVDA", 10); len(nv) != 1 || nv[0].ID != "c1" {
		t.Fatalf("NVDA list after mention edit = %v, want the mentioning comment", nv)
	}

	// Non-author cannot edit.
	if _, ok, _ := s.UpdateComment(ctx, "c1", "u2", "hijack", nil); ok {
		t.Error("non-author edit should fail")
	}
	// Unknown id.
	if _, ok, _ := s.UpdateComment(ctx, "nope", "u1", "x", nil); ok {
		t.Error("unknown id edit should fail")
	}
	// The non-author attempt must not have changed the body.
	list, _ := s.ListComments(ctx, "AAPL", 10)
	if len(list) != 1 || list[0].Body != "edited body $NVDA" {
		t.Fatalf("after edits, list=%+v", list)
	}
}

func TestLikeComment(t *testing.T) {
	s := New()
	ctx := context.Background()
	if err := s.SaveComment(ctx, store.Comment{ID: "c1", UserID: "u1", Ticker: "AAPL", Body: "hi", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	// u2 likes → liked, count 1.
	liked, n, ok, _ := s.LikeComment(ctx, "c1", "u2")
	if !ok || !liked || n != 1 {
		t.Fatalf("like: ok=%v liked=%v n=%d, want true/true/1", ok, liked, n)
	}
	// u3 likes → count 2.
	if _, n, _, _ := s.LikeComment(ctx, "c1", "u3"); n != 2 {
		t.Fatalf("second like count=%d, want 2", n)
	}
	// u2 toggles off → liked=false, count 1.
	if liked, n, _, _ := s.LikeComment(ctx, "c1", "u2"); liked || n != 1 {
		t.Fatalf("toggle off: liked=%v n=%d, want false/1", liked, n)
	}
	// Count surfaces in ListComments.
	list, _ := s.ListComments(ctx, "AAPL", 10)
	if len(list) != 1 || list[0].Likes != 1 {
		t.Fatalf("list likes=%d, want 1", list[0].Likes)
	}
	// Unknown comment → ok=false.
	if _, _, ok, _ := s.LikeComment(ctx, "nope", "u2"); ok {
		t.Error("like unknown comment should be ok=false")
	}
}
