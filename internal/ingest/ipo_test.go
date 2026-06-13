package ingest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/nasdaq"
)

// fakeIPOSource returns a scripted result/error per call, so we can drive the
// ingestor's keep-previous-on-failure behaviour deterministically.
type fakeIPOSource struct {
	calls int
	cal   nasdaq.Calendar
	err   error
}

func (f *fakeIPOSource) Calendar(context.Context, time.Time) (nasdaq.Calendar, error) {
	f.calls++
	return f.cal, f.err
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestIPOIngestorRefreshSuccess(t *testing.T) {
	src := &fakeIPOSource{cal: nasdaq.Calendar{
		Priced: []nasdaq.IPO{{Ticker: "FRBT", Company: "Forbright", Kind: nasdaq.KindPriced}},
	}}
	ing := NewIPOIngestor(src, time.Hour, quietLogger())
	ing.refresh(context.Background())

	cal, at := ing.Calendar()
	if len(cal.Priced) != 1 || cal.Priced[0].Ticker != "FRBT" {
		t.Fatalf("calendar = %+v", cal)
	}
	if at.IsZero() {
		t.Fatalf("updatedAt should be set after a successful refresh")
	}
}

func TestIPOIngestorKeepsPreviousOnError(t *testing.T) {
	src := &fakeIPOSource{cal: nasdaq.Calendar{
		Priced: []nasdaq.IPO{{Ticker: "FRBT", Kind: nasdaq.KindPriced}},
	}}
	ing := NewIPOIngestor(src, time.Hour, quietLogger())
	ing.refresh(context.Background()) // first: success
	cal0, at0 := ing.Calendar()

	// Next fetch fails (e.g. the datacenter-IP block) — the snapshot must stand.
	src.cal = nasdaq.Calendar{}
	src.err = errors.New("empty body (proxy?)")
	ing.refresh(context.Background())

	cal1, at1 := ing.Calendar()
	if len(cal1.Priced) != 1 || cal1.Priced[0].Ticker != "FRBT" {
		t.Fatalf("failed refresh blanked the board: %+v", cal1)
	}
	if !at1.Equal(at0) {
		t.Fatalf("updatedAt changed on a failed refresh: %v != %v", at1, at0)
	}
	_ = cal0
}

func TestIPOIngestorEmptyBeforeFirstRefresh(t *testing.T) {
	ing := NewIPOIngestor(&fakeIPOSource{}, time.Hour, quietLogger())
	cal, at := ing.Calendar()
	if cal.Priced != nil || cal.Upcoming != nil || cal.Filed != nil {
		t.Fatalf("expected nil slices before first refresh, got %+v", cal)
	}
	if !at.IsZero() {
		t.Fatalf("expected zero updatedAt before first refresh, got %v", at)
	}
}
