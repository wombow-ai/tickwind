package brapi

import (
	"testing"
	"time"
)

// A captured /quote/PETR4 response (live 2026-06-10, trimmed).
const samplePETR4 = `{"results":[{"symbol":"PETR4","shortName":"PETR4","longName":"Petroleo Brasileiro SA Pfd","currency":"BRL","regularMarketPrice":41.79,"regularMarketPreviousClose":41.17,"regularMarketTime":"2026-06-10T17:57:30.000Z","marketCap":561900483336}],"requestedAt":"2026-06-10T17:57:31.933Z","took":160}`

func TestParseQuote(t *testing.T) {
	q, ok, err := parseQuote([]byte(samplePETR4))
	if err != nil || !ok {
		t.Fatalf("parseQuote ok=%v err=%v", ok, err)
	}
	if q.Symbol != "PETR4" || q.Name != "Petroleo Brasileiro SA Pfd" || q.Currency != "BRL" {
		t.Fatalf("identity = %+v", q)
	}
	if q.Price != 41.79 || q.PrevClose != 41.17 {
		t.Fatalf("prices = %+v", q)
	}
	want := time.Date(2026, 6, 10, 17, 57, 30, 0, time.UTC)
	if !q.At.Equal(want) {
		t.Fatalf("At = %v, want %v", q.At, want)
	}
}

func TestParseQuoteEmptyAndError(t *testing.T) {
	if _, ok, err := parseQuote([]byte(`{"results":[]}`)); ok || err != nil {
		t.Fatalf("empty results: ok=%v err=%v, want ok=false err=nil", ok, err)
	}
	if _, ok, _ := parseQuote([]byte(`{"error":true,"message":"bad token"}`)); ok {
		t.Fatal("error payload should yield ok=false")
	}
	// A zero/absent price is treated as no-data, not a fake quote.
	if _, ok, _ := parseQuote([]byte(`{"results":[{"symbol":"X","regularMarketPrice":0}]}`)); ok {
		t.Fatal("zero price should yield ok=false")
	}
}

func TestEnabled(t *testing.T) {
	if New("").Enabled() {
		t.Fatal("empty token should be disabled")
	}
	if !New(" tok ").Enabled() {
		t.Fatal("non-empty token should be enabled (trimmed)")
	}
}
