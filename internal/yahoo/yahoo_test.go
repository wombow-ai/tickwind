package yahoo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestExtendedQuoteLastPrint(t *testing.T) {
	// Regular close 17.09, then post-market prints up to 18.06, with a trailing
	// null (the current minute hasn't printed yet). ExtendedQuote must return the
	// last NON-null print (18.06) — the meta block would only show the frozen
	// 17.09 close — with its timestamp and the prior regular close.
	const body = `{"chart":{"result":[{
		"meta":{"chartPreviousClose":14.87,"regularMarketPrice":17.09},
		"timestamp":[1781208000,1781211600,1781215200,1781218800],
		"indicators":{"quote":[{"close":[17.0,17.09,18.06,null]}]}}]}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.RawQuery; got == "" || !contains(got, "includePrePost=true") {
			t.Errorf("query %q missing includePrePost", got)
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := New()
	c.base = srv.URL + "/"
	price, prev, at, ok, err := c.ExtendedQuote(context.Background(), "RDW")
	if err != nil || !ok {
		t.Fatalf("ExtendedQuote ok=%v err=%v", ok, err)
	}
	if price != 18.06 {
		t.Errorf("price=%v, want 18.06 (last non-null post-market print)", price)
	}
	if prev != 14.87 {
		t.Errorf("prevClose=%v, want 14.87", prev)
	}
	if at.Unix() != 1781215200 {
		t.Errorf("at=%v (unix %d), want unix 1781215200", at, at.Unix())
	}
}

func TestExtendedQuoteNoData(t *testing.T) {
	for _, body := range []string{
		`{"chart":{"result":[]}}`,
		`{"chart":{"result":[{"indicators":{"quote":[{"close":[null,null]}]}}]}}`,
		`{"chart":{"result":[{"indicators":{"quote":[]}}]}}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		c := New()
		c.base = srv.URL + "/"
		_, _, _, ok, err := c.ExtendedQuote(context.Background(), "BOGUS")
		srv.Close()
		if ok || err != nil {
			t.Errorf("body=%s: ok=%v err=%v, want ok=false err=nil", body, ok, err)
		}
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

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
