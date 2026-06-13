package ratecut

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// polymarketEventFixture is a trimmed GET /events?slug=how-many-fed-rate-cuts-in-2026
// response captured live. outcomes/outcomePrices are JSON-string-encoded arrays;
// each sub-market's "Yes" price is the bucket probability. Includes a closed
// sub-market (must be skipped) and one with a malformed price array.
const polymarketEventFixture = `[{
  "title":"How many Fed rate cuts in 2026?",
  "slug":"how-many-fed-rate-cuts-in-2026",
  "closed":false,
  "markets":[
    {"groupItemTitle":"0 (0 bps)","closed":false,"active":true,"outcomes":"[\"Yes\", \"No\"]","outcomePrices":"[\"0.7685\", \"0.2315\"]"},
    {"groupItemTitle":"1 (25 bps)","closed":false,"active":true,"outcomes":"[\"Yes\", \"No\"]","outcomePrices":"[\"0.1550\", \"0.8450\"]"},
    {"groupItemTitle":"2 (50 bps)","closed":false,"active":true,"outcomes":"[\"Yes\", \"No\"]","outcomePrices":"[\"0.0310\", \"0.9690\"]"},
    {"groupItemTitle":"old closed bucket","closed":true,"active":false,"outcomes":"[\"Yes\", \"No\"]","outcomePrices":"[\"0.99\", \"0.01\"]"},
    {"groupItemTitle":"garbage prices","closed":false,"active":true,"outcomes":"[\"Yes\", \"No\"]","outcomePrices":"not-json"}
  ]
}]`

func newPolymarketTestClient(srvURL string) *PolymarketClient {
	c := NewPolymarket()
	c.baseURL = srvURL
	c.now = func() time.Time { return time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC) }
	return c
}

func TestPolymarketFetchParsesAndMaps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != userAgent {
			t.Errorf("User-Agent = %q, want %q", got, userAgent)
		}
		if r.URL.Path != "/events" {
			t.Errorf("path = %q, want /events", r.URL.Path)
		}
		if got := r.URL.Query().Get("slug"); got != "how-many-fed-rate-cuts-in-2026" {
			t.Errorf("slug = %q", got)
		}
		_, _ = w.Write([]byte(polymarketEventFixture))
	}))
	defer srv.Close()

	c := newPolymarketTestClient(srv.URL)
	m, err := c.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if m.Source != "polymarket" {
		t.Errorf("Source = %q, want polymarket", m.Source)
	}
	if m.Question != "How many Fed rate cuts in 2026?" {
		t.Errorf("Question = %q", m.Question)
	}
	if m.URL != "https://polymarket.com/event/how-many-fed-rate-cuts-in-2026" {
		t.Errorf("URL = %q", m.URL)
	}
	// 3 open, parseable buckets; closed + malformed are skipped.
	if len(m.Outcomes) != 3 {
		t.Fatalf("got %d outcomes, want 3: %+v", len(m.Outcomes), m.Outcomes)
	}
	// Sorted by probability desc → "0 (0 bps)" at 0.7685 first.
	if m.Outcomes[0].Label != "0 (0 bps)" {
		t.Errorf("top label = %q, want 0 (0 bps)", m.Outcomes[0].Label)
	}
	if p := m.Outcomes[0].Probability; p < 0.768 || p > 0.769 {
		t.Errorf("top probability = %v, want 0.7685 (Yes price)", p)
	}
	by := map[string]float64{}
	for _, o := range m.Outcomes {
		by[o.Label] = o.Probability
	}
	if p := by["1 (25 bps)"]; p < 0.154 || p > 0.156 {
		t.Errorf("1-cut probability = %v, want ~0.155", p)
	}
}

func TestPolymarketFetchErrorOnEmptyEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := newPolymarketTestClient(srv.URL)
	if _, err := c.Fetch(context.Background()); err == nil {
		t.Fatal("expected error when event slug returns [], got nil")
	}
}

func TestPolymarketFetchErrorOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	c := newPolymarketTestClient(srv.URL)
	if _, err := c.Fetch(context.Background()); err == nil {
		t.Fatal("expected error on 502, got nil")
	}
}

func TestPolymarketFetchErrorWhenAllSkipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"title":"t","slug":"s","closed":false,"markets":[
		  {"groupItemTitle":"a","closed":true,"outcomes":"[\"Yes\",\"No\"]","outcomePrices":"[\"0.5\",\"0.5\"]"}
		]}]`))
	}))
	defer srv.Close()

	c := newPolymarketTestClient(srv.URL)
	if _, err := c.Fetch(context.Background()); err == nil {
		t.Fatal("expected error when every market is closed, got nil")
	}
}

func TestPolymarketYesProbability(t *testing.T) {
	tests := []struct {
		name string
		m    polymarketMarket
		want float64
		ok   bool
	}{
		{"yes first", polymarketMarket{Outcomes: `["Yes","No"]`, OutcomePrices: `["0.62","0.38"]`}, 0.62, true},
		{"yes second", polymarketMarket{Outcomes: `["No","Yes"]`, OutcomePrices: `["0.30","0.70"]`}, 0.70, true},
		{"case insensitive", polymarketMarket{Outcomes: `["YES","NO"]`, OutcomePrices: `["0.10","0.90"]`}, 0.10, true},
		{"clamp", polymarketMarket{Outcomes: `["Yes","No"]`, OutcomePrices: `["1.4","-0.4"]`}, 1.0, true},
		{"no yes leg", polymarketMarket{Outcomes: `["A","B"]`, OutcomePrices: `["0.5","0.5"]`}, 0, false},
		{"length mismatch", polymarketMarket{Outcomes: `["Yes","No"]`, OutcomePrices: `["0.5"]`}, 0, false},
		{"bad prices json", polymarketMarket{Outcomes: `["Yes","No"]`, OutcomePrices: `nope`}, 0, false},
		{"bad outcomes json", polymarketMarket{Outcomes: `nope`, OutcomePrices: `["0.5","0.5"]`}, 0, false},
		{"empty", polymarketMarket{}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := polymarketYesProbability(tt.m)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && (got < tt.want-1e-9 || got > tt.want+1e-9) {
				t.Errorf("prob = %v, want %v", got, tt.want)
			}
		})
	}
}
