package finrashvol

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// sampleFile mirrors a real CNMSshvol file: a header row, data rows (one thin
// name with TotalVolume 0, one with a zero-padded date), a blank line, and the
// trailing "Total" summary row that must be skipped.
const sampleFile = `Date|Symbol|ShortVolume|ShortExemptVolume|TotalVolume|Market
20260605|AAPL|24250000|10000|50000000|Q
20260605|GME|6130000|5000|10000000|N
20260605|MSTR|4030000|0|10000000|Q
20260605|THIN|0|0|0|N

20260605|Total|999999999|0|1234567890|`

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := New()
	c.base = srv.URL + "/" // FetchDaily appends the file name
	return c
}

func TestFetchDailyParse(t *testing.T) {
	wantPath := "/" + FileName(time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC))
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath {
			t.Errorf("requested path = %q, want %q", r.URL.Path, wantPath)
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(sampleFile))
	})

	rows, err := c.FetchDaily(context.Background(), time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("FetchDaily: %v", err)
	}
	// Header skipped, blank line skipped, Total row skipped → 4 data rows.
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4 (header/blank/Total skipped)", len(rows))
	}

	byNeedle := map[string]ShortVol{}
	for _, r := range rows {
		byNeedle[r.Symbol] = r
		if r.Symbol == "Total" {
			t.Fatalf("Total summary row was not skipped: %+v", r)
		}
	}

	aapl := byNeedle["AAPL"]
	if aapl.ShortVolume != 24250000 || aapl.TotalVolume != 50000000 {
		t.Fatalf("AAPL volumes = %+v", aapl)
	}
	if aapl.Date != "2026-06-05" {
		t.Fatalf("AAPL date = %q, want 2026-06-05", aapl.Date)
	}
	if !approx(aapl.ShortPct, 48.5) {
		t.Fatalf("AAPL ShortPct = %v, want ~48.5", aapl.ShortPct)
	}
	if gme := byNeedle["GME"]; !approx(gme.ShortPct, 61.3) {
		t.Fatalf("GME ShortPct = %v, want ~61.3", gme.ShortPct)
	}
	if mstr := byNeedle["MSTR"]; !approx(mstr.ShortPct, 40.3) {
		t.Fatalf("MSTR ShortPct = %v, want ~40.3", mstr.ShortPct)
	}
	// TotalVolume 0 → ShortPct must be 0, not NaN/Inf.
	if thin := byNeedle["THIN"]; thin.ShortPct != 0 {
		t.Fatalf("THIN ShortPct = %v, want 0 for zero total volume", thin.ShortPct)
	}
}

func TestFetchDailyNoData(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"404", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "nope", http.StatusNotFound) }},
		{"403", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", http.StatusForbidden) }},
		{"empty body", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("")) }},
		{"header only", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("Date|Symbol|ShortVolume|ShortExemptVolume|TotalVolume|Market\n"))
		}},
		{"only total row", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("20260605|Total|1|0|2|\n"))
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, tc.handler)
			_, err := c.FetchDaily(context.Background(), time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC))
			if !errors.Is(err, ErrNoData) {
				t.Fatalf("err = %v, want ErrNoData", err)
			}
		})
	}
}

func TestFetchDailyServerError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	_, err := c.FetchDaily(context.Background(), time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected an error on 500")
	}
	if errors.Is(err, ErrNoData) {
		t.Fatalf("500 should NOT be ErrNoData, got %v", err)
	}
}

func TestParseFileColumnOrderFromHeader(t *testing.T) {
	// Columns deliberately reordered; the parser must map by header name.
	reordered := "Symbol|TotalVolume|ShortVolume|Date\n" +
		"NVDA|100|34|20260605\n"
	rows, err := parseFile([]byte(reordered))
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	r := rows[0]
	if r.Symbol != "NVDA" || r.ShortVolume != 34 || r.TotalVolume != 100 {
		t.Fatalf("reordered row mismapped: %+v", r)
	}
	if !approx(r.ShortPct, 34) {
		t.Fatalf("ShortPct = %v, want 34", r.ShortPct)
	}
	if r.Date != "2026-06-05" {
		t.Fatalf("Date = %q, want 2026-06-05", r.Date)
	}
}

func TestParseFileSkipsMalformed(t *testing.T) {
	body := "Date|Symbol|ShortVolume|ShortExemptVolume|TotalVolume|Market\n" +
		"20260605|GOOD|10|0|100|Q\n" +
		"20260605|BADNUM|notanumber|0|100|Q\n" + // non-numeric short → skipped
		"20260605|SHORT|5\n" + // too few columns → skipped
		"\n"
	rows, err := parseFile([]byte(body))
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	if len(rows) != 1 || rows[0].Symbol != "GOOD" {
		t.Fatalf("rows = %+v, want only GOOD", rows)
	}
}

func TestFileName(t *testing.T) {
	got := FileName(time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC))
	if got != "CNMSshvol20260605.txt" {
		t.Fatalf("FileName = %q, want CNMSshvol20260605.txt", got)
	}
}

func approx(a, b float64) bool { return math.Abs(a-b) < 0.1 }
