package ratecut

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// kalshiEventsFixture is a trimmed GET /events?series_ticker=KXFED&status=open
// response captured live: events are unsorted and span future FOMC meetings.
const kalshiEventsFixture = `{"cursor":"x","events":[
  {"event_ticker":"KXFED-27MAR","series_ticker":"KXFED","strike_date":"2027-03-17T18:00:00Z","title":"Fed funds rate after Mar 2027 meeting?"},
  {"event_ticker":"KXFED-26DEC","series_ticker":"KXFED","strike_date":"2026-12-09T19:00:00Z","title":"Fed funds rate after Dec 2026 meeting?"},
  {"event_ticker":"KXFED-26OCT","series_ticker":"KXFED","strike_date":"2026-10-28T18:00:00Z","title":"Fed funds rate after Oct 2026 meeting?"}
]}`

// kalshiEventFixture is a trimmed GET /events/KXFED-26OCT?with_nested_markets=true
// response. Prices are decimal-dollar strings (0–1). Includes an inactive market
// (must be skipped) and a one-sided quote (only yes_ask) to exercise fallbacks.
const kalshiEventFixture = `{"event":{"event_ticker":"KXFED-26OCT","title":"Fed funds rate after Oct 2026 meeting?","markets":[
  {"ticker":"KXFED-26OCT-T3.25","status":"active","strike_type":"greater","floor_strike":3.25,"subtitle":"3.25%","yes_sub_title":"Above 3.25%","yes_bid_dollars":"0.7500","yes_ask_dollars":"0.9500","last_price_dollars":"0.9500"},
  {"ticker":"KXFED-26OCT-T3.50","status":"active","strike_type":"greater","floor_strike":3.50,"subtitle":"3.50%","yes_sub_title":"Above 3.50%","yes_bid_dollars":"0.1100","yes_ask_dollars":"0.2200","last_price_dollars":"0.7800"},
  {"ticker":"KXFED-26OCT-T4.00","status":"active","strike_type":"greater","floor_strike":4.00,"subtitle":"4.00%","yes_sub_title":"Above 4.00%","yes_bid_dollars":"","yes_ask_dollars":"0.2300","last_price_dollars":"0.0700"},
  {"ticker":"KXFED-26OCT-SETTLED","status":"settled","strike_type":"greater","floor_strike":2.75,"subtitle":"2.75%","yes_sub_title":"Above 2.75%","yes_bid_dollars":"0.8900","yes_ask_dollars":"0.9800","last_price_dollars":"0.8900"}
]}}`

// newKalshiTestClient wires a KalshiClient to a mock server with a fixed clock
// (2026-06-13) so nextEvent deterministically picks KXFED-26OCT.
func newKalshiTestClient(srvURL string) *KalshiClient {
	c := NewKalshi()
	c.baseURL = srvURL
	c.now = func() time.Time { return time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC) }
	return c
}

func TestKalshiFetchParsesAndPrices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != userAgent {
			t.Errorf("User-Agent = %q, want %q", got, userAgent)
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/events/KXFED-26OCT"):
			if r.URL.Query().Get("with_nested_markets") != "true" {
				t.Errorf("missing with_nested_markets=true: %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(kalshiEventFixture))
		case r.URL.Path == "/events":
			if r.URL.Query().Get("series_ticker") != "KXFED" || r.URL.Query().Get("status") != "open" {
				t.Errorf("unexpected events query %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(kalshiEventsFixture))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := newKalshiTestClient(srv.URL)
	m, err := c.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if m.Source != "kalshi" {
		t.Errorf("Source = %q, want kalshi", m.Source)
	}
	if !strings.Contains(m.Question, "Oct 2026") {
		t.Errorf("Question = %q, want the Oct 2026 event (soonest upcoming)", m.Question)
	}
	if m.AsOf == "" {
		t.Error("AsOf is empty")
	}
	// 3 active markets priced; the settled one is dropped.
	if len(m.Outcomes) != 3 {
		t.Fatalf("got %d outcomes, want 3: %+v", len(m.Outcomes), m.Outcomes)
	}
	// Highest probability first: T3.25 mid = (0.75+0.95)/2 = 0.85.
	top := m.Outcomes[0]
	if top.Label != "Above 3.25%" {
		t.Errorf("top label = %q, want Above 3.25%%", top.Label)
	}
	if top.Probability < 0.849 || top.Probability > 0.851 {
		t.Errorf("top probability = %v, want ~0.85 (bid/ask mid)", top.Probability)
	}
	// The one-sided market (only yes_ask=0.23) must fall back to the ask, not last.
	var t4 *Outcome
	for i := range m.Outcomes {
		if m.Outcomes[i].Label == "Above 4.00%" {
			t4 = &m.Outcomes[i]
		}
	}
	if t4 == nil {
		t.Fatal("missing Above 4.00% outcome")
	}
	if t4.Probability < 0.229 || t4.Probability > 0.231 {
		t.Errorf("Above 4.00%% probability = %v, want ~0.23 (yes_ask fallback)", t4.Probability)
	}
}

func TestKalshiNextEventPicksSoonest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/events" {
			_, _ = w.Write([]byte(kalshiEventsFixture))
			return
		}
		// Capture which event detail was requested.
		if !strings.Contains(r.URL.Path, "KXFED-26OCT") {
			t.Errorf("expected detail fetch for soonest event KXFED-26OCT, got %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(kalshiEventFixture))
	}))
	defer srv.Close()

	c := newKalshiTestClient(srv.URL)
	ev, err := c.nextEvent(context.Background())
	if err != nil {
		t.Fatalf("nextEvent: %v", err)
	}
	if ev.EventTicker != "KXFED-26OCT" {
		t.Errorf("nextEvent = %q, want KXFED-26OCT (soonest future strike_date)", ev.EventTicker)
	}
}

func TestKalshiNextEventSkipsPast(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(kalshiEventsFixture))
	}))
	defer srv.Close()

	c := NewKalshi()
	c.baseURL = srv.URL
	// Clock after Oct & Dec 2026 → soonest remaining is Mar 2027.
	c.now = func() time.Time { return time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC) }
	ev, err := c.nextEvent(context.Background())
	if err != nil {
		t.Fatalf("nextEvent: %v", err)
	}
	if ev.EventTicker != "KXFED-27MAR" {
		t.Errorf("nextEvent = %q, want KXFED-27MAR (past meetings skipped)", ev.EventTicker)
	}
}

func TestKalshiFetchErrorOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := newKalshiTestClient(srv.URL)
	if _, err := c.Fetch(context.Background()); err == nil {
		t.Fatal("expected error on 503, got nil")
	}
}

func TestKalshiFetchErrorOnNoPricedMarkets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/events" {
			_, _ = w.Write([]byte(kalshiEventsFixture))
			return
		}
		// All markets settled/closed → nothing priced.
		_, _ = w.Write([]byte(`{"event":{"event_ticker":"KXFED-26OCT","title":"t","markets":[
		  {"ticker":"x","status":"settled","yes_bid_dollars":"0.5","yes_ask_dollars":"0.6"}
		]}}`))
	}))
	defer srv.Close()

	c := newKalshiTestClient(srv.URL)
	if _, err := c.Fetch(context.Background()); err == nil {
		t.Fatal("expected error when no active priced markets, got nil")
	}
}

func TestKalshiProbabilityFallbacks(t *testing.T) {
	tests := []struct {
		name string
		m    kalshiMarket
		want float64
		ok   bool
	}{
		{"two-sided mid", kalshiMarket{YesBidDollars: "0.40", YesAskDollars: "0.60"}, 0.50, true},
		{"bid only", kalshiMarket{YesBidDollars: "0.33"}, 0.33, true},
		{"ask only", kalshiMarket{YesAskDollars: "0.77"}, 0.77, true},
		{"last fallback", kalshiMarket{LastPriceDollar: "0.91"}, 0.91, true},
		{"clamp over 1", kalshiMarket{LastPriceDollar: "1.50"}, 1.0, true},
		{"no price", kalshiMarket{}, 0, false},
		{"crossed quote falls through to bid", kalshiMarket{YesBidDollars: "0.90", YesAskDollars: "0.10"}, 0.90, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := kalshiProbability(tt.m)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && (got < tt.want-1e-9 || got > tt.want+1e-9) {
				t.Errorf("prob = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKalshiLabelFallback(t *testing.T) {
	tests := []struct {
		m    kalshiMarket
		want string
	}{
		{kalshiMarket{YesSubTitle: "Above 3.25%"}, "Above 3.25%"},
		{kalshiMarket{Subtitle: "3.25%"}, "3.25%"},
		{kalshiMarket{StrikeType: "greater", FloorStrike: 3.5}, "Above 3.5%"},
		{kalshiMarket{StrikeType: "less", FloorStrike: 2}, "Below 2%"},
	}
	for _, tt := range tests {
		if got := kalshiLabel(tt.m); got != tt.want {
			t.Errorf("kalshiLabel(%+v) = %q, want %q", tt.m, got, tt.want)
		}
	}
}
