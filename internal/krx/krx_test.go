package krx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// sampleKOSPI is a real-shaped stk_bydd_trd response (Samsung + a halted row).
const sampleKOSPI = `{"OutBlock_1":[
  {"ISU_CD":"005930","ISU_NM":"삼성전자","MKT_NM":"KOSPI","TDD_CLSPRC":"71,500","CMPPREVDD_PRC":"-300","TDD_OPNPRC":"71,800"},
  {"ISU_CD":"000000","ISU_NM":"정지","TDD_CLSPRC":"","CMPPREVDD_PRC":""}
]}`

func testClient(t *testing.T, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("AUTH_KEY") == "" {
			t.Error("missing AUTH_KEY header")
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := New("test-key")
	c.http = srv.Client()
	c.base = srv.URL
	return c
}

func TestDisabledWithoutKey(t *testing.T) {
	q, err := New("").EODQuotes(context.Background())
	if err != nil || q != nil {
		t.Fatalf("disabled client should return (nil,nil), got (%v,%v)", q, err)
	}
}

func TestEODQuotes(t *testing.T) {
	// The same body answers both boards; we only assert the KOSPI mapping.
	q, err := testClient(t, sampleKOSPI).EODQuotes(context.Background())
	if err != nil {
		t.Fatalf("EODQuotes: %v", err)
	}
	s, ok := q["005930.KS"]
	if !ok {
		t.Fatalf("missing 005930.KS: %v", q)
	}
	if s.Price != 71500 { // comma-stripped
		t.Errorf("price=%v want 71500", s.Price)
	}
	if s.PrevClose != 71800 { // 71500 - (-300)
		t.Errorf("prev_close=%v want 71800", s.PrevClose)
	}
	if s.Source != "krx" || s.Session != "closed" {
		t.Errorf("source/session = %q/%q", s.Source, s.Session)
	}
	// The empty-priced halted row is dropped (KOSPI=1 + same body for KOSDAQ as
	// .KQ=1) → 2 distinct tickers (005930.KS + 005930.KQ).
	if len(q) != 2 {
		t.Errorf("quotes=%d want 2 (KS+KQ from the shared fixture)", len(q))
	}
}

func TestCompanies(t *testing.T) {
	cos, err := testClient(t, sampleKOSPI).Companies(context.Background())
	if err != nil {
		t.Fatalf("Companies: %v", err)
	}
	if len(cos) == 0 || cos[0].Country != "KR" {
		t.Fatalf("companies = %+v", cos)
	}
}
