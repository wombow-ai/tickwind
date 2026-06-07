package symbols

import "testing"

func TestForeignSeedsSearchable(t *testing.T) {
	idx := Build(ForeignSeeds())
	cases := map[string]string{
		"tencent": "0700.HK", // name-token
		"tsmc":    "2330.TW", // name-token (parenthetical alias)
		"minimax": "0100.HK",
		"zhipu":   "2513.HK",
		"2330.TW": "2330.TW", // exact ticker
		"media":   "2454.TW", // token prefix → MediaTek
	}
	for q, want := range cases {
		got := idx.Search(q, 5)
		first := "(none)"
		if len(got) > 0 {
			first = got[0].Ticker
		}
		if first != want {
			t.Errorf("Search(%q)[0]=%s, want %s", q, first, want)
		}
	}
}
