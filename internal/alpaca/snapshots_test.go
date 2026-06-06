package alpaca

import (
	"context"
	"net/http"
	"net/http/httptest"
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
