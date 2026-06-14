package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wombow-ai/tickwind/internal/symbols"
)

// fakeSymbols satisfies SymbolSearcher from a canned ticker list. Only
// AllUSTickers is exercised here; Search/ByCIK return empties.
type fakeSymbols struct{ tickers []string }

func (f fakeSymbols) Search(string, int) []symbols.Symbol { return nil }
func (f fakeSymbols) ByCIK(int) (symbols.Symbol, bool)    { return symbols.Symbol{}, false }
func (f fakeSymbols) AllUSTickers() []string              { return f.tickers }

func TestGetSymbols(t *testing.T) {
	// nil source → empty, non-null list, 200.
	srv := httptest.NewServer(newBareServer())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/symbols")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("nil-source /v1/symbols = %d, want 200", resp.StatusCode)
	}
	var empty struct {
		Symbols []string `json:"symbols"`
		Count   int      `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&empty); err != nil {
		t.Fatal(err)
	}
	if empty.Symbols == nil {
		t.Fatal("symbols must marshal as [] not null when the source is unset")
	}
	if empty.Count != 0 || len(empty.Symbols) != 0 {
		t.Fatalf("nil source = %+v, want empty list / count 0", empty)
	}

	// Populated source: full list passes through verbatim (order + dotted tickers).
	all := []string{"AAPL", "MSFT", "BRK.B", "BF.B"}
	s := newBareServer()
	s.symbols = fakeSymbols{tickers: all}
	srv2 := httptest.NewServer(s)
	defer srv2.Close()

	resp2, err := http.Get(srv2.URL + "/v1/symbols")
	if err != nil {
		t.Fatal(err)
	}
	if cc := resp2.Header.Get("Cache-Control"); cc == "" {
		t.Fatal("expected a Cache-Control header (list changes ~daily)")
	}
	var got struct {
		Symbols []string `json:"symbols"`
		Count   int      `json:"count"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Count != 4 || len(got.Symbols) != 4 {
		t.Fatalf("full list = %+v, want all 4 tickers", got)
	}
	for i, want := range all {
		if got.Symbols[i] != want {
			t.Fatalf("symbols[%d] = %q, want %q (order + dotted tickers must pass through)", i, got.Symbols[i], want)
		}
	}

	// ?limit= caps the returned slice.
	resp3, err := http.Get(srv2.URL + "/v1/symbols?limit=2")
	if err != nil {
		t.Fatal(err)
	}
	var capped struct {
		Symbols []string `json:"symbols"`
		Count   int      `json:"count"`
	}
	if err := json.NewDecoder(resp3.Body).Decode(&capped); err != nil {
		t.Fatal(err)
	}
	if capped.Count != 2 || len(capped.Symbols) != 2 || capped.Symbols[0] != "AAPL" || capped.Symbols[1] != "MSFT" {
		t.Fatalf("limit=2 = %+v, want first 2 tickers (AAPL, MSFT)", capped)
	}
}
