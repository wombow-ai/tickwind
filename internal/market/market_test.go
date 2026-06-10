package market

import "testing"

func TestOf(t *testing.T) {
	cases := map[string]Market{
		"AAPL": US, "brk.b": US, // bare / US dotted class-share stays US
		"005930.KS": KR, "247540.KQ": KR,
		"2330.TW": TW, "006201.TWO": TW,
		"00700.HK": HK,
		"PETR4.SA": BR, "vale3.sa": BR, // Brazil B3 (São Paulo)
		"  2330.tw  ": TW, // trimmed + case-insensitive
	}
	for in, want := range cases {
		if got := Of(in); got != want {
			t.Errorf("Of(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBase(t *testing.T) {
	cases := map[string]string{
		"005930.KS": "005930", "2330.TW": "2330", "006201.TWO": "006201",
		"AAPL": "AAPL", "00700.HK": "00700", "PETR4.SA": "PETR4",
	}
	for in, want := range cases {
		if got := Base(in); got != want {
			t.Errorf("Base(%q) = %q, want %q", in, got, want)
		}
	}
}
