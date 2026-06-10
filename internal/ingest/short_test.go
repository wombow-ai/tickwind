package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/finra"
)

// fakeShortSource serves canned partitions; dates in errOn fail.
type fakeShortSource struct {
	byDate map[string][]finra.ShortInterest
	errOn  map[string]bool
	calls  []string
}

func (f *fakeShortSource) Rows(_ context.Context, date string, _, offset int) ([]finra.ShortInterest, error) {
	f.calls = append(f.calls, date)
	if f.errOn[date] {
		return nil, errors.New("boom")
	}
	if offset > 0 {
		return nil, nil // single-page fakes
	}
	return f.byDate[date], nil
}

func TestShortCacheSweep(t *testing.T) {
	// sweep() probes finra.LatestSettlementCandidates(time.Now(), 2), so the
	// test publishes periods at real candidate dates: an older one first, then
	// the newest, to exercise fall-through and the upgrade path date-proofly.
	cands := finra.LatestSettlementCandidates(time.Now().UTC(), 2)
	if len(cands) < 3 {
		t.Fatalf("unexpectedly few candidates: %v", cands)
	}
	newest, older := cands[0], cands[2]
	gme := finra.ShortInterest{Symbol: "GME", SettlementDate: older, ShortQty: 50e6, DaysToCover: 7.4}
	src := &fakeShortSource{
		byDate: map[string][]finra.ShortInterest{older: {gme}},
		errOn:  map[string]bool{},
	}
	c := NewShortCache(src, time.Hour, nil)

	// First sweep: newer candidates are unpublished → falls through to `older`.
	c.sweep(context.Background())
	if got := c.Settlement(); got != older {
		t.Fatalf("settlement = %q, want %q", got, older)
	}
	si, ok := c.ShortInterest("GME")
	if !ok || si.DaysToCover != 7.4 {
		t.Fatalf("GME = %+v ok=%v", si, ok)
	}

	// Re-sweep with nothing new: must stop at the held period, not refetch it.
	src.calls = nil
	c.sweep(context.Background())
	for _, d := range src.calls {
		if d == older {
			t.Fatal("refetched the already-held period")
		}
	}

	// The newest period publishes → table swaps to it.
	src.byDate[newest] = []finra.ShortInterest{
		{Symbol: "GME", SettlementDate: newest, ShortQty: 60e6, DaysToCover: 9.1},
	}
	c.sweep(context.Background())
	if got := c.Settlement(); got != newest {
		t.Fatalf("settlement after publish = %q, want %q", got, newest)
	}
	if si, _ = c.ShortInterest("GME"); si.DaysToCover != 9.1 {
		t.Fatalf("GME after swap = %+v", si)
	}

	// A probe error aborts the sweep (no half-built table): a fresh cache
	// whose newest candidate errors must stay empty rather than fall through.
	bad := &fakeShortSource{
		byDate: map[string][]finra.ShortInterest{older: {gme}},
		errOn:  map[string]bool{newest: true},
	}
	c2 := NewShortCache(bad, time.Hour, nil)
	c2.sweep(context.Background())
	if got := c2.Settlement(); got != "" {
		t.Fatalf("settlement after aborted sweep = %q, want empty", got)
	}
}
