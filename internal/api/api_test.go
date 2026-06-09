package api

import (
	"context"
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
		nil, // no universe source in tests
		nil, // no guru source in tests
		nil, // no ticker ingestor in tests
		nil, // no symbol searcher in tests
		nil, // no event source in tests
		nil, // no fundamentals source in tests
		nil, // no earnings source in tests
		nil, // no congress source in tests
		nil, // no admin user ids in tests
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

func TestAlertsAPI(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	// Unauthenticated read → 401.
	if resp, err := http.Get(srv.URL + "/v1/alerts"); err != nil {
		t.Fatal(err)
	} else if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET /v1/alerts (no auth) = %d, want 401", resp.StatusCode)
	}

	// Create (ticker is upper-cased).
	resp := authed(t, "POST", srv.URL+"/v1/alerts", `{"ticker":"aapl","kind":"price_above","threshold":200}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /v1/alerts = %d, want 201", resp.StatusCode)
	}
	var created struct {
		ID     string `json:"id"`
		Ticker string `json:"ticker"`
		Active bool   `json:"active"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if created.ID == "" || created.Ticker != "AAPL" || !created.Active {
		t.Fatalf("created = %+v, want id + ticker AAPL + active", created)
	}

	// Invalid kind → 400.
	if r := authed(t, "POST", srv.URL+"/v1/alerts", `{"ticker":"AAPL","kind":"nope","threshold":1}`); r.StatusCode != http.StatusBadRequest {
		t.Errorf("POST invalid kind = %d, want 400", r.StatusCode)
	}

	// List → exactly the one created.
	r := authed(t, "GET", srv.URL+"/v1/alerts", "")
	var list struct {
		Count int `json:"count"`
	}
	_ = json.NewDecoder(r.Body).Decode(&list)
	r.Body.Close()
	if list.Count != 1 {
		t.Fatalf("list count = %d, want 1", list.Count)
	}

	// Delete → 200, then the list is empty.
	if r := authed(t, "DELETE", srv.URL+"/v1/alerts/"+created.ID, ""); r.StatusCode != http.StatusOK {
		t.Errorf("DELETE = %d, want 200", r.StatusCode)
	}
	r2 := authed(t, "GET", srv.URL+"/v1/alerts", "")
	var list2 struct {
		Count int `json:"count"`
	}
	_ = json.NewDecoder(r2.Body).Decode(&list2)
	r2.Body.Close()
	if list2.Count != 0 {
		t.Errorf("after delete count = %d, want 0", list2.Count)
	}
}

func TestHoldingsAPI(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	// Unauthenticated read → 401.
	if resp, err := http.Get(srv.URL + "/v1/holdings"); err != nil {
		t.Fatal(err)
	} else if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET /v1/holdings (no auth) = %d, want 401", resp.StatusCode)
	}

	// Create (ticker upper-cased).
	resp := authed(t, "POST", srv.URL+"/v1/holdings", `{"ticker":"aapl","shares":10,"avg_cost":150}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /v1/holdings = %d, want 201", resp.StatusCode)
	}
	var created struct {
		ID     string  `json:"id"`
		Ticker string  `json:"ticker"`
		Shares float64 `json:"shares"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if created.ID == "" || created.Ticker != "AAPL" || created.Shares != 10 {
		t.Fatalf("created = %+v, want id + AAPL + 10 shares", created)
	}

	// Non-positive shares → 400.
	if r := authed(t, "POST", srv.URL+"/v1/holdings", `{"ticker":"AAPL","shares":0,"avg_cost":1}`); r.StatusCode != http.StatusBadRequest {
		t.Errorf("POST shares=0 = %d, want 400", r.StatusCode)
	}

	// Re-saving AAPL upserts → still one row.
	authed(t, "POST", srv.URL+"/v1/holdings", `{"ticker":"AAPL","shares":20,"avg_cost":160}`).Body.Close()
	r := authed(t, "GET", srv.URL+"/v1/holdings", "")
	var list struct {
		Count int `json:"count"`
	}
	_ = json.NewDecoder(r.Body).Decode(&list)
	r.Body.Close()
	if list.Count != 1 {
		t.Fatalf("list count = %d, want 1 (upsert by ticker)", list.Count)
	}

	// Delete → 200.
	if r := authed(t, "DELETE", srv.URL+"/v1/holdings/"+created.ID, ""); r.StatusCode != http.StatusOK {
		t.Errorf("DELETE = %d, want 200", r.StatusCode)
	}
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

// fakeIngestor records IngestOne calls for the on-add trigger test.
type fakeIngestor struct{ called chan string }

func (f fakeIngestor) IngestOne(_ context.Context, ticker string) { f.called <- ticker }

// Adding a ticker should fire a one-shot ingest for it (normalized, async).
func TestWatchlistAddTriggersIngest(t *testing.T) {
	ing := fakeIngestor{called: make(chan string, 1)}
	h := New(
		memory.New(), stream.NewHub(), enrich.Noop{},
		auth.NewVerifier(testSecret, ""),
		nil, nil, nil, nil, nil, // bars, topics, opps, universe, gurus
		ing,
		nil, // no symbol searcher
		nil, // no event source
		nil, // no fundamentals source
		nil, // no earnings source
		nil, // no congress source
		nil, // no admin user ids
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp := authed(t, http.MethodPost, srv.URL+"/v1/watchlist", `{"ticker":"nvda"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	select {
	case got := <-ing.called:
		if got != "NVDA" { // lower-case input is normalized before ingest
			t.Errorf("ingested %q, want NVDA", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("IngestOne was not called on add")
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
