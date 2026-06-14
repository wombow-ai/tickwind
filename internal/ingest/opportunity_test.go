package ingest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/opportunity"
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/store/memory"
)

// fakeSnapshotter returns a fixed price map (or an error) for Snapshots, so the
// test can simulate a healthy fetch, a rate-limit error, and an empty result.
type fakeSnapshotter struct {
	prices map[string]float64
	err    error
}

func (f fakeSnapshotter) Snapshots(_ context.Context, _ []string) (map[string]float64, error) {
	return f.prices, f.err
}

// TestRecomputeKeepsLastGoodBoardOnPriceFailure is the regression for the empty
// Opportunity board: a transient price-fetch failure (an Alpaca 429) once
// overwrote a healthy board with an empty one, because recompute proceeded with
// an empty price map and every row was gated out on the price<=0 check. The fix
// keeps the last-good board when the price fetch fails (or prices nothing).
func TestRecomputeKeepsLastGoodBoardOnPriceFailure(t *testing.T) {
	now := time.Now().UTC()
	st := memory.New()
	// One qualifying small-cap buy: $300k, with dei shares → market cap in band
	// once a price is supplied.
	buy := store.InsiderBuy{
		Accession: "acc-1", Ticker: "AAT", CIK: 1500217, Company: "American Assets Trust",
		OwnerName: "Jane Insider", FiledDate: now.AddDate(0, 0, -1), Shares: 10000, Value: 300_000,
		FilingURL: "https://example.com/filing",
	}
	if err := st.SaveInsiderBuys(context.Background(), []store.InsiderBuy{buy}); err != nil {
		t.Fatalf("seed buys: %v", err)
	}
	cache := opportunity.NewCache()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	o := NewOpportunityIngestor(st, nil, fakeSnapshotter{prices: map[string]float64{"AAT": 21.0}}, cache, time.Hour, 0, log)
	o.shares = map[int]int64{1500217: 61_390_936} // ~$1.3B @ $21 → small-cap band

	// 1) Healthy fetch → board populates.
	o.recompute(context.Background())
	if got := len(cache.Get()); got != 1 {
		t.Fatalf("after healthy recompute: got %d rows, want 1", got)
	}

	// 2) Rate-limit error → board MUST be preserved (the bug).
	o.prices = fakeSnapshotter{err: errors.New("alpaca: snapshots 429 Too Many Requests")}
	o.recompute(context.Background())
	if got := len(cache.Get()); got != 1 {
		t.Fatalf("after 429: got %d rows, want last-good 1 (board was clobbered)", got)
	}

	// 3) Empty price map, no error (priced nothing) → also preserved.
	o.prices = fakeSnapshotter{prices: map[string]float64{}}
	o.recompute(context.Background())
	if got := len(cache.Get()); got != 1 {
		t.Fatalf("after empty prices: got %d rows, want last-good 1", got)
	}

	// 4) Healthy fetch again → board still recomputes normally.
	o.prices = fakeSnapshotter{prices: map[string]float64{"AAT": 21.0}}
	o.recompute(context.Background())
	if got := len(cache.Get()); got != 1 {
		t.Fatalf("after recovery: got %d rows, want 1", got)
	}
}

// TestRecomputeEmptyWhenNoBuys confirms the genuine-no-data path still empties
// the board (so a real quiet period is honestly reflected, not frozen stale).
func TestRecomputeEmptyWhenNoBuys(t *testing.T) {
	st := memory.New()
	cache := opportunity.NewCache()
	cache.Set([]opportunity.Stock{{Ticker: "STALE"}}) // pretend a prior board exists
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	o := NewOpportunityIngestor(st, nil, fakeSnapshotter{prices: map[string]float64{}}, cache, time.Hour, 0, log)

	o.recompute(context.Background())
	if got := len(cache.Get()); got != 0 {
		t.Fatalf("no buys should empty the board: got %d rows, want 0", got)
	}
}
