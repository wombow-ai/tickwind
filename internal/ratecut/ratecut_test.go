package ratecut

import (
	"context"
	"errors"
	"testing"
)

func TestSortOutcomesDescending(t *testing.T) {
	got := sortOutcomes([]Outcome{
		{Label: "b", Probability: 0.2},
		{Label: "a", Probability: 0.5},
		{Label: "c", Probability: 0.2},
	})
	if got[0].Label != "a" {
		t.Errorf("first = %q, want a (highest prob)", got[0].Label)
	}
	// Ties broken by label ascending → "b" before "c".
	if got[1].Label != "b" || got[2].Label != "c" {
		t.Errorf("tie order = %q,%q, want b,c", got[1].Label, got[2].Label)
	}
}

func TestCacheSetGet(t *testing.T) {
	c := NewCache()
	if c.Len() != 0 || len(c.Get()) != 0 {
		t.Fatal("fresh cache should be empty")
	}
	if !c.UpdatedAt().IsZero() {
		t.Error("fresh cache UpdatedAt should be zero")
	}

	c.Set(Market{Source: "polymarket", Question: "q1"})
	c.Set(Market{Source: "kalshi", Question: "q2"})
	if c.Len() != 2 {
		t.Fatalf("Len = %d, want 2", c.Len())
	}
	if c.UpdatedAt().IsZero() {
		t.Error("UpdatedAt should be set after Set")
	}

	// Get is sorted by source name.
	got := c.Get()
	if got[0].Source != "kalshi" || got[1].Source != "polymarket" {
		t.Errorf("Get order = %q,%q, want kalshi,polymarket", got[0].Source, got[1].Source)
	}

	// Per-source lookup.
	if m, ok := c.Source("kalshi"); !ok || m.Question != "q2" {
		t.Errorf("Source(kalshi) = %+v,%v", m, ok)
	}
	if _, ok := c.Source("nope"); ok {
		t.Error("Source(nope) should be missing")
	}

	// Set replaces in place (no duplicate source).
	c.Set(Market{Source: "kalshi", Question: "q2-updated"})
	if c.Len() != 2 {
		t.Errorf("Len after replace = %d, want 2", c.Len())
	}
	if m, _ := c.Source("kalshi"); m.Question != "q2-updated" {
		t.Errorf("kalshi question = %q, want q2-updated", m.Question)
	}
}

// stubFetcher is a Fetcher for testing the Aggregator.
type stubFetcher struct {
	source string
	market Market
	err    error
}

func (s stubFetcher) Source() string { return s.source }
func (s stubFetcher) Fetch(context.Context) (Market, error) {
	if s.err != nil {
		return Market{}, s.err
	}
	return s.market, nil
}

func TestAggregatorRefreshAllSucceed(t *testing.T) {
	agg := NewAggregator(
		stubFetcher{source: "kalshi", market: Market{Source: "kalshi", Question: "k"}},
		stubFetcher{source: "polymarket", market: Market{Source: "polymarket", Question: "p"}},
	)
	errs := agg.Refresh(context.Background())
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if agg.Cache().Len() != 2 {
		t.Errorf("cache Len = %d, want 2", agg.Cache().Len())
	}
}

func TestAggregatorRefreshOneFailsIsolated(t *testing.T) {
	agg := NewAggregator(
		stubFetcher{source: "kalshi", market: Market{Source: "kalshi", Question: "k"}},
		stubFetcher{source: "polymarket", err: errors.New("boom")},
	)
	errs := agg.Refresh(context.Background())
	if len(errs) != 1 {
		t.Fatalf("errors = %v, want exactly polymarket failing", errs)
	}
	if errs["polymarket"] == nil {
		t.Error("expected polymarket error")
	}
	// The healthy source is still cached.
	if _, ok := agg.Cache().Source("kalshi"); !ok {
		t.Error("kalshi should be cached despite polymarket failing")
	}
	if _, ok := agg.Cache().Source("polymarket"); ok {
		t.Error("polymarket should not be cached on failure")
	}
}

func TestAggregatorFailureKeepsLastGood(t *testing.T) {
	// First refresh: both succeed.
	good := stubFetcher{source: "polymarket", market: Market{Source: "polymarket", Question: "good"}}
	agg := NewAggregator(good)
	agg.Refresh(context.Background())
	if m, _ := agg.Cache().Source("polymarket"); m.Question != "good" {
		t.Fatalf("first refresh question = %q", m.Question)
	}

	// Swap in a failing fetcher; the previous snapshot must survive.
	agg.fetchers = []Fetcher{stubFetcher{source: "polymarket", err: errors.New("down")}}
	errs := agg.Refresh(context.Background())
	if errs["polymarket"] == nil {
		t.Fatal("expected polymarket error on second refresh")
	}
	if m, ok := agg.Cache().Source("polymarket"); !ok || m.Question != "good" {
		t.Errorf("after failure cache = %+v,%v, want last-good 'good'", m, ok)
	}
}
