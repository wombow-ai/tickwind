package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

// fakeUniverse satisfies UniverseSource from a canned ticker list. Tickers
// returns the list verbatim (the handler does not re-sort); the other methods
// return inert values — only Tickers is exercised by /v1/universe/symbols.
type fakeUniverse struct{ tickers []string }

func (f fakeUniverse) Get(string) (store.Quote, bool)   { return store.Quote{}, false }
func (f fakeUniverse) Snapshot() map[string]store.Quote { return map[string]store.Quote{} }
func (f fakeUniverse) Tickers() []string                { return f.tickers }
func (f fakeUniverse) Len() int                         { return len(f.tickers) }
func (f fakeUniverse) UpdatedAt() time.Time             { return time.Time{} }

func TestGetUniverseSymbols(t *testing.T) {
	// nil source → empty, non-null list, 200 (graceful before the first sweep).
	srv := httptest.NewServer(newBareServer())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/universe/symbols")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("nil-source /v1/universe/symbols = %d, want 200", resp.StatusCode)
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

	// Populated source: full quote-bearing list passes through (incl. dotted).
	all := []string{"AAPL", "BRK.B", "MSFT", "NVDA"}
	s := newBareServer()
	s.universe = fakeUniverse{tickers: all}
	srv2 := httptest.NewServer(s)
	defer srv2.Close()

	resp2, err := http.Get(srv2.URL + "/v1/universe/symbols")
	if err != nil {
		t.Fatal(err)
	}
	if cc := resp2.Header.Get("Cache-Control"); cc != "public, max-age=3600" {
		t.Fatalf("Cache-Control = %q, want public, max-age=3600", cc)
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
			t.Fatalf("symbols[%d] = %q, want %q (dotted tickers must pass through verbatim)", i, got.Symbols[i], want)
		}
	}

	// ?limit= caps the returned slice (mirrors getSymbols).
	resp3, err := http.Get(srv2.URL + "/v1/universe/symbols?limit=2")
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
	if capped.Count != 2 || len(capped.Symbols) != 2 || capped.Symbols[0] != "AAPL" || capped.Symbols[1] != "BRK.B" {
		t.Fatalf("limit=2 = %+v, want first 2 tickers (AAPL, BRK.B)", capped)
	}
}
