package ingest

import (
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/indicators"
)

// TestReactionPercentile checks the earnings-reaction percentile feeding the report's relative
// section: the population floor (insufficient-not-wrong), the strictly-below percentile math,
// case-normalization, and the omit-when-absent contract.
func TestReactionPercentile(t *testing.T) {
	c := &EarningsReactionCache{m: map[string]indicators.ReactionSummary{}}

	// Below the floor (empty) → withheld.
	if _, _, _, ok := c.ReactionPercentile("AAA"); ok {
		t.Fatal("empty population: want withheld")
	}

	// A population of 10 with strictly increasing typical move magnitude (AvgAbsMove 1..10).
	for i, tk := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"} {
		c.m[tk] = indicators.ReactionSummary{AvgAbsMove: float64(i + 1), UpRate: 0.5, Samples: 8}
	}
	c.at = time.Unix(1_700_000_000, 0).UTC()

	// J has the largest AvgAbsMove (10) → 9 of 10 strictly below → 90th percentile.
	if pct, n, asOf, ok := c.ReactionPercentile("J"); !ok || n != 10 || pct != 90 || asOf.IsZero() {
		t.Fatalf("J: pct=%v n=%v ok=%v asOf-zero=%v; want 90,10,true,false", pct, n, ok, asOf.IsZero())
	}
	// A is the smallest → 0 below → 0th percentile; lowercase input must normalize to the key.
	if pct, _, _, ok := c.ReactionPercentile("a"); !ok || pct != 0 {
		t.Fatalf("a: pct=%v ok=%v; want 0,true (case-normalized)", pct, ok)
	}
	// A ticker not in the tracked set → omitted, never fabricated.
	if _, _, _, ok := c.ReactionPercentile("ZZZ"); ok {
		t.Fatal("absent ticker: want withheld")
	}

	// Exactly at the floor (8) still ranks; just below (7) is withheld.
	c.m = map[string]indicators.ReactionSummary{}
	for i := 0; i < minReactionPopulation; i++ {
		c.m[string(rune('A'+i))] = indicators.ReactionSummary{AvgAbsMove: float64(i + 1), Samples: 8}
	}
	if _, n, _, ok := c.ReactionPercentile("A"); !ok || n != minReactionPopulation {
		t.Fatalf("at floor: n=%v ok=%v; want %d,true", n, ok, minReactionPopulation)
	}
	delete(c.m, "H") // drop to 7 (below the floor)
	if _, _, _, ok := c.ReactionPercentile("A"); ok {
		t.Fatal("below floor (7): want withheld")
	}
}
