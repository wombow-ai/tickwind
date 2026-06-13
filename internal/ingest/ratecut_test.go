package ingest

import (
	"context"
	"errors"
	"testing"

	"github.com/wombow-ai/tickwind/internal/ratecut"
)

// fakeAggregator records Refresh calls and pre-seeds a cache so the ingestor's
// per-source error logging and cache exposure can be exercised without network.
type fakeAggregator struct {
	cache    *ratecut.Cache
	errs     map[string]error
	refreshN int
}

func (f *fakeAggregator) Refresh(_ context.Context) map[string]error {
	f.refreshN++
	return f.errs
}

func (f *fakeAggregator) Cache() *ratecut.Cache { return f.cache }

func TestRateCutIngestorRefresh(t *testing.T) {
	c := ratecut.NewCache()
	c.Set(ratecut.Market{Source: "kalshi", Question: "q", Outcomes: []ratecut.Outcome{{Label: "-25bps", Probability: 0.62}}})
	agg := &fakeAggregator{
		cache: c,
		// One source failed — the ingestor must tolerate it (the aggregator keeps
		// the failing source's last-good snapshot; here kalshi is still cached).
		errs: map[string]error{"polymarket": errors.New("boom")},
	}
	ing := NewRateCutIngestor(agg, 0, nil)

	if ing.Cache() != c {
		t.Fatal("Cache() should expose the aggregator's cache")
	}
	ing.refresh(context.Background())
	if agg.refreshN != 1 {
		t.Fatalf("Refresh called %d times, want 1", agg.refreshN)
	}
	if got := ing.Cache().Get(); len(got) != 1 || got[0].Source != "kalshi" {
		t.Fatalf("cache = %+v, want the kalshi snapshot retained", got)
	}
}
