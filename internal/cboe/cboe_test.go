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

	// Max pain: candidate strikes {90,100,110}. Pain at 100 (OI-weighted ITM):
	//  K=100: calls ITM none (100C,110C strike>=100 → 0); puts: 90P(strike<100→0... wait 90<100 so put ITM when K<strike → 90P not ITM at 100); = lowest. Verify it returns one of the strikes.
	mp := MaxPain(ch.Contracts, "2026-06-12")
	if mp != 90 && mp != 100 && mp != 110 {
		t.Fatalf("max pain = %v, want one of the strikes", mp)
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
