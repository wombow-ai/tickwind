package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/indicators"
)

type fakeRS struct {
	pop []indicators.TickerRelStrength
}

func (f fakeRS) RankByWindow(window string) ([]indicators.RSRank, time.Time) {
	return indicators.RankRelativeStrength(f.pop, window), time.Unix(1_700_000_000, 0)
}

func rsServer(t *testing.T, src RSScanSource) *httptest.Server {
	t.Helper()
	h := barsTestServer(t, nil)
	if src != nil {
		h.SetRSScan(src)
	}
	return httptest.NewServer(h)
}

func rsPop() []indicators.TickerRelStrength {
	return []indicators.TickerRelStrength{
		{Ticker: "A", RS: indicators.RelativeStrength{Windows: []indicators.RelStrengthWindow{{Label: "3M", Relative: 5}}}},
		{Ticker: "B", RS: indicators.RelativeStrength{Windows: []indicators.RelStrengthWindow{{Label: "3M", Relative: 15}}}},
		{Ticker: "C", RS: indicators.RelativeStrength{Windows: []indicators.RelStrengthWindow{{Label: "3M", Relative: 9}}}},
	}
}

func TestGetRSScreen(t *testing.T) {
	t.Run("nil source → 404", func(t *testing.T) {
		srv := rsServer(t, nil)
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/screen/relative-strength?window=3M")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("bad window → 400", func(t *testing.T) {
		srv := rsServer(t, fakeRS{pop: rsPop()})
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/screen/relative-strength?window=2W")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("default window 3M → ranked desc by excess", func(t *testing.T) {
		srv := rsServer(t, fakeRS{pop: rsPop()})
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/screen/relative-strength") // no window → default 3M
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var body struct {
			Window  string `json:"window"`
			Count   int    `json:"count"`
			Total   int    `json:"total"`
			AsOf    string `json:"as_of"`
			Results []struct {
				Ticker   string  `json:"ticker"`
				Relative float64 `json:"relative"`
			} `json:"results"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Window != "3M" || body.Count != 3 || body.Total != 3 || body.AsOf == "" {
			t.Fatalf("unexpected meta: %+v", body)
		}
		if body.Results[0].Ticker != "B" || body.Results[1].Ticker != "C" || body.Results[2].Ticker != "A" {
			t.Fatalf("order = %v, want B,C,A (15,9,5)", body.Results)
		}
	})

	t.Run("limit truncates but total is the full count", func(t *testing.T) {
		srv := rsServer(t, fakeRS{pop: rsPop()})
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/screen/relative-strength?window=3M&limit=1")
		defer resp.Body.Close()
		var body struct {
			Count   int `json:"count"`
			Total   int `json:"total"`
			Results []struct {
				Ticker string `json:"ticker"`
			} `json:"results"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Count != 1 || body.Total != 3 || len(body.Results) != 1 || body.Results[0].Ticker != "B" {
			t.Fatalf("count/total = %d/%d top=%v, want 1/3 B", body.Count, body.Total, body.Results)
		}
	})
}
