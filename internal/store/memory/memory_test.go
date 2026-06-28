package memory

import (
	"context"
	"encoding/json"
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
	// Trigger then re-arm: ReactivateAlert clears triggered_at + re-activates,
	// and only for the owner.
	if err := s.MarkAlertTriggered(ctx, "a1", time.Now()); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.ReactivateAlert(ctx, "u2", "a1"); ok {
		t.Error("u2 re-armed u1's alert (ownership not enforced)")
	}
	if ok, _ := s.ReactivateAlert(ctx, "u1", "a1"); !ok {
		t.Error("owner reactivate returned false")
	}
	if got, _ := s.ListAlerts(ctx, "u1"); len(got) != 1 || !got[0].TriggeredAt.IsZero() || !got[0].Active {
		t.Fatalf("after reactivate: %+v, want active + triggered_at zero", got)
	}
	if ok, _ := s.ReactivateAlert(ctx, "u1", "nope"); ok {
		t.Error("reactivate unknown id returned true")
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

func TestFearGreedHistory(t *testing.T) {
	s := New()
	ctx := context.Background()

	// Empty store → non-nil, empty slice.
	if got, err := s.FearGreedHistory(ctx, 0); err != nil {
		t.Fatalf("history (empty): %v", err)
	} else if got == nil || len(got) != 0 {
		t.Fatalf("history (empty) = %#v, want non-nil empty slice", got)
	}

	// Save out of order; a blank date is ignored.
	for _, p := range []store.FearGreedPoint{
		{Date: "2026-06-12", Score: 40},
		{Date: "2026-06-10", Score: 20},
		{Date: "2026-06-11", Score: 30},
		{Date: "", Score: 99},
	} {
		if err := s.SaveFearGreed(ctx, p.Date, p.Score); err != nil {
			t.Fatalf("save %q: %v", p.Date, err)
		}
	}

	// All days, chronological (oldest→newest); the blank date never stored.
	got, err := s.FearGreedHistory(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []store.FearGreedPoint{
		{Date: "2026-06-10", Score: 20},
		{Date: "2026-06-11", Score: 30},
		{Date: "2026-06-12", Score: 40},
	}
	if len(got) != len(want) {
		t.Fatalf("history = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("history[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	// Upsert: re-saving the same date replaces the score, not appends.
	if err := s.SaveFearGreed(ctx, "2026-06-11", 35); err != nil {
		t.Fatal(err)
	}
	got, _ = s.FearGreedHistory(ctx, 0)
	if len(got) != 3 {
		t.Fatalf("len after upsert = %d, want 3 (no new row)", len(got))
	}
	if got[1].Date != "2026-06-11" || got[1].Score != 35 {
		t.Fatalf("upserted point = %+v, want {2026-06-11 35}", got[1])
	}

	// Limit is a tail: the most recent N days, still chronological.
	got, _ = s.FearGreedHistory(ctx, 2)
	if len(got) != 2 || got[0].Date != "2026-06-11" || got[1].Date != "2026-06-12" {
		t.Fatalf("limited history = %+v, want the last two days chronologically", got)
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
	if nv, _ := s.ListComments(ctx, "NVDA", 10, ""); len(nv) != 1 || nv[0].ID != "c1" {
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
	list, _ := s.ListComments(ctx, "AAPL", 10, "")
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
	list, _ := s.ListComments(ctx, "AAPL", 10, "")
	if len(list) != 1 || list[0].Likes != 1 {
		t.Fatalf("list likes=%d, want 1", list[0].Likes)
	}
	// Unknown comment → ok=false.
	if _, _, ok, _ := s.LikeComment(ctx, "nope", "u2"); ok {
		t.Error("like unknown comment should be ok=false")
	}
}

func TestPrefsRoundTrip(t *testing.T) {
	s := New()
	ctx := context.Background()

	// Before any PutPrefs → ok=false, nil blob.
	if blob, ok, err := s.GetPrefs(ctx, "u1"); err != nil || ok || blob != nil {
		t.Fatalf("GetPrefs before any set = (%s, %v, %v); want (nil, false, nil)", blob, ok, err)
	}

	// Set → get returns the stored blob.
	first := json.RawMessage(`{"indicators":{"ids":["technical.rsi"]}}`)
	if err := s.PutPrefs(ctx, "u1", first); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.GetPrefs(ctx, "u1")
	if err != nil || !ok || string(got) != string(first) {
		t.Fatalf("GetPrefs after set = (%s, %v, %v); want the stored blob, true, nil", got, ok, err)
	}

	// The returned slice is a COPY: mutating it must not corrupt the store.
	got[0] = 'X'
	if again, _, _ := s.GetPrefs(ctx, "u1"); string(again) != string(first) {
		t.Fatalf("mutating the returned slice leaked into the store: %s", again)
	}

	// PutPrefs must store a COPY: mutating the caller's slice must not leak in.
	src := json.RawMessage(`{"indicators":{"ids":["a"]}}`)
	if err := s.PutPrefs(ctx, "u2", src); err != nil {
		t.Fatal(err)
	}
	src[2] = 'Z' // mutate after the call
	if got2, _, _ := s.GetPrefs(ctx, "u2"); string(got2) != `{"indicators":{"ids":["a"]}}` {
		t.Fatalf("PutPrefs aliased the caller's slice: %s", got2)
	}

	// Overwrite replaces the whole blob.
	second := json.RawMessage(`{"indicators":{"ids":["technical.macd","technical.boll"]}}`)
	if err := s.PutPrefs(ctx, "u1", second); err != nil {
		t.Fatal(err)
	}
	if got3, _, _ := s.GetPrefs(ctx, "u1"); string(got3) != string(second) {
		t.Fatalf("after overwrite GetPrefs = %s; want %s", got3, second)
	}

	// Prefs are per-user: u2 is unaffected by u1's writes.
	if got4, ok, _ := s.GetPrefs(ctx, "u2"); !ok || string(got4) != `{"indicators":{"ids":["a"]}}` {
		t.Fatalf("u2 prefs = (%s, %v); want u2's own blob", got4, ok)
	}
}

// TestDeepQuotaMonthly verifies the deep-research quota counter is keyed by
// (userID, period) where period is now an ET-MONTH string: it is per-user and
// per-period, a new period starts fresh, and an OLD per-day-style key (the legacy
// daily scheme) never collides with a month key (so old rows are harmless).
func TestDeepQuotaMonthly(t *testing.T) {
	s := New()
	ctx := context.Background()
	const month = "2026-06" // ET-month period key (the new scheme)

	// No row yet → 0.
	if used, err := s.GetDeepQuotaUsed(ctx, "u1", month); err != nil || used != 0 {
		t.Fatalf("GetDeepQuotaUsed before any incr = (%d, %v); want (0, nil)", used, err)
	}

	// Two increments in the same month accumulate.
	for i := 0; i < 2; i++ {
		if err := s.IncrDeepQuotaUsed(ctx, "u1", month); err != nil {
			t.Fatal(err)
		}
	}
	if used, _ := s.GetDeepQuotaUsed(ctx, "u1", month); used != 2 {
		t.Fatalf("u1 %s used = %d; want 2", month, used)
	}

	// Per-user: u2 is unaffected.
	if used, _ := s.GetDeepQuotaUsed(ctx, "u2", month); used != 0 {
		t.Fatalf("u2 %s used = %d; want 0 (per-user)", month, used)
	}

	// Per-period: the next month starts fresh (the monthly allowance rolls over).
	if used, _ := s.GetDeepQuotaUsed(ctx, "u1", "2026-07"); used != 0 {
		t.Fatalf("u1 next-month used = %d; want 0 (monthly rollover)", used)
	}

	// A legacy per-DAY key ("2026-06-15") is a DISTINCT counter from the month key
	// ("2026-06"): incrementing the day key must not affect the month total, proving
	// old daily rows are harmless dead weight after the day→month switch.
	if err := s.IncrDeepQuotaUsed(ctx, "u1", "2026-06-15"); err != nil {
		t.Fatal(err)
	}
	if used, _ := s.GetDeepQuotaUsed(ctx, "u1", month); used != 2 {
		t.Fatalf("u1 %s used after a legacy day-key incr = %d; want still 2 (no collision)", month, used)
	}
}

func TestConversationFlow(t *testing.T) {
	s := New()
	ctx := context.Background()
	u := "user-1"

	// GetOrCreateStockConversation is idempotent per (user, ticker).
	id1, _ := s.GetOrCreateStockConversation(ctx, u, "AAPL")
	id2, _ := s.GetOrCreateStockConversation(ctx, u, "AAPL")
	if id1 == "" || id1 != id2 {
		t.Fatalf("stock conversation not idempotent: %q vs %q", id1, id2)
	}
	// A different ticker (and a general conversation) are distinct.
	idMsft, _ := s.GetOrCreateStockConversation(ctx, u, "MSFT")
	idGen, _ := s.CreateConversation(ctx, u, "My ideas", "")
	if idMsft == id1 || idGen == id1 || idMsft == idGen {
		t.Fatal("conversations should be distinct")
	}

	// Append messages → they read back chronological + bump ordering.
	_ = s.AppendChatMessage(ctx, store.ChatMessage{ConversationID: id1, UserID: u, Ticker: "AAPL", Role: "user", Content: "hi"})
	_ = s.AppendChatMessage(ctx, store.ChatMessage{ConversationID: id1, UserID: u, Ticker: "AAPL", Role: "assistant", Content: "hello"})
	msgs, _ := s.ListChatMessages(ctx, id1, 10)
	if len(msgs) != 2 || msgs[0].Role != "user" || msgs[1].Content != "hello" {
		t.Fatalf("messages = %+v", msgs)
	}

	// List shows all three; newest-updated (id1, just messaged) is first.
	convs, _ := s.ListConversations(ctx, u)
	if len(convs) != 3 || convs[0].ID != id1 {
		t.Fatalf("list = %+v", convs)
	}
	// Ownership isolation: another user sees none of these.
	if other, _ := s.ListConversations(ctx, "user-2"); len(other) != 0 {
		t.Fatalf("cross-user leak: %+v", other)
	}
	if _, found, _ := s.GetConversation(ctx, "user-2", id1); found {
		t.Fatal("cross-user GetConversation should not find another user's conversation")
	}

	// Rename + delete.
	_ = s.RenameConversation(ctx, u, idGen, "Renamed")
	if c, _, _ := s.GetConversation(ctx, u, idGen); c.Title != "Renamed" {
		t.Fatalf("rename failed: %q", c.Title)
	}
	_ = s.DeleteConversation(ctx, u, id1)
	if _, found, _ := s.GetConversation(ctx, u, id1); found {
		t.Fatal("delete failed")
	}
	if m, _ := s.ListChatMessages(ctx, id1, 10); len(m) != 0 {
		t.Fatal("delete should drop messages")
	}
}

func TestFunnelEvents(t *testing.T) {
	s := New()
	ctx := context.Background()
	for _, ev := range []store.FunnelEvent{
		{Event: "paywall_view", Surface: "deep_research", UserID: "u1"},
		{Event: "paywall_view", Surface: "deep_research"},
		{Event: "paywall_view", Surface: "backtest", UserID: "u2"},
		{Event: "subscription_active", Surface: "webhook", UserID: "u1"},
	} {
		if err := s.SaveFunnelEvent(ctx, ev); err != nil {
			t.Fatalf("SaveFunnelEvent: %v", err)
		}
	}
	stats, err := s.FunnelSummary(ctx, 30)
	if err != nil {
		t.Fatalf("FunnelSummary: %v", err)
	}
	got := map[[2]string]int{}
	for _, st := range stats {
		got[[2]string{st.Event, st.Surface}] = st.Count
	}
	if got[[2]string{"paywall_view", "deep_research"}] != 2 {
		t.Errorf("deep_research paywall_view = %d, want 2", got[[2]string{"paywall_view", "deep_research"}])
	}
	if got[[2]string{"paywall_view", "backtest"}] != 1 || got[[2]string{"subscription_active", "webhook"}] != 1 {
		t.Errorf("unexpected aggregate: %+v", got)
	}
	// A far-past window excludes everything (events are stamped ~now).
	old, _ := s.FunnelSummary(ctx, 0)
	for _, st := range old {
		if st.Count != 0 {
			t.Errorf("days=0 window should be empty, got %+v", st)
		}
	}
}
