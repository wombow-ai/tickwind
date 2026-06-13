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
	"github.com/wombow-ai/tickwind/internal/research"
	"github.com/wombow-ai/tickwind/internal/store/memory"
	"github.com/wombow-ai/tickwind/internal/stream"
)

// fakeResearch is a controllable ResearchSource. Report returns the held data-only
// fact sheet; Compose fills prose on each section when enabled; Enabled/Model are
// fixed. It records how many times Compose ran so the cache/cap can be asserted.
type fakeResearch struct {
	fs       research.FactSheet
	enabled  bool
	model    string
	prose    map[string]string
	composes int
}

func (f *fakeResearch) Report(context.Context, string) research.FactSheet { return f.fs }

func (f *fakeResearch) Compose(_ context.Context, fs research.FactSheet, _ string) research.FactSheet {
	f.composes++
	if !f.enabled {
		return fs
	}
	for i := range fs.Sections {
		if p, ok := f.prose[fs.Sections[i].Key]; ok {
			fs.Sections[i].Prose = p
		}
	}
	return fs
}

func (f *fakeResearch) Enabled() bool { return f.enabled }

func (f *fakeResearch) Model() string {
	if !f.enabled {
		return ""
	}
	return f.model
}

// serverWithResearch builds an httptest server whose ResearchSource is the fake.
func serverWithResearch(src ResearchSource) *httptest.Server {
	h := New(
		memory.New(), stream.NewHub(), enrich.Noop{},
		auth.NewVerifier(testSecret, ""),
		nil, // bars
		nil, // topics
		nil, // opportunities
		nil, // universe
		nil, // gurus
		nil, // ingestor
		nil, // symbols
		nil, // events
		nil, // fundamentals
		nil, // earnings
		nil, // congress
		nil, // institutional
		nil, // live
		nil, // indices
		nil, // short
		nil, // briefing
		nil, // options
		nil, // 13f
		nil, // admin ids
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if src != nil {
		h.SetResearch(src)
	}
	return httptest.NewServer(h)
}

// researchResp is the wire shape of GET /v1/stocks/{ticker}/research (design §3.4).
type researchResp struct {
	Ticker     string                  `json:"ticker"`
	Name       string                  `json:"name"`
	AsOf       string                  `json:"as_of"`
	PriceLabel string                  `json:"price_label"`
	Model      string                  `json:"model"`
	LLM        bool                    `json:"llm"`
	Disclaimer string                  `json:"disclaimer"`
	Sections   []research.SectionFacts `json:"sections"`
}

func getResearch(t *testing.T, url string) (*http.Response, researchResp) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	var body researchResp
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	resp.Body.Close()
	return resp, body
}

// sampleSheet is a minimal data-only fact sheet with one ok fact in one section.
func sampleSheet() research.FactSheet {
	raw := 31.2
	return research.FactSheet{
		Ticker:     "AAPL",
		Name:       "Apple Inc.",
		AsOf:       "2026-06-12",
		PriceLabel: "$190.12 · alpaca · regular",
		Disclaimer: research.Disclaimer,
		Sections: []research.SectionFacts{{
			Key: "valuation", TitleZH: "估值", TitleEN: "Valuation",
			Facts: []research.Fact{{
				Key: "pe", LabelZH: "市盈率(P/E)", LabelEN: "P/E (TTM)",
				Value: "31.2x", Raw: &raw, Unit: "x", Status: research.StatusOK,
				Source: "SEC XBRL FY2024",
			}},
		}},
	}
}

func TestGetResearch_NilSource404(t *testing.T) {
	srv := serverWithResearch(nil) // never SetResearch
	defer srv.Close()

	resp, _ := getResearch(t, srv.URL+"/v1/stocks/AAPL/research")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d; want 404 for a nil research source", resp.StatusCode)
	}
}

func TestGetResearch_EmptySheet404(t *testing.T) {
	// A real-but-unknown ticker: the assembled sheet has no sections and no as_of.
	srv := serverWithResearch(&fakeResearch{fs: research.FactSheet{Ticker: "ZZZZ"}})
	defer srv.Close()

	resp, _ := getResearch(t, srv.URL+"/v1/stocks/ZZZZ/research")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d; want 404 for an empty fact sheet", resp.StatusCode)
	}
}

func TestGetResearch_EnabledHappyPath(t *testing.T) {
	fake := &fakeResearch{
		fs:      sampleSheet(),
		enabled: true,
		model:   "deepseek-chat",
		prose:   map[string]string{"valuation": "估值处于其历史区间偏高位。"},
	}
	srv := serverWithResearch(fake)
	defer srv.Close()

	resp, body := getResearch(t, srv.URL+"/v1/stocks/AAPL/research")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	if !body.LLM {
		t.Error("llm = false; want true when prose was generated")
	}
	if body.Model != "deepseek-chat" {
		t.Errorf("model = %q; want deepseek-chat", body.Model)
	}
	if len(body.Sections) != 1 {
		t.Fatalf("got %d sections; want 1", len(body.Sections))
	}
	sec := body.Sections[0]
	if sec.Prose == "" {
		t.Error("section prose is empty; want LLM prose")
	}
	if len(sec.Facts) != 1 || sec.Facts[0].Value != "31.2x" {
		t.Errorf("facts = %+v; want one ok fact 31.2x", sec.Facts)
	}
	if body.Disclaimer != research.Disclaimer {
		t.Errorf("disclaimer = %q; want the mandatory label", body.Disclaimer)
	}

	// A second request for the same (ticker, day, lang) must hit the cache — Compose
	// runs exactly once.
	if _, _ = getResearch(t, srv.URL+"/v1/stocks/AAPL/research"); fake.composes != 1 {
		t.Errorf("Compose ran %d times; want 1 (second request served from cache)", fake.composes)
	}
}

func TestGetResearch_DisabledDataOnly(t *testing.T) {
	fake := &fakeResearch{fs: sampleSheet(), enabled: false}
	srv := serverWithResearch(fake)
	defer srv.Close()

	resp, body := getResearch(t, srv.URL+"/v1/stocks/AAPL/research")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200 (data-only, never 503)", resp.StatusCode)
	}
	if body.LLM {
		t.Error("llm = true; want false when the enricher is disabled")
	}
	if body.Model != "" {
		t.Errorf("model = %q; want empty when disabled", body.Model)
	}
	if fake.composes != 0 {
		t.Errorf("Compose ran %d times; want 0 when disabled", fake.composes)
	}
	if len(body.Sections) != 1 || body.Sections[0].Prose != "" {
		t.Errorf("want one prose-less section; got %+v", body.Sections)
	}
}
