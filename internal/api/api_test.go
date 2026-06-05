package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wombow-ai/tickwind/internal/store/memory"
	"github.com/wombow-ai/tickwind/internal/stream"
)

func newTestServer() *httptest.Server {
	h := New(memory.New(), stream.NewHub(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	return httptest.NewServer(h)
}

func TestHealthz(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("body = %v", body)
	}
}

func TestWatchlistCRUD(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	if got := getTickers(t, srv.URL); len(got) != 0 {
		t.Fatalf("initial watchlist = %v; want empty", got)
	}

	resp, err := http.Post(
		srv.URL+"/v1/watchlist", "application/json",
		strings.NewReader(`{"ticker":"aapl"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST status = %d", resp.StatusCode)
	}
	if got := getTickers(t, srv.URL); len(got) != 1 || got[0] != "AAPL" {
		t.Fatalf("after add = %v", got)
	}

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/watchlist/AAPL", nil)
	dresp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	dresp.Body.Close()
	if dresp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE status = %d", dresp.StatusCode)
	}
	if got := getTickers(t, srv.URL); len(got) != 0 {
		t.Fatalf("after delete = %v", got)
	}
}

func TestWatchlistRejectsEmpty(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	resp, err := http.Post(
		srv.URL+"/v1/watchlist", "application/json",
		strings.NewReader(`{"ticker":""}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", resp.StatusCode)
	}
}

func TestStockNotFound(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/stocks/ZZZZ")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", resp.StatusCode)
	}
}

func TestClipSavesToSocial(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	// A local page the clip fetcher can read a title from.
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<title>Clipped Page</title>")
	}))
	defer target.Close()

	resp, err := http.Post(
		srv.URL+"/v1/stocks/AAPL/clip", "application/json",
		strings.NewReader(`{"url":"`+target.URL+`"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("clip status = %d", resp.StatusCode)
	}

	sresp, err := http.Get(srv.URL + "/v1/stocks/AAPL/social")
	if err != nil {
		t.Fatal(err)
	}
	defer sresp.Body.Close()
	var body struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(sresp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Count != 1 {
		t.Fatalf("social count = %d; want 1 (the clip)", body.Count)
	}
}

func getTickers(t *testing.T, base string) []string {
	t.Helper()
	resp, err := http.Get(base + "/v1/watchlist")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body struct {
		Tickers []string `json:"tickers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body.Tickers
}
