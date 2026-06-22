package api

import (
	"testing"

	"github.com/wombow-ai/tickwind/internal/indicators"
	"github.com/wombow-ai/tickwind/internal/store"
)

type fakeReactionSrc struct {
	m map[string]indicators.ReactionSummary
}

func (f fakeReactionSrc) Reaction(t string) (indicators.ReactionSummary, bool) {
	r, ok := f.m[t]
	return r, ok
}

func TestWithReactions(t *testing.T) {
	s := &Server{earningsReactions: fakeReactionSrc{m: map[string]indicators.ReactionSummary{
		"AAPL": {AvgAbsMove: 3.2, UpRate: 0.6, Samples: 10},
		"ZZZ":  {AvgAbsMove: 1.0, Samples: 0}, // present but no samples → omitted (insufficient)
	}}}
	rows := s.withReactions([]store.Earning{
		{Ticker: "AAPL"}, {Ticker: "MSFT"}, {Ticker: "ZZZ"}, {Ticker: "aapl"},
	})
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4", len(rows))
	}
	if rows[0].Reaction == nil || rows[0].Reaction.AvgAbsMove != 3.2 || rows[0].Reaction.Samples != 10 {
		t.Fatalf("AAPL reaction = %+v, want {3.2,_,10}", rows[0].Reaction)
	}
	if rows[1].Reaction != nil {
		t.Fatalf("MSFT is untracked → reaction must be nil, got %+v", rows[1].Reaction)
	}
	if rows[2].Reaction != nil {
		t.Fatalf("ZZZ has 0 samples → reaction must be omitted, got %+v", rows[2].Reaction)
	}
	if rows[3].Reaction == nil {
		t.Fatal("lowercase 'aapl' should match (ticker upper-cased before lookup)")
	}
	// The embedded ticker is preserved verbatim.
	if rows[3].Ticker != "aapl" {
		t.Fatalf("row ticker mutated: %q", rows[3].Ticker)
	}
}

func TestWithReactions_NilSource(t *testing.T) {
	s := &Server{} // no reaction source
	rows := s.withReactions([]store.Earning{{Ticker: "AAPL"}})
	if len(rows) != 1 || rows[0].Reaction != nil {
		t.Fatalf("nil source → no reactions, got %+v", rows)
	}
}
