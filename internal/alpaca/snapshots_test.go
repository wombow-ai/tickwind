package alpaca

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestSnapshots(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("APCA-API-KEY-ID") == "" {
			t.Error("missing Alpaca key header")
		}
		// AAPL: normal daily close; TINY: daily empty → prevDailyBar fallback;
		// DEAD: no price anywhere → omitted.
		_, _ = w.Write([]byte(`{
			"AAPL":{"dailyBar":{"c":307.23},"prevDailyBar":{"c":311.21}},
			"TINY":{"dailyBar":{"c":0},"prevDailyBar":{"c":4.50}},
			"DEAD":{"dailyBar":{"c":0},"prevDailyBar":{"c":0},"latestTrade":{"p":0}}
		}`))
	}))
	defer srv.Close()

	c := New("k", "s", srv.URL, "iex")
	m, err := c.Snapshots(context.Background(), []string{"AAPL", "TINY", "DEAD"})
	if err != nil {
		t.Fatalf("Snapshots: %v", err)
	}
	if m["AAPL"] != 307.23 {
		t.Errorf("AAPL = %v want 307.23", m["AAPL"])
	}
	if m["TINY"] != 4.50 {
		t.Errorf("TINY = %v want 4.50 (prevDailyBar fallback)", m["TINY"])
	}
	if _, ok := m["DEAD"]; ok {
		t.Error("DEAD has no usable price and should be omitted")
	}
}

func TestNormalizeSymbol(t *testing.T) {
	tests := []struct{ in, want string }{
		{"AAPL", "AAPL"},       // plain ticker unchanged
		{"BRK-B", "BRK.B"},     // SEC class share -> Alpaca dot form
		{"BF-A", "BF.A"},       // class A
		{"BAC-PK", "BAC.PK"},   // preferred series
		{"BRK.B", "BRK.B"},     // already dotted -> unchanged
		{"0700.HK", "0700.HK"}, // foreign suffix untouched
		{"-LEAD", "-LEAD"},     // leading hyphen -> not a class suffix, untouched
		{"TRAIL-", "TRAIL-"},   // trailing hyphen -> untouched
		{"A-B-C", "A-B.C"},     // only the LAST hyphen becomes a dot
	}
	for _, tc := range tests {
		if got := NormalizeSymbol(tc.in); got != tc.want {
			t.Errorf("NormalizeSymbol(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestSnapshotQuotesPoisonBatch is the regression test for the missing-mega-caps
// bug: a batch containing one symbol Alpaca rejects (BRK-B, SEC hyphen form) used
// to 400 the whole request and silently drop every other symbol in it — and the
// SEC directory front-loads the mega-caps into that same first batch. The fetch
// must now (a) normalize BRK-B -> BRK.B (which Alpaca accepts) and (b) bisect on a
// 400 so a genuinely-invalid symbol can't take the batch down with it.
func TestSnapshotQuotesPoisonBatch(t *testing.T) {
	// Server: 400s any request containing the invalid "ZZZZ", else prices the
	// known good symbols. This proves the bisection — without it, the first
	// request (AAPL,MSFT,ZZZZ,...) 400s and drops AAPL+MSFT.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		syms := r.URL.Query().Get("symbols")
		if strings.Contains(syms, "ZZZZ") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"code=400, message=invalid symbol: ZZZZ"}`))
			return
		}
		prices := map[string]float64{"AAPL": 291.08, "MSFT": 390.67, "BRK.B": 600.5}
		var b strings.Builder
		b.WriteByte('{')
		first := true
		for _, s := range strings.Split(syms, ",") {
			p, ok := prices[s]
			if !ok {
				continue
			}
			if !first {
				b.WriteByte(',')
			}
			first = false
			b.WriteString(`"` + s + `":{"dailyBar":{"c":`)
			b.WriteString(strconv.FormatFloat(p, 'f', -1, 64))
			b.WriteString(`},"prevDailyBar":{"c":1}}`)
		}
		b.WriteByte('}')
		_, _ = w.Write([]byte(b.String()))
	}))
	defer srv.Close()

	c := New("k", "s", srv.URL, "iex")
	// AAPL/MSFT are the mega-caps; BRK-B is the SEC class-share form that must be
	// normalized; ZZZZ is the poison symbol that must NOT drop the others.
	q, err := c.SnapshotQuotes(context.Background(), []string{"AAPL", "MSFT", "BRK-B", "ZZZZ"})
	if err != nil {
		t.Fatalf("SnapshotQuotes: %v", err)
	}
	for _, sym := range []string{"AAPL", "MSFT"} {
		if _, ok := q[sym]; !ok {
			t.Errorf("%s missing — a poison symbol dropped a mega-cap (the bug)", sym)
		}
	}
	// BRK-B must be priced under its normalized canonical key BRK.B (matching the
	// rest of the app), never the SEC hyphen form.
	if _, ok := q["BRK.B"]; !ok {
		t.Error("BRK.B missing — class-share symbol was not normalized to the dot form")
	}
	if _, ok := q["BRK-B"]; ok {
		t.Error("quote keyed by the SEC hyphen form BRK-B; want canonical BRK.B")
	}
	// ZZZZ is genuinely invalid and is correctly dropped.
	if _, ok := q["ZZZZ"]; ok {
		t.Error("ZZZZ is invalid and should be dropped")
	}
}
