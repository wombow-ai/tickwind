package ingest

import (
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

func TestOverlayConsolidated(t *testing.T) {
	old := time.Date(2026, 5, 27, 20, 57, 0, 0, time.UTC)
	fresh := time.Date(2026, 6, 10, 13, 0, 0, 0, time.UTC)

	// Stale IEX quote with a RegularClose + an inconsistent (stale/sparse) prev
	// bar, extended hours: price/at/source/session overlaid, RegularClose kept,
	// prev_close ANCHORED to RegularClose so there's no phantom day-change — the
	// 1.36-vs-0.7049 pairing would otherwise headline ~+93% (the HOTH bug).
	q := store.Quote{Ticker: "HOTH", Price: 1.48, PrevClose: 0.7049, RegularClose: 1.36, Session: "post", Source: "alpaca", At: old}
	got := overlayConsolidated(q, 1.52, 1.41, fresh, "pre")
	if got.Price != 1.52 || got.Source != "finnhub" || got.Session != "pre" || !got.At.Equal(fresh) {
		t.Errorf("overlay = %+v", got)
	}
	if got.RegularClose != 1.36 || got.PrevClose != 1.36 {
		t.Errorf("extended: want prev==reg==1.36 (no phantom change), got prev=%v reg=%v", got.PrevClose, got.RegularClose)
	}

	// Missing baselines, extended → RegularClose filled from the consolidated
	// prev close, prev_close anchored to it (day-change 0).
	empty := store.Quote{Ticker: "XYZ"}
	got = overlayConsolidated(empty, 10, 9.5, fresh, "pre")
	if got.PrevClose != 9.5 || got.RegularClose != 9.5 {
		t.Errorf("baselines not filled: %+v", got)
	}

	// Regular session → regular close tracks the live price; prev_close from the
	// same-source consolidated prev close, so the day-change is real.
	got = overlayConsolidated(empty, 10, 9.5, fresh, "regular")
	if got.RegularClose != 10 || got.PrevClose != 9.5 {
		t.Errorf("regular session: want reg=10 prev=9.5, got reg=%v prev=%v", got.RegularClose, got.PrevClose)
	}
}
