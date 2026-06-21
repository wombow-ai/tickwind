package symbols

import "testing"

func sampleIndex() *Index {
	return Build([]Symbol{
		{Ticker: "AAPL", Name: "Apple Inc.", Exchange: "Nasdaq", Country: "US"},
		{Ticker: "AMAT", Name: "Applied Materials Inc", Exchange: "Nasdaq", Country: "US"},
		{Ticker: "APP", Name: "AppLovin Corp", Exchange: "Nasdaq", Country: "US"},
		{Ticker: "MSFT", Name: "Microsoft Corp", Exchange: "Nasdaq", Country: "US"},
		{Ticker: "GE", Name: "General Electric Co", Exchange: "NYSE", Country: "US"},
		{Ticker: "AAPL", Name: "dup should be dropped", Exchange: "OTC", Country: "US"},
	})
}

func tickers(syms []Symbol) []string {
	out := make([]string, len(syms))
	for i, s := range syms {
		out[i] = s.Ticker
	}
	return out
}

func TestSearchChineseAlias(t *testing.T) {
	idx := sampleIndex()
	cases := map[string]string{
		"苹果":   "AAPL",
		"微软":   "MSFT",
		"通用电气": "GE",
	}
	for q, want := range cases {
		got := idx.Search(q, 10)
		if len(got) == 0 || got[0].Ticker != want {
			t.Errorf("Search(%q) = %v, want %s first", q, tickers(got), want)
		}
	}
	// Partial CJK ("苹") surfaces Apple via alias substring.
	if got := idx.Search("苹", 10); len(got) == 0 || got[0].Ticker != "AAPL" {
		t.Errorf("Search(\"苹\") = %v, want AAPL", tickers(got))
	}
	// A CJK term with no alias match returns nothing (no noise).
	if got := idx.Search("螺丝钉", 10); len(got) != 0 {
		t.Errorf("Search(\"螺丝钉\") = %v, want empty", tickers(got))
	}
}

func TestSearchExactTickerWins(t *testing.T) {
	idx := sampleIndex()
	got := idx.Search("app", 10) // matches APP ticker (exact), AAPL/AMAT/APP via name token "app..."
	if len(got) == 0 || got[0].Ticker != "APP" {
		t.Fatalf("exact ticker APP should rank first, got %v", tickers(got))
	}
	// Apple, Applied, AppLovin should all be present (name-token prefix "app").
	want := map[string]bool{"AAPL": false, "AMAT": false, "APP": false}
	for _, s := range got {
		if _, ok := want[s.Ticker]; ok {
			want[s.Ticker] = true
		}
	}
	for tk, found := range want {
		if !found {
			t.Errorf("expected %s in results %v", tk, tickers(got))
		}
	}
}

func TestSearchTickerPrefix(t *testing.T) {
	idx := sampleIndex()
	got := idx.Search("AA", 10)
	if len(got) == 0 || got[0].Ticker != "AAPL" {
		t.Fatalf("AA should prefix-match AAPL first, got %v", tickers(got))
	}
}

func TestSearchNameSubstring(t *testing.T) {
	idx := sampleIndex()
	got := idx.Search("electric", 10)
	if len(got) != 1 || got[0].Ticker != "GE" {
		t.Fatalf("substring 'electric' should find GE, got %v", tickers(got))
	}
}

func TestBuildDedupesAndLimits(t *testing.T) {
	idx := sampleIndex()
	if idx.Len() != 5 { // AAPL duplicate dropped
		t.Fatalf("len=%d want 5", idx.Len())
	}
	if got := idx.Search("a", 2); len(got) != 2 {
		t.Fatalf("limit not applied: %d", len(got))
	}
	if got := (*Index)(nil).Search("a", 5); got != nil {
		t.Errorf("nil index should return nil, got %v", got)
	}
}

func TestCanonical(t *testing.T) {
	tests := []struct{ in, want string }{
		{"AAPL", "AAPL"},       // plain ticker unchanged
		{"BRK-B", "BRK.B"},     // SEC class share -> canonical dot form
		{"BF-A", "BF.A"},       // class A
		{"USB-PA", "USB.PA"},   // preferred series
		{"BAC-PK", "BAC.PK"},   // preferred series
		{"BRK.B", "BRK.B"},     // already dotted -> unchanged
		{"0700.HK", "0700.HK"}, // foreign suffix untouched
		{"-LEAD", "-LEAD"},     // leading hyphen -> not a class suffix
		{"TRAIL-", "TRAIL-"},   // trailing hyphen -> untouched
		{"A-B-C", "A-B.C"},     // only the LAST hyphen becomes a dot
	}
	for _, tc := range tests {
		if got := Canonical(tc.in); got != tc.want {
			t.Errorf("Canonical(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestBuildMergesClassShareKeepingCIK is the FIX-2 dedup regression: after the SEC
// fetch canonicalizes class shares to the dot form, the index gets a SEC entry
// (BRK.B + CIK, no quote) AND a Nasdaq-Trader entry (BRK.B + listing, no CIK) for
// the SAME canonical ticker. The SEC entry is appended first (ingest order), so the
// dedup must collapse them into ONE searchable row that KEEPS the CIK — otherwise
// /stock/BRK.B loses EDGAR resolution and search returns duplicate hits.
func TestBuildMergesClassShareKeepingCIK(t *testing.T) {
	idx := Build([]Symbol{
		// SEC first (carries the CIK, canonicalized to the dot form).
		{Ticker: "BRK.B", Name: "BERKSHIRE HATHAWAY INC", Exchange: "NYSE", Country: "US", CIK: 1067983},
		// Nasdaq-Trader second (carries the listing, no CIK) — must be deduped away.
		{Ticker: "BRK.B", Name: "Berkshire Hathaway Inc Class B", Exchange: "NYSE", Country: "US"},
	})
	if idx.Len() != 1 {
		t.Fatalf("Len() = %d, want 1 (the two BRK.B entries must collapse)", idx.Len())
	}
	// The surviving entry must keep the CIK so EDGAR resolves.
	s, ok := idx.ByCIK(1067983)
	if !ok || s.Ticker != "BRK.B" {
		t.Fatalf("ByCIK(1067983) = %q,%v; want BRK.B,true (CIK must survive the merge)", s.Ticker, ok)
	}
	// ...and it must still be searchable by the canonical dot ticker.
	got := idx.Search("BRK.B", 10)
	if len(got) != 1 || got[0].Ticker != "BRK.B" {
		t.Fatalf("Search(BRK.B) = %v, want exactly one BRK.B (no duplicate hits)", tickers(got))
	}
	if got[0].CIK != 1067983 {
		t.Errorf("searched entry CIK = %d, want 1067983", got[0].CIK)
	}
}

// TestBuildMergesETFFlag: a SEC-listed ETF (SPY) appears first WITHOUT the ETF flag (SEC's
// feed doesn't carry it), then again from Nasdaq-Trader WITH ETF=true. The flag must be OR'd
// onto the kept entry so SEC-listed ETFs are flagged too, not just SEC-absent ones (DRAM).
func TestBuildMergesETFFlag(t *testing.T) {
	idx := Build([]Symbol{
		{Ticker: "SPY", Name: "SPDR S&P 500 ETF Trust", Exchange: "NYSE Arca", Country: "US", CIK: 884394}, // SEC first, ETF unset
		{Ticker: "SPY", Name: "SPDR S&P 500", Exchange: "NYSE Arca", Country: "US", ETF: true},             // Nasdaq-Trader, ETF=Y
		{Ticker: "DRAM", Name: "Roundhill Memory ETF", Exchange: "Cboe BZX", Country: "US", ETF: true},     // SEC-absent ETF
		{Ticker: "AAPL", Name: "Apple Inc.", Exchange: "Nasdaq", Country: "US", CIK: 320193},               // not an ETF
	})
	if s, ok := idx.ByTicker("SPY"); !ok || !s.ETF {
		t.Fatalf("ByTicker(SPY).ETF = %v,%v; want the flag OR'd on from the Nasdaq-Trader entry", s.ETF, ok)
	}
	if s, ok := idx.ByTicker("DRAM"); !ok || !s.ETF {
		t.Fatalf("ByTicker(DRAM).ETF = %v,%v; want true", s.ETF, ok)
	}
	if s, ok := idx.ByTicker("AAPL"); !ok || s.ETF {
		t.Fatalf("ByTicker(AAPL).ETF = %v,%v; want false", s.ETF, ok)
	}
	if _, ok := idx.ByTicker("ZZZZ"); ok {
		t.Error("ByTicker(unknown) should be ok=false")
	}
}

func TestByCIK(t *testing.T) {
	idx := Build([]Symbol{
		{Ticker: "NVDA", Name: "NVIDIA Corp", Exchange: "Nasdaq", Country: "US", CIK: 1045810},
		{Ticker: "AAPL", Name: "Apple Inc.", Exchange: "Nasdaq", Country: "US", CIK: 320193},
		{Ticker: "NOCIK", Name: "No CIK Co", Exchange: "NYSE", Country: "US"}, // CIK 0 → not indexed
	})
	if s, ok := idx.ByCIK(1045810); !ok || s.Ticker != "NVDA" {
		t.Errorf("ByCIK(1045810) = %q,%v; want NVDA,true", s.Ticker, ok)
	}
	if _, ok := idx.ByCIK(999999); ok {
		t.Error("ByCIK(unknown) should be ok=false")
	}
	if _, ok := idx.ByCIK(0); ok { // CIK 0 is the "unknown" sentinel, never indexed
		t.Error("ByCIK(0) should be ok=false")
	}
	if _, ok := (*Index)(nil).ByCIK(1045810); ok {
		t.Error("nil Index ByCIK should be ok=false")
	}
}
