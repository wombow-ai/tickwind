package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/wombow-ai/tickwind/internal/store"
)

// fakeQuoteBars serves canned LatestQuote results — the on-demand fallback path
// getQuotesBatch takes when the (empty test) store has no quote — and records
// which tickers were requested, so dedupe/cap can be asserted.
type fakeQuoteBars struct {
	mu    sync.Mutex
	calls map[string]int
	data  map[string]store.Quote
}

func newFakeQuoteBars(data map[string]store.Quote) *fakeQuoteBars {
	return &fakeQuoteBars{calls: map[string]int{}, data: data}
}

func (f *fakeQuoteBars) DailyBars(context.Context, string) ([]float64, error) { return nil, nil }
func (f *fakeQuoteBars) DailyCandles(context.Context, string) ([]store.Candle, error) {
	return nil, nil
}
func (f *fakeQuoteBars) IntradayCandles(context.Context, string, string) ([]store.Candle, error) {
	return nil, nil
}

func (f *fakeQuoteBars) LatestQuote(_ context.Context, ticker string) (store.Quote, bool, error) {
	f.mu.Lock()
	f.calls[ticker]++
	f.mu.Unlock()
	q, ok := f.data[ticker]
	return q, ok, nil
}

func (f *fakeQuoteBars) distinctCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeQuoteBars) callsFor(ticker string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[ticker]
}

func decodeQuotes(t *testing.T, resp *http.Response) map[string]store.Quote {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	var body struct {
		Quotes map[string]store.Quote `json:"quotes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.Quotes
}

func TestGetQuotesBatch_DedupeAndOmitUnknown(t *testing.T) {
	fake := newFakeQuoteBars(map[string]store.Quote{
		"AAPL": {Ticker: "AAPL", Price: 190, Source: "alpaca"},
		"NVDA": {Ticker: "NVDA", Price: 200, Source: "alpaca"},
		// MSFT intentionally absent → omitted from the response.
	})
	srv := serverWithBars(fake)
	defer srv.Close()

	// Mixed case + a duplicate AAPL + an empty entry + an unknown ticker.
	resp, err := http.Get(srv.URL + "/v1/quotes?tickers=aapl,AAPL,nvda,,MSFT")
	if err != nil {
		t.Fatal(err)
	}
	quotes := decodeQuotes(t, resp)

	if len(quotes) != 2 {
		t.Fatalf("got %d quotes; want 2 (AAPL,NVDA): %v", len(quotes), quotes)
	}
	if quotes["AAPL"].Price != 190 {
		t.Errorf("AAPL price = %v; want 190", quotes["AAPL"].Price)
	}
	if _, ok := quotes["MSFT"]; ok {
		t.Error("MSFT should be omitted (no quote)")
	}
	if n := fake.callsFor("AAPL"); n != 1 {
		t.Errorf("AAPL fetched %d times; want 1 (deduped, uppercased)", n)
	}
}

func TestGetQuotesBatch_Cap(t *testing.T) {
	data := map[string]store.Quote{}
	var sb strings.Builder
	for i := 0; i < maxQuotesBatch+10; i++ {
		tk := "T" + strconv.Itoa(i)
		data[tk] = store.Quote{Ticker: tk, Price: float64(i), Source: "alpaca"}
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(tk)
	}
	srv := serverWithBars(newFakeQuoteBars(data))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/quotes?tickers=" + sb.String())
	if err != nil {
		t.Fatal(err)
	}
	quotes := decodeQuotes(t, resp)
	if len(quotes) > maxQuotesBatch {
		t.Errorf("returned %d quotes; want <= cap %d", len(quotes), maxQuotesBatch)
	}
}

func TestGetQuotesBatch_NilSourceEmptyStore(t *testing.T) {
	srv := newTestServer() // nil bar source + empty memory store
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/quotes?tickers=AAPL,NVDA")
	if err != nil {
		t.Fatal(err)
	}
	if quotes := decodeQuotes(t, resp); len(quotes) != 0 {
		t.Errorf("nil bars + empty store should yield no quotes; got %v", quotes)
	}
}
