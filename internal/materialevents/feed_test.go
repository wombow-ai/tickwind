package materialevents

import (
	"testing"

	"github.com/wombow-ai/tickwind/internal/edgar"
)

func TestNotableItems(t *testing.T) {
	items := []edgar.EventItem{
		{Code: "2.02", LabelEN: "Results of Operations"}, // routine (earnings) → dropped
		{Code: "5.02", LabelEN: "Officer change"},        // notable → kept
		{Code: "9.01", LabelEN: "Exhibits"},              // routine → dropped
		{Code: "1.03", LabelEN: "Bankruptcy"},            // notable → kept
	}
	got := NotableItems(items)
	if len(got) != 2 {
		t.Fatalf("want 2 notable items, got %d: %+v", len(got), got)
	}
	codes := map[string]bool{}
	for _, it := range got {
		codes[it.Code] = true
	}
	if !codes["5.02"] || !codes["1.03"] {
		t.Fatalf("want 5.02 + 1.03 kept, got %v", codes)
	}
	// Only routine codes → nil (the event is dropped from the feed).
	if got := NotableItems([]edgar.EventItem{{Code: "2.02"}, {Code: "7.01"}, {Code: "8.01"}, {Code: "9.01"}}); got != nil {
		t.Fatalf("routine-only items should yield nil, got %+v", got)
	}
	if got := NotableItems(nil); got != nil {
		t.Fatalf("nil items → nil, got %+v", got)
	}
}
