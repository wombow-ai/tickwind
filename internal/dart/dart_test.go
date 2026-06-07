package dart

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDisabledWithoutKey(t *testing.T) {
	m, err := New("").CorpCodeMap(context.Background())
	if err != nil || m != nil {
		t.Fatalf("disabled CorpCodeMap = (%v,%v), want (nil,nil)", m, err)
	}
	f, err := New("").RecentFilings(context.Background(), "005930.KS", "00126380", 10)
	if err != nil || f != nil {
		t.Fatalf("disabled RecentFilings = (%v,%v), want (nil,nil)", f, err)
	}
}

func zipped(t *testing.T, xml string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("CORPCODE.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(xml)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCorpCodeMap(t *testing.T) {
	const xml = `<?xml version="1.0" encoding="UTF-8"?><result>
	  <list><corp_code>00126380</corp_code><corp_name>삼성전자</corp_name><stock_code>005930</stock_code></list>
	  <list><corp_code>00999999</corp_code><corp_name>비상장</corp_name><stock_code> </stock_code></list>
	</result>`
	zip := zipped(t, xml)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(zip)
	}))
	defer srv.Close()
	c := New("k")
	c.http = srv.Client()
	c.base = srv.URL

	m, err := c.CorpCodeMap(context.Background())
	if err != nil {
		t.Fatalf("CorpCodeMap: %v", err)
	}
	if len(m) != 1 || m["005930"] != "00126380" { // blank stock_code dropped
		t.Fatalf("map=%v want {005930:00126380}", m)
	}
}

func TestRecentFilings(t *testing.T) {
	const listJSON = `{"status":"000","message":"정상","list":[
	  {"corp_code":"00126380","corp_name":"삼성전자","stock_code":"005930","report_nm":"주요사항보고서","rcept_no":"20260603000123","rcept_dt":"20260603"}
	]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(listJSON))
	}))
	defer srv.Close()
	c := New("k")
	c.http = srv.Client()
	c.base = srv.URL

	f, err := c.RecentFilings(context.Background(), "005930.KS", "00126380", 10)
	if err != nil {
		t.Fatalf("RecentFilings: %v", err)
	}
	if len(f) != 1 {
		t.Fatalf("filings=%d want 1", len(f))
	}
	if f[0].Ticker != "005930.KS" || f[0].AccessionNo != "20260603000123" {
		t.Errorf("filing = %+v", f[0])
	}
	if !strings.Contains(f[0].URL, "rcpNo=20260603000123") {
		t.Errorf("url = %q", f[0].URL)
	}
	if f[0].FiledAt.Year() != 2026 {
		t.Errorf("filedAt = %v", f[0].FiledAt)
	}
}

func TestRecentFilingsNoData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"013","message":"조회된 데이타가 없습니다."}`))
	}))
	defer srv.Close()
	c := New("k")
	c.http = srv.Client()
	c.base = srv.URL
	f, err := c.RecentFilings(context.Background(), "005930.KS", "00126380", 10)
	if err != nil || f != nil {
		t.Fatalf("status 013 = (%v,%v), want (nil,nil)", f, err)
	}
}
