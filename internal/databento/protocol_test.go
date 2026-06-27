package databento

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"testing"
)

func putU32(b []byte, off int, v uint32) { binary.LittleEndian.PutUint32(b[off:off+4], v) }
func putI64(b []byte, off int, v int64)  { binary.LittleEndian.PutUint64(b[off:off+8], uint64(v)) }
func putU64(b []byte, off int, v uint64) { binary.LittleEndian.PutUint64(b[off:off+8], v) }

func TestCramResponse(t *testing.T) {
	const key = "0123456789abcdef0123456789abXYZ!" // exactly 32 chars
	const challenge = "deadbeefchallenge"
	got, err := cramResponse(challenge, key)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(challenge + "|" + key))
	want := hex.EncodeToString(sum[:]) + "-" + key[27:32]
	if got != want {
		t.Fatalf("cramResponse = %q; want %q", got, want)
	}
	if len(got) != 70 || got[64] != '-' {
		t.Fatalf("shape wrong: %q (want 64 hex + '-' + 5-char bucket)", got)
	}
	if got[65:] != key[27:32] {
		t.Fatalf("bucket = %q; want last 5 key chars %q", got[65:], key[27:32])
	}
	if _, err := cramResponse("x", "tooshort"); err == nil {
		t.Fatal("want error for a non-32-char key")
	}
}

func TestDecodeBbo(t *testing.T) {
	b := make([]byte, 80)
	b[0] = 20 // length in 32-bit words (80/4)
	b[1] = rtypeBbo
	putU32(b, 4, 42)                         // instrument_id
	putI64(b, 16, 277_000_000_000)           // last $277.00
	putU32(b, 24, 33)                        // last size
	putU64(b, 32, 1_700_000_000_000_000_000) // ts_recv
	putI64(b, 48, 276_950_000_000)           // bid $276.95
	putI64(b, 56, 277_050_000_000)           // ask $277.05
	putU32(b, 64, 100)                       // bid_sz
	putU32(b, 68, 200)                       // ask_sz

	q, ok := decodeBbo(b)
	if !ok {
		t.Fatal("decodeBbo ok=false")
	}
	if q.InstrumentID != 42 || q.Last != 277.00 || q.Bid != 276.95 || q.Ask != 277.05 || q.BidSize != 100 || q.AskSize != 200 {
		t.Fatalf("decodeBbo = %+v", q)
	}
	if q.TsRecv.IsZero() {
		t.Fatal("ts_recv should be set")
	}
	// UNDEF bid → 0 (never a real price)
	putI64(b, 48, undefPrice)
	if q2, _ := decodeBbo(b); q2.Bid != 0 {
		t.Fatalf("UNDEF bid must map to 0, got %v", q2.Bid)
	}
	if _, ok := decodeBbo(b[:60]); ok {
		t.Fatal("short buffer should be ok=false")
	}
}

func TestDecodeTrade(t *testing.T) {
	b := make([]byte, 48)
	b[0] = 12
	b[1] = rtypeTrade
	putU32(b, 4, 7)
	putI64(b, 16, 100_250_000_000) // $100.25
	putU32(b, 24, 50)
	putU64(b, 32, 1_700_000_000_000_000_000)
	tr, ok := decodeTrade(b)
	if !ok || tr.InstrumentID != 7 || tr.Price != 100.25 || tr.Size != 50 || tr.TsRecv.IsZero() {
		t.Fatalf("decodeTrade = %+v ok=%v", tr, ok)
	}
}

func TestDecodeSymbolMapping(t *testing.T) {
	b := make([]byte, 176)
	b[0] = 44
	b[1] = rtypeSymbolMapping
	putU32(b, 4, 42)
	copy(b[89:160], "AAPL\x00rest-is-padding")
	id, sym, ok := decodeSymbolMapping(b)
	if !ok || id != 42 || sym != "AAPL" {
		t.Fatalf("decodeSymbolMapping = (%d,%q,%v); want (42,\"AAPL\",true)", id, sym, ok)
	}
}

func TestFieldEquals(t *testing.T) {
	if !fieldEquals("success=1|session_id=abc\n", "success", "1") {
		t.Fatal("should match success=1")
	}
	if fieldEquals("success=0|error=nope", "success", "1") {
		t.Fatal("should not match success=0")
	}
	if !fieldEquals("a=b|error=bad key here", "error", "bad key here") {
		t.Fatal("should match a value containing spaces")
	}
}

func TestPriceScale(t *testing.T) {
	if price(undefPrice) != 0 {
		t.Fatal("UNDEF must be 0")
	}
	if price(277_000_000_000) != 277.0 {
		t.Fatalf("scale wrong: %v (want 277.0)", price(277_000_000_000))
	}
}
