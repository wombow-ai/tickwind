package memory

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func saveNews(t *testing.T, s *Store, ticker, id string, pub time.Time) {
	t.Helper()
	must(t, s.SaveNews(context.Background(), ticker, []store.News{{Ticker: ticker, ID: id, Published: pub}}))
}

func savePost(t *testing.T, s *Store, ticker, id, source string, created time.Time) {
	t.Helper()
	must(t, s.SaveSocial(context.Background(), ticker, []store.Post{{Ticker: ticker, ID: id, Source: source, CreatedAt: created}}))
}

func idSet(items func() []string) map[string]bool {
	ids := items()
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}

func newsIDs(t *testing.T, s *Store, ticker string) map[string]bool {
	t.Helper()
	return idSet(func() []string {
		items, err := s.ListNews(context.Background(), ticker, 100)
		must(t, err)
		out := make([]string, len(items))
		for i, it := range items {
			out[i] = it.ID
		}
		return out
	})
}

func socialIDs(t *testing.T, s *Store, ticker string) map[string]bool {
	t.Helper()
	return idSet(func() []string {
		items, err := s.ListSocial(context.Background(), ticker, 100)
		must(t, err)
		out := make([]string, len(items))
		for i, it := range items {
			out[i] = it.ID
		}
		return out
	})
}

func TestPruneNewsWindowAndHotProtection(t *testing.T) {
	ctx := context.Background()
	s := New()
	now := time.Now().UTC()
	d := func(days int) time.Time { return now.AddDate(0, 0, -days) }

	// HOT sits on a hot board → longer (hotBefore) window; COLD uses the normal one.
	must(t, s.SaveHotList(ctx, "hot", []store.HotStock{{Board: "hot", Ticker: "HOT"}}))
	saveNews(t, s, "COLD", "c-old", d(100)) // > 60d, not hot → prune
	saveNews(t, s, "COLD", "c-new", d(10))  // recent → keep
	saveNews(t, s, "HOT", "h-mid", d(100))  // 60–120d but hot → keep
	saveNews(t, s, "HOT", "h-anc", d(200))  // > 120d → prune even when hot

	n, err := s.PruneNews(ctx, d(60), d(120))
	must(t, err)
	if n != 2 {
		t.Errorf("pruned %d news, want 2 (c-old, h-anc)", n)
	}
	if got := newsIDs(t, s, "COLD"); len(got) != 1 || !got["c-new"] {
		t.Errorf("COLD ids = %v, want {c-new}", got)
	}
	if got := newsIDs(t, s, "HOT"); len(got) != 1 || !got["h-mid"] {
		t.Errorf("HOT ids = %v, want {h-mid}", got)
	}
}

func TestPruneSocialProtectsSourcesAndWindow(t *testing.T) {
	ctx := context.Background()
	s := New()
	now := time.Now().UTC()
	d := func(days int) time.Time { return now.AddDate(0, 0, -days) }

	savePost(t, s, "T", "p-old", "stocktwits", d(100)) // old, unprotected → prune
	savePost(t, s, "T", "p-kol", "substack", d(100))   // old, protected → keep (大V rail)
	savePost(t, s, "T", "p-new", "stocktwits", d(5))   // recent → keep

	n, err := s.PruneSocial(ctx, d(30), d(90), []string{"substack"})
	must(t, err)
	if n != 1 {
		t.Errorf("pruned %d posts, want 1 (p-old)", n)
	}
	got := socialIDs(t, s, "T")
	if len(got) != 2 || !got["p-kol"] || !got["p-new"] {
		t.Errorf("social ids = %v, want {p-kol, p-new}", got)
	}
}

func TestPruneFilingsInsiderSeen(t *testing.T) {
	ctx := context.Background()
	s := New()
	now := time.Now().UTC()
	d := func(days int) time.Time { return now.AddDate(0, 0, -days) }

	must(t, s.SaveFilings(ctx, "T", []store.Filing{
		{Ticker: "T", AccessionNo: "f-old", FiledAt: d(800)},
		{Ticker: "T", AccessionNo: "f-new", FiledAt: d(10)},
	}))
	if n, err := s.PruneFilings(ctx, d(730)); err != nil || n != 1 {
		t.Fatalf("prune filings n=%d err=%v, want 1", n, err)
	}
	if fs, _ := s.ListFilings(ctx, "T", 10); len(fs) != 1 || fs[0].AccessionNo != "f-new" {
		t.Errorf("filings after prune = %+v, want [f-new]", fs)
	}

	must(t, s.SaveInsiderBuys(ctx, []store.InsiderBuy{
		{Accession: "i-old", Ticker: "T", FiledDate: d(200)},
		{Accession: "i-new", Ticker: "T", FiledDate: d(5)},
	}))
	if n, err := s.PruneInsiderBuys(ctx, d(90)); err != nil || n != 1 {
		t.Fatalf("prune insider n=%d err=%v, want 1", n, err)
	}
	if bs, _ := s.RecentInsiderBuys(ctx, d(365)); len(bs) != 1 || bs[0].Accession != "i-new" {
		t.Errorf("insider after prune = %+v, want [i-new]", bs)
	}

	must(t, s.MarkForm4Seen(ctx, []string{"s-old"}, d(100)))
	must(t, s.MarkForm4Seen(ctx, []string{"s-new"}, d(5)))
	if n, err := s.PruneSeenForm4(ctx, d(60)); err != nil || n != 1 {
		t.Fatalf("prune seen n=%d err=%v, want 1", n, err)
	}
	if seen, _ := s.SeenForm4Since(ctx, d(365)); len(seen) != 1 || seen[0] != "s-new" {
		t.Errorf("seen after prune = %v, want [s-new]", seen)
	}
}

func TestCapPerTickerKeepsNewest(t *testing.T) {
	ctx := context.Background()
	s := New()
	now := time.Now().UTC()
	for i := 0; i < 5; i++ { // n0 newest … n4 oldest
		saveNews(t, s, "T", "n"+strconv.Itoa(i), now.Add(-time.Duration(i)*time.Hour))
	}
	n, err := s.CapPerTicker(ctx, "news", 2, nil)
	must(t, err)
	if n != 3 {
		t.Errorf("capped %d, want 3 removed", n)
	}
	if got := newsIDs(t, s, "T"); len(got) != 2 || !got["n0"] || !got["n1"] {
		t.Errorf("kept %v, want {n0, n1} (newest)", got)
	}
	if _, err := s.CapPerTicker(ctx, "bogus", 2, nil); err == nil {
		t.Error("cap on unknown table: want error")
	}
}

func TestCapPerTickerProtectsSource(t *testing.T) {
	ctx := context.Background()
	s := New()
	now := time.Now().UTC()
	// An OLD guru (substack) post + 3 newer stocktwits posts on the same ticker.
	savePost(t, s, "T", "kol", "substack", now.Add(-100*time.Hour))
	for i := 0; i < 3; i++ {
		savePost(t, s, "T", "st"+strconv.Itoa(i), "stocktwits", now.Add(-time.Duration(i)*time.Hour))
	}
	// Cap to 1, protecting substack: the 3 stocktwits → keep 1 (st0), drop 2;
	// the older guru post is never counted nor evicted.
	n, err := s.CapPerTicker(ctx, "social", 1, []string{"substack"})
	must(t, err)
	if n != 2 {
		t.Errorf("capped %d, want 2 (stocktwits only)", n)
	}
	got := socialIDs(t, s, "T")
	if !got["kol"] {
		t.Errorf("guru post evicted by the cap; ids=%v", got)
	}
	if len(got) != 2 || !got["st0"] {
		t.Errorf("social ids=%v, want {kol, st0}", got)
	}
}
