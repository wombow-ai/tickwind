package api

import "testing"

// TestDeriveChatTitle guards the auto-naming of a conversation from its first message:
// trim, first line only, cap at 48 runes (rune-safe for CJK).
func TestDeriveChatTitle(t *testing.T) {
	long := ""
	for i := 0; i < 60; i++ {
		long += "x"
	}
	cases := []struct{ in, want string }{
		{"  How is AAPL's valuation?  ", "How is AAPL's valuation?"},
		{"Compare NVDA and AMD\nsecond line ignored", "Compare NVDA and AMD"},
		{"特斯拉的基本面怎么样?", "特斯拉的基本面怎么样?"},
		{long, long[:48] + "…"},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := deriveChatTitle(c.in); got != c.want {
			t.Errorf("deriveChatTitle(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// A 50-rune CJK string must cap at 48 runes + ellipsis (byte-truncation would corrupt).
	cjk := ""
	for i := 0; i < 50; i++ {
		cjk += "中"
	}
	if got := deriveChatTitle(cjk); len([]rune(got)) != 49 { // 48 runes + the … ellipsis
		t.Errorf("CJK truncation = %d runes, want 49", len([]rune(got)))
	}
}

// TestAssistantProseSkipsTrace locks the critical invariant for the persisted execution chain:
// a stored assistant message's display-only "trace" block (the gray ReAct steps) is NEVER fed
// back to the model on a later turn — assistantProse joins ONLY the "text" blocks.
func TestAssistantProseSkipsTrace(t *testing.T) {
	// A stored assistant message with the INTERLEAVED structure: a narration preamble + a trace
	// group + a widget + the final prose answer. Only the final answer ("text") may re-enter the model.
	stored := `[{"kind":"narration","text":"Let me pull the valuation."},{"kind":"trace","steps":[{"kind":"facts","label":"Reading AAPL valuation"},{"kind":"web","label":"Searching the web for NVDA earnings"}]},{"kind":"widget","widget":"valuation_table","params":{"ticker":"AAPL"}},{"kind":"text","text":"P/E (TTM) is 34.2x."}]`
	got := assistantProse(stored)
	if got != "P/E (TTM) is 34.2x." {
		t.Fatalf("assistantProse = %q, want only the text block", got)
	}
	// The narration, step labels, and widget refs must NOT leak into the LLM context.
	for _, bad := range []string{"Let me pull", "Reading AAPL", "Searching the web", "valuation_table"} {
		if contains(got, bad) {
			t.Errorf("narration/trace/widget content leaked into LLM context: %q", got)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
