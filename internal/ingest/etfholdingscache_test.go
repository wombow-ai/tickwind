package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/edgar"
)

type fakeETFFetcher struct {
	calls    int
	holdings []edgar.ETFHolding
	asOf     time.Time
	err      error
}

func (f *fakeETFFetcher) ETFHoldings(_ context.Context, _ string, max int) ([]edgar.ETFHolding, time.Time, error) {
	f.calls++
	if f.err != nil {
		return nil, time.Time{}, f.err
	}
	h := f.holdings
	if len(h) > max {
		h = h[:max]
	}
	out := make([]edgar.ETFHolding, len(h)) // copy so in-place enrichment doesn't touch the fixture
	copy(out, h)
	return out, f.asOf, nil
}

type fakeMapper struct {
	calls int
	m     map[string]string
}

func (f *fakeMapper) Map(_ context.Context, cusips []string) (map[string]string, error) {
	f.calls++
	out := map[string]string{}
	for _, cu := range cusips {
		if tk, ok := f.m[cu]; ok {
			out[cu] = tk
		}
	}
	return out, nil
}

func TestETFHoldingsCache(t *testing.T) {
	ctx := context.Background()
	fetcher := &fakeETFFetcher{
		asOf: time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC),
		holdings: []edgar.ETFHolding{
			{Name: "NVIDIA Corp.", CUSIP: "67066G104", PctVal: 8.68},               // ticker-less → enriched
			{Name: "Apple Inc.", Ticker: "AAPL", CUSIP: "037833100", PctVal: 7.63}, // has a ticker → kept
			{Name: "Mystery Co", CUSIP: "999999999", PctVal: 1.0},                  // unmapped → stays empty
		},
	}
	mapper := &fakeMapper{m: map[string]string{"67066G104": "NVDA"}}
	c := NewETFHoldingsCache(fetcher, mapper)

	hs, asOf, err := c.ETFHoldings(ctx, "qqq", 25)
	if err != nil {
		t.Fatal(err)
	}
	if asOf.IsZero() || len(hs) != 3 {
		t.Fatalf("first call: asOf=%v len=%d; want set, 3", asOf, len(hs))
	}
	if hs[0].Ticker != "NVDA" {
		t.Fatalf("NVIDIA should be enriched to NVDA, got %q", hs[0].Ticker)
	}
	if hs[1].Ticker != "AAPL" {
		t.Fatalf("Apple's filing ticker should be kept, got %q", hs[1].Ticker)
	}
	if hs[2].Ticker != "" {
		t.Fatalf("unmapped CUSIP must stay ticker-less (never fabricated), got %q", hs[2].Ticker)
	}

	// Second call within TTL → served from cache (no re-fetch, no re-map).
	if _, _, err := c.ETFHoldings(ctx, "QQQ", 25); err != nil {
		t.Fatal(err)
	}
	if fetcher.calls != 1 {
		t.Fatalf("fetcher called %d times; want 1 (second served from cache)", fetcher.calls)
	}
	if mapper.calls != 1 {
		t.Fatalf("mapper called %d times; want 1 (enrich once)", mapper.calls)
	}

	// cap: top 2 of the cached 3.
	if two, _, _ := c.ETFHoldings(ctx, "QQQ", 2); len(two) != 2 || two[0].Name != "NVIDIA Corp." {
		t.Fatalf("cap=2 failed: %+v", two)
	}

	// Negative caching: an errored ticker caches the error briefly (no re-fetch within negTTL).
	errFetcher := &fakeETFFetcher{err: edgar.ErrNoNPORT}
	ec := NewETFHoldingsCache(errFetcher, mapper)
	if _, _, err := ec.ETFHoldings(ctx, "AAPL", 25); !errors.Is(err, edgar.ErrNoNPORT) {
		t.Fatalf("want ErrNoNPORT, got %v", err)
	}
	_, _, _ = ec.ETFHoldings(ctx, "AAPL", 25)
	if errFetcher.calls != 1 {
		t.Fatalf("errored ticker fetched %d times; want 1 (neg-cached)", errFetcher.calls)
	}
}
