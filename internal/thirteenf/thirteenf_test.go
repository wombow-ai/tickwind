package thirteenf

import (
	"context"
	"fmt"
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

	fd, ok := computeFund(context.Background(), f, m, Fund{CIK: 1, Name: "Test", Manager: "X", Slug: "test"})
	if !ok {
		t.Fatal("computeFund returned ok=false")
	}
	fh := fd.holdings
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

// TestComputeFundPRNHeldBothQuarters is the end-to-end BUG 3 check: a PRN
// (fixed-income) holding carries Shares=0 in both the current and prior filing
// (sec's parseInfoTable counts only SH amounts), so the old share-keyed classify
// tagged it "new" every quarter. It must now read "hold" (or value-based add/trim)
// when the CUSIP existed last quarter, while equities are untouched.
func TestComputeFundPRNHeldBothQuarters(t *testing.T) {
	f := fakeFiler{
		filings: map[int][]sec.Filing13F{
			1: {
				{Accession: "Q2", Filed: "2026-05-15", Period: "2026-03-31"},
				{Accession: "Q1", Filed: "2026-02-15", Period: "2025-12-31"},
			},
		},
		hold: map[string][]sec.Holding{
			// A bond (PRN → Shares=0) held flat, plus an equity (SH) that grew.
			"Q2": {
				{Issuer: "ACME 5% NOTES", CUSIP: "BOND_C", Value: 1000, Shares: 0},
				{Issuer: "APPLE", CUSIP: "AAPL_C", Value: 300, Shares: 30},
			},
			"Q1": {
				{Issuer: "ACME 5% NOTES", CUSIP: "BOND_C", Value: 1000, Shares: 0},
				{Issuer: "APPLE", CUSIP: "AAPL_C", Value: 180, Shares: 20},
			},
		},
	}
	m := fakeMapper{m: map[string]string{"BOND_C": "ACME", "AAPL_C": "AAPL"}}

	fd, ok := computeFund(context.Background(), f, m, Fund{CIK: 1, Slug: "test"})
	if !ok {
		t.Fatal("computeFund ok=false")
	}
	byCusip := map[string]Position{}
	for _, p := range fd.allPos {
		byCusip[p.Issuer] = p
	}
	bond := byCusip["ACME 5% NOTES"]
	if bond.Change != "hold" {
		t.Errorf("PRN bond held flat across quarters: change = %q, want hold (NOT new)", bond.Change)
	}
	// The equity is unaffected: +50% shares → add.
	if apple := byCusip["APPLE"]; apple.Change != "add" {
		t.Errorf("SH equity +50%%: change = %q, want add (no regression)", apple.Change)
	}
}

func TestBuildIndexes(t *testing.T) {
	funds := []fundData{
		{
			holdings: FundHoldings{
				Slug: "alpha", Name: "Alpha Capital", Manager: "A", Period: "2026-03-31",
				Positions: []Position{
					{Ticker: "AAPL", Value: 300, Pct: 60, Change: "add"},
					{Ticker: "", Value: 100, Change: "trim"}, // unmapped → skipped in reverse index
				},
			},
			allPos: []Position{
				{Ticker: "AAPL", Value: 300, Pct: 60, Change: "add"},
				{Ticker: "", Value: 100, Change: "trim"},
			},
		},
		{
			holdings: FundHoldings{
				Slug: "beta", Name: "Beta Partners", Manager: "B", Period: "2026-03-31",
				Positions: []Position{
					{Ticker: "AAPL", Value: 500, Pct: 25, Change: "new"},
				},
			},
			allPos: []Position{
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
	// Per-slug index is keyed lower-case and carries the rendered holdings.
	if fh, ok := bySlug["alpha"]; !ok || fh.Name != "Alpha Capital" {
		t.Errorf("bySlug[alpha] = %+v ok=%v", fh, ok)
	}
}

// TestBuildIndexesCountsBeyondTopN asserts a fund that holds the ticker as a #16+
// position (so it is NOT in the topN-capped rendered Positions list) is still
// counted in the reverse holder index — the BUG 4 undercount fix. buildIndexes
// walks allPos, so the holder count is complete regardless of the display cap.
func TestBuildIndexesCountsBeyondTopN(t *testing.T) {
	// Rendered Positions holds only the topN largest (here a single placeholder that
	// is NOT the target ticker), while allPos contains the #16+ target position.
	funds := []fundData{
		{
			holdings: FundHoldings{
				Slug: "gamma", Name: "Gamma Fund", Manager: "G", Period: "2026-03-31",
				Positions: []Position{
					{Ticker: "BIG", Value: 9999, Pct: 90, Change: "hold"}, // a top position; TGT is not rendered
				},
			},
			allPos: []Position{
				{Ticker: "BIG", Value: 9999, Pct: 90, Change: "hold"},
				{Ticker: "TGT", Value: 12, Pct: 0.1, Change: "add"}, // the #16+ small position
			},
		},
	}
	byTicker, _ := buildIndexes(funds)

	tgt := byTicker["TGT"]
	if len(tgt) != 1 {
		t.Fatalf("TGT holders = %d, want 1 (a #16+ position must still be counted)", len(tgt))
	}
	if tgt[0].FundSlug != "gamma" || tgt[0].Value != 12 || tgt[0].Change != "add" {
		t.Errorf("tgt[0] = %+v (want gamma/12/add)", tgt[0])
	}
	// The rendered FundHoldings keeps its display cap (TGT absent from Positions).
	for _, p := range funds[0].holdings.Positions {
		if p.Ticker == "TGT" {
			t.Error("TGT leaked into the rendered top-N Positions; the cap must be display-only")
		}
	}
}

// TestComputeFundCountsBeyondTopN is the end-to-end BUG 4 check: a fund with more
// than topN holdings still indexes every ticker (the #(topN+1) holding is counted),
// while the rendered Positions stays capped at topN.
func TestComputeFundCountsBeyondTopN(t *testing.T) {
	// Build topN+1 distinct holdings, descending in value so the smallest is #16.
	const extra = topN + 1
	q2 := make([]sec.Holding, 0, extra)
	tickerMap := map[string]string{}
	for i := 0; i < extra; i++ {
		cusip := fmt.Sprintf("C%02d", i)
		tkr := fmt.Sprintf("T%02d", i)
		q2 = append(q2, sec.Holding{Issuer: tkr + " CORP", CUSIP: cusip, Value: int64(extra - i), Shares: int64(extra - i)})
		tickerMap[cusip] = tkr
	}
	smallest := fmt.Sprintf("T%02d", extra-1) // the #(topN+1) position by value

	f := fakeFiler{
		filings: map[int][]sec.Filing13F{1: {{Accession: "Q2", Period: "2026-03-31", Filed: "2026-05-15"}}},
		hold:    map[string][]sec.Holding{"Q2": q2},
	}
	fd, ok := computeFund(context.Background(), f, fakeMapper{m: tickerMap}, Fund{CIK: 1, Slug: "test"})
	if !ok {
		t.Fatal("computeFund ok=false")
	}
	if len(fd.holdings.Positions) != topN {
		t.Errorf("rendered Positions = %d, want capped at topN=%d", len(fd.holdings.Positions), topN)
	}
	if len(fd.allPos) != extra {
		t.Errorf("allPos = %d, want all %d holdings for indexing", len(fd.allPos), extra)
	}
	byTicker, _ := buildIndexes([]fundData{fd})
	if len(byTicker[smallest]) != 1 {
		t.Errorf("#%d position %q holder count = %d, want 1 (must be indexed despite the display cap)", extra, smallest, len(byTicker[smallest]))
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
	// SH-type (equity) path: shares drive the tag, prior values are irrelevant here.
	prevShares := map[string]int64{"A": 100}
	prevValues := map[string]int64{"A": 1000}
	for _, c := range []struct {
		shares, value int64
		cusip, want   string
	}{
		{150, 1500, "A", "add"},  // +50% shares
		{90, 900, "A", "trim"},   // -10% shares
		{103, 1030, "A", "hold"}, // +3% shares, within ±5
		{40, 400, "Z", "new"},    // absent in prior
	} {
		if got, _ := classify(c.shares, c.value, prevShares, prevValues, c.cusip); got != c.want {
			t.Errorf("classify(shares=%d, %q) = %q, want %q", c.shares, c.cusip, got, c.want)
		}
	}
	if got, _ := classify(10, 100, nil, nil, "A"); got != "hold" {
		t.Errorf("classify with no prior filing = %q, want hold", got)
	}
}

// TestClassifyPRNValueBased asserts a PRN (fixed-income) holding — Shares=0 in
// both quarters but a real Value — is diffed by VALUE, so a bond held across
// quarters reads hold/add/trim, never "new" (the BUG 3 regression). SH holdings,
// which always carry nonzero shares in at least one quarter, are unaffected.
func TestClassifyPRNValueBased(t *testing.T) {
	prevShares := map[string]int64{"BOND": 0, "EQUITY": 50} // PRN carried Shares=0 last quarter
	prevValues := map[string]int64{"BOND": 1000, "EQUITY": 500}
	for _, c := range []struct {
		name          string
		shares, value int64
		cusip, want   string
	}{
		{"bond held flat → hold (NOT new)", 0, 1000, "BOND", "hold"},
		{"bond principal up >5% → add", 0, 1100, "BOND", "add"},
		{"bond principal down >5% → trim", 0, 900, "BOND", "trim"},
		{"bond present last qtr but new this is still value-diffed", 0, 1030, "BOND", "hold"}, // +3% within band
		{"PRN with no prior value → genuinely new", 0, 800, "NEWBOND", "new"},
	} {
		if got, _ := classify(c.shares, c.value, prevShares, prevValues, c.cusip); got != c.want {
			t.Errorf("%s: classify(shares=0, value=%d, %q) = %q, want %q", c.name, c.value, c.cusip, got, c.want)
		}
	}
}
