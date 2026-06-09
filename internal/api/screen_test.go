package api

import (
	"strings"
	"testing"

	"github.com/wombow-ai/tickwind/internal/store"
)

func screenFixture() map[string]store.Quote {
	return map[string]store.Quote{
		"AAA": {Ticker: "AAA", Price: 10, PrevClose: 8, Session: "regular"},    // +25%
		"BBB": {Ticker: "BBB", Price: 100, PrevClose: 110, Session: "regular"}, // -9.09%
		"CCC": {Ticker: "CCC", Price: 50, PrevClose: 50, Session: "post"},      // 0%
		"DDD": {Ticker: "DDD", Price: 5, PrevClose: 0, Session: "regular"},     // change uncomputable
		"ZZZ": {Ticker: "ZZZ", Price: 0, PrevClose: 1, Session: "regular"},     // no price → excluded
	}
}

func tickerList(rows []screenResult) string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Ticker
	}
	return strings.Join(out, ",")
}

func TestScreenQuotes_MinChange(t *testing.T) {
	got := screenQuotes(screenFixture(), screenCriteria{minChange: 5, hasMinChange: true, sort: "change_desc"})
	if tickerList(got) != "AAA" {
		t.Fatalf("min_change=5 → %q, want AAA", tickerList(got))
	}
	if got[0].ChangePct == nil || *got[0].ChangePct < 24.9 || *got[0].ChangePct > 25.1 {
		t.Errorf("AAA change_pct = %v, want ~25", got[0].ChangePct)
	}
}

func TestScreenQuotes_PriceRange(t *testing.T) {
	got := screenQuotes(screenFixture(), screenCriteria{minPrice: 10, maxPrice: 60, sort: "price_asc"})
	if tickerList(got) != "AAA,CCC" {
		t.Fatalf("price[10,60] asc → %q, want AAA,CCC", tickerList(got))
	}
}

func TestScreenQuotes_SortChangeDescNilLast(t *testing.T) {
	// Default sort (change_desc); ZZZ (price 0) excluded, DDD (no change) sorts last.
	got := screenQuotes(screenFixture(), screenCriteria{})
	if tickerList(got) != "AAA,CCC,BBB,DDD" {
		t.Fatalf("change_desc → %q, want AAA,CCC,BBB,DDD", tickerList(got))
	}
}

func TestScreenQuotes_SessionAndLimit(t *testing.T) {
	got := screenQuotes(screenFixture(), screenCriteria{session: "regular", sort: "change_desc"})
	// regular + price>0: AAA, BBB, DDD (CCC is post, ZZZ no price).
	if tickerList(got) != "AAA,BBB,DDD" {
		t.Fatalf("session=regular → %q, want AAA,BBB,DDD", tickerList(got))
	}
	lim := screenQuotes(screenFixture(), screenCriteria{sort: "change_desc", limit: 2})
	if tickerList(lim) != "AAA,CCC" {
		t.Fatalf("limit=2 → %q, want AAA,CCC", tickerList(lim))
	}
}
