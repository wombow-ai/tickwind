package alpacaws

import (
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
	trades, note := parseTrades(data)
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
	// Control messages surface in the note for logging.
	if note == "" {
		t.Error("expected a note from control messages")
	}
}

func TestParseTradesError(t *testing.T) {
	trades, note := parseTrades([]byte(`[{"T":"error","code":406,"msg":"connection limit exceeded"}]`))
	if len(trades) != 0 {
		t.Fatalf("got %d trades, want 0", len(trades))
	}
	if note == "" {
		t.Error("expected error note")
	}
}

func TestNewCapsSymbols(t *testing.T) {
	syms := make([]string, 50)
	for i := range syms {
		syms[i] = "T"
	}
	s := New("ws://x", "k", "s", syms, nil, func(_ time.Time) string { return "regular" }, nil, nil, nil)
	if len(s.symbols) != MaxSymbols {
		t.Fatalf("symbols capped to %d, want %d", len(s.symbols), MaxSymbols)
	}
}
