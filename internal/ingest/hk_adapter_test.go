package ingest

import (
	"context"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/market"
	"github.com/wombow-ai/tickwind/internal/yahoo"
)

type fakeYahoo struct {
	q     yahoo.Quote
	ok    bool
	calls int
}

func (f *fakeYahoo) Quote(ctx context.Context, symbol string) (yahoo.Quote, bool, error) {
	f.calls++
	return f.q, f.ok, nil
}

func TestHKAdapterQuoteFilingsAndCache(t *testing.T) {
	f := &fakeYahoo{
		q:  yahoo.Quote{Price: 453.2, PrevClose: 459, Name: "TENCENT", MarketState: "REGULAR", At: time.Now()},
		ok: true,
	}
	a := &HKAdapter{yahoo: f, ttl: time.Minute, cache: map[string]hkEntry{}}

	q, ok, err := a.Quote(context.Background(), "0700.HK")
	if err != nil || !ok {
		t.Fatalf("Quote ok=%v err=%v", ok, err)
	}
	if q.Price != 453.2 || q.PrevClose != 459 || q.Session != "regular" || q.Source != "yahoo" || q.Ticker != "0700.HK" {
		t.Errorf("quote=%+v", q)
	}

	// Second call within the TTL is served from cache (no extra Yahoo hit).
	if _, _, err := a.Quote(context.Background(), "0700.HK"); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 {
		t.Errorf("yahoo calls=%d, want 1 (cached)", f.calls)
	}

	// Filings surfaces the Security (name + market) from the same cached entry.
	sec, filings, ok, err := a.Filings(context.Background(), "0700.HK")
	if err != nil || !ok || sec.Name != "TENCENT" || sec.Market != string(market.HK) || filings != nil {
		t.Errorf("filings sec=%+v filings=%v ok=%v err=%v", sec, filings, ok, err)
	}
}

func TestHKAdapterUnknownTicker(t *testing.T) {
	a := &HKAdapter{yahoo: &fakeYahoo{ok: false}, ttl: time.Minute, cache: map[string]hkEntry{}}
	if _, ok, err := a.Quote(context.Background(), "9999.HK"); ok || err != nil {
		t.Errorf("unknown: ok=%v err=%v, want ok=false err=nil", ok, err)
	}
}

func TestYahooSessionMapping(t *testing.T) {
	cases := map[string]struct {
		want  string
		known bool
	}{
		"REGULAR": {"regular", true},
		"PRE":     {"pre", true},
		"POST":    {"post", true},
		"CLOSED":  {"closed", true},
		"":        {"", false},
		"WEIRD":   {"", false},
	}
	for state, c := range cases {
		got, known := yahooSession(state)
		if got != c.want || known != c.known {
			t.Errorf("yahooSession(%q)=(%q,%v), want (%q,%v)", state, got, known, c.want, c.known)
		}
	}
}
