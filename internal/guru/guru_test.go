package guru

import (
	"testing"
	"time"
)

func TestRank(t *testing.T) {
	d := func(day int) time.Time { return time.Date(2026, 5, day, 0, 0, 0, 0, time.UTC) }
	items := []Item{
		{URL: "u1", Published: d(1), Tickers: []string{"AAA"}},
		{URL: "u2", Published: d(3), Tickers: []string{"BBB"}},
		{URL: "u3", Published: d(2), Tickers: nil},             // dropped: no tickers
		{URL: "u2", Published: d(4), Tickers: []string{"DDD"}}, // dropped: duplicate url
		{URL: "", Published: d(5), Tickers: []string{"EEE"}},   // dropped: no url
	}
	got := Rank(items, 10)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2 (%v)", len(got), got)
	}
	if got[0].URL != "u2" || got[1].URL != "u1" { // newest-first
		t.Errorf("order=%s,%s want u2,u1", got[0].URL, got[1].URL)
	}
	if capped := Rank(items, 1); len(capped) != 1 || capped[0].URL != "u2" {
		t.Errorf("cap not applied: %v", capped)
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
