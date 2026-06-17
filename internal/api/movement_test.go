package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
	explains   int
	reportLang string // records the lang the handler threaded into Report
}

func (f *fakeMovement) Report(_ context.Context, _, lang string) movement.Explanation {
	f.reportLang = lang
	return f.exp
}

func (f *fakeMovement) Explain(_ context.Context, _, _ string) movement.Explanation {
	f.explains++
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
	if fake.explains != 0 {
		t.Errorf("Explain ran %d times; want 0 when disabled (data-only Report path)", fake.explains)
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

	resp, body := getMovement(t, srv.URL+"/v1/stocks/AAPL/movement")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
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

	// A second request for the same (ticker, day, lang) must hit the cache — Explain
	// runs exactly once.
	if _, _ = getMovement(t, srv.URL+"/v1/stocks/AAPL/movement"); fake.explains != 1 {
		t.Errorf("Explain ran %d times; want 1 (second request served from cache)", fake.explains)
	}
}

// slowExplainEnricher is a movement.Enricher whose ExplainMove HONORS its context:
// it blocks until the context is canceled, then returns the cancellation error. The
// per-call timeout the handler imposes is what cancels it — proving the deadline
// reaches the (would-be HTTP) enrich call, not just the goroutine. It records that
// it ran so the cache/cap refund can be asserted.
type slowExplainEnricher struct {
	calls   int
	gotDone bool
}

func (s *slowExplainEnricher) Enabled() bool { return true }
func (s *slowExplainEnricher) ExplainMove(ctx context.Context, _, _ string) (string, error) {
	s.calls++
	<-ctx.Done() // block until the handler's per-call timeout (or parent cancel) fires
	s.gotDone = true
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

	resp, body := getMovement(t, srv.URL+"/v1/stocks/AAPL/movement")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200 (compose timeout must degrade to data-only, not 5xx)", resp.StatusCode)
	}
	if slow.calls == 0 { // ExplainMove must have been attempted
		t.Fatal("ExplainMove was not called")
	}
	if !slow.gotDone {
		t.Error("ExplainMove did not observe context cancellation; the deadline must reach the enrich call")
	}
	if body.LLM {
		t.Error("llm = true; want false after a compose timeout (data-only)")
	}
	if !body.Significant || body.ChangePct != 10 || body.Direction != "up" {
		t.Errorf("got significant=%v %v %q; want the Go-owned significant +10 up", body.Significant, body.ChangePct, body.Direction)
	}
	if body.Explanation == "" {
		t.Error("explanation empty; want the canned data-only line after a timeout")
	}
	if len(body.Evidence) != 1 || body.Evidence[0].Type != "news" {
		t.Errorf("evidence = %+v; want one attributed news item", body.Evidence)
	}
	// The daily cap must be refunded exactly like any other failed/empty generation:
	// a timed-out compose produced no LLM sentence, so it must not burn budget.
	h.moveMu.Lock()
	count := h.moveDayCount
	h.moveMu.Unlock()
	if count != 0 {
		t.Errorf("moveDayCount = %d; want 0 (a timed-out compose must refund the cap)", count)
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
