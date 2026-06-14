package movement

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

// asOf is a fixed reference time the tests anchor evidence freshness to (the
// quote's At). Evidence is judged recent relative to this.
var asOf = time.Date(2026, 6, 12, 20, 0, 0, 0, time.UTC)

func quoteAt(price, prev float64) store.Quote {
	return store.Quote{Ticker: "AAPL", Price: price, PrevClose: prev, Session: "regular", At: asOf}
}

func TestAssemble_ChangePctMathAndDirection(t *testing.T) {
	tests := []struct {
		name    string
		price   float64
		prev    float64
		wantPct float64
		wantDir string
		wantSig bool
	}{
		{"up 10%", 110, 100, 10, "up", true},
		{"down 8%", 92, 100, -8, "down", true},
		{"exactly +5% is significant", 105, 100, 5, "up", true},
		{"exactly -5% is significant", 95, 100, -5, "down", true},
		{"small +2% not significant", 102, 100, 2, "up", false},
		{"small -3% not significant", 97, 100, -3, "down", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exp := Assemble("aapl", Inputs{Quote: quoteAt(tc.price, tc.prev)})
			if got := round1(exp.ChangePct); got != tc.wantPct {
				t.Errorf("change_pct = %v; want %v", got, tc.wantPct)
			}
			if exp.Direction != tc.wantDir {
				t.Errorf("direction = %q; want %q", exp.Direction, tc.wantDir)
			}
			if exp.Significant != tc.wantSig {
				t.Errorf("significant = %v; want %v", exp.Significant, tc.wantSig)
			}
			if exp.Ticker != "AAPL" {
				t.Errorf("ticker = %q; want AAPL (uppercased)", exp.Ticker)
			}
		})
	}
}

func TestAssemble_SubThresholdHasNoExplanationOrEvidence(t *testing.T) {
	in := Inputs{
		Quote: quoteAt(102, 100), // +2%, below threshold
		News:  []store.News{{Headline: "Apple unveils new chip", URL: "u", Published: asOf.Add(-time.Hour)}},
	}
	exp := Assemble("AAPL", in)
	if exp.Significant {
		t.Fatal("significant = true; want false for a +2% move")
	}
	if exp.Text != "" {
		t.Errorf("explanation = %q; want empty for a sub-threshold move", exp.Text)
	}
	if len(exp.Evidence) != 0 {
		t.Errorf("evidence = %+v; want none for a sub-threshold move", exp.Evidence)
	}
}

func TestAssemble_NoQuoteIsNotSignificant(t *testing.T) {
	exp := Assemble("AAPL", Inputs{}) // zero quote → no usable reference
	if exp.Significant {
		t.Fatal("significant = true; want false with no usable quote")
	}
	if exp.Text != "" {
		t.Errorf("explanation = %q; want empty with no quote", exp.Text)
	}
}

func TestAssemble_CannedFallbackWithEvidence(t *testing.T) {
	in := Inputs{
		Quote: quoteAt(112, 100), // +12%
		News: []store.News{
			{Headline: "Apple beats earnings estimates", URL: "https://n/1", Published: asOf.Add(-2 * time.Hour)},
		},
	}
	exp := Assemble("AAPL", in)
	if !exp.Significant {
		t.Fatal("significant = false; want true for a +12% move")
	}
	// The canned line is hedged and quotes the top evidence headline.
	if !strings.Contains(exp.Text, "今日涨12.0%") {
		t.Errorf("explanation = %q; want it to state the Go-owned +12.0%% move", exp.Text)
	}
	if !strings.Contains(exp.Text, "Apple beats earnings estimates") {
		t.Errorf("explanation = %q; want it to quote the top evidence headline", exp.Text)
	}
	// It must NOT assert a definitive cause — it uses the hedged "近期消息" framing.
	if !strings.Contains(exp.Text, "近期消息") {
		t.Errorf("explanation = %q; want the hedged 近期消息 framing, not a definitive cause", exp.Text)
	}
}

func TestAssemble_CannedFallbackNoEvidence(t *testing.T) {
	exp := Assemble("AAPL", Inputs{Quote: quoteAt(85, 100)}) // -15%, no evidence
	if !exp.Significant {
		t.Fatal("significant = false; want true for a -15% move")
	}
	if exp.Text != "今日跌15.0%,暂无明确催化消息。" {
		t.Errorf("explanation = %q; want the no-catalyst canned line", exp.Text)
	}
	if exp.LLM {
		t.Error("llm = true; want false for the data-only assemble")
	}
}

func TestGatherEvidence_AttributionAndFreshness(t *testing.T) {
	in := Inputs{
		Quote: quoteAt(110, 100),
		News: []store.News{
			{Headline: "Recent headline", URL: "https://n/recent", Published: asOf.Add(-time.Hour)},
			{Headline: "Stale headline", URL: "https://n/stale", Published: asOf.Add(-72 * time.Hour)}, // outside 48h
			{HeadlineZH: "中文标题", Headline: "EN title", URL: "https://n/zh", Published: asOf.Add(-3 * time.Hour)},
		},
		Filings: []store.Filing{
			{Form: "8-K", Title: "Item 2.02 Results", URL: "https://f/8k", FiledAt: asOf.Add(-2 * time.Hour)},
			{Form: "10-K", Title: "Annual report", URL: "https://f/10k", FiledAt: asOf.Add(-72 * time.Hour)}, // outside 24h
		},
		Insider: []store.InsiderBuy{
			{Ticker: "AAPL", OwnerName: "Jane Doe", Value: 1_200_000, FilingURL: "https://i/1", FiledDate: asOf.Add(-5 * time.Hour)},
			{Ticker: "MSFT", OwnerName: "Other Co", Value: 500_000, FilingURL: "https://i/2", FiledDate: asOf.Add(-5 * time.Hour)}, // wrong ticker
		},
	}
	exp := Assemble("AAPL", in)

	// Stale news, stale filing, and the wrong-ticker insider buy are all excluded.
	wantTitles := map[string]string{
		"Recent headline":      "news",
		"中文标题":                 "news",
		"Item 2.02 Results":    "filing",
		"Jane Doe 内部人买入 $1.2M": "insider",
	}
	if len(exp.Evidence) != len(wantTitles) {
		t.Fatalf("got %d evidence items; want %d: %+v", len(exp.Evidence), len(wantTitles), exp.Evidence)
	}
	for _, e := range exp.Evidence {
		wantType, ok := wantTitles[e.Title]
		if !ok {
			t.Errorf("unexpected evidence title %q (a stale/wrong-ticker item leaked through)", e.Title)
			continue
		}
		if e.Type != wantType {
			t.Errorf("evidence %q type = %q; want %q", e.Title, e.Type, wantType)
		}
		if e.URL == "" {
			t.Errorf("evidence %q has no URL; want it attributed to its source", e.Title)
		}
	}
	// Newest-first across types: the -1h news leads.
	if exp.Evidence[0].Title != "Recent headline" {
		t.Errorf("first evidence = %q; want the newest item (Recent headline)", exp.Evidence[0].Title)
	}
}

func TestGatherEvidence_CapsAtMax(t *testing.T) {
	var news []store.News
	for i := 0; i < maxEvidence+3; i++ {
		news = append(news, store.News{
			Headline:  "Headline " + string(rune('A'+i)),
			URL:       "https://n",
			Published: asOf.Add(-time.Duration(i+1) * time.Minute),
		})
	}
	exp := Assemble("AAPL", Inputs{Quote: quoteAt(110, 100), News: news})
	if len(exp.Evidence) != maxEvidence {
		t.Errorf("evidence count = %d; want it capped at %d", len(exp.Evidence), maxEvidence)
	}
}

func TestMaterial_GoOwnsNumberAndAttributesEvidence(t *testing.T) {
	exp := Assemble("AAPL", Inputs{
		Quote: quoteAt(110, 100),
		News:  []store.News{{Headline: "Apple beats estimates", URL: "u", Published: asOf.Add(-time.Hour)}},
	})
	mat := Material(exp)
	if !strings.Contains(mat, "今日上涨 10.0%") {
		t.Errorf("material = %q; want the Go-owned +10.0%% move stated", mat)
	}
	if !strings.Contains(mat, "不得改动或重新计算") {
		t.Errorf("material = %q; want the do-not-recompute instruction", mat)
	}
	if !strings.Contains(mat, "[新闻] Apple beats estimates") {
		t.Errorf("material = %q; want the evidence attributed with its source type", mat)
	}
}

func TestMaterial_EmptyForInsignificant(t *testing.T) {
	exp := Assemble("AAPL", Inputs{Quote: quoteAt(101, 100)}) // +1%, not significant
	if got := Material(exp); got != "" {
		t.Errorf("material = %q; want empty for a sub-threshold move (no LLM call warranted)", got)
	}
}

// fakeEnricher is a controllable movement.Enricher: ExplainMove returns a fixed
// sentence (or error) and records the material it saw, so the service's LLM
// overlay + degrade paths can be asserted.
type fakeEnricher struct {
	enabled  bool
	sentence string
	err      error
	gotMat   string
	calls    int
}

func (f *fakeEnricher) Enabled() bool { return f.enabled }
func (f *fakeEnricher) ExplainMove(_ context.Context, material, _ string) (string, error) {
	f.calls++
	f.gotMat = material
	return f.sentence, f.err
}

func TestService_ExplainLLMOverlay(t *testing.T) {
	st := &fakeStore{quote: quoteAt(110, 100)}
	enr := &fakeEnricher{enabled: true, sentence: "今日涨10.0%,可能与新闻有关。"}
	svc := NewService(st, nil, enr, "deepseek-chat")

	exp := svc.Explain(context.Background(), "AAPL", "zh")
	if !exp.LLM {
		t.Error("llm = false; want true when the LLM produced a sentence")
	}
	if exp.Text != "今日涨10.0%,可能与新闻有关。" {
		t.Errorf("explanation = %q; want the LLM sentence", exp.Text)
	}
	if exp.Model != "deepseek-chat" {
		t.Errorf("model = %q; want deepseek-chat", exp.Model)
	}
	if exp.Disclaimer != Disclaimer {
		t.Errorf("disclaimer = %q; want the mandatory label", exp.Disclaimer)
	}
	// The Go-owned number is untouched by the LLM.
	if round1(exp.ChangePct) != 10 {
		t.Errorf("change_pct = %v; want the Go-owned 10 (LLM must not alter it)", exp.ChangePct)
	}
}

func TestService_ExplainDegradesToCannedOnError(t *testing.T) {
	st := &fakeStore{
		quote: quoteAt(110, 100),
		news:  []store.News{{Headline: "Apple beats estimates", URL: "u", Published: asOf.Add(-time.Hour)}},
	}
	enr := &fakeEnricher{enabled: true, err: errors.New("llm down")}
	svc := NewService(st, nil, enr, "deepseek-chat")

	exp := svc.Explain(context.Background(), "AAPL", "zh")
	if exp.LLM {
		t.Error("llm = true; want false when the LLM call errored")
	}
	if !strings.Contains(exp.Text, "今日涨10.0%") || !strings.Contains(exp.Text, "Apple beats estimates") {
		t.Errorf("explanation = %q; want the canned data-only line on LLM error", exp.Text)
	}
}

func TestService_ExplainDataOnlyWhenDisabled(t *testing.T) {
	st := &fakeStore{quote: quoteAt(85, 100)} // -15%, no evidence
	enr := &fakeEnricher{enabled: false}
	svc := NewService(st, nil, enr, "")

	exp := svc.Explain(context.Background(), "AAPL", "zh")
	if exp.LLM {
		t.Error("llm = true; want false when the enricher is disabled")
	}
	if enr.calls != 0 {
		t.Errorf("ExplainMove called %d times; want 0 when disabled", enr.calls)
	}
	if exp.Text != "今日跌15.0%,暂无明确催化消息。" {
		t.Errorf("explanation = %q; want the canned no-catalyst line", exp.Text)
	}
}

func TestService_ExplainSkipsLLMForInsignificant(t *testing.T) {
	st := &fakeStore{quote: quoteAt(102, 100)} // +2%, not significant
	enr := &fakeEnricher{enabled: true, sentence: "should not be used"}
	svc := NewService(st, nil, enr, "deepseek-chat")

	exp := svc.Explain(context.Background(), "AAPL", "zh")
	if exp.Significant {
		t.Fatal("significant = true; want false for +2%")
	}
	if enr.calls != 0 {
		t.Errorf("ExplainMove called %d times; want 0 for a sub-threshold move", enr.calls)
	}
}

// round1 rounds to one decimal so float comparisons in the table are exact.
func round1(v float64) float64 {
	return float64(int64(v*10+sign(v)*0.5)) / 10
}

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

// fakeStore is a movement.StoreReader backed by fixed slices.
type fakeStore struct {
	quote   store.Quote
	news    []store.News
	filings []store.Filing
	insider []store.InsiderBuy
}

func (f *fakeStore) GetQuote(context.Context, string) (store.Quote, bool, error) {
	if f.quote.Price <= 0 {
		return store.Quote{}, false, nil
	}
	return f.quote, true, nil
}
func (f *fakeStore) ListNews(context.Context, string, int) ([]store.News, error) {
	return f.news, nil
}
func (f *fakeStore) ListFilings(context.Context, string, int) ([]store.Filing, error) {
	return f.filings, nil
}
func (f *fakeStore) RecentInsiderBuys(context.Context, time.Time) ([]store.InsiderBuy, error) {
	return f.insider, nil
}
