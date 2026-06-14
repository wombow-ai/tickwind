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
// fact sheet; Compose / ComposeDeep fill prose on each section when enabled (the
// deep path prefixes "[deep] " so the test can tell the paths apart); Enabled /
// Model / DeepModel are fixed. It records how many times Compose / ComposeDeep ran
// so the cache/cap and the depth routing can be asserted.
type fakeResearch struct {
	fs           research.FactSheet
	enabled      bool
	model        string
	deepModel    string
	prose        map[string]string
	composes     int
	deepComposes int
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

func (f *fakeResearch) ComposeDeep(_ context.Context, fs research.FactSheet, _ string) research.FactSheet {
	f.deepComposes++
	if !f.enabled {
		return fs
	}
	for i := range fs.Sections {
		if p, ok := f.prose[fs.Sections[i].Key]; ok {
			fs.Sections[i].Prose = "[deep] " + p
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

func (f *fakeResearch) DeepModel() string {
	if !f.enabled {
		return ""
	}
	if f.deepModel != "" {
		return f.deepModel
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

func TestGetResearch_DeepDepth(t *testing.T) {
	fake := &fakeResearch{
		fs:        sampleSheet(),
		enabled:   true,
		model:     "deepseek-chat",
		deepModel: "anthropic/claude-opus",
		prose:     map[string]string{"valuation": "估值处于其历史区间偏高位。"},
	}
	srv := serverWithResearch(fake)
	defer srv.Close()

	// depth=deep routes to ComposeDeep (not Compose) and reports the deep model.
	// Deep is login-gated, so this (and every deep request below) carries a token.
	resp, body := getResearchAs(t, srv.URL+"/v1/stocks/AAPL/research?depth=deep", "user-1")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	if fake.deepComposes != 1 || fake.composes != 0 {
		t.Fatalf("deepComposes=%d composes=%d; want ComposeDeep to run once and Compose never", fake.deepComposes, fake.composes)
	}
	if body.Model != "anthropic/claude-opus" {
		t.Errorf("model = %q; want the deep model", body.Model)
	}
	if len(body.Sections) != 1 || body.Sections[0].Prose != "[deep] 估值处于其历史区间偏高位。" {
		t.Errorf("want the richer (deep) prose; got %+v", body.Sections)
	}
	// Facts are Go-owned and identical to the normal path — the deep compose only
	// touches prose.
	if len(body.Sections[0].Facts) != 1 || body.Sections[0].Facts[0].Value != "31.2x" {
		t.Errorf("facts = %+v; want the unchanged Go-owned 31.2x", body.Sections[0].Facts)
	}

	// The deep report caches under its own key: a normal (ungated, anon-OK) request
	// still hits the normal Compose path (separate cache entry), so they never collide.
	if _, normal := getResearch(t, srv.URL+"/v1/stocks/AAPL/research"); normal.Model != "deepseek-chat" {
		t.Errorf("normal model = %q; want deepseek-chat", normal.Model)
	}
	if fake.composes != 1 {
		t.Errorf("Compose ran %d times; want 1 (deep request used a separate cache key)", fake.composes)
	}
	// A second deep request for AAPL (already cached) is served free from the deep
	// cache — ComposeDeep stays at one run AND no quota is consumed, so user-1 can
	// still re-view it even though the limit is 1.
	if _, _ = getResearchAs(t, srv.URL+"/v1/stocks/AAPL/research?depth=deep", "user-1"); fake.deepComposes != 1 {
		t.Errorf("ComposeDeep ran %d times; want 1 (second deep request from cache)", fake.deepComposes)
	}
}

// getResearchAs issues GET url with an optional Bearer token (sub=="" → anonymous,
// no Authorization header) and decodes the body on 200.
func getResearchAs(t *testing.T, url, sub string) (*http.Response, researchResp) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sub != "" {
		req.Header.Set("Authorization", "Bearer "+token(sub))
	}
	resp, err := http.DefaultClient.Do(req)
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

// TestGetResearch_DeepAnon401 — depth=deep requires login: an anonymous (no token)
// deep request is rejected 401, while the normal (ungated) path stays open to anon.
func TestGetResearch_DeepAnon401(t *testing.T) {
	fake := &fakeResearch{fs: sampleSheet(), enabled: true, model: "deepseek-chat", deepModel: "deep-x", prose: map[string]string{"valuation": "估值处于其历史区间偏高位。"}}
	srv := serverWithResearch(fake)
	defer srv.Close()

	// Anonymous deep → 401, and NO compose ran (gated before generation).
	resp, _ := getResearchAs(t, srv.URL+"/v1/stocks/AAPL/research?depth=deep", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon deep status = %d; want 401", resp.StatusCode)
	}
	if fake.deepComposes != 0 {
		t.Errorf("ComposeDeep ran %d times for an anon deep request; want 0 (gated before generation)", fake.deepComposes)
	}

	// The normal path is unaffected by deep gating — anon still gets a 200.
	if resp2, _ := getResearchAs(t, srv.URL+"/v1/stocks/AAPL/research", ""); resp2.StatusCode != http.StatusOK {
		t.Errorf("anon normal status = %d; want 200 (normal /research stays public)", resp2.StatusCode)
	}
}

// TestGetResearch_DeepQuota covers the full per-user generation quota:
//   - first logged-in deep request generates + consumes the user's daily slot;
//   - a SECOND, different-ticker deep request by the SAME user the same day is over
//     quota (limit 1) and the new ticker isn't cached → 429;
//   - a different user still has their own slot → 200;
//   - viewing an ALREADY-cached deep report is free (no quota, no new compose) — even
//     for a user who is over quota.
func TestGetResearch_DeepQuota(t *testing.T) {
	fake := &fakeResearch{fs: sampleSheet(), enabled: true, model: "deepseek-chat", deepModel: "deep-x", prose: map[string]string{"valuation": "估值处于其历史区间偏高位。"}}
	srv := serverWithResearch(fake)
	defer srv.Close()
	// Default deep limit is 1 (the owner spec); the test server uses that default.

	// user-1's first deep generation: 200, consumes the quota, ComposeDeep ran once.
	if resp, body := getResearchAs(t, srv.URL+"/v1/stocks/AAPL/research?depth=deep", "user-1"); resp.StatusCode != http.StatusOK {
		t.Fatalf("user-1 first deep status = %d; want 200", resp.StatusCode)
	} else if !body.LLM || body.Model != "deep-x" {
		t.Fatalf("user-1 first deep: llm=%v model=%q; want true / deep-x", body.LLM, body.Model)
	}
	if fake.deepComposes != 1 {
		t.Fatalf("ComposeDeep ran %d times; want 1", fake.deepComposes)
	}

	// user-1, SAME day, DIFFERENT ticker (not cached) → over quota → 429, no compose.
	if resp, _ := getResearchAs(t, srv.URL+"/v1/stocks/MSFT/research?depth=deep", "user-1"); resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("user-1 second-ticker deep status = %d; want 429 (over daily quota)", resp.StatusCode)
	}
	if fake.deepComposes != 1 {
		t.Errorf("ComposeDeep ran %d times after the over-quota request; want still 1", fake.deepComposes)
	}

	// A DIFFERENT user has their own daily slot → MSFT deep generates for them: 200.
	if resp, _ := getResearchAs(t, srv.URL+"/v1/stocks/MSFT/research?depth=deep", "user-2"); resp.StatusCode != http.StatusOK {
		t.Fatalf("user-2 deep status = %d; want 200 (own quota)", resp.StatusCode)
	}
	if fake.deepComposes != 2 {
		t.Errorf("ComposeDeep ran %d times; want 2 (user-2 generated MSFT)", fake.deepComposes)
	}

	// Viewing an ALREADY-cached deep report is free even for the over-quota user-1:
	// MSFT is now globally cached (user-2 generated it) → user-1 gets it 200, NO new
	// compose, NO quota touched (the served report benefits everyone).
	if resp, body := getResearchAs(t, srv.URL+"/v1/stocks/MSFT/research?depth=deep", "user-1"); resp.StatusCode != http.StatusOK {
		t.Fatalf("over-quota user-1 viewing cached MSFT deep = %d; want 200 (cache hit is free)", resp.StatusCode)
	} else if !body.LLM {
		t.Errorf("cached MSFT deep llm=false; want the cached prose served")
	}
	if fake.deepComposes != 2 {
		t.Errorf("ComposeDeep ran %d times; want still 2 (cache hit, no new generation)", fake.deepComposes)
	}
}

// TestGetResearch_DeepDataOnlyNoQuota — when the LLM is OFF, a deep request from a
// logged-in user serves the data-only report (200) and does NOT consume the user's
// quota (no LLM ran). So a second different-ticker deep request still succeeds.
func TestGetResearch_DeepDataOnlyNoQuota(t *testing.T) {
	fake := &fakeResearch{fs: sampleSheet(), enabled: false} // LLM off → data-only
	srv := serverWithResearch(fake)
	defer srv.Close()

	for _, tk := range []string{"AAPL", "MSFT"} {
		resp, body := getResearchAs(t, srv.URL+"/v1/stocks/"+tk+"/research?depth=deep", "user-1")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s data-only deep status = %d; want 200 (no quota consumed when LLM off)", tk, resp.StatusCode)
		}
		if body.LLM {
			t.Errorf("%s data-only deep llm=true; want false", tk)
		}
	}
	if fake.deepComposes != 0 {
		t.Errorf("ComposeDeep ran %d times with the LLM off; want 0", fake.deepComposes)
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
