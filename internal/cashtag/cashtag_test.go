package cashtag

import (
	"reflect"
	"strings"
	"testing"
)

func TestExtract(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"看好 $RKLB 下半年", []string{"RKLB"}},
		{"$aapl vs $MSFT, 还是 $aapl 稳", []string{"AAPL", "MSFT"}}, // case-fold + dedupe, first-mention order
		{"巴西的 $PETR4.SA 和港股 $0700.HK", []string{"PETR4.SA", "0700.HK"}},
		{"类别股 $BRK.B 也行", []string{"BRK.B"}},
		{"成本 $100, 目标 $1.5B — 都不是票", nil},
		{"x$AAPL 粘住不算, $$AAPL 也不算", nil},
		{"$AAPLextra 超长不算", nil},
		{"[$NVDA](https://x.com) markdown 里也认", []string{"NVDA"}},
		{"行首 $TSLA", []string{"TSLA"}},
		{"", nil},
	}
	for _, c := range cases {
		if got := Extract(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Extract(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestExtractCap(t *testing.T) {
	var b strings.Builder
	for _, s := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"} {
		b.WriteString("$" + s + s + " ") // $AA $BB ... 10 distinct tags
	}
	got := Extract(b.String())
	if len(got) != MaxTags {
		t.Fatalf("got %d tags, want capped at %d: %v", len(got), MaxTags, got)
	}
}
