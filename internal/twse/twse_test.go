package twse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// sampleSTOCKDAYALL is a real-shaped STOCK_DAY_ALL slice (TSMC + a halted row).
const sampleSTOCKDAYALL = `[
  {"Date":"1150605","Code":"2330","Name":"台積電","TradeVolume":"43403895","ClosingPrice":"2365.00","Change":"-20.0000","OpeningPrice":"2395.00","HighestPrice":"2405.00","LowestPrice":"2350.00"},
  {"Date":"1150605","Code":"9999","Name":"暫停交易","ClosingPrice":"--","Change":"--"}
]`

func testClient(t *testing.T, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &Client{http: srv.Client(), base: srv.URL}
}

func TestEODQuotes(t *testing.T) {
	q, err := testClient(t, sampleSTOCKDAYALL).EODQuotes(context.Background())
	if err != nil {
		t.Fatalf("EODQuotes: %v", err)
	}
	if len(q) != 1 { // the halted "--" row is dropped
		t.Fatalf("quotes=%d want 1", len(q))
	}
	tsmc, ok := q["2330.TW"]
	if !ok {
		t.Fatalf("missing 2330.TW: %v", q)
	}
	if tsmc.Price != 2365 {
		t.Errorf("price=%v want 2365", tsmc.Price)
	}
	if tsmc.PrevClose != 2385 { // 2365 - (-20)
		t.Errorf("prev_close=%v want 2385", tsmc.PrevClose)
	}
	if tsmc.Session != "closed" || tsmc.Source != "twse" {
		t.Errorf("session/source = %q/%q", tsmc.Session, tsmc.Source)
	}
	if !tsmc.At.Equal(time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)) { // ROC 1150605
		t.Errorf("at=%v want 2026-06-05", tsmc.At)
	}
}

func TestCompanies(t *testing.T) {
	cos, err := testClient(t, sampleSTOCKDAYALL).Companies(context.Background())
	if err != nil {
		t.Fatalf("Companies: %v", err)
	}
	if len(cos) != 2 || cos[0].Ticker != "2330.TW" || cos[0].Country != "TW" {
		t.Fatalf("companies = %+v", cos)
	}
}

func TestRocDate(t *testing.T) {
	if got := rocDate("1150605"); !got.Equal(time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("rocDate = %v", got)
	}
	if got := rocDate("bad"); !got.IsZero() {
		t.Errorf("rocDate(bad) = %v want zero", got)
	}
}
