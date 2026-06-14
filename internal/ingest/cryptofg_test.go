package ingest

import (
	"context"
	"errors"
	"testing"

	"github.com/wombow-ai/tickwind/internal/cryptofg"
)

// fakeCryptoFG returns a canned snapshot or an error, recording call count.
type fakeCryptoFG struct {
	idx   cryptofg.Index
	err   error
	calls int
}

func (f *fakeCryptoFG) Latest(_ context.Context) (cryptofg.Index, error) {
	f.calls++
	return f.idx, f.err
}

func TestCryptoFGIngestorRefreshInstallsSnapshot(t *testing.T) {
	cache := cryptofg.NewCache()
	src := &fakeCryptoFG{idx: cryptofg.Index{
		Score: 63, Label: "Greed", AsOf: "2026-06-14",
		BTC: cryptofg.Price{USD: 64413, Change24h: 1.01, Present: true},
	}}
	ing := NewCryptoFGIngestor(src, cache, 0, quietLog())
	if ing.Cache() != cache {
		t.Fatal("Cache() should expose the injected cache")
	}
	ing.refresh(context.Background())
	got, ok := cache.Latest()
	if !ok || got.Score != 63 || got.Label != "Greed" || !got.BTC.Present {
		t.Fatalf("cache = %+v ok=%v, want the fetched snapshot", got, ok)
	}
}

func TestCryptoFGIngestorRefreshKeepsLastGoodOnError(t *testing.T) {
	cache := cryptofg.NewCache()
	good := cryptofg.Index{Score: 50, Label: "Neutral", AsOf: "2026-06-13"}
	cache.Set(good) // pretend a prior successful refresh

	src := &fakeCryptoFG{err: errors.New("alternative.me down")}
	ing := NewCryptoFGIngestor(src, cache, 0, quietLog())
	ing.refresh(context.Background())

	if src.calls != 1 {
		t.Fatalf("Latest called %d times, want 1", src.calls)
	}
	got, ok := cache.Latest()
	if !ok || got.Score != 50 || got.Label != "Neutral" {
		t.Fatalf("cache = %+v ok=%v, want the last-good snapshot retained on error", got, ok)
	}
}
