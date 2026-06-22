package api

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/indicators"
)

type fakeScorecard struct{ pop []indicators.FactorMetrics }

func (f fakeScorecard) Population() ([]indicators.FactorMetrics, time.Time) {
	return f.pop, time.Unix(1_700_000_000, 0)
}

// scorecardServer wires a per-stock compute source + a factor population onto a test server.
func scorecardServer(t *testing.T, compute IndicatorComputeSource, sc ScorecardSource) *httptest.Server {
	t.Helper()
	h := barsTestServer(t, nil)
	if compute != nil {
		h.SetIndicatorCompute(compute)
	}
	if sc != nil {
		h.SetScorecard(sc)
	}
	return httptest.NewServer(h)
}

// pop10 builds a 10-name factor population with rising P/E + ROE (other metrics unavailable).
func pop10() []indicators.FactorMetrics {
	nan := math.NaN()
	pop := make([]indicators.FactorMetrics, 10)
	for i := range pop {
		pop[i] = indicators.FactorMetrics{
			PE: float64(10 + i*5), ROE: float64(i+1) * 0.05,
			PB: nan, PS: nan, RevGrowth: nan, EarnGrowth: nan,
			ROIC: nan, EBITMargin: nan, Piotroski: nan, TSR: nan,
		}
	}
	return pop
}

func TestGetScorecard(t *testing.T) {
	t.Run("nil sources → 404", func(t *testing.T) {
		srv := scorecardServer(t, nil, nil)
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/stocks/AAPL/scorecard")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("target has no factor metrics → 422", func(t *testing.T) {
		empty := fakeIndicatorCompute{res: indicators.StockIndicatorsResult{Ticker: "ETF"}}
		srv := scorecardServer(t, empty, fakeScorecard{pop: pop10()})
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/stocks/ETF/scorecard")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422", resp.StatusCode)
		}
	})

	t.Run("valid → 200 with factor percentiles", func(t *testing.T) {
		// Target: a cheap (P/E 10 = lowest) + high-quality (ROE 0.50 = highest) name.
		target := fakeIndicatorCompute{res: indicators.StockIndicatorsResult{
			Ticker: "AAPL", AsOf: "2026-06-22",
			Indicators: []indicators.StockIndicator{
				{Indicator: indicators.Indicator{ID: "fundamental.pe-ttm"}, Status: indicators.StatusOK, Value: fptr(10)},
				{Indicator: indicators.Indicator{ID: "fundamental.roe"}, Status: indicators.StatusOK, Value: fptr(0.50)},
			},
		}}
		srv := scorecardServer(t, target, fakeScorecard{pop: pop10()})
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/stocks/aapl/scorecard")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var body struct {
			Ticker    string `json:"ticker"`
			Scorecard *struct {
				Value *struct {
					Percentile float64 `json:"percentile"`
				} `json:"value"`
				Quality *struct {
					Percentile float64 `json:"percentile"`
				} `json:"quality"`
				Growth     *json.RawMessage `json:"growth"`
				Population int              `json:"population"`
			} `json:"scorecard"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Ticker != "AAPL" || body.Scorecard == nil {
			t.Fatalf("unexpected body: %+v", body)
		}
		if body.Scorecard.Population != 10 {
			t.Fatalf("population = %d, want 10", body.Scorecard.Population)
		}
		// Cheapest P/E → high value percentile; highest ROE → 100th quality; no growth metric → nil.
		if body.Scorecard.Value == nil || body.Scorecard.Value.Percentile < 80 {
			t.Fatalf("value percentile = %+v, want high (cheapest)", body.Scorecard.Value)
		}
		if body.Scorecard.Quality == nil || body.Scorecard.Quality.Percentile != 100 {
			t.Fatalf("quality percentile = %+v, want 100", body.Scorecard.Quality)
		}
		if body.Scorecard.Growth != nil {
			t.Fatalf("growth should be omitted (no growth metric), got %s", string(*body.Scorecard.Growth))
		}
	})
}
