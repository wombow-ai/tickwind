package congress

import (
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/congress/ptr"
)

func TestSlugify(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Nancy Pelosi", "nancy-pelosi"},
		{"Richard W. Allen", "richard-w-allen"},
		{"  Robert  B.  Aderholt ", "robert-b-aderholt"},
		{"O'Brien", "o-brien"},
		{"", ""},
	}
	for _, c := range cases {
		if got := Slugify(c.in); got != c.want {
			t.Errorf("Slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCacheTransactionsAndIndexes(t *testing.T) {
	c := NewCache()
	older := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	byMember := map[string]MemberTx{
		"nancy-pelosi": {
			Slug: "nancy-pelosi", Name: "Nancy Pelosi", State: "CA",
			Transactions: []ptr.Transaction{
				{Ticker: "AAPL", Type: ptr.TxPurchase, AmountRange: "$1 - $2", TxDate: older},
				{Ticker: "", Type: ptr.TxPurchase, TxDate: newer}, // no ticker → not indexed
			},
		},
		"jane-doe": {
			Slug: "jane-doe", Name: "Jane Doe", State: "TX",
			Transactions: []ptr.Transaction{
				{Ticker: "AAPL", Type: ptr.TxSale, AmountRange: "$3 - $4", TxDate: newer},
			},
		},
	}
	c.SetWithTransactions([]Filing{{DocID: "1"}}, byMember)

	// ByTicker is case-insensitive, newest-first, and skips tickerless trades.
	aapl := c.ByTicker("aapl")
	if len(aapl) != 2 {
		t.Fatalf("ByTicker(AAPL) len = %d, want 2: %+v", len(aapl), aapl)
	}
	if !aapl[0].TxDate.Equal(newer) || aapl[0].MemberName != "Jane Doe" {
		t.Errorf("ByTicker not newest-first: %+v", aapl)
	}

	// ByMember.
	m, ok := c.ByMember("NANCY-PELOSI") // case-insensitive
	if !ok || m.Name != "Nancy Pelosi" || len(m.Transactions) != 2 {
		t.Fatalf("ByMember = %+v ok=%v", m, ok)
	}
	if _, ok := c.ByMember("ghost"); ok {
		t.Error("ByMember(ghost) should be ok=false")
	}

	// Members sorted by name.
	mem := c.Members()
	if len(mem) != 2 || mem[0].Name != "Jane Doe" || mem[1].Name != "Nancy Pelosi" {
		t.Errorf("Members not name-sorted: %+v", mem)
	}

	// Filing index preserved.
	if c.Len() != 1 {
		t.Errorf("Len = %d, want 1", c.Len())
	}
}

// TestCacheTickerValidatorDropsNonUniverse verifies BUG 6: a parsed parenthetical
// that is NOT a real US symbol (a crypto / description acronym the ptr parser
// extracts verbatim) is dropped from the by-ticker index, while a real ticker is
// kept — mirroring how CUSIPs / empty tickers are already dropped. The member's
// raw transactions are unaffected (only ticker-indexing is gated).
func TestCacheTickerValidatorDropsNonUniverse(t *testing.T) {
	c := NewCache()
	// Stub universe: AAPL is real; BTC (a crypto symbol the parser might extract
	// from "(BTC)") is NOT in the US equity universe.
	universe := map[string]bool{"AAPL": true}
	c.SetTickerValidator(func(t string) bool { return universe[t] })

	byMember := map[string]MemberTx{
		"jane-doe": {
			Slug: "jane-doe", Name: "Jane Doe", State: "TX",
			Transactions: []ptr.Transaction{
				{Ticker: "AAPL", Type: ptr.TxPurchase, AmountRange: "$1 - $2", TxDate: time.Now().UTC()},
				{Ticker: "BTC", Type: ptr.TxPurchase, AmountRange: "$3 - $4", TxDate: time.Now().UTC()},
			},
		},
	}
	c.SetWithTransactions([]Filing{{DocID: "1"}}, byMember)

	// Real ticker kept.
	if got := c.ByTicker("AAPL"); len(got) != 1 {
		t.Errorf("ByTicker(AAPL) = %+v, want one trade (real ticker kept)", got)
	}
	// Non-universe parenthetical dropped (would otherwise assert a congress trade
	// on an unrelated real BTC-like stock / a false ticker).
	if got := c.ByTicker("BTC"); len(got) != 0 {
		t.Errorf("ByTicker(BTC) = %+v, want none (non-universe parenthetical dropped)", got)
	}
	// Member page still carries BOTH raw transactions — only ticker-indexing is gated.
	if m, ok := c.ByMember("jane-doe"); !ok || len(m.Transactions) != 2 {
		t.Errorf("ByMember(jane-doe) transactions = %d, want 2 (raw txns unaffected)", len(m.Transactions))
	}
}

// TestCacheNilValidatorKeepsAll verifies the nil-safe fallback: with no validator
// wired (tests, or before the symbol universe loads), every parsed ticker is
// served — today's behavior, never "drop everything".
func TestCacheNilValidatorKeepsAll(t *testing.T) {
	c := NewCache() // no SetTickerValidator
	byMember := map[string]MemberTx{
		"jane-doe": {
			Slug: "jane-doe", Name: "Jane Doe",
			Transactions: []ptr.Transaction{
				{Ticker: "AAPL", Type: ptr.TxPurchase, TxDate: time.Now().UTC()},
				{Ticker: "BTC", Type: ptr.TxPurchase, TxDate: time.Now().UTC()}, // would be dropped if validated
			},
		},
	}
	c.SetWithTransactions(nil, byMember)
	if got := c.ByTicker("AAPL"); len(got) != 1 {
		t.Errorf("nil validator: ByTicker(AAPL) = %+v, want kept", got)
	}
	if got := c.ByTicker("BTC"); len(got) != 1 {
		t.Errorf("nil validator: ByTicker(BTC) = %+v, want kept (no universe gate)", got)
	}
}

// TestCacheSetKeepsTransactions verifies the index-only Set() retains previously
// parsed transactions (the degraded path shouldn't wipe ticker/member detail).
func TestCacheSetKeepsTransactions(t *testing.T) {
	c := NewCache()
	c.SetWithTransactions(nil, map[string]MemberTx{
		"nancy-pelosi": {Slug: "nancy-pelosi", Name: "Nancy Pelosi", Transactions: []ptr.Transaction{{Ticker: "NVDA", Type: ptr.TxPurchase}}},
	})
	c.Set([]Filing{{DocID: "9"}}) // a later filings-only refresh
	if got := c.ByTicker("NVDA"); len(got) != 1 {
		t.Errorf("Set() dropped prior transactions: ByTicker(NVDA) = %+v", got)
	}
}
