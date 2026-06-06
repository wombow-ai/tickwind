package alphavantage

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

// sampleFeed: AAPL appears twice (rel-weighted (0.5·0.4 + 0.5·0.2)/1.0 = 0.30 →
// Somewhat-Bullish, 2 articles); MSFT once (−0.1 → Neutral, 1 article).
const sampleFeed = `{
  "items": "2",
  "feed": [
    { "title": "A", "ticker_sentiment": [
        {"ticker":"AAPL","relevance_score":"0.5","ticker_sentiment_score":"0.4","ticker_sentiment_label":"Bullish"},
        {"ticker":"MSFT","relevance_score":"0.2","ticker_sentiment_score":"-0.1","ticker_sentiment_label":"Neutral"}
    ]},
    { "title": "B", "ticker_sentiment": [
        {"ticker":"AAPL","relevance_score":"0.5","ticker_sentiment_score":"0.2","ticker_sentiment_label":"Somewhat-Bullish"}
    ]}
  ]
}`

func TestSignalsAggregates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("function"); got != "NEWS_SENTIMENT" {
			t.Errorf("function = %q", got)
		}
		if got := r.URL.Query().Get("apikey"); got != "testkey" {
			t.Errorf("apikey = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "Tickwind/0.1 (+https://tickwind.com)" {
			t.Errorf("User-Agent = %q", got)
		}
		_, _ = w.Write([]byte(sampleFeed))
	}))
	defer srv.Close()

	c := New("testkey")
	c.baseURL = srv.URL

	sigs, err := c.Signals(context.Background(), []string{"AAPL", "MSFT", "NFLX"})
	if err != nil {
		t.Fatalf("Signals: %v", err)
	}
	by := map[string]store.Signal{}
	for _, s := range sigs {
		by[s.Ticker] = s
	}
	if _, ok := by["NFLX"]; ok {
		t.Error("NFLX has no coverage and should be omitted")
	}

	aapl, ok := by["AAPL"]
	if !ok {
		t.Fatal("missing AAPL signal")
	}
	if aapl.Source != "alphavantage" || aapl.Kind != "sentiment" {
		t.Errorf("AAPL source/kind = %q/%q, want alphavantage/sentiment", aapl.Source, aapl.Kind)
	}
	if math.Abs(aapl.Score-0.30) > 1e-9 {
		t.Errorf("AAPL score = %v, want 0.30", aapl.Score)
	}
	if aapl.Label != "Somewhat-Bullish" {
		t.Errorf("AAPL label = %q, want Somewhat-Bullish", aapl.Label)
	}
	if aapl.SampleSize != 2 {
		t.Errorf("AAPL sample size = %d, want 2", aapl.SampleSize)
	}
	if aapl.UpdatedAt.IsZero() {
		t.Error("AAPL UpdatedAt is zero")
	}

	msft := by["MSFT"]
	if math.Abs(msft.Score-(-0.1)) > 1e-9 {
		t.Errorf("MSFT score = %v, want -0.1", msft.Score)
	}
	if msft.Label != "Neutral" || msft.SampleSize != 1 {
		t.Errorf("MSFT label/sample = %q/%d, want Neutral/1", msft.Label, msft.SampleSize)
	}
}

func TestLabelBands(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{-0.50, "Bearish"}, {-0.35, "Bearish"},
		{-0.20, "Somewhat-Bearish"}, {-0.15, "Somewhat-Bearish"},
		{-0.10, "Neutral"}, {0.0, "Neutral"}, {0.14, "Neutral"},
		{0.15, "Somewhat-Bullish"}, {0.30, "Somewhat-Bullish"},
		{0.35, "Bullish"}, {0.80, "Bullish"},
	}
	for _, tc := range cases {
		if got := label(tc.score); got != tc.want {
			t.Errorf("label(%v) = %q, want %q", tc.score, got, tc.want)
		}
	}
}

func TestSignalsDisabledWithoutKey(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { hits++ }))
	defer srv.Close()

	c := New("") // no key → disabled
	c.baseURL = srv.URL

	sigs, err := c.Signals(context.Background(), []string{"AAPL"})
	if err != nil {
		t.Fatalf("Signals: %v", err)
	}
	if sigs != nil {
		t.Errorf("want nil signals when disabled, got %v", sigs)
	}
	if hits != 0 {
		t.Errorf("made %d HTTP calls while disabled, want 0", hits)
	}
}

func TestSignalsCachesWithinInterval(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(sampleFeed))
	}))
	defer srv.Close()

	fixed := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	c := New("k")
	c.baseURL = srv.URL
	c.now = func() time.Time { return fixed }

	for i := 0; i < 2; i++ {
		if _, err := c.Signals(context.Background(), []string{"AAPL"}); err != nil {
			t.Fatalf("Signals %d: %v", i, err)
		}
	}
	if hits != 1 {
		t.Errorf("server hits = %d, want 1 (second call served from cache)", hits)
	}
}

func TestSignalsRespectsDailyCap(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(sampleFeed))
	}))
	defer srv.Close()

	fixed := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	c := New("k")
	c.baseURL = srv.URL
	c.refreshInterval = 0 // bypass freshness so only the cap gates
	c.dailyCap = 1
	c.now = func() time.Time { return fixed }

	for i := 0; i < 3; i++ {
		if _, err := c.Signals(context.Background(), []string{"AAPL"}); err != nil {
			t.Fatalf("Signals %d: %v", i, err)
		}
	}
	if hits != 1 {
		t.Errorf("server hits = %d, want 1 (daily cap)", hits)
	}
}

func TestSignalsRateLimitBodyExhaustsDay(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"Information":"thank you ... 25 requests per day ..."}`))
	}))
	defer srv.Close()

	fixed := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	c := New("k")
	c.baseURL = srv.URL
	c.refreshInterval = 0
	c.dailyCap = 5 // generous, but a rate-limit body should still stop the day
	c.now = func() time.Time { return fixed }

	sigs, err := c.Signals(context.Background(), []string{"AAPL"})
	if err != nil {
		t.Fatalf("Signals: %v", err)
	}
	if len(sigs) != 0 {
		t.Errorf("want empty cache on rate-limit, got %v", sigs)
	}
	if _, err := c.Signals(context.Background(), []string{"AAPL"}); err != nil {
		t.Fatalf("Signals (2nd): %v", err)
	}
	if hits != 1 {
		t.Errorf("server hits = %d, want 1 (rate-limit marks day exhausted)", hits)
	}
}

func TestSignalsResetsBudgetNextDay(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(sampleFeed))
	}))
	defer srv.Close()

	day1 := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	day2 := day1.Add(24 * time.Hour)
	cur := day1
	c := New("k")
	c.baseURL = srv.URL
	c.refreshInterval = 0
	c.dailyCap = 1
	c.now = func() time.Time { return cur }

	if _, err := c.Signals(context.Background(), []string{"AAPL"}); err != nil { // day1: fetch
		t.Fatal(err)
	}
	if _, err := c.Signals(context.Background(), []string{"AAPL"}); err != nil { // day1: capped, cache
		t.Fatal(err)
	}
	cur = day2
	if _, err := c.Signals(context.Background(), []string{"AAPL"}); err != nil { // day2: budget reset, fetch
		t.Fatal(err)
	}
	if hits != 2 {
		t.Errorf("server hits = %d, want 2 (one per day)", hits)
	}
}
