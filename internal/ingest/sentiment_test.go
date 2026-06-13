package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/cboe"
	"github.com/wombow-ai/tickwind/internal/finrashvol"
	"github.com/wombow-ai/tickwind/internal/sentiment"
	"github.com/wombow-ai/tickwind/internal/yahoo"
)

type fakeVIX struct {
	q   yahoo.Quote
	ok  bool
	err error
}

func (f fakeVIX) Quote(_ context.Context, _ string) (yahoo.Quote, bool, error) {
	return f.q, f.ok, f.err
}

type fakeChainSource struct {
	chain cboe.Chain
	ok    bool
	err   error
}

func (f fakeChainSource) Options(_ context.Context, _ string) (cboe.Chain, bool, error) {
	return f.chain, f.ok, f.err
}

// fakeAverager satisfies ShortPctAverager from canned rows.
type fakeAverager struct{ rows []finrashvol.ShortVol }

func (f fakeAverager) Top(_ int, _ int64) []finrashvol.ShortVol { return f.rows }
func (f fakeAverager) AsOf() string                             { return "2026-06-12" }

func TestSentimentGatherAllComponents(t *testing.T) {
	ing := NewSentimentIngestor(
		fakeVIX{q: yahoo.Quote{Price: 18.0}, ok: true},
		fakeChainSource{ok: true, chain: cboe.Chain{Contracts: []cboe.Contract{
			{Type: "P", Volume: 800},
			{Type: "C", Volume: 1000},
		}}},
		fakeAverager{rows: []finrashvol.ShortVol{
			{Symbol: "GME", ShortPct: 60, TotalVolume: 2_000_000},
			{Symbol: "AMC", ShortPct: 40, TotalVolume: 2_000_000},
		}},
		sentiment.NewCache(), time.Hour, nil)

	in := ing.gather(context.Background())
	if in.VIX == nil || *in.VIX != 18.0 {
		t.Fatalf("VIX = %v, want 18.0", in.VIX)
	}
	if in.PutCallRatio == nil || *in.PutCallRatio != 0.8 {
		t.Fatalf("PutCallRatio = %v, want 0.8 (800P/1000C)", in.PutCallRatio)
	}
	if in.ShortPct == nil || *in.ShortPct != 50 {
		t.Fatalf("ShortPct = %v, want 50 (mean of 60,40)", in.ShortPct)
	}
}

func TestSentimentGatherSkipsFailingSources(t *testing.T) {
	// VIX errors, options has no chain, but the short average is present → only
	// the ShortPct component should be set (Compute re-weights to it alone).
	ing := NewSentimentIngestor(
		fakeVIX{err: errors.New("boom")},
		fakeChainSource{ok: false},
		fakeAverager{rows: []finrashvol.ShortVol{{Symbol: "GME", ShortPct: 30}}},
		sentiment.NewCache(), time.Hour, nil)

	in := ing.gather(context.Background())
	if in.VIX != nil {
		t.Fatalf("VIX should be nil on fetch error, got %v", in.VIX)
	}
	if in.PutCallRatio != nil {
		t.Fatalf("PutCallRatio should be nil when no chain, got %v", in.PutCallRatio)
	}
	if in.ShortPct == nil || *in.ShortPct != 30 {
		t.Fatalf("ShortPct = %v, want 30", in.ShortPct)
	}
}

func TestSentimentComputeStoresDailyPoint(t *testing.T) {
	cache := sentiment.NewCache()
	ing := NewSentimentIngestor(
		fakeVIX{q: yahoo.Quote{Price: 15.0}, ok: true}, nil, nil,
		cache, time.Hour, nil)
	ing.now = func() time.Time { return time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC) }

	ing.compute(context.Background())

	res, ok := cache.Latest()
	if !ok || res.Available != 1 {
		t.Fatalf("Latest = %+v ok=%v, want 1 component (VIX only)", res, ok)
	}
	hist := cache.History()
	if len(hist) != 1 || hist[0].Date != "2026-06-12" {
		t.Fatalf("history = %+v, want one point dated 2026-06-12", hist)
	}
}

func TestAverageShortPctEmpty(t *testing.T) {
	if _, ok := averageShortPct(nil); ok {
		t.Fatal("averageShortPct(nil) should report ok=false")
	}
}
