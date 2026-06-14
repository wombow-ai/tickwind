package cboe

import "testing"

func TestDecodeOCC(t *testing.T) {
	cases := []struct {
		sym, typ, expiry string
		strike           float64
		ok               bool
	}{
		{"AAPL260612C00110000", "C", "2026-06-12", 110, true},
		{"AAPL260612P00250500", "P", "2026-06-12", 250.5, true},
		{"BRKB261218C00500000", "C", "2026-12-18", 500, true}, // multi-char root
		{"SPXW260101C05000000", "C", "2026-01-01", 5000, true},
		{"GARBAGE", "", "", 0, false},
		{"AAPL260612X00110000", "", "", 0, false}, // bad type
	}
	for _, c := range cases {
		typ, strike, expiry, ok := decodeOCC(c.sym)
		if ok != c.ok || (ok && (typ != c.typ || strike != c.strike || expiry != c.expiry)) {
			t.Errorf("decodeOCC(%q) = (%q,%v,%q,%v), want (%q,%v,%q,%v)",
				c.sym, typ, strike, expiry, ok, c.typ, c.strike, c.expiry, c.ok)
		}
	}
}

// sampleBody mirrors the Cboe CDN shape (trimmed, real field names).
const sampleBody = `{"timestamp":"2026-06-11 19:34:28","data":{"options":[
  {"option":"XYZ260612C00100000","iv":0.5,"open_interest":1000,"volume":50},
  {"option":"XYZ260612P00100000","iv":0.6,"open_interest":500,"volume":200},
  {"option":"XYZ260612C00110000","iv":0.4,"open_interest":2000,"volume":10},
  {"option":"XYZ260612P00090000","iv":0.7,"open_interest":300,"volume":5}
]}}`

func TestParseAndCompute(t *testing.T) {
	ch, ok, err := parseOptions([]byte(sampleBody))
	if err != nil || !ok {
		t.Fatalf("parseOptions ok=%v err=%v", ok, err)
	}
	if len(ch.Contracts) != 4 {
		t.Fatalf("got %d contracts, want 4", len(ch.Contracts))
	}
	if ch.At.IsZero() {
		t.Error("timestamp not parsed")
	}

	// P/C by volume = (200+5)/(50+10)=205/60≈3.417; by OI = (500+300)/(1000+2000)=800/3000≈0.267.
	pv, po := PutCallRatio(ch.Contracts)
	if pv < 3.4 || pv > 3.45 {
		t.Errorf("pc by volume = %v, want ~3.417", pv)
	}
	if po < 0.26 || po > 0.27 {
		t.Errorf("pc by OI = %v, want ~0.267", po)
	}

	exp := NearestExpiry(ch.Contracts, "2026-06-01")
	if exp != "2026-06-12" {
		t.Fatalf("nearest expiry = %q, want 2026-06-12", exp)
	}

	// Max pain over the 3 distinct OI-bearing strikes {90,100,110}:
	//  K=90:  calls ITM none; puts ITM 100P→500*(100-90)=5000 → 5000.
	//  K=100: calls ITM none; puts ITM none → 0 (unique minimum).
	//  K=110: calls ITM 100C→1000*(110-100)=10000; puts ITM none → 10000.
	// The minimum is unique at K=100, so the deterministic result is exactly 100.
	mp := MaxPain(ch.Contracts, "2026-06-12")
	if mp != 100 {
		t.Fatalf("max pain = %v, want 100 (unique minimum)", mp)
	}

	top := OITop(ch.Contracts, 2)
	if len(top) != 2 || top[0].OI != 2000 || top[1].OI != 1000 {
		t.Fatalf("OITop = %+v, want [2000,1000]", top)
	}
}

func TestParseEmpty(t *testing.T) {
	if _, ok, _ := parseOptions([]byte(`{"data":{"options":[]}}`)); ok {
		t.Error("empty options should yield ok=false")
	}
}

// TestMaxPainDeterministicTie builds a symmetric chain whose total-pain valley
// is flat across several strikes (a tie), then runs MaxPain many times. Go
// randomizes map iteration, so a pre-sort+lower-strike tie-break is required for
// the result to be stable across runs (and thus across cache/restart boundaries).
func TestMaxPainDeterministicTie(t *testing.T) {
	// A genuine flat valley: total pain ties at the minimum across two adjacent
	// strikes (105 and 110). The lower strike (105) must win deterministically
	// regardless of map iteration order.
	flatTie := []Contract{
		{Type: "P", Strike: 90, Expiry: "2026-06-12", OI: 1}, // ITM only for K<90
		{Type: "C", Strike: 100, Expiry: "2026-06-12", OI: 0},
		{Type: "C", Strike: 100, Expiry: "2026-06-12", OI: 7}, // OI strike 100
		{Type: "C", Strike: 110, Expiry: "2026-06-12", OI: 0},
		{Type: "P", Strike: 110, Expiry: "2026-06-12", OI: 7}, // OI strike 110
		{Type: "P", Strike: 105, Expiry: "2026-06-12", OI: 7}, // OI strike 105
	}
	// OI-bearing strikes = {90, 100, 105, 110}. Pain:
	//  K=90:  100C no; put90 no (90<90 false); put110 ITM 7*(110-90)=140; put105 ITM 7*(105-90)=105 → 245.
	//  K=100: 100C no (100>100 false); put110 7*10=70; put105 7*5=35 → 105.
	//  K=105: 100C ITM 7*5=35; put110 7*5=35; put105 no → 70.
	//  K=110: 100C ITM 7*10=70; put110 no; put105 no → 70.
	// MINIMUM = 70, attained at BOTH K=105 and K=110 (a genuine tie). Lower strike
	// (105) must win deterministically.
	const wantTie = 105.0
	first := MaxPain(flatTie, "2026-06-12")
	if first != wantTie {
		t.Fatalf("tie-valley max pain = %v, want %v (lower of the tied strikes)", first, wantTie)
	}
	for i := 0; i < 500; i++ {
		if got := MaxPain(flatTie, "2026-06-12"); got != wantTie {
			t.Fatalf("run %d: max pain = %v, want stable %v across runs", i, got, wantTie)
		}
	}
}

// TestMaxPainSingleStrikeGuard verifies the insufficient-not-wrong guard: a thin
// expiry with fewer than minMaxPainStrikes distinct OI-bearing strikes returns 0
// (so optionsFacts omits the fact) instead of a meaningless 0-pain magnet.
func TestMaxPainSingleStrikeGuard(t *testing.T) {
	one := []Contract{
		{Type: "C", Strike: 50, Expiry: "2026-07-17", OI: 100},
		{Type: "P", Strike: 50, Expiry: "2026-07-17", OI: 80}, // same strike → still 1 distinct
	}
	if got := MaxPain(one, "2026-07-17"); got != 0 {
		t.Fatalf("single-strike max pain = %v, want 0 (below minMaxPainStrikes)", got)
	}
	two := []Contract{
		{Type: "C", Strike: 50, Expiry: "2026-07-17", OI: 100},
		{Type: "P", Strike: 60, Expiry: "2026-07-17", OI: 80},
	}
	if got := MaxPain(two, "2026-07-17"); got != 0 {
		t.Fatalf("two-strike max pain = %v, want 0 (below minMaxPainStrikes=%d)", got, minMaxPainStrikes)
	}
	// Strikes present but with zero OI must NOT count toward the threshold.
	zeroOI := []Contract{
		{Type: "C", Strike: 50, Expiry: "2026-07-17", OI: 0},
		{Type: "C", Strike: 60, Expiry: "2026-07-17", OI: 0},
		{Type: "C", Strike: 70, Expiry: "2026-07-17", OI: 100}, // only 1 OI-bearing strike
	}
	if got := MaxPain(zeroOI, "2026-07-17"); got != 0 {
		t.Fatalf("zero-OI-padding max pain = %v, want 0 (only 1 distinct OI strike)", got)
	}
}

// TestMaxPainUniqueMinimum is a normal multi-strike chain with a single clear
// minimum; the deterministic result must equal that unique minimizer (happy-path
// regression guard).
func TestMaxPainUniqueMinimum(t *testing.T) {
	cs := []Contract{
		{Type: "C", Strike: 90, Expiry: "2026-06-12", OI: 1000},
		{Type: "P", Strike: 90, Expiry: "2026-06-12", OI: 100},
		{Type: "C", Strike: 100, Expiry: "2026-06-12", OI: 1500},
		{Type: "P", Strike: 100, Expiry: "2026-06-12", OI: 1500},
		{Type: "C", Strike: 110, Expiry: "2026-06-12", OI: 100},
		{Type: "P", Strike: 110, Expiry: "2026-06-12", OI: 1000},
	}
	// OI-bearing strikes {90,100,110}. Symmetric heavy OI at 100 pins the magnet
	// to 100:
	//  K=90:  put100 ITM 1500*10=15000; put110 ITM 1000*20=20000 → 35000.
	//  K=100: call90 ITM 1000*10=10000; put110 ITM 1000*10=10000 → 20000 (min).
	//  K=110: call90 ITM 1000*20=20000; call100 ITM 1500*10=15000; put90 no → 35000.
	if got := MaxPain(cs, "2026-06-12"); got != 100 {
		t.Fatalf("unique-minimum max pain = %v, want 100", got)
	}
	for i := 0; i < 100; i++ {
		if got := MaxPain(cs, "2026-06-12"); got != 100 {
			t.Fatalf("run %d: max pain = %v, want stable 100", i, got)
		}
	}
}
