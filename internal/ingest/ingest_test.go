package ingest

import (
	"testing"

	"github.com/wombow-ai/tickwind/internal/store"
)

func TestHeatScore(t *testing.T) {
	cases := []struct {
		name           string
		mentions, prev int
		want           float64
	}{
		{"no prior data → raw volume", 100, 0, 100},
		{"flat → raw volume", 100, 100, 100},
		{"doubled (rise 1.0) → 2x", 100, 50, 200},
		{"explosive (rise 5, clamped to 2) → 3x", 120, 20, 360},
		{"cooling (rise<0 → 0) → raw volume", 50, 200, 50},
	}
	for _, tc := range cases {
		if got := heatScore(tc.mentions, tc.prev); got != tc.want {
			t.Errorf("%s: heatScore(%d,%d)=%v want %v", tc.name, tc.mentions, tc.prev, got, tc.want)
		}
	}
}

func TestRankHotList(t *testing.T) {
	stocks := []store.HotStock{
		{Ticker: "AAA", Mentions: 100, MentionsPrev: 100}, // heat 100
		{Ticker: "BBB", Mentions: 100, MentionsPrev: 25},  // rise 3 → clamp 2 → heat 300
		{Ticker: "CCC", Mentions: 200, MentionsPrev: 200}, // heat 200
	}
	rankHotList(stocks)

	// Hottest first: BBB (300) > CCC (200) > AAA (100).
	wantOrder := []string{"BBB", "CCC", "AAA"}
	for i, w := range wantOrder {
		if stocks[i].Ticker != w {
			t.Errorf("position %d: got %s want %s", i, stocks[i].Ticker, w)
		}
		if stocks[i].Rank != i+1 {
			t.Errorf("%s rank=%d want %d", stocks[i].Ticker, stocks[i].Rank, i+1)
		}
		if stocks[i].UpdatedAt.IsZero() {
			t.Errorf("%s UpdatedAt is zero", stocks[i].Ticker)
		}
	}
	for _, s := range stocks {
		switch s.Ticker {
		case "BBB":
			if s.Change != 3.0 { // (100-25)/25
				t.Errorf("BBB change=%v want 3.0", s.Change)
			}
		case "AAA":
			if s.Change != 0.0 {
				t.Errorf("AAA change=%v want 0", s.Change)
			}
		}
	}
}
