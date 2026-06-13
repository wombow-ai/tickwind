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
