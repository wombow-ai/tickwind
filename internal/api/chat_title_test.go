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
