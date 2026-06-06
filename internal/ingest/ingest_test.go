package ingest

import (
	"math"
	"testing"

	"github.com/wombow-ai/tickwind/internal/store"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestScoreHelpers(t *testing.T) {
	if g := growth(100, 50); g != 1.0 {
		t.Errorf("growth(100,50)=%v want 1", g)
	}
	if g := growth(50, 200); g != 0 { // cooling floored at 0
		t.Errorf("growth cooling=%v want 0", g)
	}
	if g := growth(100, 0); g != 0 { // no prior data
		t.Errorf("growth no-prior=%v want 0", g)
	}
	if s := shrink(50); s != 0.5 { // 50/(50+50)
		t.Errorf("shrink(50)=%v want 0.5", s)
	}
	if c := clamp(5, 3); c != 3 {
		t.Errorf("clamp(5,3)=%v want 3", c)
	}
	if c := clamp(1, 3); c != 1 {
		t.Errorf("clamp(1,3)=%v want 1", c)
	}
}

func TestHeatAndSurge(t *testing.T) {
	// Flat / no-prior → heat == raw volume, surge == 0.
	if h := heatScore(100, 100); !approx(h, 100) {
		t.Errorf("flat heat=%v want 100", h)
	}
	if h := heatScore(100, 0); !approx(h, 100) {
		t.Errorf("no-prior heat=%v want 100", h)
	}
	if s := surgeScore(100, 100); !approx(s, 0) {
		t.Errorf("flat surge=%v want 0", s)
	}
	// Tripled mentions (growth 3, clamped to 3): surge = shrink(100)*3 = 2.0.
	if s := surgeScore(100, 25); !approx(s, 2.0) {
		t.Errorf("surge(100,25)=%v want 2.0", s)
	}
	// Surge is volume-independent momentum: a riser out-surges a louder flat name.
	if surgeScore(100, 25) <= surgeScore(400, 400) {
		t.Error("a riser should out-surge a louder flat name")
	}
	// Heat is volume-led: a louder flat name is hotter than a small riser.
	if heatScore(400, 400) <= heatScore(60, 20) {
		t.Error("a louder name should be hotter than a small riser")
	}
}

func TestRankBoardFloorAndOrder(t *testing.T) {
	raw := []store.HotStock{
		{Ticker: "AAA", Mentions: 100, MentionsPrev: 100}, // surge 0
		{Ticker: "BBB", Mentions: 100, MentionsPrev: 25},  // surge 2.0
		{Ticker: "CCC", Mentions: 10, MentionsPrev: 2},    // below surging floor
	}
	surging := rankBoard(raw, "surging", surgingMinMentions, func(h store.HotStock) float64 {
		return surgeScore(h.Mentions, h.MentionsPrev)
	})
	if len(surging) != 2 {
		t.Fatalf("surging len=%d want 2 (CCC floored out)", len(surging))
	}
	if surging[0].Ticker != "BBB" || surging[0].Rank != 1 || surging[0].Board != "surging" {
		t.Errorf("surging[0]=%+v want BBB rank1 board=surging", surging[0])
	}
	if surging[1].Ticker != "AAA" || surging[1].Rank != 2 {
		t.Errorf("surging[1]=%+v want AAA rank2", surging[1])
	}
}

func TestBuildBoards(t *testing.T) {
	raw := []store.HotStock{
		{Ticker: "AAA", Mentions: 100, MentionsPrev: 100},
		{Ticker: "BBB", Mentions: 100, MentionsPrev: 25},
		{Ticker: "CCC", Mentions: 10, MentionsPrev: 2},
	}
	boards := buildBoards(raw)

	hot := boards["hot"]
	if len(hot) != 3 {
		t.Fatalf("hot len=%d want 3", len(hot))
	}
	if hot[0].Ticker != "BBB" { // highest heat (volume × momentum)
		t.Errorf("hot[0]=%s want BBB", hot[0].Ticker)
	}
	if len(boards["surging"]) != 2 { // CCC floored out
		t.Fatalf("surging len=%d want 2", len(boards["surging"]))
	}
	for _, h := range hot {
		if h.Ticker == "BBB" && !approx(h.Change, 3.0) { // (100-25)/25
			t.Errorf("BBB change=%v want 3.0", h.Change)
		}
	}
	if hot[0].UpdatedAt.IsZero() {
		t.Error("UpdatedAt not set")
	}
}
