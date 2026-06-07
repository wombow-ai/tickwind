package yahoo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQuoteParsesMeta(t *testing.T) {
	const body = `{"chart":{"result":[{"meta":{
		"regularMarketPrice":453.2,"chartPreviousClose":459.0,"currency":"HKD",
		"shortName":"TENCENT","marketState":"REGULAR","regularMarketTime":1717000000}}]}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent (Yahoo 429s without one)")
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := New()
	c.base = srv.URL + "/"
	q, ok, err := c.Quote(context.Background(), "0700.HK")
	if err != nil || !ok {
		t.Fatalf("Quote ok=%v err=%v", ok, err)
	}
	if q.Price != 453.2 || q.PrevClose != 459.0 || q.Currency != "HKD" || q.Name != "TENCENT" {
		t.Errorf("got %+v", q)
	}
	if q.MarketState != "REGULAR" || q.At.IsZero() {
		t.Errorf("marketState=%q at=%v", q.MarketState, q.At)
	}
}

func TestQuotePrevCloseFallback(t *testing.T) {
	// No chartPreviousClose → fall back to previousClose.
	const body = `{"chart":{"result":[{"meta":{"regularMarketPrice":100,"previousClose":98}}]}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	c := New()
	c.base = srv.URL + "/"
	q, ok, _ := c.Quote(context.Background(), "X.HK")
	if !ok || q.PrevClose != 98 {
		t.Errorf("prevClose=%v ok=%v, want 98/true", q.PrevClose, ok)
	}
}

func TestQuoteEmptyOrNoPrice(t *testing.T) {
	for _, body := range []string{
		`{"chart":{"result":[]}}`,
		`{"chart":{"result":[{"meta":{"regularMarketPrice":0}}]}}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		c := New()
		c.base = srv.URL + "/"
		_, ok, err := c.Quote(context.Background(), "BOGUS.HK")
		srv.Close()
		if ok || err != nil {
			t.Errorf("body=%s: ok=%v err=%v, want ok=false err=nil", body, ok, err)
		}
	}
}
