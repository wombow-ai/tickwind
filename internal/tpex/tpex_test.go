package tpex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// sampleOTC is a real-shaped tpex_mainboard_daily_close_quotes slice. Note the
// trailing space in "Change" and the "--" halted row.
const sampleOTC = `[
  {"Date":"1150605","SecuritiesCompanyCode":"006201","CompanyName":"元大富櫃50","Close":"48.12","Change":"-1.72 ","Open":"49.84"},
  {"Date":"1150605","SecuritiesCompanyCode":"0000","CompanyName":"暫停","Close":"--","Change":"--"}
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
	q, err := testClient(t, sampleOTC).EODQuotes(context.Background())
	if err != nil {
		t.Fatalf("EODQuotes: %v", err)
	}
	if len(q) != 1 { // halted "--" dropped
		t.Fatalf("quotes=%d want 1", len(q))
	}
	x, ok := q["006201.TWO"]
	if !ok {
		t.Fatalf("missing 006201.TWO: %v", q)
	}
	if x.Price != 48.12 {
		t.Errorf("price=%v want 48.12", x.Price)
	}
	if d := x.PrevClose - 49.84; d < -1e-9 || d > 1e-9 { // 48.12 - (-1.72) = 49.84
		t.Errorf("prev_close=%v want 49.84", x.PrevClose)
	}
	if x.Source != "tpex" || x.Session != "closed" {
		t.Errorf("source/session = %q/%q", x.Source, x.Session)
	}
}

func TestCompanies(t *testing.T) {
	cos, err := testClient(t, sampleOTC).Companies(context.Background())
	if err != nil {
		t.Fatalf("Companies: %v", err)
	}
	if len(cos) != 2 || cos[0].Ticker != "006201.TWO" || cos[0].Exchange != "TPEx" {
		t.Fatalf("companies = %+v", cos)
	}
}
