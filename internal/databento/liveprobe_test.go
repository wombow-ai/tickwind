package databento

import (
	"bufio"
	"net"
	"os"
	"testing"
	"time"
)

// TestLiveProbe is a LIVE end-to-end protocol validation against the real Databento
// gateway. It is SKIPPED unless DATABENTO_API_KEY is set, so it never runs in CI or a
// normal `go test ./...`. Run explicitly:
//
//	DATABENTO_API_KEY=db-... go test ./internal/databento -run TestLiveProbe -v
//
// It proves, in one shot, that: CRAM auth works, subscribe + start_session work, the
// DBN metadata + record framing parse, the decoders run on real bytes, and — most
// importantly — decoded prices have a PLAUSIBLE MAGNITUDE (a 1e9-scale or UNDEF-sentinel
// bug would make prices ~1e9× too big or ~9.2e9). Minimal billable quota: a few liquid
// symbols for ~12s. Extended-hours windows show the richest data; even regular/overnight
// proves the protocol (symbol mappings always arrive at session start).
func TestLiveProbe(t *testing.T) {
	key := os.Getenv("DATABENTO_API_KEY")
	if key == "" {
		t.Skip("DATABENTO_API_KEY unset — skipping live protocol probe")
	}
	symbols := []string{"AAPL", "MSFT", "NVDA"}

	conn, err := net.DialTimeout("tcp", DefaultHost, 10*time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", DefaultHost, err)
	}
	defer conn.Close()
	r := bufio.NewReader(conn)

	if err := authenticate(conn, r, key, DefaultDataset); err != nil {
		t.Fatalf("CRAM AUTH FAILED: %v", err)
	}
	t.Log("✓ CRAM auth succeeded")
	if err := subscribe(conn, DefaultSchema, symbols); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := startSession(conn); err != nil {
		t.Fatalf("start_session: %v", err)
	}
	if err := readMetadata(r); err != nil {
		t.Fatalf("metadata header: %v", err)
	}
	t.Log("✓ DBN metadata header parsed (magic + version 3)")

	symMap := map[uint32]string{}
	var mappings, bbos, trades, sysmsgs int
	maxPrice := 0.0
	deadline := time.Now().Add(12 * time.Second)
	_ = conn.SetReadDeadline(deadline)
	for time.Now().Before(deadline) {
		rec, err := nextRecord(r)
		if err != nil {
			break // read deadline or EOF
		}
		switch rec.rtype {
		case rtypeSymbolMapping:
			if id, sym, ok := decodeSymbolMapping(rec.body); ok {
				symMap[id] = sym
				mappings++
			}
		case rtypeBbo:
			if q, ok := decodeBbo(rec.body); ok {
				bbos++
				for _, p := range []float64{q.Last, q.Bid, q.Ask} {
					if p > maxPrice {
						maxPrice = p
					}
				}
				if bbos <= 8 {
					t.Logf("  BBO %-6s last=%.2f bid=%.2f ask=%.2f (%d x %d)", symMap[q.InstrumentID], q.Last, q.Bid, q.Ask, q.BidSize, q.AskSize)
				}
			}
		case rtypeTrade:
			if tr, ok := decodeTrade(rec.body); ok {
				trades++
				if tr.Price > maxPrice {
					maxPrice = tr.Price
				}
			}
		case rtypeSystem:
			sysmsgs++
		case rtypeError:
			end := 318
			if end > len(rec.body) {
				end = len(rec.body)
			}
			t.Logf("  ERROR record: %q", cstr(rec.body[16:end]))
		}
	}
	t.Logf("=== probe: %d mappings · %d bbo · %d trades · %d system · maxPrice=%.2f ===", mappings, bbos, trades, sysmsgs, maxPrice)
	if mappings == 0 {
		t.Error("no symbol mappings received — auth ok but framing/decode is likely wrong")
	}
	if maxPrice > 1e7 {
		t.Errorf("implausible max price %.2f — the 1e9 fixed-point scale is wrong", maxPrice)
	}
}
