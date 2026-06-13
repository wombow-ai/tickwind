package thirteenf

import (
	"context"
	"testing"

	"github.com/wombow-ai/tickwind/internal/sec"
)

type fakeFiler struct {
	filings map[int][]sec.Filing13F
	hold    map[string][]sec.Holding // accession → holdings
}

func (f fakeFiler) ThirteenFFilings(_ context.Context, cik, n int) ([]sec.Filing13F, error) {
	fs := f.filings[cik]
	if n < len(fs) {
		fs = fs[:n]
	}
	return fs, nil
}

func (f fakeFiler) Holdings(_ context.Context, _ int, acc string) ([]sec.Holding, error) {
	return f.hold[acc], nil
}

type fakeMapper struct{ m map[string]string }

func (f fakeMapper) Map(_ context.Context, cusips []string) (map[string]string, error) {
	out := map[string]string{}
	for _, c := range cusips {
		if t, ok := f.m[c]; ok {
			out[c] = t
		}
	}
	return out, nil
}

func TestComputeFund(t *testing.T) {
	f := fakeFiler{
		filings: map[int][]sec.Filing13F{
			1: {
				{Accession: "Q2", Filed: "2026-05-15", Period: "2026-03-31"},
				{Accession: "Q1", Filed: "2026-02-15", Period: "2025-12-31"},
			},
		},
		hold: map[string][]sec.Holding{
			"Q2": {
				{Issuer: "APPLE", CUSIP: "AAPL_C", Value: 300, Shares: 30},  // +50% → add
				{Issuer: "NVIDIA", CUSIP: "NVDA_C", Value: 200, Shares: 10}, // not in Q1 → new
				{Issuer: "FOO CORP", CUSIP: "FOO_C", Value: 100, Shares: 5}, // -95% → trim
			},
			"Q1": {
				{Issuer: "APPLE", CUSIP: "AAPL_C", Value: 180, Shares: 20},
				{Issuer: "FOO CORP", CUSIP: "FOO_C", Value: 1000, Shares: 100},
			},
		},
	}
	m := fakeMapper{m: map[string]string{"AAPL_C": "AAPL", "NVDA_C": "NVDA"}} // FOO unmapped

	fh, ok := computeFund(context.Background(), f, m, Fund{CIK: 1, Name: "Test", Manager: "X", Slug: "test"})
	if !ok {
		t.Fatal("computeFund returned ok=false")
	}
	if fh.Period != "2026-03-31" || fh.Count != 3 || fh.Value != 600 {
		t.Errorf("meta: period=%q count=%d value=%d (want 2026-03-31/3/600)", fh.Period, fh.Count, fh.Value)
	}
	if len(fh.Positions) != 3 {
		t.Fatalf("want 3 positions, got %d", len(fh.Positions))
	}
	p := fh.Positions
	// Sorted by value desc: AAPL(300) > NVDA(200) > FOO(100).
	if p[0].Ticker != "AAPL" || p[0].Change != "add" || p[0].Pct != 50 {
		t.Errorf("p0 = %+v (want AAPL/add/50%%)", p[0])
	}
	if p[1].Ticker != "NVDA" || p[1].Change != "new" {
		t.Errorf("p1 = %+v (want NVDA/new)", p[1])
	}
	if p[2].Ticker != "" || p[2].Change != "trim" {
		t.Errorf("p2 = %+v (want unmapped ticker/trim)", p[2])
	}
}

func TestBuildIndexes(t *testing.T) {
	funds := []FundHoldings{
		{
			Slug: "alpha", Name: "Alpha Capital", Manager: "A", Period: "2026-03-31",
			Positions: []Position{
				{Ticker: "AAPL", Value: 300, Pct: 60, Change: "add"},
				{Ticker: "", Value: 100, Change: "trim"}, // unmapped → skipped in reverse index
			},
		},
		{
			Slug: "beta", Name: "Beta Partners", Manager: "B", Period: "2026-03-31",
			Positions: []Position{
				{Ticker: "AAPL", Value: 500, Pct: 25, Change: "new"},
			},
		},
	}
	byTicker, bySlug := buildIndexes(funds)

	// Reverse index: AAPL held by both, largest position first (Beta 500 > Alpha 300).
	aapl := byTicker["AAPL"]
	if len(aapl) != 2 {
		t.Fatalf("AAPL holders = %d, want 2", len(aapl))
	}
	if aapl[0].FundSlug != "beta" || aapl[0].Value != 500 || aapl[0].Change != "new" {
		t.Errorf("aapl[0] = %+v (want beta/500/new)", aapl[0])
	}
	if aapl[1].FundSlug != "alpha" || aapl[1].Weight != 60 {
		t.Errorf("aapl[1] = %+v (want alpha/weight 60)", aapl[1])
	}
	// Unmapped position is not indexed under any ticker key.
	if _, ok := byTicker[""]; ok {
		t.Error("empty ticker should not be indexed")
	}
	// Per-slug index is keyed lower-case.
	if fh, ok := bySlug["alpha"]; !ok || fh.Name != "Alpha Capital" {
		t.Errorf("bySlug[alpha] = %+v ok=%v", fh, ok)
	}
}

func TestCacheLookups(t *testing.T) {
	c := &Cache{
		byTicker: map[string][]Holder{"AAPL": {{FundSlug: "alpha", Value: 1}}},
		bySlug:   map[string]FundHoldings{"alpha": {Slug: "alpha", Name: "Alpha"}},
	}
	if got := c.Holders("aapl"); len(got) != 1 { // case-insensitive
		t.Errorf("Holders(aapl) = %v, want 1 holder", got)
	}
	if got := c.Holders("ZZZZ"); got != nil {
		t.Errorf("Holders(ZZZZ) = %v, want nil", got)
	}
	if fh, ok := c.Fund("ALPHA"); !ok || fh.Name != "Alpha" { // case-insensitive
		t.Errorf("Fund(ALPHA) = %+v ok=%v", fh, ok)
	}
	if _, ok := c.Fund("nope"); ok {
		t.Error("Fund(nope) ok=true, want false")
	}
	// Empty cache (never built) yields no panics, empty results.
	var empty Cache
	if empty.Holders("AAPL") != nil {
		t.Error("empty cache Holders should be nil")
	}
	if _, ok := empty.Fund("alpha"); ok {
		t.Error("empty cache Fund should be ok=false")
	}
}

func TestClassify(t *testing.T) {
	prev := map[string]int64{"A": 100}
	for _, c := range []struct {
		shares      int64
		cusip, want string
	}{
		{150, "A", "add"},  // +50%
		{90, "A", "trim"},  // -10%
		{103, "A", "hold"}, // +3%, within ±5
		{40, "Z", "new"},   // absent in prior
	} {
		if got, _ := classify(c.shares, prev, c.cusip); got != c.want {
			t.Errorf("classify(%d, %q) = %q, want %q", c.shares, c.cusip, got, c.want)
		}
	}
	if got, _ := classify(10, nil, "A"); got != "hold" {
		t.Errorf("classify with no prior filing = %q, want hold", got)
	}
}
