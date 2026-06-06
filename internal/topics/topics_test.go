package topics

import (
	"testing"
	"time"
)

func TestRecomputeRanksAndGates(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	mk := func(h string, agoH float64, tickers ...string) Article {
		return Article{
			Headline:    h,
			PublishedAt: now.Add(-time.Duration(agoH * float64(time.Hour))),
			Tickers:     tickers,
		}
	}
	arts := []Article{
		// AI capex — 4 current (above floor), accelerating.
		mk("Nvidia data center GPU demand surges", 1, "NVDA"),
		mk("Hyperscaler AI capex hits record", 2, "MSFT"),
		mk("New GPU accelerator unveiled", 3, "AMD"),
		mk("Datacenter buildout accelerates", 5, "NVDA"),
		mk("Old GPU news from yesterday", 30, "NVDA"), // prior window (momentum baseline)
		// Fed — 3 current.
		mk("Fed signals a rate cut", 1),
		mk("Powell on interest rates", 2),
		mk("FOMC minutes released", 4),
		// Earnings — 3 current, but generic (demoted).
		mk("Apple earnings beat estimates", 1, "AAPL"),
		mk("Tesla quarterly results out", 2, "TSLA"),
		mk("Guidance raised on strong earnings", 3, "MSFT"),
		// Crypto — only 2, below the floor.
		mk("Bitcoin rallies hard", 1),
		mk("Ethereum network upgrade", 2),
	}

	snap := Recompute(now, arts)
	by := map[string]HotTopic{}
	for _, tp := range snap.Topics {
		by[tp.Key] = tp
	}

	if _, ok := by["crypto"]; ok {
		t.Error("crypto (2 articles) is below the min-count floor and should be dropped")
	}
	ai, ok := by["ai_capex"]
	if !ok {
		t.Fatal("ai_capex should be present")
	}
	if ai.Count != 4 {
		t.Errorf("ai_capex count=%d want 4", ai.Count)
	}
	if len(ai.RelatedTickers) == 0 {
		t.Error("ai_capex should carry related tickers")
	}
	if ai.Momentum <= 1 {
		t.Errorf("ai_capex momentum=%v should be >1 (accelerating)", ai.Momentum)
	}
	// AI capex (specific) should out-rank Earnings (generic, demoted) despite both
	// having healthy counts.
	if earn, ok := by["earnings"]; ok && ai.hotness <= earn.hotness {
		t.Errorf("specific ai_capex (%.2f) should beat demoted earnings (%.2f)", ai.hotness, earn.hotness)
	}
}

func TestMatch(t *testing.T) {
	if !Match("fed", "The Fed cut rates today") {
		t.Error("should match fed")
	}
	if !Match("semis", "TSMC ramps chip production") {
		t.Error("should match semis")
	}
	if Match("semis", "Chipotle opens a new store") {
		t.Error("word-boundary must stop 'chip' matching 'Chipotle'")
	}
	if Match("does_not_exist", "anything") {
		t.Error("unknown key should never match")
	}
}
