package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/indicators"
	"github.com/wombow-ai/tickwind/internal/store"
)

type fakeReactionSrc struct {
	m map[string]indicators.ReactionSummary
}

func (f fakeReactionSrc) Reaction(t string) (indicators.ReactionSummary, bool) {
	r, ok := f.m[t]
	return r, ok
}

// PopulationRanked ranks the fake's whole map via the real ranking fn (so the screen handler test
// exercises indicators.RankEarningsReaction end-to-end). Fixed AsOf for a stable assertion.
func (f fakeReactionSrc) PopulationRanked(view string) ([]indicators.ReactionRank, time.Time) {
	pop := make([]indicators.TickerReaction, 0, len(f.m))
	for tk, rs := range f.m {
		pop = append(pop, indicators.TickerReaction{Ticker: tk, ReactionSummary: rs})
	}
	return indicators.RankEarningsReaction(pop, view), time.Unix(1_700_000_000, 0)
}

func TestWithReactions(t *testing.T) {
	s := &Server{earningsReactions: fakeReactionSrc{m: map[string]indicators.ReactionSummary{
		"AAPL": {AvgAbsMove: 3.2, UpRate: 0.6, Samples: 10},
		"ZZZ":  {AvgAbsMove: 1.0, Samples: 0}, // present but no samples → omitted (insufficient)
	}}}
	rows := s.withReactions([]store.Earning{
		{Ticker: "AAPL"}, {Ticker: "MSFT"}, {Ticker: "ZZZ"}, {Ticker: "aapl"},
	})
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4", len(rows))
	}
	if rows[0].Reaction == nil || rows[0].Reaction.AvgAbsMove != 3.2 || rows[0].Reaction.Samples != 10 {
		t.Fatalf("AAPL reaction = %+v, want {3.2,_,10}", rows[0].Reaction)
	}
	if rows[1].Reaction != nil {
		t.Fatalf("MSFT is untracked → reaction must be nil, got %+v", rows[1].Reaction)
	}
	if rows[2].Reaction != nil {
		t.Fatalf("ZZZ has 0 samples → reaction must be omitted, got %+v", rows[2].Reaction)
	}
	if rows[3].Reaction == nil {
		t.Fatal("lowercase 'aapl' should match (ticker upper-cased before lookup)")
	}
	// The embedded ticker is preserved verbatim.
	if rows[3].Ticker != "aapl" {
		t.Fatalf("row ticker mutated: %q", rows[3].Ticker)
	}
}

func TestWithReactions_NilSource(t *testing.T) {
	s := &Server{} // no reaction source
	rows := s.withReactions([]store.Earning{{Ticker: "AAPL"}})
	if len(rows) != 1 || rows[0].Reaction != nil {
		t.Fatalf("nil source → no reactions, got %+v", rows)
	}
}

func erScreenServer(t *testing.T, src EarningsReactionSource) *httptest.Server {
	t.Helper()
	h := barsTestServer(t, nil)
	if src != nil {
		h.SetEarningsReactions(src)
	}
	return httptest.NewServer(h)
}

func erScreenPop() fakeReactionSrc {
	return fakeReactionSrc{m: map[string]indicators.ReactionSummary{
		"AAPL": {AvgAbsMove: 8.0, UpRate: 0.5, Samples: 10},
		"TSLA": {AvgAbsMove: 12.0, UpRate: 0.7, Samples: 8},
		"NVDA": {AvgAbsMove: 6.0, UpRate: 0.9, Samples: 6},
		"KO":   {AvgAbsMove: 3.0, UpRate: 0.9, Samples: 12}, // ties NVDA on up-rate, more samples → ranks above it
		"LOWS": {AvgAbsMove: 2.0, UpRate: 0.4, Samples: 3},  // below minEarningsSamples → filtered out of every view
	}}
}

type erScreenBody struct {
	View    string `json:"view"`
	Count   int    `json:"count"`
	Total   int    `json:"total"`
	AsOf    string `json:"as_of"`
	Results []struct {
		Ticker     string  `json:"ticker"`
		AvgAbsMove float64 `json:"avg_abs_move"`
		UpRate     float64 `json:"up_rate"`
		Samples    int     `json:"samples"`
	} `json:"results"`
}

func decodeERScreen(t *testing.T, resp *http.Response) erScreenBody {
	t.Helper()
	var body erScreenBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body
}

func erTickers(b erScreenBody) []string {
	out := make([]string, len(b.Results))
	for i, r := range b.Results {
		out[i] = r.Ticker
	}
	return out
}

func eqStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestGetEarningsReactionScreen(t *testing.T) {
	t.Run("nil source → 404", func(t *testing.T) {
		srv := erScreenServer(t, nil)
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/screen/earnings-reaction?view=most-volatile")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("bad view → 400", func(t *testing.T) {
		srv := erScreenServer(t, erScreenPop())
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/screen/earnings-reaction?view=calmest")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("default view = most-volatile, ranked by abs move desc, sub-floor filtered", func(t *testing.T) {
		srv := erScreenServer(t, erScreenPop())
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/screen/earnings-reaction") // no view → default most-volatile
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		body := decodeERScreen(t, resp)
		if body.View != "most-volatile" || body.Count != 4 || body.Total != 4 || body.AsOf == "" {
			t.Fatalf("unexpected meta: %+v", body)
		}
		if want := []string{"TSLA", "AAPL", "NVDA", "KO"}; !eqStrs(erTickers(body), want) {
			t.Fatalf("order = %v, want %v (12,8,6,3; LOWS@3-samples filtered)", erTickers(body), want)
		}
	})

	t.Run("highest-up-rate ranks by up-rate desc, ties break by more samples", func(t *testing.T) {
		srv := erScreenServer(t, erScreenPop())
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/screen/earnings-reaction?view=highest-up-rate")
		defer resp.Body.Close()
		body := decodeERScreen(t, resp)
		// KO & NVDA both 0.9 → KO (12 samples) before NVDA (6); then TSLA 0.7, AAPL 0.5.
		if want := []string{"KO", "NVDA", "TSLA", "AAPL"}; !eqStrs(erTickers(body), want) {
			t.Fatalf("order = %v, want %v", erTickers(body), want)
		}
	})

	t.Run("limit truncates but total is the full population", func(t *testing.T) {
		srv := erScreenServer(t, erScreenPop())
		defer srv.Close()
		resp := mustGet(t, srv.URL+"/v1/screen/earnings-reaction?view=most-volatile&limit=2")
		defer resp.Body.Close()
		body := decodeERScreen(t, resp)
		if body.Count != 2 || body.Total != 4 || !eqStrs(erTickers(body), []string{"TSLA", "AAPL"}) {
			t.Fatalf("count/total/top = %d/%d/%v, want 2/4 [TSLA AAPL]", body.Count, body.Total, erTickers(body))
		}
	})
}
