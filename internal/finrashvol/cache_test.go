package finrashvol

import (
	"sync"
	"testing"
)

func sv(sym, date string, short, total int64) ShortVol {
	return ShortVol{
		Symbol:      sym,
		ShortVolume: short,
		TotalVolume: total,
		ShortPct:    shortPct(short, total),
		Date:        date,
	}
}

func TestCacheLatestAndAsOf(t *testing.T) {
	c := NewCache()
	if c.AsOf() != "" || c.Len() != 0 {
		t.Fatalf("empty cache: AsOf=%q Len=%d, want \"\" 0", c.AsOf(), c.Len())
	}
	if _, ok := c.Latest("AAPL"); ok {
		t.Fatal("Latest on empty cache should report not-present")
	}

	c.Set([]ShortVol{
		sv("AAPL", "2026-06-04", 100, 200),
		sv("GME", "2026-06-04", 600, 1000),
	})
	if c.AsOf() != "2026-06-04" {
		t.Fatalf("AsOf = %q, want 2026-06-04", c.AsOf())
	}
	if c.Len() != 2 {
		t.Fatalf("Len = %d, want 2", c.Len())
	}
	got, ok := c.Latest("AAPL")
	if !ok || got.ShortVolume != 100 || !approx(got.ShortPct, 50) {
		t.Fatalf("Latest AAPL = %+v ok=%v", got, ok)
	}
}

func TestCacheHistoryRolls(t *testing.T) {
	c := NewCache()
	c.Set([]ShortVol{sv("AAPL", "2026-06-03", 10, 100)})
	c.Set([]ShortVol{sv("AAPL", "2026-06-04", 20, 100)})
	c.Set([]ShortVol{sv("AAPL", "2026-06-05", 30, 100)})

	h := c.History("AAPL")
	if len(h) != 3 {
		t.Fatalf("history len = %d, want 3", len(h))
	}
	// Oldest first.
	if h[0].Date != "2026-06-03" || h[2].Date != "2026-06-05" {
		t.Fatalf("history order = %v..%v, want 06-03..06-05", h[0].Date, h[2].Date)
	}
	if c.AsOf() != "2026-06-05" {
		t.Fatalf("AsOf = %q, want 2026-06-05", c.AsOf())
	}
	if c.History("UNKNOWN") != nil {
		t.Fatal("History for unseen symbol should be nil")
	}
}

func TestCacheHistoryTrimsToWindow(t *testing.T) {
	c := NewCacheWithHistory(3)
	dates := []string{"2026-06-01", "2026-06-02", "2026-06-03", "2026-06-04", "2026-06-05"}
	for i, d := range dates {
		c.Set([]ShortVol{sv("AAPL", d, int64(i+1), 100)})
	}
	h := c.History("AAPL")
	if len(h) != 3 {
		t.Fatalf("history len = %d, want 3 (trimmed)", len(h))
	}
	if h[0].Date != "2026-06-03" || h[2].Date != "2026-06-05" {
		t.Fatalf("trimmed window = %v..%v, want 06-03..06-05", h[0].Date, h[2].Date)
	}
}

func TestCacheReingestSameDayIdempotent(t *testing.T) {
	c := NewCache()
	c.Set([]ShortVol{sv("AAPL", "2026-06-05", 10, 100)})
	c.Set([]ShortVol{sv("AAPL", "2026-06-05", 30, 100)}) // same date, revised
	h := c.History("AAPL")
	if len(h) != 1 {
		t.Fatalf("history len = %d, want 1 (same day must not duplicate)", len(h))
	}
	if h[0].ShortVolume != 30 {
		t.Fatalf("history value = %d, want 30 (latest wins)", h[0].ShortVolume)
	}
	if got, _ := c.Latest("AAPL"); got.ShortVolume != 30 {
		t.Fatalf("Latest = %d, want 30", got.ShortVolume)
	}
}

func TestCacheTopRanksAndFilters(t *testing.T) {
	c := NewCache()
	c.Set([]ShortVol{
		sv("HI", "2026-06-05", 800, 1000),  // 80%
		sv("MID", "2026-06-05", 500, 1000), // 50%
		sv("LO", "2026-06-05", 100, 1000),  // 10%
		sv("THIN", "2026-06-05", 90, 100),  // 90% but tiny volume
	})
	// minTotalVolume filters out THIN.
	top := c.Top(2, 1000)
	if len(top) != 2 {
		t.Fatalf("Top len = %d, want 2", len(top))
	}
	if top[0].Symbol != "HI" || top[1].Symbol != "MID" {
		t.Fatalf("Top order = %s,%s, want HI,MID", top[0].Symbol, top[1].Symbol)
	}
	for _, r := range c.Top(0, 1000) {
		if r.Symbol == "THIN" {
			t.Fatal("THIN should be filtered by minTotalVolume")
		}
	}
}

// TestCacheConcurrentReadWrite exercises the atomic snapshot under -race:
// concurrent Sets and reads must never tear.
func TestCacheConcurrentReadWrite(t *testing.T) {
	c := NewCache()
	c.Set([]ShortVol{sv("AAPL", "2026-06-04", 1, 100)})

	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				c.Set([]ShortVol{sv("AAPL", "2026-06-05", int64(i), 100)})
			}
		}(w)
	}
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				_, _ = c.Latest("AAPL")
				_ = c.History("AAPL")
				_ = c.AsOf()
				_ = c.Top(5, 0)
			}
		}()
	}
	wg.Wait()
}
