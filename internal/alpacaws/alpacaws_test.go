package alpacaws

import (
	"strings"
	"testing"
	"time"
)

func TestParseTrades(t *testing.T) {
	// A realistic batch: control messages + two trades + an unrelated quote type.
	data := []byte(`[
		{"T":"success","msg":"authenticated"},
		{"T":"subscription","trades":["AAPL","TSLA"]},
		{"T":"t","S":"AAPL","p":288.35,"t":"2026-06-10T13:45:01.2Z"},
		{"T":"q","S":"AAPL","bp":288.3,"ap":288.4},
		{"T":"t","S":"TSLA","p":410.1,"t":"2026-06-10T13:45:02Z"}
	]`)
	trades, controls := parseMessages(data)
	if len(trades) != 2 {
		t.Fatalf("got %d trades, want 2 (%+v)", len(trades), trades)
	}
	if trades[0].Symbol != "AAPL" || trades[0].Price != 288.35 {
		t.Errorf("trade[0] = %+v, want AAPL/288.35", trades[0])
	}
	if trades[0].Time.IsZero() {
		t.Error("trade[0] time not parsed")
	}
	if trades[1].Symbol != "TSLA" || trades[1].Price != 410.1 {
		t.Errorf("trade[1] = %+v, want TSLA/410.1", trades[1])
	}
	// Control messages surface for the handshake + logging (success + subscription here).
	if len(controls) != 2 {
		t.Errorf("got %d controls, want 2 (%+v)", len(controls), controls)
	}
	var sawAuth bool
	for _, c := range controls {
		if c.Type == "success" && strings.Contains(c.Msg, "authenticat") {
			sawAuth = true
		}
	}
	if !sawAuth {
		t.Error("expected the authenticated success control")
	}
}

func TestParseMessagesError(t *testing.T) {
	trades, controls := parseMessages([]byte(`[{"T":"error","code":400,"msg":"invalid syntax"}]`))
	if len(trades) != 0 {
		t.Fatalf("got %d trades, want 0", len(trades))
	}
	if len(controls) != 1 || controls[0].Code != 400 || controls[0].Type != "error" {
		t.Errorf("want one error control with code 400, got %+v", controls)
	}
}

func TestNewCapsSymbols(t *testing.T) {
	syms := make([]string, 50)
	for i := range syms {
		syms[i] = "T"
	}
	s := New("ws://x", "k", "s", syms, nil, func(_ time.Time) string { return "regular" }, nil, nil, nil)
	if want := MaxSymbols - viewedSlots; len(s.base) != want {
		t.Fatalf("base capped to %d, want %d", len(s.base), want)
	}
}

func TestLruAdd(t *testing.T) {
	// Most-recent goes to the end; re-adding bumps it; oldest is trimmed at max.
	lru := lruAdd(nil, "A", 3)
	lru = lruAdd(lru, "B", 3)
	lru = lruAdd(lru, "C", 3)
	if got := join(lru); got != "A,B,C" {
		t.Fatalf("after A,B,C → %q", got)
	}
	lru = lruAdd(lru, "A", 3) // bump A to most-recent
	if got := join(lru); got != "B,C,A" {
		t.Fatalf("after bump A → %q, want B,C,A", got)
	}
	lru = lruAdd(lru, "D", 3) // over cap → evict oldest (B)
	if got := join(lru); got != "C,A,D" {
		t.Fatalf("after D (cap 3) → %q, want C,A,D", got)
	}
}

func TestSubscribeViewed(t *testing.T) {
	s := New("ws://x", "k", "s", []string{"AAPL", "MSFT"}, nil, func(_ time.Time) string { return "regular" }, nil, nil, nil)
	s.Subscribe("nvda")    // lowercased + added
	s.Subscribe("AAPL")    // already base → ignored
	s.Subscribe("0700.HK") // foreign → ignored
	if got := join(s.viewed); got != "NVDA" {
		t.Fatalf("viewed = %q, want NVDA", got)
	}
	if got := join(s.desired()); got != "AAPL,MSFT,NVDA" {
		t.Fatalf("desired = %q, want AAPL,MSFT,NVDA", got)
	}
}

func join(s []string) string { return strings.Join(s, ",") }
