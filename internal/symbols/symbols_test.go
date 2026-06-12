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
