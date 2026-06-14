package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/auth"
	"github.com/wombow-ai/tickwind/internal/enrich"
	"github.com/wombow-ai/tickwind/internal/finrashvol"
	"github.com/wombow-ai/tickwind/internal/ratecut"
	"github.com/wombow-ai/tickwind/internal/sentiment"
	"github.com/wombow-ai/tickwind/internal/store/memory"
	"github.com/wombow-ai/tickwind/internal/stream"
	"github.com/wombow-ai/tickwind/internal/treasury"
)

// newBareServer builds an *api.Server with every optional source nil, so a test
// can inject just the macro sources it cares about via the Set* methods.
func newBareServer() *Server {
	return New(
		memory.New(), stream.NewHub(), enrich.Noop{},
		auth.NewVerifier(testSecret, ""),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, // no admin user ids
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
}

// fakeShortVolume satisfies ShortVolumeSource from canned data.
type fakeShortVolume struct {
	top     []finrashvol.ShortVol
	latest  map[string]finrashvol.ShortVol
	history map[string][]finrashvol.ShortVol
	asOf    string
}

func (f *fakeShortVolume) Top(n int, _ int64) []finrashvol.ShortVol {
	if n > 0 && len(f.top) > n {
		return f.top[:n]
	}
	return f.top
}
func (f *fakeShortVolume) Latest(sym string) (finrashvol.ShortVol, bool) {
	v, ok := f.latest[sym]
	return v, ok
}
func (f *fakeShortVolume) History(sym string) []finrashvol.ShortVol { return f.history[sym] }
func (f *fakeShortVolume) AsOf() string                             { return f.asOf }

func TestShortVolumeEndpoint(t *testing.T) {
	// nil source → empty list, 200, never null.
	srv := httptest.NewServer(newBareServer())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/short-volume")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("nil-source /v1/short-volume = %d, want 200", resp.StatusCode)
	}
	var empty struct {
		AsOf   string                `json:"as_of"`
		Count  int                   `json:"count"`
		Stocks []finrashvol.ShortVol `json:"stocks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&empty); err != nil {
		t.Fatal(err)
	}
	if empty.Stocks == nil {
		t.Fatal("stocks must marshal as [] not null")
	}

	// Populated source.
	s := newBareServer()
	s.SetShortVolume(&fakeShortVolume{
		asOf: "2026-06-12",
		top: []finrashvol.ShortVol{
			{Symbol: "GME", ShortPct: 61.3, ShortVolume: 123, TotalVolume: 456},
			{Symbol: "AMC", ShortPct: 55.0, ShortVolume: 50, TotalVolume: 100},
		},
	})
	srv2 := httptest.NewServer(s)
	defer srv2.Close()
	resp2, err := http.Get(srv2.URL + "/v1/short-volume?limit=1")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		AsOf   string `json:"as_of"`
		Count  int    `json:"count"`
		Stocks []struct {
			Symbol      string  `json:"symbol"`
			ShortPct    float64 `json:"short_pct"`
			ShortVolume int64   `json:"short_volume"`
			TotalVolume int64   `json:"total_volume"`
		} `json:"stocks"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.AsOf != "2026-06-12" || got.Count != 1 || len(got.Stocks) != 1 {
		t.Fatalf("response = %+v, want as_of 2026-06-12 + 1 stock (limit honored)", got)
	}
	if got.Stocks[0].Symbol != "GME" || got.Stocks[0].ShortPct != 61.3 || got.Stocks[0].TotalVolume != 456 {
		t.Fatalf("top stock = %+v, want GME 61.3%% 456 vol", got.Stocks[0])
	}
}

func TestStockShortAddsDailyField(t *testing.T) {
	// No short-interest source and no short-volume source → 404 (neither has a row).
	srv := httptest.NewServer(newBareServer())
	defer srv.Close()
	if resp, _ := http.Get(srv.URL + "/v1/stocks/GME/short"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("no sources /v1/stocks/GME/short = %d, want 404", resp.StatusCode)
	}

	// Only the daily short-volume source has a row → 200 with daily set, short null.
	s := newBareServer()
	s.SetShortVolume(&fakeShortVolume{
		latest:  map[string]finrashvol.ShortVol{"GME": {Symbol: "GME", ShortPct: 61.3, Date: "2026-06-12"}},
		history: map[string][]finrashvol.ShortVol{"GME": {{ShortPct: 60.1, Date: "2026-06-11"}}},
	})
	srv2 := httptest.NewServer(s)
	defer srv2.Close()
	resp, err := http.Get(srv2.URL + "/v1/stocks/gme/short") // lower-case → upper-cased
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("daily-only /v1/stocks/gme/short = %d, want 200", resp.StatusCode)
	}
	var got struct {
		Ticker string `json:"ticker"`
		Short  *struct {
			Symbol string `json:"symbol"`
		} `json:"short"`
		Daily *struct {
			ShortPct float64 `json:"short_pct"`
			AsOf     string  `json:"as_of"`
			History  []struct {
				Date     string  `json:"date"`
				ShortPct float64 `json:"short_pct"`
			} `json:"history"`
		} `json:"daily"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Ticker != "GME" {
		t.Fatalf("ticker = %q, want GME", got.Ticker)
	}
	if got.Short != nil {
		t.Fatalf("short should be null when no bi-monthly SI row, got %+v", got.Short)
	}
	if got.Daily == nil || got.Daily.ShortPct != 61.3 || got.Daily.AsOf != "2026-06-12" {
		t.Fatalf("daily = %+v, want short_pct 61.3 as_of 2026-06-12", got.Daily)
	}
	if len(got.Daily.History) != 1 || got.Daily.History[0].ShortPct != 60.1 {
		t.Fatalf("daily.history = %+v, want one point 60.1", got.Daily.History)
	}
}

// fakeSentiment satisfies SentimentSource.
type fakeSentiment struct {
	res     sentiment.Result
	hasRes  bool
	history []sentiment.Point
	at      time.Time
}

func (f *fakeSentiment) Latest() (sentiment.Result, bool) { return f.res, f.hasRes }
func (f *fakeSentiment) History() []sentiment.Point       { return f.history }
func (f *fakeSentiment) UpdatedAt() time.Time             { return f.at }

func TestSentimentEndpoint(t *testing.T) {
	// nil source → neutral 50 with empty components/history, 200.
	srv := httptest.NewServer(newBareServer())
	defer srv.Close()
	resp, _ := http.Get(srv.URL + "/v1/sentiment")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("nil-source /v1/sentiment = %d, want 200", resp.StatusCode)
	}
	var neutral struct {
		Score      int    `json:"score"`
		Label      string `json:"label"`
		Components []any  `json:"components"`
		History    []any  `json:"history"`
		UpdatedAt  string `json:"updated_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&neutral); err != nil {
		t.Fatal(err)
	}
	if neutral.Score != 50 || neutral.Label != "Neutral" {
		t.Fatalf("nil sentiment = %+v, want neutral 50", neutral)
	}
	if neutral.Components == nil || neutral.History == nil {
		t.Fatal("components/history must marshal as [] not null")
	}

	// Populated.
	s := newBareServer()
	s.SetSentiment(&fakeSentiment{
		hasRes: true,
		res: sentiment.Result{Score: 62, Label: "Greed", LabelZh: "贪婪", Available: 1,
			Components: []sentiment.Component{{Name: "VIX", Score: 70, Note: "VIX 18.0"}}},
		history: []sentiment.Point{{Date: "2026-06-11", Score: 58}},
		at:      time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC),
	})
	srv2 := httptest.NewServer(s)
	defer srv2.Close()
	resp2, _ := http.Get(srv2.URL + "/v1/sentiment")
	var got struct {
		Score      int    `json:"score"`
		Label      string `json:"label"`
		LabelZh    string `json:"label_zh"`
		UpdatedAt  string `json:"updated_at"`
		Components []struct {
			Name  string `json:"name"`
			Score int    `json:"score"`
			Note  string `json:"note"`
		} `json:"components"`
		History []struct {
			Date  string `json:"date"`
			Score int    `json:"score"`
		} `json:"history"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Score != 62 || got.Label != "Greed" || got.LabelZh != "贪婪" {
		t.Fatalf("sentiment = %+v, want 62/Greed/贪婪", got)
	}
	if len(got.Components) != 1 || got.Components[0].Name != "VIX" || got.Components[0].Score != 70 {
		t.Fatalf("components = %+v, want one VIX@70", got.Components)
	}
	if len(got.History) != 1 || got.History[0].Score != 58 {
		t.Fatalf("history = %+v, want one point 58", got.History)
	}
	if got.UpdatedAt != "2026-06-12T12:00:00Z" {
		t.Fatalf("updated_at = %q, want RFC3339 2026-06-12T12:00:00Z", got.UpdatedAt)
	}
}

// fakeRateCut satisfies RateCutSource.
type fakeRateCut struct {
	markets []ratecut.Market
	at      time.Time
}

func (f *fakeRateCut) Get() []ratecut.Market { return f.markets }
func (f *fakeRateCut) UpdatedAt() time.Time  { return f.at }

func TestRateCutEndpoint(t *testing.T) {
	// nil source → empty markets, 200, never null.
	srv := httptest.NewServer(newBareServer())
	defer srv.Close()
	resp, _ := http.Get(srv.URL + "/v1/ratecut")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("nil-source /v1/ratecut = %d, want 200", resp.StatusCode)
	}
	var empty struct {
		Markets []ratecut.Market `json:"markets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&empty); err != nil {
		t.Fatal(err)
	}
	if empty.Markets == nil {
		t.Fatal("markets must marshal as [] not null")
	}

	// Populated.
	s := newBareServer()
	s.SetRateCut(&fakeRateCut{
		at: time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC),
		markets: []ratecut.Market{{
			Source: "Kalshi", Question: "Fed funds after next FOMC", AsOf: "2026-06-12T11:00:00Z",
			Outcomes: []ratecut.Outcome{{Label: "-25bps", Probability: 0.62}},
		}},
	})
	srv2 := httptest.NewServer(s)
	defer srv2.Close()
	resp2, _ := http.Get(srv2.URL + "/v1/ratecut")
	var got struct {
		Markets []struct {
			Source   string `json:"source"`
			Question string `json:"question"`
			Outcomes []struct {
				Label       string  `json:"label"`
				Probability float64 `json:"probability"`
			} `json:"outcomes"`
		} `json:"markets"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Markets) != 1 || got.Markets[0].Source != "Kalshi" {
		t.Fatalf("markets = %+v, want one Kalshi market", got.Markets)
	}
	if len(got.Markets[0].Outcomes) != 1 || got.Markets[0].Outcomes[0].Label != "-25bps" {
		t.Fatalf("outcomes = %+v, want -25bps@0.62", got.Markets[0].Outcomes)
	}
	if got.UpdatedAt != "2026-06-12T12:00:00Z" {
		t.Fatalf("updated_at = %q, want RFC3339", got.UpdatedAt)
	}
}

// fakeMacro satisfies MacroSource.
type fakeMacro struct {
	curve treasury.Curve
	has   bool
	at    time.Time
}

func (f *fakeMacro) Latest() (treasury.Curve, bool) { return f.curve, f.has }
func (f *fakeMacro) UpdatedAt() time.Time           { return f.at }

func TestMacroEndpoint(t *testing.T) {
	// nil source → available=false, empty yields (never null), 200.
	srv := httptest.NewServer(newBareServer())
	defer srv.Close()
	resp, _ := http.Get(srv.URL + "/v1/macro")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("nil-source /v1/macro = %d, want 200", resp.StatusCode)
	}
	var empty struct {
		Available bool             `json:"available"`
		AsOf      string           `json:"as_of"`
		Yields    []treasury.Yield `json:"yields"`
		Source    string           `json:"source"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&empty); err != nil {
		t.Fatal(err)
	}
	if empty.Available {
		t.Fatal("nil source must report available=false")
	}
	if empty.Yields == nil {
		t.Fatal("yields must marshal as [] not null")
	}
	if empty.Source != "U.S. Treasury" {
		t.Fatalf("source = %q, want U.S. Treasury", empty.Source)
	}

	// Populated curve with both legs → spread present, inverted=false.
	s := newBareServer()
	s.SetMacro(&fakeMacro{
		has: true,
		at:  time.Date(2026, 6, 12, 22, 0, 0, 0, time.UTC),
		curve: treasury.Curve{
			Date: "2026-06-12",
			Yields: []treasury.Yield{
				{Tenor: "3M", Rate: 3.78}, {Tenor: "2Y", Rate: 4.09},
				{Tenor: "10Y", Rate: 4.48}, {Tenor: "30Y", Rate: 4.97},
			},
			Spread2s10s: 0.39, HasSpread: true, Inverted: false,
		},
	})
	srv2 := httptest.NewServer(s)
	defer srv2.Close()
	resp2, _ := http.Get(srv2.URL + "/v1/macro")
	var got struct {
		Available bool     `json:"available"`
		AsOf      string   `json:"as_of"`
		Inverted  bool     `json:"inverted"`
		Spread    *float64 `json:"spread_2s10s"`
		UpdatedAt string   `json:"updated_at"`
		Yields    []struct {
			Tenor string  `json:"tenor"`
			Rate  float64 `json:"rate"`
		} `json:"yields"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !got.Available || got.AsOf != "2026-06-12" {
		t.Fatalf("response = %+v, want available + as_of 2026-06-12", got)
	}
	if got.Spread == nil || *got.Spread != 0.39 || got.Inverted {
		t.Fatalf("spread = %v inverted = %v, want 0.39 / false", got.Spread, got.Inverted)
	}
	if len(got.Yields) != 4 || got.Yields[1].Tenor != "2Y" || got.Yields[1].Rate != 4.09 {
		t.Fatalf("yields = %+v, want 4 tenors with 2Y@4.09", got.Yields)
	}
	if got.UpdatedAt != "2026-06-12T22:00:00Z" {
		t.Fatalf("updated_at = %q, want RFC3339", got.UpdatedAt)
	}

	// Inverted curve (missing-leg guard verified in the treasury package): spread
	// present + negative → inverted=true.
	s2 := newBareServer()
	s2.SetMacro(&fakeMacro{has: true, at: time.Now(), curve: treasury.Curve{
		Date:        "2023-07-03",
		Yields:      []treasury.Yield{{Tenor: "2Y", Rate: 4.94}, {Tenor: "10Y", Rate: 3.86}},
		Spread2s10s: -1.08, HasSpread: true, Inverted: true,
	}})
	srv3 := httptest.NewServer(s2)
	defer srv3.Close()
	resp3, _ := http.Get(srv3.URL + "/v1/macro")
	var inv struct {
		Inverted bool     `json:"inverted"`
		Spread   *float64 `json:"spread_2s10s"`
	}
	if err := json.NewDecoder(resp3.Body).Decode(&inv); err != nil {
		t.Fatal(err)
	}
	if !inv.Inverted || inv.Spread == nil || *inv.Spread != -1.08 {
		t.Fatalf("inverted curve = %+v, want inverted=true spread=-1.08", inv)
	}
}
