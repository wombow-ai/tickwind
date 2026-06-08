package symbols

import "testing"

func TestParseNasdaqListed(t *testing.T) {
	data := []byte(`Symbol|Security Name|Market Category|Test Issue|Financial Status|Round Lot Size|ETF|NextShares
AAPL|Apple Inc. - Common Stock|Q|N|N|100|N|N
ZTEST|Nasdaq Test Stock|Q|Y|N|100|N|N
TQQQ|ProShares UltraPro QQQ|G|N|N|100|Y|N
File Creation Time: 0608202610:01|||||||`)
	got := parseNasdaqListed(data)
	if len(got) != 2 { // ZTEST (test issue) + trailer dropped
		t.Fatalf("got %d symbols, want 2: %+v", len(got), got)
	}
	if got[0].Ticker != "AAPL" || got[0].Exchange != "Nasdaq" || got[0].Country != "US" {
		t.Errorf("AAPL row wrong: %+v", got[0])
	}
	if got[1].Ticker != "TQQQ" {
		t.Errorf("want TQQQ second, got %+v", got[1])
	}
	for _, s := range got {
		if s.Ticker == "ZTEST" {
			t.Error("test issue ZTEST should be dropped")
		}
	}
}

func TestParseOtherListed(t *testing.T) {
	// DRAM (a Cboe-listed ETF) is the canonical "missing from SEC" case.
	data := []byte(`ACT Symbol|Security Name|Exchange|CQS Symbol|ETF|Round Lot Size|Test Issue|NASDAQ Symbol
DRAM|Roundhill Memory ETF|Z|DRAM|Y|100|N|DRAM
BRK.A|Berkshire Hathaway Inc., Class A|N|BRK A|N|1|N|
ZXYZ.A|Nasdaq Symbology Test Common|N|ZXYZ.A|N|100|Y|ZXYZ.A
File Creation Time: 0608202610:01|||||||`)
	got := parseOtherListed(data)
	if len(got) != 2 { // ZXYZ.A (test issue) + trailer dropped
		t.Fatalf("got %d symbols, want 2: %+v", len(got), got)
	}
	byT := map[string]Symbol{}
	for _, s := range got {
		byT[s.Ticker] = s
	}
	dram, ok := byT["DRAM"]
	if !ok {
		t.Fatal("DRAM (ETF) should be parsed")
	}
	if dram.Name != "Roundhill Memory ETF" || dram.Exchange != "Cboe BZX" || dram.Country != "US" {
		t.Errorf("DRAM mapped wrong: %+v", dram)
	}
	if byT["BRK.A"].Exchange != "NYSE" { // N -> NYSE; comma in name preserved
		t.Errorf("BRK.A exchange want NYSE, got %q (name %q)", byT["BRK.A"].Exchange, byT["BRK.A"].Name)
	}
	if _, bad := byT["ZXYZ.A"]; bad {
		t.Error("test issue ZXYZ.A should be dropped")
	}
}

func TestForEachDataRowSkipsHeaderAndTrailer(t *testing.T) {
	// Header line + trailer only → zero data rows.
	data := []byte("ACT Symbol|Security Name|Exchange|CQS Symbol|ETF|Round Lot Size|Test Issue|NASDAQ Symbol\nFile Creation Time: 0608202610:01|||||||")
	if got := parseOtherListed(data); len(got) != 0 {
		t.Errorf("want 0 symbols, got %d: %+v", len(got), got)
	}
}

func TestOtherExch(t *testing.T) {
	cases := map[string]string{
		"N": "NYSE", "P": "NYSE Arca", "A": "NYSE American",
		"Z": "Cboe BZX", "V": "IEX", "M": "NYSE Chicago", "?": "?",
	}
	for code, want := range cases {
		if got := otherExch(code); got != want {
			t.Errorf("otherExch(%q) = %q, want %q", code, got, want)
		}
	}
}
