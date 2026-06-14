package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/cboe"
	"github.com/wombow-ai/tickwind/internal/finrashvol"
	"github.com/wombow-ai/tickwind/internal/sentiment"
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/store/memory"
	"github.com/wombow-ai/tickwind/internal/universe"
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

// fakeBreadth satisfies BreadthSource from canned counts.
type fakeBreadth struct {
	adv, dec int
	ok       bool
}

func (f fakeBreadth) Breadth() (int, int, bool) { return f.adv, f.dec, f.ok }

// fakeHeat satisfies HeatSource from a canned value.
type fakeHeat struct {
	v  float64
	ok bool
}

func (f fakeHeat) Heat() (float64, bool) { return f.v, f.ok }

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
		sentiment.NewCache(), nil, time.Hour, nil)
	ing.SetBreadthSource(fakeBreadth{adv: 70, dec: 30, ok: true})
	ing.SetHeatSource(fakeHeat{v: 72, ok: true})

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
	if in.Advancers == nil || *in.Advancers != 70 || in.Decliners == nil || *in.Decliners != 30 {
		t.Fatalf("breadth = %v/%v, want 70/30", in.Advancers, in.Decliners)
	}
	if in.Heat == nil || *in.Heat != 72 {
		t.Fatalf("Heat = %v, want 72", in.Heat)
	}
	// New-highs/new-lows is deferred (no whole-market 52-week range), so its
	// component must stay nil even with every wired source present.
	if in.NewHighs != nil || in.NewLows != nil {
		t.Fatalf("NewHighs/NewLows should be nil (deferred), got %v/%v", in.NewHighs, in.NewLows)
	}
	// Compute now reports the extra components: VIX, Put/Call, Short, Breadth, Heat.
	if res := sentiment.Compute(in); res.Available != 5 {
		t.Fatalf("Available = %d, want 5 (VIX+put/call+short+breadth+heat)", res.Available)
	}
}

// TestSentimentGatherSkipsAbsentBreadthHeat checks the new sources are nil-safe:
// when no breadth/heat source is wired (or each reports ok=false), the
// corresponding inputs stay nil and Compute counts only the base components.
func TestSentimentGatherSkipsAbsentBreadthHeat(t *testing.T) {
	// No breadth/heat sources wired at all → both inputs nil.
	ing := NewSentimentIngestor(
		fakeVIX{q: yahoo.Quote{Price: 20.0}, ok: true}, nil, nil,
		sentiment.NewCache(), nil, time.Hour, nil)
	in := ing.gather(context.Background())
	if in.Advancers != nil || in.Decliners != nil || in.Heat != nil {
		t.Fatalf("absent sources should leave breadth/heat nil, got %v/%v heat=%v",
			in.Advancers, in.Decliners, in.Heat)
	}
	if res := sentiment.Compute(in); res.Available != 1 {
		t.Fatalf("Available = %d, want 1 (VIX only)", res.Available)
	}

	// Sources wired but reporting ok=false (no data yet) → still nil.
	ing.SetBreadthSource(fakeBreadth{ok: false})
	ing.SetHeatSource(fakeHeat{ok: false})
	in = ing.gather(context.Background())
	if in.Advancers != nil || in.Decliners != nil || in.Heat != nil {
		t.Fatalf("ok=false sources should leave breadth/heat nil, got %v/%v heat=%v",
			in.Advancers, in.Decliners, in.Heat)
	}
}

func TestSentimentGatherSkipsFailingSources(t *testing.T) {
	// VIX errors, options has no chain, but the short average is present → only
	// the ShortPct component should be set (Compute re-weights to it alone).
	ing := NewSentimentIngestor(
		fakeVIX{err: errors.New("boom")},
		fakeChainSource{ok: false},
		fakeAverager{rows: []finrashvol.ShortVol{{Symbol: "GME", ShortPct: 30}}},
		sentiment.NewCache(), nil, time.Hour, nil)

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
	st := memory.New()
	ing := NewSentimentIngestor(
		fakeVIX{q: yahoo.Quote{Price: 15.0}, ok: true}, nil, nil,
		cache, st, time.Hour, nil)
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
	// The score is also persisted to the durable store.
	got, err := st.FearGreedHistory(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Date != "2026-06-12" || got[0].Score != res.Score {
		t.Fatalf("persisted history = %+v, want one 2026-06-12 point with score %d", got, res.Score)
	}
}

func TestAverageShortPctEmpty(t *testing.T) {
	if _, ok := averageShortPct(nil); ok {
		t.Fatal("averageShortPct(nil) should report ok=false")
	}
}

func TestUniverseBreadth(t *testing.T) {
	// nil/empty cache → ok=false (no fabricated breadth).
	if _, _, ok := NewUniverseBreadth(nil).Breadth(); ok {
		t.Fatal("nil cache should report ok=false")
	}
	cache := universe.NewCache()
	if _, _, ok := NewUniverseBreadth(cache).Breadth(); ok {
		t.Fatal("unswept cache should report ok=false")
	}

	cache.Set(map[string]store.Quote{
		"UP1":   {Ticker: "UP1", Price: 11, PrevClose: 10}, // +10% advancer
		"UP2":   {Ticker: "UP2", Price: 21, PrevClose: 20}, // +5% advancer
		"DN1":   {Ticker: "DN1", Price: 9, PrevClose: 10},  // −10% decliner
		"FLAT":  {Ticker: "FLAT", Price: 10, PrevClose: 10},
		"NOREF": {Ticker: "NOREF", Price: 10},                  // no prev close → skipped
		"DEAD":  {Ticker: "DEAD", PrevClose: 10},               // no price → skipped
		"SPLIT": {Ticker: "SPLIT", Price: 1000, PrevClose: 10}, // +9900% artifact → skipped
	})
	adv, dec, ok := NewUniverseBreadth(cache).Breadth()
	if !ok || adv != 2 || dec != 1 {
		t.Fatalf("Breadth = %d adv / %d dec ok=%v, want 2/1/true", adv, dec, ok)
	}
}

// fakeHotLister satisfies hotLister from a canned board.
type fakeHotLister struct {
	rows []store.HotStock
	err  error
}

func (f fakeHotLister) HotList(_ context.Context, _ string, _ int) ([]store.HotStock, error) {
	return f.rows, f.err
}

func TestHotListHeat(t *testing.T) {
	// nil store → ok=false.
	if _, ok := NewHotListHeat(nil).Heat(); ok {
		t.Fatal("nil store should report ok=false")
	}
	// Empty board → ok=false (skipped, not a fake 50).
	if _, ok := NewHotListHeat(fakeHotLister{}).Heat(); ok {
		t.Fatal("empty board should report ok=false")
	}
	// Store error → ok=false.
	if _, ok := NewHotListHeat(fakeHotLister{err: errors.New("boom")}).Heat(); ok {
		t.Fatal("store error should report ok=false")
	}

	// A flat board (no 24h growth) sits at the 50 neutral midpoint.
	flat := fakeHotLister{rows: []store.HotStock{
		{Ticker: "A", Mentions: 100, Change: 0},
		{Ticker: "B", Mentions: 50, Change: 0},
	}}
	if v, ok := NewHotListHeat(flat).Heat(); !ok || v != 50 {
		t.Fatalf("flat heat = %v ok=%v, want 50/true", v, ok)
	}

	// Mention-weighted growth: A (+100%, 300 mentions) dominates B (0%, 100
	// mentions). weighted = (1.0·300 + 0·100)/400 = 0.75 → 50 + 50·0.75 = 87.5.
	hot := fakeHotLister{rows: []store.HotStock{
		{Ticker: "A", Mentions: 300, Change: 1.0},
		{Ticker: "B", Mentions: 100, Change: 0},
	}}
	if v, ok := NewHotListHeat(hot).Heat(); !ok || v != 87.5 {
		t.Fatalf("hot heat = %v ok=%v, want 87.5/true", v, ok)
	}

	// Growth above +100% saturates at 100, and zero-mention rows are ignored.
	sat := fakeHotLister{rows: []store.HotStock{
		{Ticker: "A", Mentions: 100, Change: 3.0},
		{Ticker: "Z", Mentions: 0, Change: 5.0}, // no mention base → ignored
	}}
	if v, ok := NewHotListHeat(sat).Heat(); !ok || v != 100 {
		t.Fatalf("saturated heat = %v ok=%v, want 100/true", v, ok)
	}

	// A board with only zero-mention rows has no usable base → ok=false.
	noBase := fakeHotLister{rows: []store.HotStock{{Ticker: "Z", Mentions: 0, Change: 5}}}
	if _, ok := NewHotListHeat(noBase).Heat(); ok {
		t.Fatal("zero-mention-only board should report ok=false")
	}
}
