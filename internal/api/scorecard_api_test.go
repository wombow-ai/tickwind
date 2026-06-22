package api

import (
	"encoding/json"
	"fmt"
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

// PopulationRanked names the fake population (T00, T01, …) and ranks it via the real RankFactor, so
// the factor-screen handler test exercises the genuine ranking path.
func (f fakeScorecard) PopulationRanked(factor string) ([]indicators.FactorRank, time.Time) {
	tfm := make([]indicators.TickerFactorMetrics, len(f.pop))
	for i, m := range f.pop {
		tfm[i] = indicators.TickerFactorMetrics{Ticker: fmt.Sprintf("T%02d", i), Metrics: m}
	}
	return indicators.RankFactor(tfm, factor), time.Unix(1_700_000_000, 0)
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

func TestGetFactorScreen(t *testing.T) {
	t.Run("nil source → 404", func(t *testing.T) {
		srv := scorecardServer(t, nil, nil)
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/screen/factors?factor=value")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("missing/invalid factor → 400", func(t *testing.T) {
		srv := scorecardServer(t, nil, fakeScorecard{pop: pop10()})
		defer srv.Close()
		for _, q := range []string{"", "?factor=", "?factor=bogus"} {
			resp := mustGet(t, srv.URL+"/v1/screen/factors"+q)
			if resp.StatusCode != http.StatusBadRequest {
				resp.Body.Close()
				t.Fatalf("factor %q: status = %d, want 400", q, resp.StatusCode)
			}
			resp.Body.Close()
		}
	})

	t.Run("value factor → cheapest ranks first", func(t *testing.T) {
		srv := scorecardServer(t, nil, fakeScorecard{pop: pop10()})
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/screen/factors?factor=value")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var body struct {
			Factor     string `json:"factor"`
			Count      int    `json:"count"`
			Population int    `json:"population"`
			AsOf       string `json:"as_of"`
			Results    []struct {
				Ticker     string  `json:"ticker"`
				Percentile float64 `json:"percentile"`
			} `json:"results"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Factor != "value" || body.Population != 10 || body.Count != 10 || len(body.Results) != 10 {
			t.Fatalf("unexpected meta: %+v", body)
		}
		if body.AsOf == "" {
			t.Fatal("expected a non-empty as_of")
		}
		// pop10 P/E rises with index (T00 cheapest) → T00 has the HIGHEST value percentile → ranks #1.
		if body.Results[0].Ticker != "T00" {
			t.Fatalf("rank 1 = %s (%.1f), want T00 (cheapest)", body.Results[0].Ticker, body.Results[0].Percentile)
		}
		// Sorted high→low.
		for i := 1; i < len(body.Results); i++ {
			if body.Results[i].Percentile > body.Results[i-1].Percentile {
				t.Fatalf("results not sorted desc at %d: %.1f > %.1f", i, body.Results[i].Percentile, body.Results[i-1].Percentile)
			}
		}
	})

	t.Run("limit truncates but population is the full count", func(t *testing.T) {
		srv := scorecardServer(t, nil, fakeScorecard{pop: pop10()})
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/screen/factors?factor=quality&limit=3")
		defer resp.Body.Close()
		var body struct {
			Count      int `json:"count"`
			Population int `json:"population"`
			Results    []struct {
				Ticker string `json:"ticker"`
			} `json:"results"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Count != 3 || len(body.Results) != 3 || body.Population != 10 {
			t.Fatalf("count/pop = %d/%d, want 3/10", body.Count, body.Population)
		}
		// Quality rises with ROE → T09 (highest ROE) ranks #1.
		if body.Results[0].Ticker != "T09" {
			t.Fatalf("rank 1 = %s, want T09 (highest ROE)", body.Results[0].Ticker)
		}
	})
}
