package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wombow-ai/tickwind/internal/auth"
	"github.com/wombow-ai/tickwind/internal/enrich"
	"github.com/wombow-ai/tickwind/internal/indicators"
	"github.com/wombow-ai/tickwind/internal/store/memory"
	"github.com/wombow-ai/tickwind/internal/stream"
)

// serverWithIndicators builds a test server with the static indicator catalog
// injected via the setter (mirroring main.go's wiring).
func serverWithIndicators(t *testing.T, src IndicatorSource) *httptest.Server {
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
	h.SetIndicators(src)
	return httptest.NewServer(h)
}

type indicatorsResp struct {
	Count      int                    `json:"count"`
	Total      int                    `json:"total"`
	Indicators []indicators.Indicator `json:"indicators"`
	Facets     indicators.Facets      `json:"facets"`
}

func getIndicatorsResp(t *testing.T, srv *httptest.Server, query string) indicatorsResp {
	t.Helper()
	resp, err := http.Get(srv.URL + "/v1/indicators" + query)
	if err != nil {
		t.Fatalf("GET /v1/indicators%s: %v", query, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body indicatorsResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body
}

func TestGetIndicators(t *testing.T) {
	cat := indicators.MustLoad()
	srv := serverWithIndicators(t, cat)
	defer srv.Close()

	t.Run("unfiltered returns whole catalog", func(t *testing.T) {
		body := getIndicatorsResp(t, srv, "")
		if body.Total != cat.Len() {
			t.Errorf("total = %d, want %d", body.Total, cat.Len())
		}
		if body.Count != cat.Len() {
			t.Errorf("count = %d, want %d", body.Count, cat.Len())
		}
		if len(body.Facets.Domains) == 0 || len(body.Facets.Priorities) == 0 {
			t.Error("facets missing in response")
		}
	})

	t.Run("domain filter", func(t *testing.T) {
		body := getIndicatorsResp(t, srv, "?domain=technical")
		if body.Count == 0 {
			t.Fatal("technical filter returned nothing")
		}
		if body.Count >= body.Total {
			t.Errorf("count %d should be < total %d when filtered", body.Count, body.Total)
		}
		for _, ind := range body.Indicators {
			if ind.Domain != "technical" {
				t.Errorf("got domain %q, want technical", ind.Domain)
			}
		}
		// total stays the full catalog even when filtered (drives the facet UI).
		if body.Total != cat.Len() {
			t.Errorf("total = %d, want %d (unfiltered)", body.Total, cat.Len())
		}
	})

	t.Run("text search", func(t *testing.T) {
		body := getIndicatorsResp(t, srv, "?q=RSI")
		found := false
		for _, ind := range body.Indicators {
			if ind.ID == "technical.rsi" {
				found = true
			}
		}
		if !found {
			t.Error("q=RSI did not surface technical.rsi")
		}
	})
}

func TestGetIndicatorsNilSource(t *testing.T) {
	srv := serverWithIndicators(t, nil)
	defer srv.Close()
	body := getIndicatorsResp(t, srv, "")
	if body.Total != 0 || body.Count != 0 {
		t.Errorf("nil source: total=%d count=%d, want 0/0", body.Total, body.Count)
	}
	if body.Indicators == nil {
		t.Error("nil source: indicators should be an empty array, not null")
	}
}

// fakeIndicatorCompute is a stub IndicatorComputeSource returning a fixed
// result, so the handler can be tested without the real compute layer / stores.
type fakeIndicatorCompute struct {
	res indicators.StockIndicatorsResult
}

func (f fakeIndicatorCompute) StockIndicators(_ context.Context, _ string) indicators.StockIndicatorsResult {
	return f.res
}

// serverWithIndicatorCompute builds a test server with a per-stock indicator
// compute source injected via the setter (mirroring main.go's wiring).
func serverWithIndicatorCompute(t *testing.T, src IndicatorComputeSource) *httptest.Server {
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
	return httptest.NewServer(h)
}

type stockIndicatorsBody struct {
	Ticker        string `json:"ticker"`
	AsOf          string `json:"as_of"`
	MarketContext *struct {
		VIX       *float64 `json:"vix"`
		FearGreed *struct {
			Score int    `json:"score"`
			Label string `json:"label"`
		} `json:"fear_greed"`
	} `json:"market_context"`
	Indicators []indicators.StockIndicator `json:"indicators"`
}

func TestGetStockIndicatorsNilSource(t *testing.T) {
	srv := serverWithIndicatorCompute(t, nil)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/stocks/AAPL/indicators")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("nil source: status = %d, want 404", resp.StatusCode)
	}
}

func TestGetStockIndicatorsEmpty404(t *testing.T) {
	// Nothing computed at all (no ok indicator, no as_of, no market context) →
	// 404 so the frontend hides the panel for an unknown/non-US ticker.
	src := fakeIndicatorCompute{res: indicators.StockIndicatorsResult{
		Ticker: "ZZZZ",
		Indicators: []indicators.StockIndicator{
			{Indicator: indicators.Indicator{ID: "technical.rsi"}, Status: indicators.StatusInsufficient, Reason: "no bars"},
		},
	}}
	srv := serverWithIndicatorCompute(t, src)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/stocks/ZZZZ/indicators")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("empty result: status = %d, want 404", resp.StatusCode)
	}
}

func TestGetStockIndicatorsHappyPath(t *testing.T) {
	rsi := 56.7
	src := fakeIndicatorCompute{res: indicators.StockIndicatorsResult{
		Ticker:    "AAPL",
		AsOf:      "2026-06-12",
		FearGreed: &indicators.FearGreed{Score: 62, Label: "Greed"},
		Indicators: []indicators.StockIndicator{
			{Indicator: indicators.Indicator{ID: "technical.rsi", Domain: "technical"}, Status: indicators.StatusOK, Value: &rsi, Unit: ""},
			{Indicator: indicators.Indicator{ID: "fundamental.pe-ttm", Domain: "fundamental"}, Status: indicators.StatusInsufficient, Reason: "no SEC fundamentals available"},
		},
	}}
	srv := serverWithIndicatorCompute(t, src)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/stocks/aapl/indicators") // lowercase → handler uppercases
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body stockIndicatorsBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Ticker != "AAPL" {
		t.Errorf("ticker = %q, want AAPL", body.Ticker)
	}
	if body.AsOf != "2026-06-12" {
		t.Errorf("as_of = %q, want 2026-06-12", body.AsOf)
	}
	if len(body.Indicators) != 2 {
		t.Fatalf("indicators len = %d, want 2", len(body.Indicators))
	}
	if body.MarketContext == nil || body.MarketContext.FearGreed == nil {
		t.Fatal("market_context.fear_greed missing")
	}
	if body.MarketContext.FearGreed.Score != 62 || body.MarketContext.FearGreed.Label != "Greed" {
		t.Errorf("fear_greed = %+v, want {62 Greed}", *body.MarketContext.FearGreed)
	}
	if body.MarketContext.VIX != nil {
		t.Errorf("vix should be omitted when unavailable, got %v", *body.MarketContext.VIX)
	}
}
