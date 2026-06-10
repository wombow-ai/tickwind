package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/yahoo"
)

// fakeIndexQuoter serves canned quotes; symbols absent from the map error.
type fakeIndexQuoter struct {
	quotes map[string]yahoo.Quote
}

func (f *fakeIndexQuoter) Quote(_ context.Context, symbol string) (yahoo.Quote, bool, error) {
	q, ok := f.quotes[symbol]
	if !ok {
		return yahoo.Quote{}, false, errors.New("boom")
	}
	return q, true, nil
}

func TestIndicesCacheSweep(t *testing.T) {
	at := time.Date(2026, 6, 10, 14, 30, 0, 0, time.UTC)
	src := &fakeIndexQuoter{quotes: map[string]yahoo.Quote{
		"^GSPC": {Price: 7312.99, PrevClose: 7386.65, At: at},
		"^DJI":  {Price: 44210.1, PrevClose: 44100.0, At: at},
		"^IXIC": {Price: 23890.5, PrevClose: 24010.2, At: at},
	}}
	c := NewIndicesCache(src, time.Minute, nil)

	if got := c.Indices(); got != nil {
		t.Fatalf("Indices() before first sweep = %v, want nil", got)
	}

	c.sweep(context.Background())
	got := c.Indices()
	if len(got) != 3 {
		t.Fatalf("after full sweep: %d quotes, want 3", len(got))
	}
	if got[0].Symbol != "^GSPC" || got[0].Price != 7312.99 || got[0].Source != "yahoo" {
		t.Fatalf("first index = %+v, want ^GSPC @ 7312.99 via yahoo", got[0])
	}
	if got[2].Name != "Nasdaq" {
		t.Fatalf("third index name = %q, want Nasdaq", got[2].Name)
	}

	// One symbol starts failing: its previous level must survive the sweep.
	delete(src.quotes, "^DJI")
	src.quotes["^GSPC"] = yahoo.Quote{Price: 7300.0, PrevClose: 7386.65, At: at.Add(time.Minute)}
	c.sweep(context.Background())
	got = c.Indices()
	if len(got) != 3 {
		t.Fatalf("after partial sweep: %d quotes, want 3 (stale ^DJI kept)", len(got))
	}
	if got[0].Price != 7300.0 {
		t.Fatalf("^GSPC price = %v, want refreshed 7300.0", got[0].Price)
	}
	if got[1].Symbol != "^DJI" || got[1].Price != 44210.1 {
		t.Fatalf("^DJI = %+v, want the previous 44210.1 kept", got[1])
	}

	// Total outage: the cache must keep serving the last good set.
	src.quotes = map[string]yahoo.Quote{}
	c.sweep(context.Background())
	if got = c.Indices(); len(got) != 3 {
		t.Fatalf("after total outage: %d quotes, want 3 (all kept)", len(got))
	}
}
