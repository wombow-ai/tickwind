package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/indicators"
)

type fakeDividend struct {
	pop []indicators.TickerDividend
}

func (f fakeDividend) PopulationRanked(view string) ([]indicators.DividendRank, time.Time) {
	return indicators.RankDividend(f.pop, view), time.Unix(1_700_000_000, 0)
}

func df64(v float64) *float64 { return &v }

func dividendScreenServer(t *testing.T, src DividendScanSource) *httptest.Server {
	t.Helper()
	h := barsTestServer(t, nil)
	if src != nil {
		h.SetDividendScan(src)
	}
	return httptest.NewServer(h)
}

func dividendPop() fakeDividend {
	return fakeDividend{pop: []indicators.TickerDividend{
		{Ticker: "T", Dividend: indicators.DividendView{Yield: df64(6.5), PayoutRatio: df64(60)}},
		{Ticker: "KO", Dividend: indicators.DividendView{Yield: df64(3.0), PayoutRatio: df64(70)}},
		{Ticker: "AAPL", Dividend: indicators.DividendView{Yield: df64(0.5), PayoutRatio: df64(15)}},
	}}
}

type divScreenBody struct {
	View    string `json:"view"`
	Count   int    `json:"count"`
	Total   int    `json:"total"`
	AsOf    string `json:"as_of"`
	Results []struct {
		Ticker string   `json:"ticker"`
		Yield  *float64 `json:"yield"`
	} `json:"results"`
}

func TestGetDividendScreen(t *testing.T) {
	t.Run("nil source → 404", func(t *testing.T) {
		srv := dividendScreenServer(t, nil)
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/screen/dividends?view=highest-yield")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("bad view → 400", func(t *testing.T) {
		srv := dividendScreenServer(t, dividendPop())
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/screen/dividends?view=biggest")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("default view = highest-yield, ranked desc", func(t *testing.T) {
		srv := dividendScreenServer(t, dividendPop())
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/screen/dividends") // no view → default highest-yield
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var body divScreenBody
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.View != "highest-yield" || body.Count != 3 || body.Total != 3 || body.AsOf == "" {
			t.Fatalf("unexpected meta: %+v", body)
		}
		if body.Results[0].Ticker != "T" || body.Results[1].Ticker != "KO" || body.Results[2].Ticker != "AAPL" {
			t.Fatalf("order = %v, want T,KO,AAPL (6.5,3.0,0.5)", body.Results)
		}
		// Each row carries the full profile (yield present even though that's the ranked metric).
		if body.Results[0].Yield == nil || *body.Results[0].Yield != 6.5 {
			t.Fatalf("top yield = %v, want 6.5", body.Results[0].Yield)
		}
	})

	t.Run("limit truncates but total is the full count", func(t *testing.T) {
		srv := dividendScreenServer(t, dividendPop())
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/screen/dividends?view=highest-yield&limit=1")
		defer resp.Body.Close()
		var body divScreenBody
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Count != 1 || body.Total != 3 || len(body.Results) != 1 || body.Results[0].Ticker != "T" {
			t.Fatalf("count/total/top = %d/%d/%v, want 1/3 T", body.Count, body.Total, body.Results)
		}
	})
}
