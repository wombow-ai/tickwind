package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/auth"
	"github.com/wombow-ai/tickwind/internal/enrich"
	"github.com/wombow-ai/tickwind/internal/store/memory"
	"github.com/wombow-ai/tickwind/internal/stream"
)

const testSecret = "api-test-secret"

func newTestServer() *httptest.Server {
	h := New(
		memory.New(), stream.NewHub(), enrich.Noop{},
		auth.NewVerifier(testSecret, ""),
		nil, // no bar source in tests
		nil, // no topic source in tests
		nil, // no opportunity source in tests
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	return httptest.NewServer(h)
}

// token mints a valid HS256 JWT for the test secret.
func token(sub string) string {
	enc := base64.RawURLEncoding
	hdr, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	pl, _ := json.Marshal(map[string]any{"sub": sub, "exp": time.Now().Add(time.Hour).Unix()})
	signing := enc.EncodeToString(hdr) + "." + enc.EncodeToString(pl)
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write([]byte(signing))
	return signing + "." + enc.EncodeToString(mac.Sum(nil))
}

func authed(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	var r *http.Request
	var err error
	if body != "" {
		r, err = http.NewRequest(method, url, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer "+token("user-1"))
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	return resp
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
}

func TestWatchlistRequiresAuth(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/watchlist") // no token
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401 without a token", resp.StatusCode)
	}
}

func TestWatchlistCRUD(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	if got := tickers(t, srv.URL); len(got) != 0 {
		t.Fatalf("initial = %v; want empty", got)
	}

	resp := authed(t, http.MethodPost, srv.URL+"/v1/watchlist", `{"ticker":"aapl"}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST status = %d", resp.StatusCode)
	}
	if got := tickers(t, srv.URL); len(got) != 1 || got[0] != "AAPL" {
		t.Fatalf("after add = %v", got)
	}

	resp = authed(t, http.MethodDelete, srv.URL+"/v1/watchlist/AAPL", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE status = %d", resp.StatusCode)
	}
	if got := tickers(t, srv.URL); len(got) != 0 {
		t.Fatalf("after delete = %v", got)
	}
}

func TestWatchlistRejectsEmpty(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	resp := authed(t, http.MethodPost, srv.URL+"/v1/watchlist", `{"ticker":""}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", resp.StatusCode)
	}
}

func TestStockNotFoundIsPublic(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/stocks/ZZZZ") // public, no token
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", resp.StatusCode)
	}
}

func TestClipSavesToUsersClips(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<title>Clipped Page</title>")
	}))
	defer target.Close()

	resp := authed(t, http.MethodPost, srv.URL+"/v1/stocks/AAPL/clip", `{"url":"`+target.URL+`"}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("clip status = %d", resp.StatusCode)
	}

	cresp := authed(t, http.MethodGet, srv.URL+"/v1/stocks/AAPL/clips", "")
	defer cresp.Body.Close()
	var body struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(cresp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Count != 1 {
		t.Fatalf("clips count = %d; want 1", body.Count)
	}
}

func TestSummaryDisabledReturns503(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/stocks/AAPL/summary")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; want 503 when LLM disabled", resp.StatusCode)
	}
}

func tickers(t *testing.T, base string) []string {
	t.Helper()
	resp := authed(t, http.MethodGet, base+"/v1/watchlist", "")
	defer resp.Body.Close()
	var body struct {
		Tickers []string `json:"tickers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body.Tickers
}
