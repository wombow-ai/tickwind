package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/alpaca"
)

// TestLatestQuote_DailyCandleFallback is the regression test for the "new IPO
// shows no price on the cards but the K-line has it" bug. A brand-new / very
// thin listing has NO live IEX trade in the snapshot (latestTrade empty) and no
// consolidated-tape fallback, so the live quote price is 0 — yet the daily-bar
// path (the /bars endpoint backing the K-line) returns real candles. LatestQuote
// must fall back to the latest REAL daily close so the detail-card PriceTag and
// market cap populate, labeled as a closed (non-live) as-of-the-candle-date price.
func TestLatestQuote_DailyCandleFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/snapshot"):
			// Brand-new IPO: no IEX trade, no daily bar, no prev bar → price 0.
			_, _ = w.Write([]byte(`{"latestTrade":{"p":0},"dailyBar":{"c":0},"prevDailyBar":{"c":0}}`))
		case strings.Contains(r.URL.Path, "/bars"):
			// The candle path DOES have bars (newest-first, sort=desc): the most
			// recent real close is 42.50 on 2026-06-12.
			_, _ = w.Write([]byte(`{"bars":[
				{"t":"2026-06-12T00:00:00Z","o":41,"h":43,"l":40,"c":42.5,"v":1000},
				{"t":"2026-06-11T00:00:00Z","o":39,"h":41,"l":38,"c":40.0,"v":900}
			]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := alpaca.New("k", "s", srv.URL, "iex")
	bc := NewBarCache(client, 30, time.Minute)

	q, ok, err := bc.LatestQuote(context.Background(), "SPCX")
	if err != nil {
		t.Fatalf("LatestQuote: %v", err)
	}
	if !ok {
		t.Fatal("LatestQuote ok=false — the daily-candle fallback did not fire (cards would stay empty)")
	}
	if q.Price != 42.5 {
		t.Errorf("Price = %v; want 42.5 (latest real daily close)", q.Price)
	}
	if q.Source != "daily" {
		t.Errorf("Source = %q; want %q (must not be mislabeled as a live trade)", q.Source, "daily")
	}
	if q.Session != "closed" {
		t.Errorf("Session = %q; want %q (as-of the candle date, not live)", q.Session, "closed")
	}
	if !q.At.Equal(time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("At = %v; want the candle date 2026-06-12", q.At)
	}
	// No phantom day-change: prev_close anchored to the close itself.
	if q.PrevClose != 42.5 || q.RegularClose != 42.5 {
		t.Errorf("prev/regular close = %v/%v; want both 42.5 (no phantom move)", q.PrevClose, q.RegularClose)
	}
}

// TestLatestQuote_NoCandlesStaysEmpty proves the fallback can't fabricate: when
// there's no live trade AND no daily candles either, LatestQuote stays empty
// (ok=false) exactly as before — the card renders "—".
func TestLatestQuote_NoCandlesStaysEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/snapshot"):
			_, _ = w.Write([]byte(`{"latestTrade":{"p":0},"dailyBar":{"c":0},"prevDailyBar":{"c":0}}`))
		case strings.Contains(r.URL.Path, "/bars"):
			_, _ = w.Write([]byte(`{"bars":[]}`)) // no candles either
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := alpaca.New("k", "s", srv.URL, "iex")
	bc := NewBarCache(client, 30, time.Minute)

	if _, ok, err := bc.LatestQuote(context.Background(), "NADA"); err != nil || ok {
		t.Fatalf("LatestQuote ok=%v err=%v; want ok=false, nil err (no real price → stay empty, never fabricate)", ok, err)
	}
}
