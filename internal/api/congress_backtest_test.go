package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/auth"
	"github.com/wombow-ai/tickwind/internal/congress"
	"github.com/wombow-ai/tickwind/internal/congress/ptr"
	"github.com/wombow-ai/tickwind/internal/enrich"
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/store/memory"
	"github.com/wombow-ai/tickwind/internal/stream"
)

// candleBarSource serves canned OHLC candles per ticker for the backtest handler.
type candleBarSource struct {
	candles map[string][]store.Candle
}

func (c *candleBarSource) DailyBars(context.Context, string) ([]float64, error) { return nil, nil }
func (c *candleBarSource) DailyCandles(_ context.Context, ticker string) ([]store.Candle, error) {
	return c.candles[ticker], nil
}
func (c *candleBarSource) IntradayCandles(context.Context, string, string) ([]store.Candle, error) {
	return nil, nil
}
func (c *candleBarSource) LatestQuote(context.Context, string) (store.Quote, bool, error) {
	return store.Quote{}, false, nil
}

// fakeCongressTx is a minimal CongressTxSource for the member backtest.
type fakeCongressTx struct {
	members map[string]congress.MemberTx
}

func (f *fakeCongressTx) ByTicker(string) []congress.TickerTrade { return nil }
func (f *fakeCongressTx) ByMember(slug string) (congress.MemberTx, bool) {
	m, ok := f.members[slug]
	return m, ok
}

func serverWithBacktest(bars BarSource, ctx CongressTxSource) *httptest.Server {
	h := New(
		memory.New(), stream.NewHub(), enrich.Noop{},
		auth.NewVerifier(testSecret, ""),
		bars,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	h.SetCongressTx(ctx)
	return httptest.NewServer(h)
}

func ohlc(start time.Time, closes ...float64) []store.Candle {
	cs := make([]store.Candle, len(closes))
	for i, c := range closes {
		cs[i] = store.Candle{Time: start.AddDate(0, 0, i), Close: c}
	}
	return cs
}

type backtestEnvelope struct {
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Backtest struct {
		Insufficient    bool    `json:"insufficient"`
		MemberReturnPct float64 `json:"member_return_pct"`
		SpyReturnPct    float64 `json:"spy_return_pct"`
		TradesUsed      int     `json:"trades_used"`
		TradesSkipped   int     `json:"trades_skipped"`
		Curve           []struct {
			Date string `json:"date"`
		} `json:"curve"`
	} `json:"backtest"`
}

func getBacktest(t *testing.T, srv *httptest.Server, slug string) backtestEnvelope {
	t.Helper()
	resp, err := http.Get(srv.URL + "/v1/congress/member/" + slug + "/backtest")
	if err != nil {
		t.Fatalf("GET backtest: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (the endpoint must never error)", resp.StatusCode)
	}
	var env backtestEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return env
}

func TestCongressBacktestEndpoint(t *testing.T) {
	day := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	// A member who bought AAPL; AAPL rises, SPY is flat.
	members := map[string]congress.MemberTx{
		"jane-doe": {
			Slug: "jane-doe", Name: "Jane Doe", State: "CA",
			Transactions: []ptr.Transaction{
				{Ticker: "AAPL", Type: ptr.TxPurchase, TxDate: day},
			},
		},
	}
	bars := &candleBarSource{candles: map[string][]store.Candle{
		"AAPL": ohlc(day, 100, 110, 120, 130, 140, 150, 160, 170, 180, 200),
		"SPY":  ohlc(day, 400, 400, 400, 400, 400, 400, 400, 400, 400, 400),
	}}
	srv := serverWithBacktest(bars, &fakeCongressTx{members: members})
	defer srv.Close()

	env := getBacktest(t, srv, "jane-doe")
	if env.Slug != "jane-doe" || env.Name != "Jane Doe" {
		t.Errorf("identity = %q/%q, want jane-doe/Jane Doe", env.Slug, env.Name)
	}
	if env.Backtest.Insufficient {
		t.Fatalf("Insufficient = true, want a real result")
	}
	if env.Backtest.MemberReturnPct <= env.Backtest.SpyReturnPct {
		t.Errorf("member %v should beat SPY %v", env.Backtest.MemberReturnPct, env.Backtest.SpyReturnPct)
	}
	if env.Backtest.TradesUsed != 1 {
		t.Errorf("TradesUsed = %d, want 1", env.Backtest.TradesUsed)
	}
	if len(env.Backtest.Curve) == 0 {
		t.Errorf("Curve is empty; want at least one point")
	}
}

func TestCongressBacktestUnknownMember(t *testing.T) {
	srv := serverWithBacktest(&candleBarSource{}, &fakeCongressTx{members: map[string]congress.MemberTx{}})
	defer srv.Close()
	env := getBacktest(t, srv, "nobody") // unknown slug → 200 + insufficient, NOT 404/500
	if !env.Backtest.Insufficient {
		t.Errorf("unknown member should be insufficient, got %+v", env.Backtest)
	}
}

func TestCongressBacktestNilSources(t *testing.T) {
	// No congressTx injected and a nil bar source → still 200 + insufficient.
	h := New(
		memory.New(), stream.NewHub(), enrich.Noop{},
		auth.NewVerifier(testSecret, ""),
		nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	srv := httptest.NewServer(h)
	defer srv.Close()
	env := getBacktest(t, srv, "jane-doe")
	if !env.Backtest.Insufficient {
		t.Errorf("nil sources should be insufficient, got %+v", env.Backtest)
	}
}
