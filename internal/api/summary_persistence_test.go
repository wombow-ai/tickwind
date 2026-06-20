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
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/store/memory"
	"github.com/wombow-ai/tickwind/internal/stream"
)

// countingEnricher is an enabled Enricher that records how many times Summarize
// was called, so the persistence test can assert a store hit served WITHOUT an
// LLM generation.
type countingEnricher struct {
	summary string
	calls   atomic.Int32
}

func (c *countingEnricher) Enabled() bool { return true }
func (c *countingEnricher) Summarize(context.Context, string, string) (string, error) {
	c.calls.Add(1)
	return c.summary, nil
}
func (c *countingEnricher) TranslateTitles(context.Context, []string) ([]string, error) {
	return nil, enrich.ErrDisabled
}
func (c *countingEnricher) Brief(context.Context, string, string) (string, error) {
	return "", enrich.ErrDisabled
}
func (c *countingEnricher) ComposeReport(context.Context, string, string) (map[string]string, error) {
	return nil, enrich.ErrDisabled
}
func (c *countingEnricher) ComposeDeepReport(context.Context, string, string) (map[string]string, error) {
	return nil, enrich.ErrDisabled
}
func (c *countingEnricher) ExplainMove(context.Context, string, string) (string, error) {
	return "", enrich.ErrDisabled
}
func (c *countingEnricher) SummarizeFiling(context.Context, string, string) (string, error) {
	return "", enrich.ErrDisabled
}
func (c *countingEnricher) Chat(context.Context, []enrich.ChatMessage, []enrich.ChatTool, string) (string, []enrich.ChatToolCall, enrich.Usage, error) {
	return "", nil, enrich.Usage{}, enrich.ErrDisabled
}

func (c *countingEnricher) ChatStream(context.Context, []enrich.ChatMessage, []enrich.ChatTool, string, func(string)) (string, []enrich.ChatToolCall, enrich.Usage, error) {
	return "", nil, enrich.Usage{}, enrich.ErrDisabled
}

// newSummaryServer builds an API server over the given store + enricher and
// returns both the underlying *Server (to inspect the daily cap) and an
// httptest server.
func newSummaryServer(st store.Store, enr enrich.Enricher) (*Server, *httptest.Server) {
	h := New(
		st, stream.NewHub(), enr,
		auth.NewVerifier(testSecret, ""),
		nil, nil, nil, nil, nil, nil, nil, nil, nil,
		st, // earnings source = the same memory store
		nil, nil, nil, nil, nil, nil, nil, nil,
		nil, // admins
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	return h, httptest.NewServer(h)
}

func getSummaryBody(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body struct {
		Summary string `json:"summary"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body.Summary
}

// pollSummary requests the digest endpoint until its prose_status is no longer
// "generating" (the async bg gen completed and cached a terminal result), or fails
// after a cap. Used to await the detached background digest in tests. NOTE: a FAILED
// bg gen caches nothing (the next request re-spawns) — use this only for the success
// path; await the cap counter for the failure/timeout path.
func pollSummary(t *testing.T, url string) (int, string) {
	t.Helper()
	for i := 0; i < 80; i++ {
		resp, err := http.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Summary     string `json:"summary"`
			ProseStatus string `json:"prose_status"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || body.ProseStatus != proseStatusGenerating {
			return resp.StatusCode, body.Summary
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("summary stayed prose_status=generating; the bg gen never completed")
	return 0, ""
}

// waitSumCapZero polls the server's summary daily-cap counter until it returns to 0
// (the detached bg gen finished and a failed gen refunded its reserved slot), or fails
// after a cap. Acquiring sumMu establishes happens-before with the bg goroutine, so the
// fake enricher's atomics are safe to read after this returns.
func waitSumCapZero(t *testing.T, s *Server) {
	t.Helper()
	for i := 0; i < 80; i++ {
		s.sumMu.Lock()
		c := s.sumDayCount
		s.sumMu.Unlock()
		if c == 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("sumDayCount never returned to 0 (a failed bg gen must refund the cap)")
}

// slowSummaryEnricher is an enabled Enricher whose Summarize HONORS its context: it
// blocks until the context is canceled, then returns the cancellation error. The
// per-call timeout the handler imposes is what cancels it.
type slowSummaryEnricher struct {
	calls   atomic.Int32
	gotDone atomic.Bool
}

func (s *slowSummaryEnricher) Enabled() bool { return true }
func (s *slowSummaryEnricher) Summarize(ctx context.Context, _, _ string) (string, error) {
	s.calls.Add(1)
	<-ctx.Done()
	s.gotDone.Store(true)
	return "", ctx.Err()
}
func (s *slowSummaryEnricher) TranslateTitles(context.Context, []string) ([]string, error) {
	return nil, enrich.ErrDisabled
}
func (s *slowSummaryEnricher) Brief(context.Context, string, string) (string, error) {
	return "", enrich.ErrDisabled
}
func (s *slowSummaryEnricher) ComposeReport(context.Context, string, string) (map[string]string, error) {
	return nil, enrich.ErrDisabled
}
func (s *slowSummaryEnricher) ComposeDeepReport(context.Context, string, string) (map[string]string, error) {
	return nil, enrich.ErrDisabled
}
func (s *slowSummaryEnricher) ExplainMove(context.Context, string, string) (string, error) {
	return "", enrich.ErrDisabled
}
func (s *slowSummaryEnricher) SummarizeFiling(context.Context, string, string) (string, error) {
	return "", enrich.ErrDisabled
}
func (s *slowSummaryEnricher) Chat(context.Context, []enrich.ChatMessage, []enrich.ChatTool, string) (string, []enrich.ChatToolCall, enrich.Usage, error) {
	return "", nil, enrich.Usage{}, enrich.ErrDisabled
}
func (s *slowSummaryEnricher) ChatStream(context.Context, []enrich.ChatMessage, []enrich.ChatTool, string, func(string)) (string, []enrich.ChatToolCall, enrich.Usage, error) {
	return "", nil, enrich.Usage{}, enrich.ErrDisabled
}

// TestSummaryComposeTimeoutDegradesTo200 proves a slow LLM digest degrades to an
// empty-summary 200 (NOT a 5xx, never a hang) and refunds the daily cap, using the
// real handler path with the package timeout temporarily shortened.
func TestSummaryComposeTimeoutDegradesTo200(t *testing.T) {
	orig := llmComposeTimeout
	llmComposeTimeout = 50 * time.Millisecond
	defer func() { llmComposeTimeout = orig }()

	st := memory.New()
	ctx := context.Background()
	// Seed material so the handler reaches the LLM call (not the empty-news 200).
	_ = st.SaveNews(ctx, "AAPL", []store.News{{Ticker: "AAPL", ID: "n1", Headline: "Apple news"}})

	enr := &slowSummaryEnricher{}
	srv, ts := newSummaryServer(st, enr)
	defer ts.Close()

	// ASYNC: the digest endpoint returns an empty summary INSTANTLY with
	// prose_status=generating; the LLM digest is attempted in a detached bg goroutine
	// and times out.
	status, got := getSummaryBody(t, ts.URL+"/v1/stocks/AAPL/summary")
	if status != http.StatusOK {
		t.Fatalf("status = %d; want 200 (async: returns instantly, never a 5xx)", status)
	}
	if got != "" {
		t.Errorf("summary = %q; want empty while generating", got)
	}
	// Await the detached bg gen: it blocks until the (shortened) compose deadline fires,
	// then refunds the cap — a timed-out generation produced no digest, so it must not
	// burn budget. Waiting on the cap counter establishes happens-before with the bg
	// goroutine, so the enricher's atomics are safe to read after.
	waitSumCapZero(t, srv)
	if !enr.gotDone.Load() {
		t.Error("Summarize did not observe context cancellation; the deadline must reach the enrich call")
	}
}

// TestSummaryServedFromStoreSkipsLLM seeds the store with today's digest for a
// ticker, then requests it on a server with an EMPTY in-memory cache (mimicking
// a fresh process after a redeploy). The persisted entry must be served without
// calling the LLM and without consuming the daily cap.
func TestSummaryServedFromStoreSkipsLLM(t *testing.T) {
	st := memory.New()
	ctx := context.Background()
	// Seed material so a cold MISS would otherwise generate (proves the store
	// hit, not the empty-news short-circuit, is what skips the LLM).
	_ = st.SaveNews(ctx, "AAPL", []store.News{{Ticker: "AAPL", ID: "n1", Headline: "Apple news"}})

	const want = "已缓存的AI摘要"
	raw, err := json.Marshal(summaryEntry{Summary: want})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveAISummary(ctx, "AAPL", summaryDay(), "zh", raw); err != nil {
		t.Fatal(err)
	}

	enr := &countingEnricher{summary: "FRESHLY-GENERATED"}
	srv, ts := newSummaryServer(st, enr)
	defer ts.Close()

	status, got := getSummaryBody(t, ts.URL+"/v1/stocks/AAPL/summary")
	if status != http.StatusOK {
		t.Fatalf("status = %d; want 200", status)
	}
	if got != want {
		t.Fatalf("summary = %q; want the persisted %q (store hit, no LLM)", got, want)
	}
	if n := enr.calls.Load(); n != 0 {
		t.Fatalf("Summarize calls = %d; want 0 (a store hit must not call the LLM)", n)
	}
	srv.sumMu.Lock()
	count := srv.sumDayCount
	srv.sumMu.Unlock()
	if count != 0 {
		t.Fatalf("sumDayCount = %d; want 0 (a store hit must not consume the daily cap)", count)
	}
}

// TestSummaryColdStoreGeneratesAndPersists requests a digest on a server whose
// store has no entry: the first request must generate exactly once (one LLM
// call, cap consumed once) and persist the result, so the digest survives a
// restart. A second request against a fresh process reading the same store then
// serves the persisted copy with no new generation.
func TestSummaryColdStoreGeneratesAndPersists(t *testing.T) {
	st := memory.New()
	ctx := context.Background()
	_ = st.SaveNews(ctx, "MSFT", []store.News{{Ticker: "MSFT", ID: "n1", Headline: "Microsoft news"}})

	enr := &countingEnricher{summary: "新生成的摘要"}
	srv, ts := newSummaryServer(st, enr)
	defer ts.Close()

	// First (cold) request: ASYNC — returns generating instantly, composes in a detached
	// bg goroutine, then a poll lands the digest. Generates exactly once (single-flight),
	// consumes the cap, persists.
	status, got := pollSummary(t, ts.URL+"/v1/stocks/MSFT/summary")
	if status != http.StatusOK || got != enr.summary {
		t.Fatalf("cold request (after polling): status=%d summary=%q; want 200 + %q", status, got, enr.summary)
	}
	if n := enr.calls.Load(); n != 1 {
		t.Fatalf("Summarize calls = %d after cold request; want 1", n)
	}
	srv.sumMu.Lock()
	count := srv.sumDayCount
	srv.sumMu.Unlock()
	if count != 1 {
		t.Fatalf("sumDayCount = %d; want 1 (a genuine generation consumes the cap)", count)
	}

	// The generation must have been persisted to the store.
	raw, ok, err := st.GetAISummary(ctx, "MSFT", summaryDay(), "zh")
	if err != nil || !ok {
		t.Fatalf("GetAISummary ok=%v err=%v; want a persisted entry", ok, err)
	}
	var persisted summaryEntry
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("persisted payload decode: %v", err)
	}
	if persisted.Summary != enr.summary {
		t.Fatalf("persisted summary = %q; want %q", persisted.Summary, enr.summary)
	}

	// Simulate a redeploy: a NEW process (empty in-memory cache) reading the same
	// store. The persisted digest is served with no new generation and no cap use.
	enr2 := &countingEnricher{summary: "SHOULD-NOT-BE-USED"}
	srv2, ts2 := newSummaryServer(st, enr2)
	defer ts2.Close()

	status, got = getSummaryBody(t, ts2.URL+"/v1/stocks/MSFT/summary")
	if status != http.StatusOK || got != enr.summary {
		t.Fatalf("post-restart request: status=%d summary=%q; want 200 + the persisted %q", status, got, enr.summary)
	}
	if n := enr2.calls.Load(); n != 0 {
		t.Fatalf("Summarize calls = %d after restart; want 0 (served from the store)", n)
	}
	srv2.sumMu.Lock()
	count2 := srv2.sumDayCount
	srv2.sumMu.Unlock()
	if count2 != 0 {
		t.Fatalf("sumDayCount = %d after restart; want 0 (store hit is free)", count2)
	}
}
