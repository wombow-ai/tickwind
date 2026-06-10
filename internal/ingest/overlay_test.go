package ingest

import (
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

func TestOverlayConsolidated(t *testing.T) {
	old := time.Date(2026, 5, 27, 20, 57, 0, 0, time.UTC)
	fresh := time.Date(2026, 6, 10, 13, 0, 0, 0, time.UTC)

	// Stale IEX quote with good baselines → price/at/source/session overlaid,
	// baselines kept.
	q := store.Quote{Ticker: "HOTH", Price: 1.48, PrevClose: 0.7049, RegularClose: 1.36, Session: "post", Source: "alpaca", At: old}
	got := overlayConsolidated(q, 1.52, 1.41, fresh, "pre")
	if got.Price != 1.52 || got.Source != "finnhub" || got.Session != "pre" || !got.At.Equal(fresh) {
		t.Errorf("overlay = %+v", got)
	}
	if got.PrevClose != 0.7049 || got.RegularClose != 1.36 {
		t.Errorf("baselines clobbered: prev=%v reg=%v", got.PrevClose, got.RegularClose)
	}

	// Missing baselines → filled from the consolidated prev close.
	empty := store.Quote{Ticker: "XYZ"}
	got = overlayConsolidated(empty, 10, 9.5, fresh, "pre")
	if got.PrevClose != 9.5 || got.RegularClose != 9.5 {
		t.Errorf("baselines not filled: %+v", got)
	}

	// Regular session → regular close tracks the live price.
	got = overlayConsolidated(empty, 10, 9.5, fresh, "regular")
	if got.RegularClose != 10 {
		t.Errorf("regular session regular_close = %v, want 10", got.RegularClose)
	}
}
