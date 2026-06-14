package ingest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/wombow-ai/tickwind/internal/treasury"
)

// fakeTreasury returns a canned curve or an error, recording call count.
type fakeTreasury struct {
	curve treasury.Curve
	err   error
	calls int
}

func (f *fakeTreasury) Latest(_ context.Context) (treasury.Curve, error) {
	f.calls++
	return f.curve, f.err
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestMacroIngestorRefreshInstallsCurve(t *testing.T) {
	cache := treasury.NewCache()
	src := &fakeTreasury{curve: treasury.Curve{
		Date:        "2026-06-12",
		Yields:      []treasury.Yield{{Tenor: "2Y", Rate: 4.09}, {Tenor: "10Y", Rate: 4.48}},
		Spread2s10s: 0.39,
		HasSpread:   true,
	}}
	ing := NewMacroIngestor(src, cache, 0, quietLog())
	if ing.Cache() != cache {
		t.Fatal("Cache() should expose the injected cache")
	}
	ing.refresh(context.Background())
	got, ok := cache.Latest()
	if !ok || got.Date != "2026-06-12" || got.Spread2s10s != 0.39 {
		t.Fatalf("cache = %+v ok=%v, want the fetched curve", got, ok)
	}
}

func TestMacroIngestorRefreshKeepsLastGoodOnError(t *testing.T) {
	cache := treasury.NewCache()
	good := treasury.Curve{Date: "2026-06-12", Yields: []treasury.Yield{{Tenor: "2Y", Rate: 4.09}}, HasSpread: false}
	cache.Set(good) // pretend a prior successful refresh

	src := &fakeTreasury{err: errors.New("treasury down")}
	ing := NewMacroIngestor(src, cache, 0, quietLog())
	ing.refresh(context.Background())

	if src.calls != 1 {
		t.Fatalf("Latest called %d times, want 1", src.calls)
	}
	got, ok := cache.Latest()
	if !ok || got.Date != "2026-06-12" {
		t.Fatalf("cache = %+v ok=%v, want the last-good curve retained on error", got, ok)
	}
}
