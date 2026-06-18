package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/auth"
	"github.com/wombow-ai/tickwind/internal/enrich"
	"github.com/wombow-ai/tickwind/internal/movement"
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/store/memory"
	"github.com/wombow-ai/tickwind/internal/stream"
)

// fakeMovement is a controllable MovementSource. Report returns the held data-only
// Explanation; Explain overlays an LLM sentence when enabled; Enabled/Model are
// fixed. It records how many times Explain ran so the cache/cap can be asserted.
type fakeMovement struct {
	exp        movement.Explanation
	enabled    bool
	model      string
	sentence   string
	explains   atomic.Int32 // bumped by Explain, which now runs in a bg goroutine
	reportLang string       // records the lang the handler threaded into Report
}

func (f *fakeMovement) Report(_ context.Context, _, lang string) movement.Explanation {
	f.reportLang = lang
	return f.exp
}

func (f *fakeMovement) Explain(_ context.Context, _, _ string) movement.Explanation {
	f.explains.Add(1)
	exp := f.exp
	if f.enabled && exp.Significant {
		exp.Text = f.sentence
		exp.LLM = true
		exp.Model = f.model
		exp.Disclaimer = movement.DisclaimerZH
	}
	return exp
}

func (f *fakeMovement) Enabled() bool { return f.enabled }

func (f *fakeMovement) Model() string {
	if !f.enabled {
		return ""
	}
	return f.model
}

// serverWithMovement builds an httptest server whose MovementSource is the fake.
func serverWithMovement(src MovementSource) *httptest.Server {
	h := New(
		memory.New(), stream.NewHub(), enrich.Noop{},
		auth.NewVerifier(testSecret, ""),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, // admin ids
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if src != nil {
		h.SetMovement(src)
	}
	return httptest.NewServer(h)
}

// movementResp is the wire shape of GET /v1/stocks/{ticker}/movement.
type movementResp struct {
	Ticker      string              `json:"ticker"`
	Significant bool                `json:"significant"`
	ChangePct   float64             `json:"change_pct"`
	Direction   string              `json:"direction"`
	Session     string              `json:"session"`
	Explanation string              `json:"explanation"`
	Evidence    []movement.Evidence `json:"evidence"`
	LLM         bool                `json:"llm"`
	Model       string              `json:"model"`
	Disclaimer  string              `json:"disclaimer"`
	ProseStatus string              `json:"prose_status"`
}

func getMovement(t *testing.T, url string) (*http.Response, movementResp) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	var body movementResp
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	resp.Body.Close()
	return resp, body
}

// pollMovementReady requests the movement endpoint until its prose_status is no longer
// "generating" (i.e. the async bg LLM gen completed and cached a terminal result), or
// fails after a cap. Used to await the detached background sentence in tests. NOTE: a
// FAILED bg gen caches nothing (so the next request re-spawns) — only use this for the
// success path; for the failure/timeout path await the cap counter instead.
func pollMovementReady(t *testing.T, url string) (*http.Response, movementResp) {
	t.Helper()
	for i := 0; i < 80; i++ {
		resp, body := getMovement(t, url)
		if resp.StatusCode != http.StatusOK || body.ProseStatus != proseStatusGenerating {
			return resp, body
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("movement stayed prose_status=generating; the bg gen never completed")
	return nil, movementResp{}
}

// waitMoveCapZero polls the server's movement daily-cap counter until it returns to 0
// (the detached bg gen finished and a failed/empty gen refunded its reserved slot), or
// fails after a cap. Acquiring moveMu establishes happens-before with the bg goroutine,
// so reads of the fake enricher's atomics after this returns are safe.
func waitMoveCapZero(t *testing.T, s *Server) {
	t.Helper()
	for i := 0; i < 80; i++ {
		s.moveMu.Lock()
		c := s.moveDayCount
		s.moveMu.Unlock()
		if c == 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("moveDayCount never returned to 0 (a failed bg gen must refund the cap)")
}

// significantExp is a data-only significant Explanation (the shape Report returns
// for a +10% move with one news evidence and the canned line).
func significantExp() movement.Explanation {
	return movement.Explanation{
		Ticker:      "AAPL",
		Significant: true,
		ChangePct:   10,
		Direction:   "up",
		Session:     "regular",
		Text:        "今日涨10.0%;近期消息:Apple beats estimates",
		Evidence: []movement.Evidence{
			{Type: "news", Title: "Apple beats estimates", URL: "https://n/1", Time: time.Now()},
		},
		LLM:  false,
		AsOf: time.Now().UTC(),
	}
}

func TestGetMovement_NilSource404(t *testing.T) {
	srv := serverWithMovement(nil) // never SetMovement
	defer srv.Close()

	resp, _ := getMovement(t, srv.URL+"/v1/stocks/AAPL/movement")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d; want 404 for a nil movement source", resp.StatusCode)
	}
}

func TestGetMovement_EmptyExplanation404(t *testing.T) {
	// A real-but-unknown ticker: no quote (zero as_of, zero change) and no evidence.
	srv := serverWithMovement(&fakeMovement{exp: movement.Explanation{Ticker: "ZZZZ"}})
	defer srv.Close()

	resp, _ := getMovement(t, srv.URL+"/v1/stocks/ZZZZ/movement")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d; want 404 for an empty explanation", resp.StatusCode)
	}
}

func TestGetMovement_InsignificantIsServed200(t *testing.T) {
	// A sub-threshold move with a real quote: significant:false, but still a 200 with
	// the number (the frontend hides the card, it is not a 404).
	exp := movement.Explanation{
		Ticker: "AAPL", Significant: false, ChangePct: 2.1, Direction: "up",
		Session: "regular", AsOf: time.Now().UTC(),
	}
	srv := serverWithMovement(&fakeMovement{exp: exp})
	defer srv.Close()

	resp, body := getMovement(t, srv.URL+"/v1/stocks/AAPL/movement")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200 for a sub-threshold move with a quote", resp.StatusCode)
	}
	if body.Significant {
		t.Error("significant = true; want false")
	}
	if body.Explanation != "" {
		t.Errorf("explanation = %q; want empty when not significant", body.Explanation)
	}
	if len(body.Evidence) != 0 {
		t.Errorf("evidence = %+v; want none when not significant", body.Evidence)
	}
}

func TestGetMovement_DataOnlyWithNoopEnricher(t *testing.T) {
	// LLM disabled: the significant explanation serves 200 with the canned line +
	// evidence, llm:false, no Explain (LLM) call.
	fake := &fakeMovement{exp: significantExp(), enabled: false}
	srv := serverWithMovement(fake)
	defer srv.Close()

	resp, body := getMovement(t, srv.URL+"/v1/stocks/AAPL/movement")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200 (data-only, never 503)", resp.StatusCode)
	}
	if body.LLM {
		t.Error("llm = true; want false when the enricher is disabled")
	}
	if body.Model != "" {
		t.Errorf("model = %q; want empty when disabled", body.Model)
	}
	if body.ChangePct != 10 || body.Direction != "up" {
		t.Errorf("got %v %q; want the Go-owned +10 up", body.ChangePct, body.Direction)
	}
	if body.Explanation == "" {
		t.Error("explanation is empty; want the canned data-only line")
	}
	if len(body.Evidence) != 1 || body.Evidence[0].Type != "news" {
		t.Errorf("evidence = %+v; want one attributed news item", body.Evidence)
	}
	if fake.explains.Load() != 0 {
		t.Errorf("Explain ran %d times; want 0 when disabled (data-only Report path)", fake.explains.Load())
	}
}

// TestGetMovement_ThreadsLangIntoReport proves the handler passes ?lang=en into
// Report so the data-only canned line / Go-built evidence come back in English
// (the en-mode regression — the data-only path is the one that ships when the LLM
// is off/over-cap/errors).
func TestGetMovement_ThreadsLangIntoReport(t *testing.T) {
	fake := &fakeMovement{exp: significantExp(), enabled: false}
	srv := serverWithMovement(fake)
	defer srv.Close()

	if _, _ = getMovement(t, srv.URL+"/v1/stocks/AAPL/movement?lang=en"); fake.reportLang != "en" {
		t.Errorf("Report saw lang=%q; want en (handler must thread the requested language)", fake.reportLang)
	}
}

func TestGetMovement_EnabledLLMCachedOnce(t *testing.T) {
	fake := &fakeMovement{
		exp:      significantExp(),
		enabled:  true,
		model:    "deepseek-chat",
		sentence: "今日涨10.0%,可能与财报超预期有关。",
	}
	srv := serverWithMovement(fake)
	defer srv.Close()

	// ASYNC: the first request returns the canned line INSTANTLY with
	// prose_status="generating" (llm:false) and composes the LLM sentence in a detached
	// bg goroutine; the client polls the same URL until it flips to "ready".
	resp, body := getMovement(t, srv.URL+"/v1/stocks/AAPL/movement")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	if body.ProseStatus != proseStatusGenerating {
		t.Errorf("first prose_status = %q; want generating (the LLM sentence composes in the background)", body.ProseStatus)
	}
	if body.LLM {
		t.Error("first response llm = true; want the canned line (false) while generating")
	}
	if !body.Significant || body.Explanation == "" {
		t.Error("first response should carry the canned data-only line for a significant move")
	}

	// Poll until the bg gen lands the upgrade → "ready" with the LLM sentence.
	resp, body = pollMovementReady(t, srv.URL+"/v1/stocks/AAPL/movement")
	if resp.StatusCode != http.StatusOK || body.ProseStatus != proseStatusReady {
		t.Fatalf("after polling: status=%d prose_status=%q; want 200 ready", resp.StatusCode, body.ProseStatus)
	}
	if !body.LLM {
		t.Error("llm = false; want true when the LLM produced a sentence")
	}
	if body.Model != "deepseek-chat" {
		t.Errorf("model = %q; want deepseek-chat", body.Model)
	}
	if body.Explanation != fake.sentence {
		t.Errorf("explanation = %q; want the LLM sentence", body.Explanation)
	}
	if body.Disclaimer != movement.DisclaimerZH {
		t.Errorf("disclaimer = %q; want the mandatory label", body.Disclaimer)
	}

	// Further requests hit the cache — Explain ran exactly once (the single bg gen).
	if _, _ = getMovement(t, srv.URL+"/v1/stocks/AAPL/movement"); fake.explains.Load() != 1 {
		t.Errorf("Explain ran %d times; want 1 (one bg gen, then served from cache)", fake.explains.Load())
	}
}

// slowExplainEnricher is a movement.Enricher whose ExplainMove HONORS its context:
// it blocks until the context is canceled, then returns the cancellation error. The
// per-call timeout the handler imposes is what cancels it — proving the deadline
// reaches the (would-be HTTP) enrich call, not just the goroutine. It records that
// it ran so the cache/cap refund can be asserted.
type slowExplainEnricher struct {
	calls   atomic.Int32 // bumped by ExplainMove, which now runs in a bg goroutine
	gotDone atomic.Bool
}

func (s *slowExplainEnricher) Enabled() bool { return true }
func (s *slowExplainEnricher) ExplainMove(ctx context.Context, _, _ string) (string, error) {
	s.calls.Add(1)
	<-ctx.Done() // block until the handler's per-call timeout (or parent cancel) fires
	s.gotDone.Store(true)
	return "", ctx.Err() // the enrich layer surfaces context.DeadlineExceeded
}

// TestGetMovement_ComposeTimeoutDegradesToDataOnly proves a slow LLM compose degrades
// to the canned data-only 200 (never a 5xx, never a hang) and refunds the daily cap.
// It runs the REAL movement.Service over a fake enricher that blocks until its
// context deadline fires, with the package timeout temporarily shortened so the test
// is fast — the same WithTimeout path production uses.
func TestGetMovement_ComposeTimeoutDegradesToDataOnly(t *testing.T) {
	// Shorten the per-call compose timeout for the duration of this test so the
	// deadline fires in milliseconds instead of the production 25s.
	orig := llmComposeTimeout
	llmComposeTimeout = 50 * time.Millisecond
	defer func() { llmComposeTimeout = orig }()

	st := memory.New()
	ctx := context.Background()
	now := time.Now().UTC()
	// A notable +10% move with one news headline → a significant, LLM-eligible move.
	_ = st.UpsertQuote(ctx, store.Quote{Ticker: "AAPL", Price: 110, PrevClose: 100, Session: "regular", At: now})
	_ = st.SaveNews(ctx, "AAPL", []store.News{
		{Ticker: "AAPL", ID: "n1", Headline: "Apple beats earnings estimates", URL: "https://news/1", Published: now.Add(-2 * time.Hour)},
	})

	slow := &slowExplainEnricher{}
	svc := movement.NewService(st, nil, slow, "deepseek-chat")
	h := New(
		st, stream.NewHub(), enrich.Noop{},
		auth.NewVerifier(testSecret, ""),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	h.SetMovement(svc)
	srv := httptest.NewServer(h)
	defer srv.Close()

	// ASYNC: the canned data-only line returns INSTANTLY (prose_status=generating); the
	// LLM sentence is attempted in a detached bg goroutine and will time out.
	resp, body := getMovement(t, srv.URL+"/v1/stocks/AAPL/movement")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200 (async: the canned data-only line returns instantly)", resp.StatusCode)
	}
	if body.ProseStatus != proseStatusGenerating {
		t.Errorf("prose_status = %q; want generating (the LLM sentence is composing)", body.ProseStatus)
	}
	if body.LLM {
		t.Error("llm = true; want false (the canned data-only line, never the LLM on a timeout)")
	}
	if !body.Significant || body.ChangePct != 10 || body.Direction != "up" {
		t.Errorf("got significant=%v %v %q; want the Go-owned significant +10 up", body.Significant, body.ChangePct, body.Direction)
	}
	if body.Explanation == "" {
		t.Error("explanation empty; want the canned data-only line")
	}
	if len(body.Evidence) != 1 || body.Evidence[0].Type != "news" {
		t.Errorf("evidence = %+v; want one attributed news item", body.Evidence)
	}

	// Await the detached bg gen: it blocks until the (shortened) compose deadline fires,
	// then refunds the cap — a timed-out compose produced no LLM sentence, so it must not
	// burn budget. Waiting on the cap counter establishes happens-before with the bg
	// goroutine, so the slow enricher's atomics are safe to read after.
	waitMoveCapZero(t, h)
	if slow.calls.Load() == 0 { // ExplainMove must have been attempted (in the bg)
		t.Fatal("ExplainMove was not called")
	}
	if !slow.gotDone.Load() {
		t.Error("ExplainMove did not observe context cancellation; the deadline must reach the enrich call")
	}
}

// TestGetMovement_EndToEndDataOnly wires the REAL movement.Service over a seeded
// memory store through the HTTP handler, with a Noop enricher — the exact
// production data-only path. It asserts: a +10% quote → 200 significant with the
// Go-computed number, attributed news evidence, and the canned line; a +2% quote
// → 200 significant:false with no explanation; an unseeded ticker → 404.
func TestGetMovement_EndToEndDataOnly(t *testing.T) {
	st := memory.New()
	ctx := context.Background()
	now := time.Now().UTC()

	// AAPL: a notable +10% move with one recent news headline (evidence).
	_ = st.UpsertQuote(ctx, store.Quote{Ticker: "AAPL", Price: 110, PrevClose: 100, Session: "regular", At: now})
	_ = st.SaveNews(ctx, "AAPL", []store.News{
		{Ticker: "AAPL", ID: "n1", Headline: "Apple beats earnings estimates", URL: "https://news/1", Published: now.Add(-2 * time.Hour)},
	})
	// MSFT: a quiet +2% move — below threshold.
	_ = st.UpsertQuote(ctx, store.Quote{Ticker: "MSFT", Price: 102, PrevClose: 100, Session: "regular", At: now})

	svc := movement.NewService(st, nil, enrich.Noop{}, "")
	h := New(
		st, stream.NewHub(), enrich.Noop{},
		auth.NewVerifier(testSecret, ""),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	h.SetMovement(svc)
	srv := httptest.NewServer(h)
	defer srv.Close()

	// Notable move → 200 significant, canned line, attributed evidence, llm:false.
	resp, body := getMovement(t, srv.URL+"/v1/stocks/AAPL/movement")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("AAPL status = %d; want 200", resp.StatusCode)
	}
	if !body.Significant || body.LLM {
		t.Errorf("AAPL significant=%v llm=%v; want significant=true llm=false (data-only)", body.Significant, body.LLM)
	}
	if body.ChangePct != 10 || body.Direction != "up" {
		t.Errorf("AAPL got %v %q; want +10 up (Go-computed)", body.ChangePct, body.Direction)
	}
	if body.Explanation == "" {
		t.Error("AAPL explanation empty; want the canned data-only line")
	}
	if len(body.Evidence) != 1 || body.Evidence[0].Type != "news" || body.Evidence[0].Title != "Apple beats earnings estimates" {
		t.Errorf("AAPL evidence = %+v; want one attributed news item", body.Evidence)
	}

	// Quiet move → 200 significant:false, no explanation (the frontend hides it).
	resp, body = getMovement(t, srv.URL+"/v1/stocks/MSFT/movement")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("MSFT status = %d; want 200", resp.StatusCode)
	}
	if body.Significant {
		t.Error("MSFT significant = true; want false for a +2% move")
	}
	if body.Explanation != "" {
		t.Errorf("MSFT explanation = %q; want empty for a sub-threshold move", body.Explanation)
	}

	// Unseeded ticker (no quote, no evidence) → 404.
	resp, _ = getMovement(t, srv.URL+"/v1/stocks/NONE/movement")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("NONE status = %d; want 404 for an unseeded ticker", resp.StatusCode)
	}
}
