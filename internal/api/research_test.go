package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
// (atomically — the deep path runs ComposeDeep in a background goroutine) so the
// cache/cap and the depth routing can be asserted.
//
// gate, when non-nil, blocks ComposeDeep until the test closes it — so a test can
// observe the "generating" window (the bg gen is in flight) deterministically before
// the prose lands. failDeep makes ComposeDeep return empty prose (a failed gen).
type fakeResearch struct {
	fs           research.FactSheet
	enabled      bool
	model        string
	deepModel    string
	prose        map[string]string
	composes     int32
	deepComposes int32
	gate         chan struct{}
	failDeep     atomic.Bool
}

func (f *fakeResearch) Report(context.Context, string) research.FactSheet { return f.fs }

func (f *fakeResearch) Compose(_ context.Context, fs research.FactSheet, _ string) research.FactSheet {
	atomic.AddInt32(&f.composes, 1)
	if !f.enabled {
		return fs
	}
	fs.Sections = cloneSections(fs.Sections)
	for i := range fs.Sections {
		if p, ok := f.prose[fs.Sections[i].Key]; ok {
			fs.Sections[i].Prose = p
		}
	}
	return fs
}

func (f *fakeResearch) ComposeDeep(_ context.Context, fs research.FactSheet, _ string) research.FactSheet {
	atomic.AddInt32(&f.deepComposes, 1)
	if f.gate != nil {
		<-f.gate // block until the test releases the bg gen
	}
	if !f.enabled || f.failDeep.Load() {
		return fs // failDeep → no prose set → a "failed" generation
	}
	fs.Sections = cloneSections(fs.Sections)
	for i := range fs.Sections {
		if p, ok := f.prose[fs.Sections[i].Key]; ok {
			fs.Sections[i].Prose = "[deep] " + p
		}
	}
	return fs
}

func (f *fakeResearch) composeCount() int32     { return atomic.LoadInt32(&f.composes) }
func (f *fakeResearch) deepComposeCount() int32 { return atomic.LoadInt32(&f.deepComposes) }

// cloneSections deep-copies the section slice so a background ComposeDeep mutating
// prose can't race the data-only sheet held by the caller (the real research.Service
// returns a fresh sheet too; the fake mirrors that).
func cloneSections(in []research.SectionFacts) []research.SectionFacts {
	out := make([]research.SectionFacts, len(in))
	copy(out, in)
	return out
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
	srv, _ := serverWithResearchStore(src)
	return srv
}

// serverWithResearchStore is serverWithResearch but also returns the backing memory
// store, so a test can read the per-user deep-research quota counter directly (the
// async deep path charges it from a background goroutine).
func serverWithResearchStore(src ResearchSource) (*httptest.Server, *memory.Store) {
	st := memory.New()
	h := New(
		st, stream.NewHub(), enrich.Noop{},
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
	return httptest.NewServer(h), st
}

// deepQuotaUsed reads user `sub`'s deep-research quota count for the current ET month
// (the key the async deep path charges against) from the test's memory store.
func deepQuotaUsed(t *testing.T, st *memory.Store, sub string) int {
	t.Helper()
	used, err := st.GetDeepQuotaUsed(context.Background(), sub, researchMonth())
	if err != nil {
		t.Fatalf("GetDeepQuotaUsed: %v", err)
	}
	return used
}

// waitFor polls cond until it is true or a short deadline elapses (for asserting a
// background goroutine's effect without a fixed sleep).
func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// researchResp is the wire shape of GET /v1/stocks/{ticker}/research (design §3.4).
type researchResp struct {
	Ticker      string                  `json:"ticker"`
	Name        string                  `json:"name"`
	AsOf        string                  `json:"as_of"`
	PriceLabel  string                  `json:"price_label"`
	Model       string                  `json:"model"`
	LLM         bool                    `json:"llm"`
	ProseStatus string                  `json:"prose_status"`
	Disclaimer  string                  `json:"disclaimer"`
	Sections    []research.SectionFacts `json:"sections"`
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
	if _, _ = getResearch(t, srv.URL+"/v1/stocks/AAPL/research"); fake.composeCount() != 1 {
		t.Errorf("Compose ran %d times; want 1 (second request served from cache)", fake.composeCount())
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

// pollDeepUntilReady polls the deep endpoint as `sub` until prose_status flips to
// "ready" (the bg gen landed) or a timeout. It returns the final ready body, failing
// the test if the report never becomes ready. Every poll asserts the body always
// carries the Go-owned facts (off the critical path), and never errors.
func pollDeepUntilReady(t *testing.T, url, sub string) researchResp {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, body := getResearchAs(t, url, sub)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("poll status = %d; want 200 (data-only is always served)", resp.StatusCode)
		}
		switch body.ProseStatus {
		case proseStatusReady:
			return body
		case proseStatusGenerating:
			time.Sleep(5 * time.Millisecond)
		default:
			t.Fatalf("unexpected prose_status %q while polling for ready", body.ProseStatus)
		}
	}
	t.Fatal("deep report never became ready within the poll deadline")
	return researchResp{}
}

// TestGetResearch_DeepAsyncFlow covers the async contract end-to-end:
//   - first deep request → data-only + "generating" + a bg gen is kicked off (NOT
//     Compose; the normal path is untouched);
//   - polling while generating → still "generating", NO second gen, NO double-charge;
//   - after the gen lands → "ready" with the richer (deep) prose + deep model;
//   - the quota is charged exactly once;
//   - a later view (any user) is a free "ready" cache hit (no new gen, no charge).
func TestGetResearch_DeepAsyncFlow(t *testing.T) {
	fake := &fakeResearch{
		fs:        sampleSheet(),
		enabled:   true,
		model:     "deepseek-chat",
		deepModel: "anthropic/claude-opus",
		prose:     map[string]string{"valuation": "估值处于其历史区间偏高位。"},
		gate:      make(chan struct{}), // hold the bg gen open so we can poll mid-flight
	}
	srv, st := serverWithResearchStore(fake)
	defer srv.Close()
	url := srv.URL + "/v1/stocks/AAPL/research?depth=deep"

	// First deep request → INSTANT data-only + "generating"; the bg gen is started.
	resp, body := getResearchAs(t, url, "user-1")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first deep status = %d; want 200", resp.StatusCode)
	}
	if body.ProseStatus != proseStatusGenerating {
		t.Fatalf("first deep prose_status = %q; want %q", body.ProseStatus, proseStatusGenerating)
	}
	if body.LLM {
		t.Error("first deep llm=true; want false (data-only while generating)")
	}
	// The Go-owned facts are present immediately, prose empty.
	if len(body.Sections) != 1 || body.Sections[0].Facts[0].Value != "31.2x" || body.Sections[0].Prose != "" {
		t.Errorf("first deep sections = %+v; want the Go-owned fact + empty prose", body.Sections)
	}
	waitFor(t, func() bool { return fake.deepComposeCount() == 1 }, "ComposeDeep to start")
	if fake.composeCount() != 0 {
		t.Errorf("Compose ran %d times; want 0 (deep path must not touch the normal compose)", fake.composeCount())
	}

	// Poll while the bg gen is held open → still "generating", no second gen, no charge.
	for i := 0; i < 3; i++ {
		_, pb := getResearchAs(t, url, "user-1")
		if pb.ProseStatus != proseStatusGenerating {
			t.Fatalf("poll #%d prose_status = %q; want %q (still generating)", i, pb.ProseStatus, proseStatusGenerating)
		}
	}
	if fake.deepComposeCount() != 1 {
		t.Fatalf("ComposeDeep ran %d times during polling; want exactly 1 (single-flight)", fake.deepComposeCount())
	}
	if used := deepQuotaUsed(t, st, "user-1"); used != 0 { // quota not charged until the gen succeeds
		t.Fatalf("quota used = %d during generating; want 0 (charge only on success)", used)
	}

	// Release the bg gen → it composes prose, caches "ready", and charges the quota once.
	close(fake.gate)
	body = pollDeepUntilReady(t, url, "user-1")
	if !body.LLM || body.Model != "anthropic/claude-opus" {
		t.Fatalf("ready body: llm=%v model=%q; want true / the deep model", body.LLM, body.Model)
	}
	if len(body.Sections) != 1 || body.Sections[0].Prose != "[deep] 估值处于其历史区间偏高位。" {
		t.Errorf("ready prose = %+v; want the richer (deep) prose", body.Sections)
	}
	if body.Sections[0].Facts[0].Value != "31.2x" {
		t.Errorf("ready facts changed: %+v; want the unchanged Go-owned 31.2x", body.Sections[0].Facts)
	}
	if fake.deepComposeCount() != 1 {
		t.Errorf("ComposeDeep ran %d times total; want exactly 1", fake.deepComposeCount())
	}
	if used := deepQuotaUsed(t, st, "user-1"); used != 1 {
		t.Errorf("quota used = %d after a successful gen; want exactly 1", used)
	}

	// A later view (even a DIFFERENT user) is a free "ready" cache hit — no new gen,
	// no extra charge.
	_, cb := getResearchAs(t, url, "user-2")
	if cb.ProseStatus != proseStatusReady || !cb.LLM {
		t.Errorf("cached view by user-2: prose_status=%q llm=%v; want ready/true", cb.ProseStatus, cb.LLM)
	}
	if fake.deepComposeCount() != 1 {
		t.Errorf("ComposeDeep ran %d times; want still 1 (cache hit)", fake.deepComposeCount())
	}
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
	if fake.deepComposeCount() != 0 {
		t.Errorf("ComposeDeep ran %d times for an anon deep request; want 0 (gated before generation)", fake.deepComposeCount())
	}

	// The normal path is unaffected by deep gating — anon still gets a 200.
	if resp2, _ := getResearchAs(t, srv.URL+"/v1/stocks/AAPL/research", ""); resp2.StatusCode != http.StatusOK {
		t.Errorf("anon normal status = %d; want 200 (normal /research stays public)", resp2.StatusCode)
	}
}

// TestGetResearch_DeepQuotaExhausted covers the monthly quota across users:
//   - user-1's first deep ticker generates (async) + consumes their monthly slot;
//   - a SECOND, different-ticker deep request by the SAME user (limit 1) is over quota
//     and the new ticker isn't cached → graceful data-only "quota_exhausted" (200, NOT
//     429), and NO new gen runs;
//   - a different user still has their own slot → it generates → "ready";
//   - viewing an ALREADY-cached deep report is free for the over-quota user (no charge,
//     no new gen).
func TestGetResearch_DeepQuotaExhausted(t *testing.T) {
	fake := &fakeResearch{fs: sampleSheet(), enabled: true, model: "deepseek-chat", deepModel: "deep-x", prose: map[string]string{"valuation": "估值处于其历史区间偏高位。"}}
	srv := serverWithResearch(fake)
	defer srv.Close()
	// Default deep limit is 1 (the owner spec); the test server uses that default.

	// user-1's first deep generation: kicks off async, then polls to ready, charging once.
	body := pollDeepUntilReady(t, srv.URL+"/v1/stocks/AAPL/research?depth=deep", "user-1")
	if !body.LLM || body.Model != "deep-x" {
		t.Fatalf("user-1 AAPL ready: llm=%v model=%q; want true / deep-x", body.LLM, body.Model)
	}
	if fake.deepComposeCount() != 1 {
		t.Fatalf("ComposeDeep ran %d times; want 1", fake.deepComposeCount())
	}

	// user-1, SAME month, DIFFERENT ticker (not cached) → over quota → graceful data-only
	// "quota_exhausted" (200, not 429), and NO new gen.
	resp, qb := getResearchAs(t, srv.URL+"/v1/stocks/MSFT/research?depth=deep", "user-1")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("over-quota deep status = %d; want 200 (graceful data-only, not 429)", resp.StatusCode)
	}
	if qb.ProseStatus != proseStatusQuotaExhausted {
		t.Fatalf("over-quota prose_status = %q; want %q", qb.ProseStatus, proseStatusQuotaExhausted)
	}
	if qb.LLM {
		t.Error("over-quota llm=true; want false (data-only)")
	}
	if qb.Sections[0].Facts[0].Value != "31.2x" {
		t.Errorf("over-quota facts = %+v; want the Go-owned data-only sheet", qb.Sections)
	}
	// Give any (erroneously) spawned gen a chance to run, then assert none did.
	time.Sleep(20 * time.Millisecond)
	if fake.deepComposeCount() != 1 {
		t.Errorf("ComposeDeep ran %d times after the over-quota request; want still 1", fake.deepComposeCount())
	}

	// A DIFFERENT user has their own monthly slot → MSFT deep generates for them → ready.
	mb := pollDeepUntilReady(t, srv.URL+"/v1/stocks/MSFT/research?depth=deep", "user-2")
	if !mb.LLM {
		t.Error("user-2 MSFT ready llm=false; want true")
	}
	if fake.deepComposeCount() != 2 {
		t.Errorf("ComposeDeep ran %d times; want 2 (user-2 generated MSFT)", fake.deepComposeCount())
	}

	// Viewing the now-cached MSFT deep is free even for the over-quota user-1.
	_, cb := getResearchAs(t, srv.URL+"/v1/stocks/MSFT/research?depth=deep", "user-1")
	if cb.ProseStatus != proseStatusReady || !cb.LLM {
		t.Errorf("over-quota user-1 cached MSFT: prose_status=%q llm=%v; want ready/true", cb.ProseStatus, cb.LLM)
	}
	if fake.deepComposeCount() != 2 {
		t.Errorf("ComposeDeep ran %d times; want still 2 (cache hit, no new gen)", fake.deepComposeCount())
	}
}

// TestGetResearch_DeepLLMDisabled — when the LLM is OFF, a deep request from a logged-in
// user serves the data-only report (200) with prose_status="llm_disabled", consumes NO
// quota (no LLM ran), and starts NO bg gen. So a second different-ticker deep request
// still succeeds the same way.
func TestGetResearch_DeepLLMDisabled(t *testing.T) {
	fake := &fakeResearch{fs: sampleSheet(), enabled: false} // LLM off → data-only
	srv, st := serverWithResearchStore(fake)
	defer srv.Close()

	for _, tk := range []string{"AAPL", "MSFT"} {
		resp, body := getResearchAs(t, srv.URL+"/v1/stocks/"+tk+"/research?depth=deep", "user-1")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s data-only deep status = %d; want 200 (no quota consumed when LLM off)", tk, resp.StatusCode)
		}
		if body.ProseStatus != proseStatusLLMDisabled {
			t.Errorf("%s deep prose_status = %q; want %q", tk, body.ProseStatus, proseStatusLLMDisabled)
		}
		if body.LLM {
			t.Errorf("%s data-only deep llm=true; want false", tk)
		}
	}
	time.Sleep(20 * time.Millisecond)
	if fake.deepComposeCount() != 0 {
		t.Errorf("ComposeDeep ran %d times with the LLM off; want 0", fake.deepComposeCount())
	}
	if used := deepQuotaUsed(t, st, "user-1"); used != 0 {
		t.Errorf("quota used = %d with the LLM off; want 0", used)
	}
}

// TestGetResearch_DeepFailedGenNoCharge — a failed (empty-prose) bg gen caches nothing
// and charges nothing, so a later poll RETRIES the generation and a success then charges
// exactly once.
func TestGetResearch_DeepFailedGenNoCharge(t *testing.T) {
	fake := &fakeResearch{fs: sampleSheet(), enabled: true, deepModel: "deep-x", prose: map[string]string{"valuation": "ok"}}
	fake.failDeep.Store(true)
	srv, st := serverWithResearchStore(fake)
	defer srv.Close()
	url := srv.URL + "/v1/stocks/AAPL/research?depth=deep"

	// First deep request → generating; the bg gen runs but produces NO prose (fails).
	_, body := getResearchAs(t, url, "user-1")
	if body.ProseStatus != proseStatusGenerating {
		t.Fatalf("first deep prose_status = %q; want %q", body.ProseStatus, proseStatusGenerating)
	}
	// Wait for the failed gen to finish (inflight gate released) so the retry isn't
	// deduped as "generating".
	waitFor(t, func() bool { return fake.deepComposeCount() == 1 }, "the failing bg gen to run")
	// The failed gen must not have charged the quota.
	if used := deepQuotaUsed(t, st, "user-1"); used != 0 {
		t.Fatalf("quota used = %d after a failed gen; want 0", used)
	}

	// A later poll RETRIES (cache miss + quota free) → another gen. Let it succeed now.
	fake.failDeep.Store(false)
	body = pollDeepUntilReady(t, url, "user-1")
	if !body.LLM {
		t.Error("after retry: llm=false; want true (prose now present)")
	}
	if fake.deepComposeCount() != 2 {
		t.Errorf("ComposeDeep ran %d times; want 2 (one failed + one successful retry)", fake.deepComposeCount())
	}
	if used := deepQuotaUsed(t, st, "user-1"); used != 1 {
		t.Errorf("quota used = %d; want exactly 1 (charged only on the successful gen)", used)
	}
}

// TestGetResearch_DeepConcurrentSingleFlight fires many simultaneous FIRST deep
// requests for the same (ticker, month, lang) and asserts the single-flight invariant:
// every request gets a 200 (data-only "generating" — none error), exactly ONE bg
// ComposeDeep runs, and the quota is charged exactly ONCE (no duplicate gen, no
// double-charge under concurrency).
func TestGetResearch_DeepConcurrentSingleFlight(t *testing.T) {
	fake := &fakeResearch{
		fs:        sampleSheet(),
		enabled:   true,
		deepModel: "deep-x",
		prose:     map[string]string{"valuation": "ok"},
		gate:      make(chan struct{}), // hold the one gen open until all racers are in
	}
	srv, st := serverWithResearchStore(fake)
	defer srv.Close()
	url := srv.URL + "/v1/stocks/AAPL/research?depth=deep"

	const racers = 12
	var wg sync.WaitGroup
	wg.Add(racers)
	for i := 0; i < racers; i++ {
		go func() {
			defer wg.Done()
			resp, body := getResearchAs(t, url, "user-1")
			if resp.StatusCode != http.StatusOK {
				t.Errorf("racing deep status = %d; want 200", resp.StatusCode)
				return
			}
			if body.ProseStatus != proseStatusGenerating {
				t.Errorf("racing deep prose_status = %q; want %q", body.ProseStatus, proseStatusGenerating)
			}
		}()
	}
	wg.Wait()

	// Exactly one generator was elected even under the race.
	if got := fake.deepComposeCount(); got != 1 {
		t.Fatalf("ComposeDeep ran %d times across %d concurrent first requests; want exactly 1", got, racers)
	}
	close(fake.gate) // let the single gen finish
	pollDeepUntilReady(t, url, "user-1")
	if got := fake.deepComposeCount(); got != 1 {
		t.Errorf("ComposeDeep ran %d times total; want exactly 1", got)
	}
	if used := deepQuotaUsed(t, st, "user-1"); used != 1 {
		t.Errorf("quota used = %d; want exactly 1 (charged once despite the race)", used)
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
	if fake.composeCount() != 0 {
		t.Errorf("Compose ran %d times; want 0 when disabled", fake.composeCount())
	}
	if len(body.Sections) != 1 || body.Sections[0].Prose != "" {
		t.Errorf("want one prose-less section; got %+v", body.Sections)
	}
}
