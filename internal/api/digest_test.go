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
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/store/memory"
	"github.com/wombow-ai/tickwind/internal/stream"
)

// fakeEnricher is an enabled Enricher whose Summarize echoes a canned digest,
// so the digest test can assert the AI overview is wired without a real LLM.
type fakeEnricher struct {
	summary string
	gotLang string
}

func (f *fakeEnricher) Enabled() bool { return true }
func (f *fakeEnricher) Summarize(_ context.Context, _, lang string) (string, error) {
	f.gotLang = lang
	return f.summary, nil
}
func (f *fakeEnricher) TranslateTitles(context.Context, []string) ([]string, error) {
	return nil, enrich.ErrDisabled
}
func (f *fakeEnricher) Brief(context.Context, string, string) (string, error) {
	return "", enrich.ErrDisabled
}
func (f *fakeEnricher) ComposeReport(context.Context, string, string) (map[string]string, error) {
	return nil, enrich.ErrDisabled
}
func (f *fakeEnricher) ComposeDeepReport(context.Context, string, string) (map[string]string, error) {
	return nil, enrich.ErrDisabled
}
func (f *fakeEnricher) ExplainMove(context.Context, string, string) (string, error) {
	return "", enrich.ErrDisabled
}
func (f *fakeEnricher) SummarizeFiling(context.Context, string, string) (string, error) {
	return "", enrich.ErrDisabled
}
func (f *fakeEnricher) Chat(context.Context, []enrich.ChatMessage, []enrich.ChatTool, string) (string, []enrich.ChatToolCall, enrich.Usage, error) {
	return "", nil, enrich.Usage{}, enrich.ErrDisabled
}

func (f *fakeEnricher) ChatStream(context.Context, []enrich.ChatMessage, []enrich.ChatTool, string, func(string)) (string, []enrich.ChatToolCall, enrich.Usage, error) {
	return "", nil, enrich.Usage{}, enrich.ErrDisabled
}

// newDigestServer builds an API server backed by a memory store (which also
// serves as the earnings source) and the given enricher.
func newDigestServer(st store.Store, enr enrich.Enricher) *httptest.Server {
	h := New(
		st, stream.NewHub(), enr,
		auth.NewVerifier(testSecret, ""),
		nil, // bars
		nil, // topics
		nil, // opps
		nil, // universe
		nil, // gurus
		nil, // ingestor
		nil, // symbols
		nil, // events
		nil, // fundamentals
		st,  // earnings source = the same memory store
		nil, // congress
		nil, // institutional
		nil, // live
		nil, // indices
		nil, // short
		nil, // briefing
		nil, // options
		nil, // 13F
		nil, // admins
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	return httptest.NewServer(h)
}

func decodeDigest(t *testing.T, resp *http.Response) digestPayload {
	t.Helper()
	defer resp.Body.Close()
	var p digestPayload
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatalf("decode digest: %v", err)
	}
	return p
}

func TestDigestRequiresAuth(t *testing.T) {
	srv := newDigestServer(memory.New(), enrich.Noop{})
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/me/digest") // no token
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401 without a token", resp.StatusCode)
	}
}

func TestDigestEmptyWatchlist(t *testing.T) {
	srv := newDigestServer(memory.New(), enrich.Noop{})
	defer srv.Close()
	resp := authed(t, http.MethodGet, srv.URL+"/v1/me/digest", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200 for empty watchlist", resp.StatusCode)
	}
	p := decodeDigest(t, resp)
	if p.Stocks == nil {
		t.Fatal("stocks must marshal as [] not null")
	}
	if len(p.Stocks) != 0 {
		t.Fatalf("stocks = %v; want empty", p.Stocks)
	}
	if p.Summary != "" {
		t.Errorf("summary = %q; want empty for empty watchlist", p.Summary)
	}
	if p.Date == "" {
		t.Error("date should be set")
	}
}

// seedAAPL adds AAPL to user-1's watchlist with a quote, news, and an upcoming
// earnings row, so the digest has a complete row to assemble.
func seedAAPL(t *testing.T, st store.Store) {
	t.Helper()
	ctx := context.Background()
	if err := st.AddToWatchlist(ctx, "user-1", "AAPL"); err != nil {
		t.Fatal(err)
	}
	_ = st.UpsertSecurity(ctx, store.Security{Ticker: "AAPL", Name: "Apple Inc.", Market: "US"})
	_ = st.UpsertQuote(ctx, store.Quote{Ticker: "AAPL", Price: 110, PrevClose: 100, Session: "post", At: time.Now().UTC()})
	_ = st.SaveNews(ctx, "AAPL", []store.News{{
		Ticker: "AAPL", ID: "n1", Headline: "Apple ships new chip", HeadlineZH: "苹果发布新芯片",
		URL: "https://example.com/aapl", Published: time.Now().UTC(),
	}})
	_ = st.SaveEarnings(ctx, []store.Earning{{
		Ticker: "AAPL", Date: time.Now().UTC().Add(48 * time.Hour), Hour: "amc",
	}})
}

func TestDigestWithWatchlistAndLLM(t *testing.T) {
	st := memory.New()
	seedAAPL(t, st)
	// The AI overview is a Pro feature — make user-1 Pro so it composes.
	_ = st.UpsertSubscription(context.Background(), store.Subscription{UserID: "user-1", Status: "active", Tier: tierPro})
	enr := &fakeEnricher{summary: "今夜自选股整体走高,苹果领涨。"}
	srv := newDigestServer(st, enr)
	defer srv.Close()

	// First request: the data rows serve INSTANTLY; the overview composes in the background.
	p := decodeDigest(t, authed(t, http.MethodGet, srv.URL+"/v1/me/digest", ""))
	if len(p.Stocks) != 1 {
		t.Fatalf("stocks count = %d; want 1 (rows must serve instantly)", len(p.Stocks))
	}
	row := p.Stocks[0]
	if row.Ticker != "AAPL" {
		t.Errorf("ticker = %q; want AAPL", row.Ticker)
	}
	if row.Name != "Apple Inc." {
		t.Errorf("name = %q; want Apple Inc.", row.Name)
	}
	if row.ChangePct == nil || *row.ChangePct < 9.9 || *row.ChangePct > 10.1 {
		t.Errorf("change_pct = %v; want ~+10", row.ChangePct)
	}
	if row.Headline != "苹果发布新芯片" { // zh preferred (default lang)
		t.Errorf("headline = %q; want zh headline", row.Headline)
	}
	if row.HeadURL != "https://example.com/aapl" {
		t.Errorf("headline_url = %q", row.HeadURL)
	}
	if row.NextEvent == "" {
		t.Error("next_event should be set from the upcoming earnings row")
	}
	if p.SummaryStatus != digestSummaryGenerating && p.SummaryStatus != digestSummaryReady {
		t.Fatalf("first summary_status = %q; want generating or ready", p.SummaryStatus)
	}

	// Poll until the background overview is ready.
	var got digestPayload
	for i := 0; i < 100; i++ {
		got = decodeDigest(t, authed(t, http.MethodGet, srv.URL+"/v1/me/digest", ""))
		if got.SummaryStatus == digestSummaryReady {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got.SummaryStatus != digestSummaryReady {
		t.Fatalf("overview never became ready: status=%q", got.SummaryStatus)
	}
	if got.Summary != enr.summary {
		t.Errorf("summary = %q; want canned LLM text", got.Summary)
	}
	if enr.gotLang != "zh" {
		t.Errorf("enricher lang = %q; want zh (default)", enr.gotLang)
	}
}

// TestDigestOverviewProGated: a non-Pro user gets the data rows + summary_status=pro_required
// and NO LLM call — the overview is a Pro feature, but the rows must still serve fast.
func TestDigestOverviewProGated(t *testing.T) {
	st := memory.New()
	seedAAPL(t, st) // user-1 has NO subscription → free tier
	enr := &fakeEnricher{summary: "must not compose for a free user"}
	srv := newDigestServer(st, enr)
	defer srv.Close()

	p := decodeDigest(t, authed(t, http.MethodGet, srv.URL+"/v1/me/digest", ""))
	if len(p.Stocks) != 1 {
		t.Fatalf("data rows must serve for a free user: %+v", p.Stocks)
	}
	if p.SummaryStatus != digestSummaryProRequired {
		t.Fatalf("summary_status = %q; want pro_required for a non-Pro user", p.SummaryStatus)
	}
	if p.Summary != "" {
		t.Errorf("summary = %q; want empty (no LLM for a free user)", p.Summary)
	}
	// pro_required returns before any goroutine spawns, so the LLM is never invoked.
	if enr.gotLang != "" {
		t.Errorf("LLM was called for a free user (lang=%q) — the overview must be Pro-gated", enr.gotLang)
	}
}

func TestDigestLLMDisabledStillReturnsData(t *testing.T) {
	st := memory.New()
	seedAAPL(t, st)
	srv := newDigestServer(st, enrich.Noop{}) // LLM off
	defer srv.Close()

	resp := authed(t, http.MethodGet, srv.URL+"/v1/me/digest", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	p := decodeDigest(t, resp)
	if p.Summary != "" {
		t.Errorf("summary = %q; want empty when LLM is disabled", p.Summary)
	}
	if len(p.Stocks) != 1 || p.Stocks[0].ChangePct == nil {
		t.Fatalf("data rows must still populate without the LLM: %+v", p.Stocks)
	}
}
