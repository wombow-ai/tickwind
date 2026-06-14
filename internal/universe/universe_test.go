package universe

import (
	"reflect"
	"testing"

	"github.com/wombow-ai/tickwind/internal/store"
)

func TestTickers(t *testing.T) {
	// An unswept cache returns a non-nil empty slice (never nil).
	c := NewCache()
	if got := c.Tickers(); got == nil || len(got) != 0 {
		t.Fatalf("unswept Tickers() = %#v, want non-nil empty slice", got)
	}

	// After a sweep, Tickers returns the sorted snapshot keys — exactly the
	// quote-bearing set the ingestor stored (it only keeps price>0 quotes), so
	// it matches Len and the screener universe.
	c.Set(map[string]store.Quote{
		"MSFT":  {Ticker: "MSFT", Price: 400},
		"AAPL":  {Ticker: "AAPL", Price: 200},
		"BRK.B": {Ticker: "BRK.B", Price: 600},
	})
	got := c.Tickers()
	want := []string{"AAPL", "BRK.B", "MSFT"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Tickers() = %v, want sorted %v", got, want)
	}
	if len(got) != c.Len() {
		t.Fatalf("len(Tickers())=%d != Len()=%d — must cover the same universe", len(got), c.Len())
	}
}
