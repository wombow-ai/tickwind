package ingest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/finrashvol"
)

// fakeShortVolFetcher serves canned rows per requested date; dates absent from
// the map return ErrNoData (an unpublished/weekend day).
type fakeShortVolFetcher struct {
	byDate map[string][]finrashvol.ShortVol
	calls  []string // dates requested, in order
}

func (f *fakeShortVolFetcher) FetchDaily(_ context.Context, date time.Time) ([]finrashvol.ShortVol, error) {
	d := date.Format("2006-01-02")
	f.calls = append(f.calls, d)
	rows, ok := f.byDate[d]
	if !ok {
		return nil, fmt.Errorf("%w: %s", finrashvol.ErrNoData, d)
	}
	return rows, nil
}

func sv(sym, date string, short, total int64) finrashvol.ShortVol {
	pct := 0.0
	if total > 0 {
		pct = float64(short) / float64(total) * 100
	}
	return finrashvol.ShortVol{Symbol: sym, ShortVolume: short, TotalVolume: total, ShortPct: pct, Date: date}
}

func TestShortVolumeSweepFallsBackOverWeekend(t *testing.T) {
	// "Now" is Sunday 2026-06-14. The previous trading day with data is Friday
	// 2026-06-12; Saturday/Sunday are skipped before any request.
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	f := &fakeShortVolFetcher{byDate: map[string][]finrashvol.ShortVol{
		"2026-06-12": {sv("GME", "2026-06-12", 600, 1000), sv("AAPL", "2026-06-12", 100, 400)},
	}}
	cache := finrashvol.NewCache()
	ing := NewShortVolumeIngestor(f, cache, time.Hour, nil)
	ing.now = func() time.Time { return now }

	ing.sweep(context.Background())

	if cache.AsOf() != "2026-06-12" {
		t.Fatalf("AsOf = %q, want 2026-06-12 (Friday)", cache.AsOf())
	}
	// Weekend days must never be requested (skipped before the call).
	for _, c := range f.calls {
		if c == "2026-06-13" || c == "2026-06-14" {
			t.Fatalf("requested a weekend day %q; calls=%v", c, f.calls)
		}
	}
	if got, ok := cache.Latest("GME"); !ok || got.ShortVolume != 600 {
		t.Fatalf("Latest GME = %+v ok=%v", got, ok)
	}
}

func TestShortVolumeSweepWalksBackBusinessDays(t *testing.T) {
	// Tuesday 2026-06-16; Tue+Mon unpublished, data on Friday 2026-06-12.
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	f := &fakeShortVolFetcher{byDate: map[string][]finrashvol.ShortVol{
		"2026-06-12": {sv("TSLA", "2026-06-12", 300, 500)},
	}}
	cache := finrashvol.NewCache()
	ing := NewShortVolumeIngestor(f, cache, time.Hour, nil)
	ing.now = func() time.Time { return now }

	ing.sweep(context.Background())

	if cache.AsOf() != "2026-06-12" {
		t.Fatalf("AsOf = %q, want 2026-06-12; calls=%v", cache.AsOf(), f.calls)
	}
	want := []string{"2026-06-16", "2026-06-15", "2026-06-12"} // Tue, Mon, (skip weekend) Fri
	if len(f.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", f.calls, want)
	}
	for i, c := range f.calls {
		if c != want[i] {
			t.Fatalf("call[%d] = %q, want %q (calls=%v)", i, c, want[i], f.calls)
		}
	}
}

func TestShortVolumeSweepNoDataKeepsPrevious(t *testing.T) {
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC) // Tuesday
	cache := finrashvol.NewCache()
	cache.Set([]finrashvol.ShortVol{sv("NVDA", "2026-06-05", 50, 100)})

	// No date has data → every attempt is ErrNoData; the previous snapshot stays.
	f := &fakeShortVolFetcher{byDate: map[string][]finrashvol.ShortVol{}}
	ing := NewShortVolumeIngestor(f, cache, time.Hour, nil)
	ing.now = func() time.Time { return now }

	ing.sweep(context.Background())

	if cache.AsOf() != "2026-06-05" {
		t.Fatalf("AsOf = %q, want previous 2026-06-05 retained", cache.AsOf())
	}
	if len(f.calls) != shortVolumeMaxFallback {
		t.Fatalf("attempts = %d, want the full %d-day lookback", len(f.calls), shortVolumeMaxFallback)
	}
}

func TestPreviousTradingDay(t *testing.T) {
	sat := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC) // Saturday
	if got := previousTradingDay(sat, true); got.Format("2006-01-02") != "2026-06-12" {
		t.Fatalf("Saturday clamped = %s, want Friday 2026-06-12", got.Format("2006-01-02"))
	}
	mon := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC) // Monday
	if got := previousTradingDay(mon, false); got.Format("2006-01-02") != "2026-06-12" {
		t.Fatalf("step back from Monday = %s, want Friday 2026-06-12", got.Format("2006-01-02"))
	}
}
