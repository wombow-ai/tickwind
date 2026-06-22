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
	"github.com/wombow-ai/tickwind/internal/enrich"
	"github.com/wombow-ai/tickwind/internal/indicators"
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/store/memory"
	"github.com/wombow-ai/tickwind/internal/stream"
)

func fptr(v float64) *float64 { return &v }

func mustGet(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

// tripleSignalResult is a fixed compute result that triggers exactly 3 posture
// signals (RSI oversold, KDJ overbought, MACD bullish) — enough to exercise the
// freeSignalTeaserLimit=2 truncation (2 shown, 1 locked).
func tripleSignalResult() indicators.StockIndicatorsResult {
	return indicators.StockIndicatorsResult{
		Ticker: "AAPL",
		AsOf:   "2026-06-19",
		Indicators: []indicators.StockIndicator{
			{Indicator: indicators.Indicator{ID: "technical.rsi"}, Status: indicators.StatusOK, Value: fptr(25)},
			{Indicator: indicators.Indicator{ID: "technical.stochastic-kdj"}, Status: indicators.StatusOK, Value: fptr(90), Extra: map[string]float64{"k": 90}},
			{Indicator: indicators.Indicator{ID: "technical.macd"}, Status: indicators.StatusOK, Value: fptr(2), Extra: map[string]float64{"signal": 1, "hist": 1}},
		},
	}
}

// quadSignalResult is tripleSignalResult + a price-vs-SMA signal (4 total), enough to
// exceed freeSignalTeaserLimit=3 and exercise the teaser truncation.
func quadSignalResult() indicators.StockIndicatorsResult {
	r := tripleSignalResult()
	r.Price = fptr(100)
	r.Indicators = append(r.Indicators, indicators.StockIndicator{
		Indicator: indicators.Indicator{ID: "technical.sma-ma"}, Status: indicators.StatusOK, Value: fptr(90),
	})
	return r
}

// signalsTestServer builds a test server with a per-stock compute source and an
// explicit signals-paywall flag, so both the flag-off and flag-on paths are testable.
func signalsTestServer(t *testing.T, src IndicatorComputeSource, paywallOn bool) *httptest.Server {
	t.Helper()
	h := New(
		memory.New(), stream.NewHub(), enrich.Noop{},
		auth.NewVerifier(testSecret, ""),
		nil,                // bars
		nil, nil, nil, nil, // topic, opportunity, universe, guru
		nil, nil, nil, nil, nil, // ingestor, symbols, events, fundamentals, earnings
		nil, nil, nil, nil, nil, nil, // congress, institutional, live, indices, short, briefing
		nil, nil, // options, 13f
		nil, // admin user ids
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if src != nil {
		h.SetIndicatorCompute(src)
	}
	h.SetIndicatorsPaywallEnabled(paywallOn)
	return httptest.NewServer(h)
}

type stockSignalsBody struct {
	Ticker        string              `json:"ticker"`
	AsOf          string              `json:"as_of"`
	Signals       []indicators.Signal `json:"signals"`
	TotalSignals  int                 `json:"total_signals"`
	PaywallLocked bool                `json:"paywall_locked"`
}

func getIndicatorSignals(t *testing.T, srv *httptest.Server, ticker string) (int, stockSignalsBody) {
	t.Helper()
	resp, err := http.Get(srv.URL + "/v1/stocks/" + ticker + "/indicator-signals")
	if err != nil {
		t.Fatalf("GET signals: %v", err)
	}
	defer resp.Body.Close()
	var body stockSignalsBody
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return resp.StatusCode, body
}

func TestGetStockSignalsNilSource(t *testing.T) {
	srv := signalsTestServer(t, nil, false)
	defer srv.Close()
	code, _ := getIndicatorSignals(t, srv, "AAPL")
	if code != http.StatusNotFound {
		t.Fatalf("nil source: status = %d, want 404", code)
	}
}

func TestGetStockSignalsEmpty404(t *testing.T) {
	// Nothing computed at all → 404 (unknown/non-US ticker).
	src := fakeIndicatorCompute{res: indicators.StockIndicatorsResult{
		Ticker: "ZZZZ",
		Indicators: []indicators.StockIndicator{
			{Indicator: indicators.Indicator{ID: "technical.rsi"}, Status: indicators.StatusInsufficient, Reason: "no bars"},
		},
	}}
	srv := signalsTestServer(t, src, false)
	defer srv.Close()
	code, _ := getIndicatorSignals(t, srv, "ZZZZ")
	if code != http.StatusNotFound {
		t.Fatalf("empty result: status = %d, want 404", code)
	}
}

func TestGetStockSignalsFlagOff(t *testing.T) {
	// Paywall OFF → full list for everyone (anonymous), no lock. Current-behavior-safe.
	srv := signalsTestServer(t, fakeIndicatorCompute{res: tripleSignalResult()}, false)
	defer srv.Close()
	code, body := getIndicatorSignals(t, srv, "aapl") // lowercase → handler uppercases
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if body.Ticker != "AAPL" {
		t.Errorf("ticker = %q, want AAPL", body.Ticker)
	}
	if len(body.Signals) != 3 || body.TotalSignals != 3 {
		t.Fatalf("flag off: got %d signals (total %d), want 3/3", len(body.Signals), body.TotalSignals)
	}
	if body.PaywallLocked {
		t.Error("flag off: paywall_locked must be false")
	}
	// Every signal must carry a non-empty basis (traceability / anti-hallucination).
	for _, s := range body.Signals {
		if s.Basis == "" || s.Direction == "" || s.ID == "" {
			t.Errorf("signal missing fields: %+v", s)
		}
	}
}

func TestGetStockSignalsFlagOnAnonTruncates(t *testing.T) {
	// Paywall ON + anonymous (= free) → first freeSignalTeaserLimit signals + locked,
	// but total_signals reports the full count for the "unlock N more" CTA.
	srv := signalsTestServer(t, fakeIndicatorCompute{res: quadSignalResult()}, true)
	defer srv.Close()
	code, body := getIndicatorSignals(t, srv, "AAPL")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if len(body.Signals) != freeSignalTeaserLimit {
		t.Fatalf("flag on free: got %d signals, want teaser %d", len(body.Signals), freeSignalTeaserLimit)
	}
	if body.TotalSignals != 4 {
		t.Errorf("total_signals = %d, want 4 (full count even when truncated)", body.TotalSignals)
	}
	if !body.PaywallLocked {
		t.Error("flag on free: paywall_locked must be true when signals exceed the teaser")
	}
}

// fakeScan is a stub SignalScanSource returning canned matches (the matcher itself is
// unit-tested in the indicators package; this exercises the handler's gating + shape).
type fakeScan struct{ matches []indicators.SignalMatch }

func (f fakeScan) Screen(_ indicators.SignalScreen) ([]indicators.SignalMatch, time.Time) {
	return f.matches, time.Unix(1_700_000_000, 0)
}

func screenSignalsServer(t *testing.T, scan SignalScanSource, paywallOn bool) *httptest.Server {
	t.Helper()
	h := New(
		memory.New(), stream.NewHub(), enrich.Noop{},
		auth.NewVerifier(testSecret, ""),
		nil,                // bars
		nil, nil, nil, nil, // topic, opportunity, universe, guru
		nil, nil, nil, nil, nil, // ingestor, symbols, events, fundamentals, earnings
		nil, nil, nil, nil, nil, nil, // congress, institutional, live, indices, short, briefing
		nil, nil, // options, 13f
		nil, // admin user ids
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if scan != nil {
		h.SetSignalScan(scan)
	}
	h.SetIndicatorsPaywallEnabled(paywallOn)
	return httptest.NewServer(h)
}

type screenSignalsBody struct {
	Count         int                      `json:"count"`
	Results       []indicators.SignalMatch `json:"results"`
	TotalMatches  int                      `json:"total_matches"`
	AsOf          string                   `json:"as_of"`
	PaywallLocked bool                     `json:"paywall_locked"`
}

func getScreenSignalsResp(t *testing.T, srv *httptest.Server, query string) (int, screenSignalsBody) {
	t.Helper()
	resp, err := http.Get(srv.URL + "/v1/screen/signals" + query)
	if err != nil {
		t.Fatalf("GET screen/signals: %v", err)
	}
	defer resp.Body.Close()
	var body screenSignalsBody
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return resp.StatusCode, body
}

func TestGetScreenSignals(t *testing.T) {
	matches := []indicators.SignalMatch{
		{Ticker: "AAPL", Signals: []indicators.Signal{{ID: "technical.ma-cross", Direction: indicators.DirBullish, Label: "Golden cross"}}},
		{Ticker: "MSFT", Signals: []indicators.Signal{{ID: "technical.rsi", Direction: indicators.DirBearish, Label: "RSI overbought"}}},
	}

	t.Run("nil source → 404", func(t *testing.T) {
		srv := screenSignalsServer(t, nil, false)
		defer srv.Close()
		code, _ := getScreenSignalsResp(t, srv, "")
		if code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", code)
		}
	})

	t.Run("flag off → full results + as_of", func(t *testing.T) {
		srv := screenSignalsServer(t, fakeScan{matches}, false)
		defer srv.Close()
		code, body := getScreenSignalsResp(t, srv, "")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		if body.Count != 2 || len(body.Results) != 2 {
			t.Fatalf("got count=%d results=%d, want 2/2", body.Count, len(body.Results))
		}
		if body.PaywallLocked {
			t.Error("flag off: paywall_locked must be false")
		}
		if body.AsOf == "" {
			t.Error("as_of should be set when the scan has run")
		}
	})

	t.Run("flag on + anon (free) + many → teaser of freeScreenTeaserLimit + locked", func(t *testing.T) {
		many := make([]indicators.SignalMatch, 7)
		for i := range many {
			many[i] = indicators.SignalMatch{
				Ticker:  string(rune('A' + i)),
				Signals: []indicators.Signal{{ID: "technical.rsi", Direction: indicators.DirBullish, Label: "RSI oversold"}},
			}
		}
		srv := screenSignalsServer(t, fakeScan{many}, true)
		defer srv.Close()
		code, body := getScreenSignalsResp(t, srv, "")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		if len(body.Results) != freeScreenTeaserLimit {
			t.Fatalf("flag on free: got %d results, want teaser %d", len(body.Results), freeScreenTeaserLimit)
		}
		if body.TotalMatches != 7 {
			t.Errorf("total_matches = %d, want 7 (full count even when truncated)", body.TotalMatches)
		}
		if !body.PaywallLocked {
			t.Error("flag on free + more than the teaser: paywall_locked must be true")
		}
	})

	t.Run("flag on + anon + few (≤ teaser) → full, not locked", func(t *testing.T) {
		srv := screenSignalsServer(t, fakeScan{matches}, true) // only 2 matches
		defer srv.Close()
		code, body := getScreenSignalsResp(t, srv, "")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		if len(body.Results) != 2 || body.PaywallLocked {
			t.Errorf("few matches must not lock, got %d results locked=%v", len(body.Results), body.PaywallLocked)
		}
	})
}

// fakeBars implements BarSource; only DailyCandles is exercised by the backtest.
type fakeBars struct{ candles []store.Candle }

func (f fakeBars) DailyBars(context.Context, string) ([]float64, error) { return nil, nil }
func (f fakeBars) DailyCandles(context.Context, string) ([]store.Candle, error) {
	return f.candles, nil
}
func (f fakeBars) IntradayCandles(context.Context, string, string) ([]store.Candle, error) {
	return nil, nil
}
func (f fakeBars) LatestQuote(context.Context, string) (store.Quote, bool, error) {
	return store.Quote{}, false, nil
}

func rampCandles(n int) []store.Candle {
	out := make([]store.Candle, n)
	for i := range out {
		out[i] = store.Candle{Close: 100 + float64(i)*0.1}
	}
	return out
}

func backtestServer(t *testing.T, bars BarSource, paywallOn bool) *httptest.Server {
	t.Helper()
	h := New(
		memory.New(), stream.NewHub(), enrich.Noop{},
		auth.NewVerifier(testSecret, ""),
		bars,
		nil, nil, nil, nil, // topic, opportunity, universe, guru
		nil, nil, nil, nil, nil, // ingestor, symbols, events, fundamentals, earnings
		nil, nil, nil, nil, nil, nil, // congress, institutional, live, indices, short, briefing
		nil, nil, // options, 13f
		nil, // admin user ids
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	h.SetIndicatorsPaywallEnabled(paywallOn)
	return httptest.NewServer(h)
}

func TestGetBacktest(t *testing.T) {
	t.Run("nil bars → 404", func(t *testing.T) {
		srv := backtestServer(t, nil, false)
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/stocks/AAPL/backtest?rule=golden_cross")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("invalid rule → 400", func(t *testing.T) {
		srv := backtestServer(t, fakeBars{rampCandles(300)}, false)
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/stocks/AAPL/backtest?rule=bogus")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("insufficient history → 422", func(t *testing.T) {
		srv := backtestServer(t, fakeBars{rampCandles(100)}, false)
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/stocks/AAPL/backtest?rule=golden_cross")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422", resp.StatusCode)
		}
	})

	t.Run("flag off → 200 with result", func(t *testing.T) {
		srv := backtestServer(t, fakeBars{rampCandles(300)}, false)
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/stocks/aapl/backtest?rule=golden_cross&horizon=15")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var body backtestResp
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Ticker != "AAPL" || body.Result == nil {
			t.Fatalf("want AAPL + a result, got %+v", body)
		}
		if body.Result.Horizon != 15 || body.Result.Rule != "golden_cross" {
			t.Errorf("result = %+v, want horizon 15 / golden_cross", body.Result)
		}
		if body.PaywallLocked {
			t.Error("flag off: paywall_locked must be false")
		}
	})

	t.Run("flag on + anon → paywall_locked", func(t *testing.T) {
		srv := backtestServer(t, fakeBars{rampCandles(300)}, true)
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/stocks/AAPL/backtest?rule=golden_cross")
		defer resp.Body.Close()
		var body backtestResp
		json.NewDecoder(resp.Body).Decode(&body)
		if !body.PaywallLocked || body.Result != nil {
			t.Fatalf("flag on + non-Pro must hard-lock, got %+v", body)
		}
	})
}

func TestTeaserSignals(t *testing.T) {
	mk := func(n int) []indicators.Signal {
		out := make([]indicators.Signal, n)
		for i := range out {
			out[i] = indicators.Signal{ID: "x", Direction: indicators.DirNeutral}
		}
		return out
	}
	// Fits within the limit → full slice, not locked.
	if got, locked := teaserSignals(mk(freeSignalTeaserLimit)); len(got) != freeSignalTeaserLimit || locked {
		t.Errorf("at-limit: got len=%d locked=%v, want %d/false", len(got), locked, freeSignalTeaserLimit)
	}
	// Exceeds → truncated + locked.
	full := mk(freeSignalTeaserLimit + 2)
	got, locked := teaserSignals(full)
	if len(got) != freeSignalTeaserLimit || !locked {
		t.Errorf("over-limit: got len=%d locked=%v, want %d/true", len(got), locked, freeSignalTeaserLimit)
	}
	// The teaser must not be able to clobber the original via append (3-index cap).
	got = append(got, indicators.Signal{ID: "leak"})
	if full[freeSignalTeaserLimit].ID == "leak" {
		t.Error("teaser append leaked into the original backing array")
	}
}

func TestGetIndicatorHistory(t *testing.T) {
	t.Run("nil bars → 404", func(t *testing.T) {
		srv := backtestServer(t, nil, false)
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/stocks/AAPL/indicator-history?id=technical.rsi")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("unsupported id → 400", func(t *testing.T) {
		srv := backtestServer(t, fakeBars{rampCandles(300)}, false)
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/stocks/AAPL/indicator-history?id=technical.vwap")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("insufficient history → 422", func(t *testing.T) {
		srv := backtestServer(t, fakeBars{rampCandles(5)}, false)
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/stocks/AAPL/indicator-history?id=technical.boll")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422", resp.StatusCode)
		}
	})

	t.Run("valid → 200 with dated points", func(t *testing.T) {
		srv := backtestServer(t, fakeBars{rampCandles(300)}, false)
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/stocks/aapl/indicator-history?id=technical.rsi&period=14")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var body struct {
			Ticker  string `json:"ticker"`
			History *struct {
				Indicator string `json:"indicator"`
				Points    []struct {
					Date  string  `json:"date"`
					Value float64 `json:"value"`
				} `json:"points"`
			} `json:"history"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Ticker != "AAPL" || body.History == nil || len(body.History.Points) == 0 {
			t.Fatalf("unexpected body: %+v", body)
		}
		if body.History.Indicator != "technical.rsi" {
			t.Errorf("indicator = %q, want technical.rsi", body.History.Indicator)
		}
	})
}

// monthSpanCandles builds `months` monthly candles (one per month) so ComputeSeasonality has
// multiple calendar months to aggregate.
func monthSpanCandles(months int) []store.Candle {
	base := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	cs := make([]store.Candle, months)
	for i := range cs {
		v := 100 + float64(i) // gently rising
		cs[i] = store.Candle{Time: base.AddDate(0, i, 0), Open: v, High: v, Low: v, Close: v, Volume: 1000}
	}
	return cs
}

func TestGetSeasonality(t *testing.T) {
	t.Run("nil bars → 404", func(t *testing.T) {
		srv := backtestServer(t, nil, false)
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/stocks/AAPL/seasonality")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("single month → 422", func(t *testing.T) {
		srv := backtestServer(t, fakeBars{monthSpanCandles(1)}, false)
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/stocks/AAPL/seasonality")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422", resp.StatusCode)
		}
	})

	t.Run("valid → 200 with months", func(t *testing.T) {
		srv := backtestServer(t, fakeBars{monthSpanCandles(30)}, false)
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/stocks/aapl/seasonality")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var body struct {
			Ticker      string `json:"ticker"`
			Seasonality *struct {
				Months []struct {
					Month int `json:"month"`
				} `json:"months"`
				FromYear int `json:"from_year"`
			} `json:"seasonality"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Ticker != "AAPL" || body.Seasonality == nil || len(body.Seasonality.Months) == 0 {
			t.Fatalf("unexpected body: %+v", body)
		}
	})
}

func TestGetRelativeStrength(t *testing.T) {
	t.Run("nil bars → 404", func(t *testing.T) {
		srv := backtestServer(t, nil, false)
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/stocks/AAPL/relative-strength")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("benchmark ticker itself → 422", func(t *testing.T) {
		srv := backtestServer(t, fakeBars{monthSpanCandles(30)}, false)
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/stocks/SPY/relative-strength")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422 (relative strength vs itself)", resp.StatusCode)
		}
	})

	t.Run("too little history → 422", func(t *testing.T) {
		srv := backtestServer(t, fakeBars{monthSpanCandles(1)}, false) // 1 bar → no window fits
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/stocks/AAPL/relative-strength")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422", resp.StatusCode)
		}
	})

	t.Run("valid → 200 with windows", func(t *testing.T) {
		srv := backtestServer(t, fakeBars{monthSpanCandles(30)}, false) // ≥22 bars → ≥1 window
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/stocks/aapl/relative-strength")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var body struct {
			Ticker           string `json:"ticker"`
			RelativeStrength *struct {
				Benchmark string `json:"benchmark"`
				Windows   []struct {
					Label string `json:"label"`
				} `json:"windows"`
			} `json:"relative_strength"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Ticker != "AAPL" || body.RelativeStrength == nil ||
			body.RelativeStrength.Benchmark != "SPY" || len(body.RelativeStrength.Windows) == 0 {
			t.Fatalf("unexpected body: %+v", body)
		}
	})
}
