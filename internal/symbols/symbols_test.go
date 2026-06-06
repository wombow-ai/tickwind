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
