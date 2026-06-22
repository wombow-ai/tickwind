package ingest

import (
	"context"
	"testing"

	"github.com/wombow-ai/tickwind/internal/edgar"
	"github.com/wombow-ai/tickwind/internal/store"
)

type fakeDivFunds struct{ f edgar.Fundamentals }

func (s fakeDivFunds) Fundamentals(ctx context.Context, t string) (edgar.Fundamentals, error) {
	return s.f, nil
}

type fakeDivQuotes struct{ has map[string]float64 }

func (s fakeDivQuotes) GetQuote(ctx context.Context, t string) (store.Quote, bool, error) {
	if p, ok := s.has[t]; ok {
		return store.Quote{Price: p}, true, nil
	}
	return store.Quote{}, false, nil
}

type fakeDivFallback struct {
	price float64
	calls *int
}

func (s fakeDivFallback) LatestQuote(ctx context.Context, t string) (store.Quote, bool, error) {
	*s.calls++
	return store.Quote{Price: s.price}, true, nil
}

// TestDividendCache_PriceFallback locks in the cold-start fix: a payer the poller hasn't populated
// (no store quote) still gets a yield via the bars fallback, while a polled name uses the store quote
// and never touches the fallback — so the highest-yield leaderboard isn't thin right after a restart.
func TestDividendCache_PriceFallback(t *testing.T) {
	f := edgar.Fundamentals{Revenue: 1000, NetIncome: 200, Shares: 1000, Equity: 500, DividendsPaid: 100, Period: "FY2024"}
	calls := 0
	c := NewDividendCache(
		fakeDivFunds{f: f},
		fakeDivQuotes{has: map[string]float64{"HASQ": 10}}, // only HASQ has a polled quote
		fakeDivFallback{price: 20, calls: &calls},          // NOQ resolves via the on-demand fallback
		func(ctx context.Context) []string { return []string{"HASQ", "NOQ"} },
		nil,
	)
	c.scan(context.Background())

	ranked, _ := c.PopulationRanked("highest-yield")
	if len(ranked) != 2 {
		t.Fatalf("both payers should rank (HASQ via store, NOQ via fallback), got %d", len(ranked))
	}
	// yield = DividendsPaid / (price*shares) * 100 → HASQ: 100/(10*1000)*100 = 1.0; NOQ: 0.5.
	if ranked[0].Ticker != "HASQ" || ranked[1].Ticker != "NOQ" {
		t.Fatalf("highest-yield order = %s,%s, want HASQ,NOQ", ranked[0].Ticker, ranked[1].Ticker)
	}
	if ranked[0].Yield == nil || ranked[1].Yield == nil {
		t.Fatal("both rows should carry a computed yield")
	}
	if calls != 1 {
		t.Fatalf("fallback should be called once (NOQ only — HASQ uses the store quote), got %d", calls)
	}
}
