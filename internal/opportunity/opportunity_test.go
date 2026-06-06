package opportunity

import (
	"strings"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

func TestRecompute(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	day := func(d int) time.Time { return now.AddDate(0, 0, -d) }

	buys := []store.InsiderBuy{
		// SMALL: 2 distinct buyers, $1.2M; market cap $1B (in band) → included.
		{Accession: "a1", Ticker: "SMALL", CIK: 100, Company: "Small Co", OwnerName: "Jane Doe", Title: "CFO", IsOfficer: true, FiledDate: day(2), Value: 800000, FilingURL: "u1"},
		{Accession: "a2", Ticker: "SMALL", CIK: 100, OwnerName: "John Roe", IsDirector: true, FiledDate: day(5), Value: 400000, FilingURL: "u2"},
		{Accession: "a5", Ticker: "SMALL", CIK: 100, OwnerName: "Tiny Tim", FiledDate: day(1), Value: 1000, FilingURL: "u5"}, // below MinBuyValue → dropped
		// BIG: market cap $50B → too large, excluded.
		{Accession: "a3", Ticker: "BIG", CIK: 200, OwnerName: "X", FiledDate: day(1), Value: 500000},
		// NOPRICE: no price → excluded.
		{Accession: "a4", Ticker: "NOPRICE", CIK: 300, OwnerName: "Y", FiledDate: day(1), Value: 500000},
	}
	shares := map[int]int64{100: 50_000_000, 200: 1_000_000_000, 300: 50_000_000}
	prices := map[string]float64{"SMALL": 20, "BIG": 50, "NOPRICE": 0}

	out := Recompute(now, buys, shares, prices)
	if len(out) != 1 {
		t.Fatalf("got %d rows, want 1 (only SMALL passes the small-cap + price gates)", len(out))
	}
	s := out[0]
	if s.Ticker != "SMALL" || s.Rank != 1 {
		t.Errorf("row = %+v", s)
	}
	if s.Buyers != 2 { // Jane + John; Tiny Tim's $1k buy is below the floor
		t.Errorf("buyers = %d, want 2", s.Buyers)
	}
	if s.BuyValue != 1200000 {
		t.Errorf("buy value = %v, want 1200000", s.BuyValue)
	}
	if s.MarketCap != 1_000_000_000 {
		t.Errorf("market cap = %v, want 1e9", s.MarketCap)
	}
	if s.FilingURL != "u1" { // most recent filing is day(2) = a1
		t.Errorf("filing url = %q, want u1 (most recent)", s.FilingURL)
	}
	if !strings.Contains(s.Explainer, "2 insiders") || !strings.Contains(s.Explainer, "$1.2M") {
		t.Errorf("explainer = %q", s.Explainer)
	}
	if len(s.TopBuyers) != 2 || s.TopBuyers[0].Value != 800000 { // sorted by value desc
		t.Errorf("top buyers = %+v", s.TopBuyers)
	}
	if s.TopBuyers[1].Title != "Director" { // John has no officer title → "Director"
		t.Errorf("director title = %q", s.TopBuyers[1].Title)
	}
}

func TestCache(t *testing.T) {
	c := NewCache()
	if len(c.Get()) != 0 {
		t.Error("new cache should be empty")
	}
	c.Set([]Stock{{Ticker: "ABC"}})
	if got := c.Get(); len(got) != 1 || got[0].Ticker != "ABC" {
		t.Errorf("get = %+v", got)
	}
}
