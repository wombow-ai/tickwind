package guru

import (
	"testing"
	"time"
)

func TestRank(t *testing.T) {
	d := func(day int) time.Time { return time.Date(2026, 5, day, 0, 0, 0, 0, time.UTC) }
	items := []Item{
		{URL: "u1", Title: "t1", Published: d(1), Tickers: []string{"AAA"}},
		{URL: "u2", Title: "t2", Published: d(3), Tickers: []string{"BBB"}},
		{URL: "u3", Title: "t3", Published: d(2), Tickers: nil},              // KEPT: tickers optional now
		{URL: "u2", Title: "t2b", Published: d(4), Tickers: []string{"DDD"}}, // dropped: duplicate url
		{URL: "", Title: "t5", Published: d(5), Tickers: []string{"EEE"}},    // dropped: no url
		{URL: "u6", Title: "  ", Published: d(6), Tickers: []string{"FFF"}},  // dropped: blank title
	}
	got := Rank(items, 10)
	// Kept newest-first: u2(d3), u3(d2 — no tickers), u1(d1).
	if len(got) != 3 {
		t.Fatalf("len=%d want 3 (%v)", len(got), got)
	}
	if got[0].URL != "u2" || got[1].URL != "u3" || got[2].URL != "u1" {
		t.Errorf("order=%s,%s,%s want u2,u3,u1", got[0].URL, got[1].URL, got[2].URL)
	}
	if capped := Rank(items, 1); len(capped) != 1 || capped[0].URL != "u2" {
		t.Errorf("cap not applied: %v", capped)
	}
}

func TestRankDiversifyByAuthor(t *testing.T) {
	d := func(day int) time.Time { return time.Date(2026, 5, day, 0, 0, 0, 0, time.UTC) }
	// "Nerd" published a 4-post burst (newest); two other authors once each (older).
	items := []Item{
		{URL: "n1", Title: "n1", Author: "Nerd", Published: d(10)},
		{URL: "n2", Title: "n2", Author: "Nerd", Published: d(9)},
		{URL: "n3", Title: "n3", Author: "Nerd", Published: d(8)},
		{URL: "n4", Title: "n4", Author: "Nerd", Published: d(7)},
		{URL: "a1", Title: "a1", Author: "Road", Published: d(6)},
		{URL: "b1", Title: "b1", Author: "Value", Published: d(5)},
	}
	got := Rank(items, 4)
	if len(got) != 4 {
		t.Fatalf("len=%d want 4", len(got))
	}
	nerd := 0
	for _, it := range got {
		if it.Author == "Nerd" {
			nerd++
		}
	}
	if nerd > 2 {
		t.Fatalf("top-4 has %d Nerd posts, want <=2 (%v)", nerd, got)
	}
	// The two newest Nerd posts still lead; the other authors backfill the top.
	if got[0].Author != "Nerd" || got[1].Author != "Nerd" {
		t.Errorf("two newest Nerd posts should lead: %v", got)
	}
	if got[2].Author != "Road" || got[3].Author != "Value" {
		t.Errorf("other authors should backfill the top: %v", got)
	}
}

func TestCache(t *testing.T) {
	c := NewCache()
	if got := c.Get(); got == nil || len(got) != 0 {
		t.Fatalf("seed = %v, want empty non-nil", got)
	}
	c.Set([]Item{{URL: "u1", Tickers: []string{"AAA"}}})
	if got := c.Get(); len(got) != 1 {
		t.Fatalf("after set len=%d want 1", len(got))
	}
}
