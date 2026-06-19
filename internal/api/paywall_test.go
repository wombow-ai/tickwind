package api

import (
	"testing"

	"github.com/wombow-ai/tickwind/internal/research"
)

// TestTruncateDeepForFree verifies the free-tier paywall teaser: overview prose kept
// (bull/bear verdict stripped — Pro-only), the first body section kept in full, the
// rest locked, paywall_locked set, and — critically — the shared cache fact sheet is
// NEVER mutated (truncation works on a serve-time copy).
func TestTruncateDeepForFree(t *testing.T) {
	fs := research.FactSheet{Sections: []research.SectionFacts{
		{Key: "overview", Prose: "exec summary", Bull: []string{"b1", "b2"}, Bear: []string{"r1"}},
		{Key: "valuation", Prose: "valuation prose", Facts: []research.Fact{{Key: "pe"}}},
		{Key: "fundamentals", Prose: "fundamentals prose", Facts: []research.Fact{{Key: "rev"}}},
		{Key: "technical", Prose: "technical prose"},
	}}
	e := researchEntry{fs: fs, llm: true}

	out := truncateDeepForFree(e)

	if !out.paywallLocked {
		t.Fatal("paywallLocked not set on the truncated copy")
	}
	secs := out.fs.Sections
	if secs[0].Key != "overview" || secs[0].Prose != "exec summary" {
		t.Fatalf("overview prose should be kept: %+v", secs[0])
	}
	if len(secs[0].Bull) != 0 || len(secs[0].Bear) != 0 {
		t.Fatal("overview bull/bear must be stripped for the free tier (Pro-only verdict)")
	}
	if secs[1].Key != "valuation" || secs[1].Prose != "valuation prose" || len(secs[1].Facts) != 1 || secs[1].Locked {
		t.Fatalf("first body section should be kept in full + unlocked: %+v", secs[1])
	}
	for _, i := range []int{2, 3} {
		if !secs[i].Locked || secs[i].Prose != "" || len(secs[i].Facts) != 0 {
			t.Fatalf("section %d (%s) should be locked + stripped: %+v", i, secs[i].Key, secs[i])
		}
		if secs[i].TitleEN != fs.Sections[i].TitleEN || secs[i].Key != fs.Sections[i].Key {
			t.Fatalf("locked section should retain its key/title: %+v", secs[i])
		}
	}

	// The original (cache) fact sheet must be untouched.
	if fs.Sections[2].Prose != "fundamentals prose" || len(fs.Sections[2].Facts) != 1 {
		t.Fatal("BUG: truncation mutated the shared cache fact sheet (locked section)")
	}
	if len(fs.Sections[0].Bull) != 2 {
		t.Fatal("BUG: truncation mutated the shared cache overview bull/bear")
	}
}
