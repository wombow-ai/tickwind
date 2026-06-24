package api

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/edgar"
)

type fakeETFSource struct {
	holdings []edgar.ETFHolding
	asOf     time.Time
	err      error
}

func (f fakeETFSource) ETFHoldings(_ context.Context, _ string, max int) ([]edgar.ETFHolding, time.Time, error) {
	if f.err != nil {
		return nil, time.Time{}, f.err
	}
	h := f.holdings
	if len(h) > max {
		h = h[:max]
	}
	return h, f.asOf, nil
}

func TestChatETFHoldingsText(t *testing.T) {
	src := fakeETFSource{
		asOf: time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC),
		holdings: []edgar.ETFHolding{
			{Name: "NVIDIA Corp.", PctVal: 8.68},
			{Name: "Apple Inc.", Ticker: "AAPL", PctVal: 7.63},
		},
	}
	c := NewChatETFHoldings(src)
	if c == nil {
		t.Fatal("NewChatETFHoldings returned nil for a non-nil source")
	}
	txt, ok := c.ETFHoldingsText(context.Background(), "qqq", "en")
	if !ok {
		t.Fatal("ok=false; want true")
	}
	for _, want := range []string{"QQQ", "2026-05-28", "1. NVIDIA Corp. — 8.68%", "2. Apple Inc. (AAPL) — 7.63%"} {
		if !strings.Contains(txt, want) {
			t.Fatalf("missing %q in:\n%s", want, txt)
		}
	}

	// No holdings (not a fund) → ok=false so the model answers honestly, never improvising.
	if _, ok := NewChatETFHoldings(fakeETFSource{}).ETFHoldingsText(context.Background(), "AAPL", "en"); ok {
		t.Fatal("empty holdings: want ok=false")
	}
	// A fetch error → ok=false, not a fabricated answer.
	if _, ok := NewChatETFHoldings(fakeETFSource{err: errors.New("boom")}).ETFHoldingsText(context.Background(), "QQQ", "en"); ok {
		t.Fatal("fetch error: want ok=false")
	}
	// nil source → nil lister (the tool is never offered).
	if NewChatETFHoldings(nil) != nil {
		t.Fatal("nil source: want nil lister")
	}
}
