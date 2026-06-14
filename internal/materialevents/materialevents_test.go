package materialevents

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wombow-ai/tickwind/internal/edgar"
)

// fakeFetcher is a controllable EventFetcher: it returns the held events (or
// err), and a per-URL source-text map for the summary source (empty string =
// too thin / unavailable).
type fakeFetcher struct {
	events []edgar.MaterialEvent
	err    error
	srcErr error
	src    map[string]string // PrimaryDocURL → source text
}

func (f *fakeFetcher) MaterialEvents(context.Context, string) ([]edgar.MaterialEvent, error) {
	return f.events, f.err
}

func (f *fakeFetcher) EventSummarySource(_ context.Context, ev edgar.MaterialEvent) (string, error) {
	if f.srcErr != nil {
		return "", f.srcErr
	}
	return f.src[ev.PrimaryDocURL], nil
}

// fakeEnricher is a controllable Enricher: SummarizeFiling returns the held reply
// (or err) and counts calls so the maxSummaries cap can be asserted.
type fakeEnricher struct {
	enabled bool
	reply   string
	err     error
	calls   int
}

func (e *fakeEnricher) Enabled() bool { return e.enabled }

func (e *fakeEnricher) SummarizeFiling(context.Context, string, string) (string, error) {
	e.calls++
	return e.reply, e.err
}

func sampleEvents(n int) []edgar.MaterialEvent {
	out := make([]edgar.MaterialEvent, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, edgar.MaterialEvent{
			Form:          "8-K",
			FiledDate:     "2026-05-3" + string(rune('0'+i%10)),
			AccessionURL:  "https://www.sec.gov/Archives/edgar/data/320193/x/",
			PrimaryDocURL: "https://www.sec.gov/Archives/edgar/data/320193/x/doc" + string(rune('0'+i)) + ".htm",
			Items:         []edgar.EventItem{{Code: "5.02", LabelEN: "Departure of Officers", LabelZH: "高管离任"}},
		})
	}
	return out
}

// TestReportFactsOnly: Report never calls the LLM and returns facts-only filings
// with item labels and no summary.
func TestReportFactsOnly(t *testing.T) {
	ef := &fakeFetcher{events: sampleEvents(2)}
	enr := &fakeEnricher{enabled: true, reply: "should not be called"}
	svc := NewService(ef, enr, "test-model")

	rep, err := svc.Report(context.Background(), "aapl")
	if err != nil {
		t.Fatalf("Report err: %v", err)
	}
	if rep.Ticker != "AAPL" {
		t.Errorf("ticker = %q, want AAPL (upper-cased)", rep.Ticker)
	}
	if len(rep.Filings) != 2 {
		t.Fatalf("got %d filings, want 2", len(rep.Filings))
	}
	if rep.LLM {
		t.Error("Report set LLM=true (must never call the LLM)")
	}
	if enr.calls != 0 {
		t.Errorf("Report called the enricher %d times, want 0", enr.calls)
	}
	for _, f := range rep.Filings {
		if f.Summary != "" {
			t.Errorf("facts-only filing has a summary: %q", f.Summary)
		}
		if len(f.Items) == 0 {
			t.Error("filing is missing item labels")
		}
	}
}

// TestSummarizeLLMOff: with the LLM disabled, Summarize degrades to facts-only.
func TestSummarizeLLMOff(t *testing.T) {
	ef := &fakeFetcher{
		events: sampleEvents(1),
		src:    map[string]string{"https://www.sec.gov/Archives/edgar/data/320193/x/doc0.htm": strings.Repeat("Apple announced a new officer. ", 20)},
	}
	enr := &fakeEnricher{enabled: false, reply: "x"}
	svc := NewService(ef, enr, "test-model")

	rep, err := svc.Summarize(context.Background(), "AAPL", "zh")
	if err != nil {
		t.Fatalf("Summarize err: %v", err)
	}
	if rep.LLM {
		t.Error("LLM=true with a disabled enricher")
	}
	if enr.calls != 0 {
		t.Errorf("enricher called %d times with LLM off, want 0", enr.calls)
	}
	if rep.Filings[0].Summary != "" {
		t.Errorf("got a summary with LLM off: %q", rep.Filings[0].Summary)
	}
}

// TestSummarizeLLMOn: with the LLM on and a usable source, Summarize fills the
// summary, sets LLM/Model/Disclaimer, and leaves the facts untouched.
func TestSummarizeLLMOn(t *testing.T) {
	url := "https://www.sec.gov/Archives/edgar/data/320193/x/doc0.htm"
	ef := &fakeFetcher{
		events: sampleEvents(1),
		src:    map[string]string{url: strings.Repeat("Apple Inc. named a new CFO effective next month. ", 10)},
	}
	enr := &fakeEnricher{enabled: true, reply: "苹果任命了新任首席财务官。"}
	svc := NewService(ef, enr, "test-model")

	rep, err := svc.Summarize(context.Background(), "AAPL", "zh")
	if err != nil {
		t.Fatalf("Summarize err: %v", err)
	}
	if !rep.LLM {
		t.Error("LLM=false despite a successful summary")
	}
	if rep.Model != "test-model" {
		t.Errorf("model = %q, want test-model", rep.Model)
	}
	if rep.Disclaimer != Disclaimer {
		t.Errorf("disclaimer = %q, want %q", rep.Disclaimer, Disclaimer)
	}
	if rep.Filings[0].Summary != "苹果任命了新任首席财务官。" {
		t.Errorf("summary = %q", rep.Filings[0].Summary)
	}
	// Facts untouched.
	if rep.Filings[0].Form != "8-K" || len(rep.Filings[0].Items) != 1 {
		t.Error("LLM altered the Go-owned facts")
	}
}

// TestSummarizeThinSourceNoFabrication: an empty/too-thin source → no summary
// (the LLM is never called for that filing, and nothing is fabricated).
func TestSummarizeThinSourceNoFabrication(t *testing.T) {
	ef := &fakeFetcher{
		events: sampleEvents(1),
		src:    map[string]string{}, // no source text → "" returned
	}
	enr := &fakeEnricher{enabled: true, reply: "fabricated"}
	svc := NewService(ef, enr, "test-model")

	rep, err := svc.Summarize(context.Background(), "AAPL", "zh")
	if err != nil {
		t.Fatalf("Summarize err: %v", err)
	}
	if enr.calls != 0 {
		t.Errorf("enricher called %d times for a thin source, want 0", enr.calls)
	}
	if rep.Filings[0].Summary != "" {
		t.Errorf("fabricated a summary from a thin source: %q", rep.Filings[0].Summary)
	}
	if rep.LLM {
		t.Error("LLM=true though no summary was written")
	}
}

// TestSummarizeInsufficientSentinelDropped: a model "not enough info" reply is
// dropped to "" so the UI degrades to item labels.
func TestSummarizeInsufficientSentinelDropped(t *testing.T) {
	url := "https://www.sec.gov/Archives/edgar/data/320193/x/doc0.htm"
	ef := &fakeFetcher{
		events: sampleEvents(1),
		src:    map[string]string{url: strings.Repeat("some boilerplate text here. ", 10)},
	}
	enr := &fakeEnricher{enabled: true, reply: "暂无足够信息"}
	svc := NewService(ef, enr, "test-model")

	rep, err := svc.Summarize(context.Background(), "AAPL", "zh")
	if err != nil {
		t.Fatalf("Summarize err: %v", err)
	}
	if rep.Filings[0].Summary != "" {
		t.Errorf("insufficient sentinel not dropped: %q", rep.Filings[0].Summary)
	}
	if rep.LLM {
		t.Error("LLM=true though only the sentinel was returned")
	}
}

// TestSummarizeLLMError: an LLM error degrades that filing to no summary (never
// errors the whole report).
func TestSummarizeLLMError(t *testing.T) {
	url := "https://www.sec.gov/Archives/edgar/data/320193/x/doc0.htm"
	ef := &fakeFetcher{
		events: sampleEvents(1),
		src:    map[string]string{url: strings.Repeat("real filing body text here. ", 10)},
	}
	enr := &fakeEnricher{enabled: true, err: errors.New("llm down")}
	svc := NewService(ef, enr, "test-model")

	rep, err := svc.Summarize(context.Background(), "AAPL", "zh")
	if err != nil {
		t.Fatalf("Summarize must not propagate the LLM error: %v", err)
	}
	if rep.Filings[0].Summary != "" {
		t.Errorf("got a summary despite an LLM error: %q", rep.Filings[0].Summary)
	}
}

// TestSummarizeCapsLLMCalls: only the freshest maxSummaries filings get an LLM
// call, even when many filings have a usable source.
func TestSummarizeCapsLLMCalls(t *testing.T) {
	n := defaultMaxSummaries + 4
	events := sampleEvents(n)
	src := make(map[string]string, n)
	for _, ev := range events {
		src[ev.PrimaryDocURL] = strings.Repeat("a usable filing body excerpt. ", 10)
	}
	ef := &fakeFetcher{events: events, src: src}
	enr := &fakeEnricher{enabled: true, reply: "ok summary"}
	svc := NewService(ef, enr, "test-model")

	rep, err := svc.Summarize(context.Background(), "AAPL", "zh")
	if err != nil {
		t.Fatalf("Summarize err: %v", err)
	}
	if enr.calls != defaultMaxSummaries {
		t.Errorf("enricher called %d times, want cap %d", enr.calls, defaultMaxSummaries)
	}
	if len(rep.Filings) != n {
		t.Errorf("got %d filings, want all %d (facts-only beyond the cap)", len(rep.Filings), n)
	}
	// Beyond the cap, filings are facts-only (no summary).
	for i := defaultMaxSummaries; i < n; i++ {
		if rep.Filings[i].Summary != "" {
			t.Errorf("filing %d beyond the cap has a summary: %q", i, rep.Filings[i].Summary)
		}
	}
}

// TestSummarizeEmptyIsSlice: a company with zero recent 8-Ks yields an empty
// (non-nil) Filings slice and nil error (the handler returns {"filings":[]}/200).
func TestSummarizeEmptyIsSlice(t *testing.T) {
	ef := &fakeFetcher{events: nil}
	svc := NewService(ef, &fakeEnricher{enabled: true}, "m")

	rep, err := svc.Summarize(context.Background(), "AAPL", "zh")
	if err != nil {
		t.Fatalf("Summarize err: %v", err)
	}
	if rep.Filings == nil {
		t.Error("Filings is nil (must be a non-nil empty slice)")
	}
	if len(rep.Filings) != 0 {
		t.Errorf("got %d filings, want 0", len(rep.Filings))
	}
}

// TestReportPropagatesFetchError: an unresolved ticker / feed failure propagates
// the error so the handler can 404.
func TestReportPropagatesFetchError(t *testing.T) {
	ef := &fakeFetcher{err: errors.New("ticker not found")}
	svc := NewService(ef, &fakeEnricher{}, "m")
	if _, err := svc.Report(context.Background(), "ZZZZ"); err == nil {
		t.Error("expected a fetch error to propagate")
	}
	if _, err := svc.Summarize(context.Background(), "ZZZZ", "zh"); err == nil {
		t.Error("expected a fetch error to propagate from Summarize")
	}
}

// TestBuildMaterialIncludesItemLabels: the LLM material carries the Go-owned item
// labels (anti-hallucination context) and the body excerpt, in the chosen lang.
func TestBuildMaterialIncludesItemLabels(t *testing.T) {
	ev := edgar.MaterialEvent{
		Form:  "8-K",
		Items: []edgar.EventItem{{Code: "5.02", LabelEN: "Departure of Officers", LabelZH: "高管离任"}},
	}
	zh := buildMaterial(ev, "body text", "zh")
	if !strings.Contains(zh, "高管离任") || !strings.Contains(zh, "5.02") || !strings.Contains(zh, "body text") {
		t.Errorf("zh material missing label/code/body: %q", zh)
	}
	en := buildMaterial(ev, "body text", "en")
	if !strings.Contains(en, "Departure of Officers") || !strings.Contains(en, "body text") {
		t.Errorf("en material missing label/body: %q", en)
	}
}
