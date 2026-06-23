package ingest

import (
	"context"
	"testing"

	"github.com/wombow-ai/tickwind/internal/edgar"
)

type fakeMatEvents struct {
	m map[string][]edgar.MaterialEvent
}

func (f fakeMatEvents) MaterialEventsN(ctx context.Context, t string, max int) ([]edgar.MaterialEvent, error) {
	return f.m[t], nil
}

func TestMaterialFeedCache(t *testing.T) {
	src := fakeMatEvents{m: map[string][]edgar.MaterialEvent{
		"AAPL": {
			{Form: "8-K", FiledDate: "2026-06-20", AccessionURL: "u1", Items: []edgar.EventItem{{Code: "5.02"}}},                 // notable (officer change)
			{Form: "8-K", FiledDate: "2026-06-19", AccessionURL: "u2", Items: []edgar.EventItem{{Code: "2.02"}, {Code: "9.01"}}}, // routine only → dropped
		},
		"NVDA": {
			{Form: "8-K", FiledDate: "2026-06-22", AccessionURL: "u3", Items: []edgar.EventItem{{Code: "1.01"}, {Code: "9.01"}}}, // notable (1.01) + routine → kept, routine item filtered
		},
		"KO": {}, // no events
	}}
	c := NewMaterialFeedCache(src, func(ctx context.Context) []string { return []string{"AAPL", "NVDA", "KO"} }, nil)
	c.scan(context.Background())

	feed, at := c.Feed("")
	if at.IsZero() {
		t.Fatal("as-of should be set after a non-empty scan")
	}
	// 2 notable events: AAPL 5.02 (06-20) + NVDA 1.01 (06-22). Newest first → NVDA, AAPL.
	if len(feed) != 2 {
		t.Fatalf("want 2 feed events, got %d: %+v", len(feed), feed)
	}
	if feed[0].Ticker != "NVDA" || feed[1].Ticker != "AAPL" {
		t.Fatalf("order = %s,%s, want NVDA,AAPL (newest first)", feed[0].Ticker, feed[1].Ticker)
	}
	// NVDA's routine 9.01 is filtered out — only the notable 1.01 remains on the row.
	if len(feed[0].Items) != 1 || feed[0].Items[0].Code != "1.01" {
		t.Fatalf("NVDA row items = %+v, want [1.01] (9.01 filtered)", feed[0].Items)
	}

	// ?item= filter
	if got, _ := c.Feed("5.02"); len(got) != 1 || got[0].Ticker != "AAPL" {
		t.Fatalf("Feed(5.02) = %+v, want [AAPL]", got)
	}
	if got, _ := c.Feed("9.01"); len(got) != 0 {
		t.Fatalf("Feed(9.01) should be empty (routine code never on the feed), got %+v", got)
	}

	// keep-last-good: an empty scan keeps the prior feed rather than blanking it.
	c.src = fakeMatEvents{m: map[string][]edgar.MaterialEvent{}}
	c.scan(context.Background())
	if feed2, _ := c.Feed(""); len(feed2) != 2 {
		t.Fatalf("empty scan should keep last good (2), got %d", len(feed2))
	}
}
