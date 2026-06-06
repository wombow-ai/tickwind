package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/wombow-ai/tickwind/internal/auth"
	"github.com/wombow-ai/tickwind/internal/enrich"
	"github.com/wombow-ai/tickwind/internal/store/memory"
	"github.com/wombow-ai/tickwind/internal/stream"
)

// fakeBarSource returns canned series and records which tickers were requested.
type fakeBarSource struct {
	mu    sync.Mutex
	calls map[string]int
	data  map[string][]float64
}

func newFakeBarSource(data map[string][]float64) *fakeBarSource {
	return &fakeBarSource{calls: map[string]int{}, data: data}
}

func (f *fakeBarSource) DailyBars(_ context.Context, ticker string) ([]float64, error) {
	f.mu.Lock()
	f.calls[ticker]++
	f.mu.Unlock()
	return f.data[ticker], nil // unknown ticker → nil, which the handler omits
}

func (f *fakeBarSource) distinctCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeBarSource) callsFor(ticker string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[ticker]
}

func serverWithBars(bars BarSource) *httptest.Server {
	h := New(
		memory.New(), stream.NewHub(), enrich.Noop{},
		auth.NewVerifier(testSecret, ""),
		bars,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	return httptest.NewServer(h)
}

func decodeBars(t *testing.T, resp *http.Response) map[string][]float64 {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	var body struct {
		Bars map[string][]float64 `json:"bars"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.Bars
}

func TestGetBarsBatch_DedupeAndOmitUnknown(t *testing.T) {
	fake := newFakeBarSource(map[string][]float64{
		"AAPL": {1, 2, 3},
		"NVDA": {4, 5},
		// MSFT intentionally absent → should be omitted from the response.
	})
	srv := serverWithBars(fake)
	defer srv.Close()

	// Mixed case + a duplicate AAPL + an empty entry + an unknown ticker.
	resp, err := http.Get(srv.URL + "/v1/bars?tickers=aapl,AAPL,nvda,,MSFT")
	if err != nil {
		t.Fatal(err)
	}
	bars := decodeBars(t, resp)

	if len(bars) != 2 {
		t.Fatalf("got %d series; want 2 (AAPL,NVDA): %v", len(bars), bars)
	}
	if got := bars["AAPL"]; len(got) != 3 {
		t.Errorf("AAPL closes = %v; want len 3", got)
	}
	if _, ok := bars["MSFT"]; ok {
		t.Error("MSFT should be omitted (no data)")
	}
	if n := fake.callsFor("AAPL"); n != 1 {
		t.Errorf("AAPL fetched %d times; want 1 (deduped, uppercased)", n)
	}
}

func TestGetBarsBatch_Cap(t *testing.T) {
	data := map[string][]float64{}
	var sb strings.Builder
	for i := 0; i < maxBarsBatch+10; i++ {
		tk := "T" + strconv.Itoa(i)
		data[tk] = []float64{float64(i)}
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(tk)
	}
	fake := newFakeBarSource(data)
	srv := serverWithBars(fake)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/bars?tickers=" + sb.String())
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if got := fake.distinctCalls(); got > maxBarsBatch {
		t.Errorf("fetched %d tickers; want <= cap %d", got, maxBarsBatch)
	}
}

func TestGetBarsBatch_NilSource(t *testing.T) {
	srv := newTestServer() // constructed with a nil bar source
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/bars?tickers=AAPL,NVDA")
	if err != nil {
		t.Fatal(err)
	}
	if bars := decodeBars(t, resp); len(bars) != 0 {
		t.Errorf("nil bar source should yield empty bars; got %v", bars)
	}
}

func TestGetBarsSingle(t *testing.T) {
	fake := newFakeBarSource(map[string][]float64{"AAPL": {10, 11, 12}})
	srv := serverWithBars(fake)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/stocks/aapl/bars")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body struct {
		Ticker string    `json:"ticker"`
		Closes []float64 `json:"closes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Ticker != "AAPL" || len(body.Closes) != 3 {
		t.Errorf("got %+v; want AAPL with 3 closes", body)
	}
}
