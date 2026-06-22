package chat

import (
	"context"
	"strings"
	"testing"

	"github.com/wombow-ai/tickwind/internal/enrich"
	"github.com/wombow-ai/tickwind/internal/research"
	"github.com/wombow-ai/tickwind/internal/symbols"
)

// fakeSymbols is a controllable chat.SymbolDescriber.
type fakeSymbols struct{ m map[string]symbols.Symbol }

func (f fakeSymbols) ByTicker(t string) (symbols.Symbol, bool) {
	s, ok := f.m[strings.ToUpper(strings.TrimSpace(t))]
	return s, ok
}

// scriptedLLM returns queued replies and captures the messages it was sent, so a test can
// assert the tool round-trip (what the model saw on the follow-up call).
type scriptedLLM struct {
	enabled     bool
	replies     []reply
	calls       int
	gotMessages [][]enrich.ChatMessage
	lastTools   []enrich.ChatTool
}

type reply struct {
	content string
	calls   []enrich.ChatToolCall
	usage   enrich.Usage
}

// fakeWeb is a controllable chat.WebSearcher.
type fakeWeb struct{ result string }

func (f fakeWeb) Search(_ context.Context, _, _ string) string { return f.result }

func hasTool(tools []enrich.ChatTool, name string) bool {
	for _, t := range tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

func (f *scriptedLLM) Enabled() bool { return f.enabled }

func (f *scriptedLLM) Chat(_ context.Context, msgs []enrich.ChatMessage, tools []enrich.ChatTool, _ string) (string, []enrich.ChatToolCall, enrich.Usage, error) {
	f.gotMessages = append(f.gotMessages, msgs)
	f.lastTools = tools
	if f.calls >= len(f.replies) {
		return "", nil, enrich.Usage{}, nil // default: empty final answer
	}
	r := f.replies[f.calls]
	f.calls++
	return r.content, r.calls, r.usage, nil
}

func (f *scriptedLLM) ChatStream(ctx context.Context, msgs []enrich.ChatMessage, tools []enrich.ChatTool, model string, onToken func(string)) (string, []enrich.ChatToolCall, enrich.Usage, error) {
	content, calls, usage, err := f.Chat(ctx, msgs, tools, model)
	if onToken != nil && content != "" {
		onToken(content)
	}
	return content, calls, usage, err
}

type fakeFacts struct{ fs research.FactSheet }

func (f fakeFacts) Report(_ context.Context, _, _ string) research.FactSheet { return f.fs }

func sampleSheet() research.FactSheet {
	return research.FactSheet{
		Ticker: "AAPL", Name: "Apple", AsOf: "2026-06-20", PriceLabel: "$200",
		Sections: []research.SectionFacts{
			{Key: "valuation", TitleEN: "Valuation", TitleZH: "估值", Facts: []research.Fact{
				{Key: "pe", LabelEN: "P/E", LabelZH: "市盈率", Value: "31.2x", Status: research.StatusOK, Source: "SEC"},
			}},
			{Key: "sentiment", TitleEN: "Sentiment", TitleZH: "情绪面", Context: []string{"per news (Reuters): upbeat tone"}},
		},
	}
}

func textOf(a Answer) string {
	for _, b := range a.Blocks {
		if b.Kind == "text" {
			return b.Text
		}
	}
	return ""
}

func TestAnswerDirect(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{{content: "Apple trades at 31.2x earnings."}}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "what's the P/E?", true)
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if textOf(ans) != "Apple trades at 31.2x earnings." {
		t.Fatalf("text = %q", textOf(ans))
	}
	if len(ans.Blocks) != 1 {
		t.Fatalf("want 1 block, got %d", len(ans.Blocks))
	}
	// The system prompt must carry the per-ticker facts (grounding) + the firewall.
	sys := llm.gotMessages[0][0]
	if sys.Role != "system" || !strings.Contains(sys.Content, "31.2x") || !strings.Contains(sys.Content, "NEVER invent") {
		t.Fatalf("system prompt missing facts or firewall: %q", sys.Content)
	}
}

func TestAnswerGetFactsToolRoundTrip(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{calls: []enrich.ChatToolCall{{ID: "c1", Name: "get_facts", Arguments: `{"section":"valuation"}`}}},
		{content: "Per the valuation facts, P/E is 31.2x."},
	}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "pull valuation", true)
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if !strings.Contains(textOf(ans), "31.2x") {
		t.Fatalf("final answer = %q", textOf(ans))
	}
	// The SECOND Chat call must include a tool result message with the real Go facts.
	second := llm.gotMessages[1]
	var toolMsg string
	for _, m := range second {
		if m.Role == "tool" {
			toolMsg = m.Content
		}
	}
	if !strings.Contains(toolMsg, "P/E: 31.2x [SEC]") {
		t.Fatalf("tool result did not carry the Go facts: %q", toolMsg)
	}
}

// TestAnswerGetFactsCarriesAsOf guards the staleness fix: a fact that carries a
// freshness stamp (e.g. a ~45-day-stale 13F holder) must surface its as-of inside
// the citation bracket in the tool result, so the model never quotes it as current
// with no traceable vintage. (get_facts → FactsForSection → writeSection.)
func TestAnswerGetFactsCarriesAsOf(t *testing.T) {
	sheet := research.FactSheet{
		Ticker: "AAPL", Name: "Apple", AsOf: "2026-06-20", PriceLabel: "$200",
		Sections: []research.SectionFacts{
			{Key: "flows", TitleEN: "Smart Money", TitleZH: "资金面", Facts: []research.Fact{
				{Key: "top13f", LabelEN: "Top institutional holder", Value: "Berkshire 22%", Status: research.StatusOK, Source: "SEC 13F", AsOf: "2026-03-31"},
			}},
		},
	}
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{calls: []enrich.ChatToolCall{{ID: "c1", Name: "get_facts", Arguments: `{"section":"flows"}`}}},
		{content: "Berkshire holds 22%."},
	}}
	svc := NewService(llm, fakeFacts{sheet}, nil, "")
	if _, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "who holds it?", true); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	var toolMsg string
	for _, m := range llm.gotMessages[1] {
		if m.Role == "tool" {
			toolMsg = m.Content
		}
	}
	if !strings.Contains(toolMsg, "[SEC 13F, as of 2026-03-31]") {
		t.Fatalf("tool result missing per-fact as-of stamp: %q", toolMsg)
	}
}

// TestAnswerStream: the streaming variant forwards the FINAL answer's content to onToken
// (a tool-only turn streams nothing) and returns the same advice-filtered Answer as Answer.
func TestAnswerStream(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{calls: []enrich.ChatToolCall{{ID: "c1", Name: "get_facts", Arguments: `{"section":"valuation"}`}}},
		{content: "Apple trades at 31.2x earnings."},
	}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
	var streamed strings.Builder
	ans, err := svc.AnswerStream(context.Background(), "u", "AAPL", "en", nil, "what's the P/E?", true, func(tok string) { streamed.WriteString(tok) })
	if err != nil {
		t.Fatalf("AnswerStream: %v", err)
	}
	if textOf(ans) != "Apple trades at 31.2x earnings." {
		t.Fatalf("answer text = %q", textOf(ans))
	}
	if streamed.String() != "Apple trades at 31.2x earnings." {
		t.Fatalf("streamed tokens = %q, want the final answer only (no tokens for the tool turn)", streamed.String())
	}
}

// TestAnswerSearchWeb: when a WebSearcher is wired, the search_web tool is OFFERED and its
// attributed result is delivered to the model as a tool message (the model may quote it,
// but the firewall — tested elsewhere — forbids deriving numbers).
func TestAnswerSearchWeb(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{calls: []enrich.ChatToolCall{{ID: "c1", Name: "search_web", Arguments: `{"query":"AAPL latest news"}`}}},
		{content: "Recent coverage notes a product launch."},
	}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
	svc.SetWebSearch(fakeWeb{result: "Web search results (attributed):\n- Apple unveils X — snippet [reuters.com]"})
	if _, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "any news?", true); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if !hasTool(llm.lastTools, "search_web") {
		t.Fatal("search_web tool was not offered despite a WebSearcher being set")
	}
	var toolMsg string
	for _, m := range llm.gotMessages[1] {
		if m.Role == "tool" {
			toolMsg = m.Content
		}
	}
	if !strings.Contains(toolMsg, "[reuters.com]") {
		t.Fatalf("attributed web result not delivered to the model: %q", toolMsg)
	}
}

// TestSearchWebNotOfferedWithoutWebSearcher: no WebSearcher → the tool is never offered.
func TestSearchWebNotOfferedWithoutWebSearcher(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{{content: "ok"}}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "") // no SetWebSearch
	if _, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "hi", true); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if hasTool(llm.lastTools, "search_web") {
		t.Fatal("search_web offered without a WebSearcher wired")
	}
}

func TestAnswerSurfaceWidget(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{calls: []enrich.ChatToolCall{{ID: "c1", Name: "surface_widget", Arguments: `{"type":"kline","range":"1Y"}`}}},
		{content: "Here is the 1-year chart."},
	}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "show me the chart", true)
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	var w *Block
	for i := range ans.Blocks {
		if ans.Blocks[i].Kind == "widget" {
			w = &ans.Blocks[i]
		}
	}
	if w == nil || w.Widget != "kline" || w.Params["range"] != "1Y" {
		t.Fatalf("widget block wrong: %+v", ans.Blocks)
	}
	// CRITICAL: the widget tool result must NOT carry numbers into the model context.
	tool := ""
	for _, m := range llm.gotMessages[1] {
		if m.Role == "tool" {
			tool = m.Content
		}
	}
	if tool != "rendered: kline AAPL 1Y" {
		t.Fatalf("widget tool result wrong / leaked data into context: %q", tool)
	}
}

func TestAnswerIndicatorHistoryWidget(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{calls: []enrich.ChatToolCall{{ID: "c1", Name: "surface_widget", Arguments: `{"type":"indicator_history","indicator":"rsi"}`}}},
		{content: "Here is the RSI history."},
	}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "show me RSI over the last year", true)
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	var w *Block
	for i := range ans.Blocks {
		if ans.Blocks[i].Kind == "widget" {
			w = &ans.Blocks[i]
		}
	}
	if w == nil || w.Widget != "indicator_history" {
		t.Fatalf("no indicator_history widget: %+v", ans.Blocks)
	}
	// The friendly name must be mapped to the catalog id, anchored to the conversation ticker.
	if w.Params["indicator"] != "technical.rsi" || w.Params["ticker"] != "AAPL" {
		t.Fatalf("widget params wrong: %+v", w.Params)
	}
	// Anti-hallucination: the tool result is a confirmation only — no series numbers.
	tool := ""
	for _, m := range llm.gotMessages[1] {
		if m.Role == "tool" {
			tool = m.Content
		}
	}
	if tool != "rendered: indicator_history AAPL " {
		t.Fatalf("widget tool result leaked data / wrong: %q", tool)
	}
}

func TestAnswerSeasonalityWidget(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{calls: []enrich.ChatToolCall{{ID: "c1", Name: "surface_widget", Arguments: `{"type":"seasonality"}`}}},
		{content: "Here is AAPL's seasonality."},
	}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "what's AAPL's seasonality", true)
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	var w *Block
	for i := range ans.Blocks {
		if ans.Blocks[i].Kind == "widget" {
			w = &ans.Blocks[i]
		}
	}
	if w == nil || w.Widget != "seasonality" || w.Params["ticker"] != "AAPL" {
		t.Fatalf("seasonality widget wrong: %+v", ans.Blocks)
	}
	// Anti-hallucination: the tool result is a confirmation only — no monthly numbers.
	tool := ""
	for _, m := range llm.gotMessages[1] {
		if m.Role == "tool" {
			tool = m.Content
		}
	}
	if tool != "rendered: seasonality AAPL " {
		t.Fatalf("widget tool result leaked data / wrong: %q", tool)
	}
}

func TestAnswerRelativeStrengthWidget(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{calls: []enrich.ChatToolCall{{ID: "c1", Name: "surface_widget", Arguments: `{"type":"relative_strength"}`}}},
		{content: "Here is how AAPL has done versus the market."},
	}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "is AAPL beating the market", true)
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	var w *Block
	for i := range ans.Blocks {
		if ans.Blocks[i].Kind == "widget" {
			w = &ans.Blocks[i]
		}
	}
	if w == nil || w.Widget != "relative_strength" || w.Params["ticker"] != "AAPL" {
		t.Fatalf("relative_strength widget wrong: %+v", ans.Blocks)
	}
	// Anti-hallucination: the tool result is a confirmation only — no return numbers.
	tool := ""
	for _, m := range llm.gotMessages[1] {
		if m.Role == "tool" {
			tool = m.Content
		}
	}
	if tool != "rendered: relative_strength AAPL " {
		t.Fatalf("widget tool result leaked data / wrong: %q", tool)
	}
}

func TestDedupeWidgets(t *testing.T) {
	wg := func(typ, tk string) Block {
		return Block{Kind: "widget", Widget: typ, Params: map[string]string{"ticker": tk}}
	}
	names := func(bs []Block) []string {
		out := make([]string, len(bs))
		for i, b := range bs {
			out[i] = b.Widget
		}
		return out
	}
	eq := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	tests := []struct {
		name string
		in   []Block
		want []string
	}{
		{"drop redundant chart beside specific", []Block{wg("kline", "AAPL"), wg("relative_strength", "AAPL")}, []string{"relative_strength"}},
		{"indicators chart also dropped", []Block{wg("indicators", "AAPL"), wg("earnings_reaction", "AAPL")}, []string{"earnings_reaction"}},
		{"lone chart kept", []Block{wg("kline", "AAPL")}, []string{"kline"}},
		{"two specifics both kept", []Block{wg("relative_strength", "AAPL"), wg("seasonality", "AAPL")}, []string{"relative_strength", "seasonality"}},
		{"exact dup collapsed", []Block{wg("relative_strength", "AAPL"), wg("relative_strength", "AAPL")}, []string{"relative_strength"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := names(dedupeWidgets(tc.in)); !eq(got, tc.want) {
				t.Fatalf("dedupeWidgets = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAnswerEarningsReactionWidget(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{calls: []enrich.ChatToolCall{{ID: "c1", Name: "surface_widget", Arguments: `{"type":"earnings_reaction"}`}}},
		{content: "Here is how AAPL has historically moved around earnings."},
	}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "how does AAPL move on earnings", true)
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	var w *Block
	for i := range ans.Blocks {
		if ans.Blocks[i].Kind == "widget" {
			w = &ans.Blocks[i]
		}
	}
	if w == nil || w.Widget != "earnings_reaction" || w.Params["ticker"] != "AAPL" {
		t.Fatalf("earnings_reaction widget wrong: %+v", ans.Blocks)
	}
	// Anti-hallucination: the tool result is a confirmation only — no reaction numbers.
	tool := ""
	for _, m := range llm.gotMessages[1] {
		if m.Role == "tool" {
			tool = m.Content
		}
	}
	if tool != "rendered: earnings_reaction AAPL " {
		t.Fatalf("widget tool result leaked data / wrong: %q", tool)
	}
}

func TestAnswerScorecardWidget(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{calls: []enrich.ChatToolCall{{ID: "c1", Name: "surface_widget", Arguments: `{"type":"scorecard"}`}}},
		{content: "Here is how AAPL ranks on the factors."},
	}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "how's AAPL's value and quality vs the market", true)
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	var w *Block
	for i := range ans.Blocks {
		if ans.Blocks[i].Kind == "widget" {
			w = &ans.Blocks[i]
		}
	}
	if w == nil || w.Widget != "scorecard" || w.Params["ticker"] != "AAPL" {
		t.Fatalf("scorecard widget wrong: %+v", ans.Blocks)
	}
	// Anti-hallucination: the tool result is a confirmation only — no factor numbers.
	tool := ""
	for _, m := range llm.gotMessages[1] {
		if m.Role == "tool" {
			tool = m.Content
		}
	}
	if tool != "rendered: scorecard AAPL " {
		t.Fatalf("widget tool result leaked data / wrong: %q", tool)
	}
}

func TestAnswerIndicatorHistoryRejectsUnknownIndicator(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{calls: []enrich.ChatToolCall{{ID: "c1", Name: "surface_widget", Arguments: `{"type":"indicator_history","indicator":"made_up"}`}}},
		{content: "ok"},
	}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "chart the foobar indicator", true)
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	for _, b := range ans.Blocks {
		if b.Kind == "widget" {
			t.Fatalf("an unknown indicator must NOT render a widget: %+v", b)
		}
	}
}

func TestAnswerUnknownWidgetAndSectionGraceful(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{calls: []enrich.ChatToolCall{
			{ID: "c1", Name: "surface_widget", Arguments: `{"type":"hovercraft"}`},
			{ID: "c2", Name: "get_facts", Arguments: `{"section":"nope"}`},
		}},
		{content: "ok"},
	}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "junk", true)
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	// No widget block for an invalid type.
	for _, b := range ans.Blocks {
		if b.Kind == "widget" {
			t.Fatalf("invalid widget should not render: %+v", b)
		}
	}
	var results []string
	for _, m := range llm.gotMessages[1] {
		if m.Role == "tool" {
			results = append(results, m.Content)
		}
	}
	joined := strings.Join(results, "|")
	if !strings.Contains(joined, "Unknown widget type") || !strings.Contains(joined, "No such section") {
		t.Fatalf("graceful tool errors missing: %q", joined)
	}
}

func TestAnswerIterationCap(t *testing.T) {
	// Always request a tool: the loop must stop after maxToolIters and force a final
	// answer (no infinite loop). 4 tool rounds + 1 forced no-tool call = 5 Chat calls.
	loop := []reply{}
	for i := 0; i < maxToolIters; i++ {
		loop = append(loop, reply{calls: []enrich.ChatToolCall{{ID: "x", Name: "get_facts", Arguments: `{"section":"valuation"}`}}})
	}
	loop = append(loop, reply{content: "final forced answer"})
	llm := &scriptedLLM{enabled: true, replies: loop}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "loop forever", true)
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if llm.calls != maxToolIters+1 {
		t.Fatalf("Chat calls = %d, want %d", llm.calls, maxToolIters+1)
	}
	if llm.lastTools != nil {
		t.Fatalf("the forced final call must pass NO tools")
	}
	if textOf(ans) != "final forced answer" {
		t.Fatalf("text = %q", textOf(ans))
	}
}

func TestAnswerAdviceFilter(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{content: "P/E is 31.2x.\nMy price target is $250 and you should buy."},
	}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
	ans, _ := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "q", true)
	got := textOf(ans)
	if !strings.Contains(got, "31.2x") {
		t.Fatalf("kept line missing: %q", got)
	}
	if strings.Contains(strings.ToLower(got), "price target") || strings.Contains(strings.ToLower(got), "should buy") {
		t.Fatalf("advice line not stripped: %q", got)
	}
}

func TestAnswerAllAdviceRedirects(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{content: "Strong buy.\nYou should buy now."},
	}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
	ans, _ := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "should I buy?", true)
	if textOf(ans) != redirectNote("en") {
		t.Fatalf("want redirect note, got %q", textOf(ans))
	}
}

// TestAnswerHardenedAdviceFilter covers the red-team hardening: valuation/entry synonyms
// + a buy-action-at-a-price-level all collapse to the redirect note.
func TestAnswerHardenedAdviceFilter(t *testing.T) {
	cases := []struct{ name, prose string }{
		{"fair-value", "AAPL fair value is around $250."},
		{"buy-at-price", "I'd buy at $150 here."},
		{"deserves-position", "For long-term holders it deserves a position."},
		{"undervalued", "The stock looks undervalued to me."},
		{"entry-point", "This is a great entry point."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			llm := &scriptedLLM{enabled: true, replies: []reply{{content: tc.prose}}}
			svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
			ans, _ := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "q", true)
			if textOf(ans) != redirectNote("en") {
				t.Fatalf("%s: want redirect, got %q", tc.name, textOf(ans))
			}
		})
	}
}

// TestAnswerCrossLineAdviceRedirects: advice split so NO single line trips, but the
// joined whole-text pass catches it.
func TestAnswerCrossLineAdviceRedirects(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{content: "The fundamentals look strong.\nGiven that, this is a compelling\nbuy."},
	}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
	ans, _ := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "q", true)
	if textOf(ans) != redirectNote("en") {
		t.Fatalf("cross-line advice not caught: %q", textOf(ans))
	}
}

// TestAnswerKeepsLegitInsiderFact: bare buy/sell describing a FACT must NOT be stripped
// (the contract's deliberate exclusion).
func TestAnswerKeepsLegitInsiderFact(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{content: "Insiders bought 12,000 shares last quarter, per Form 4."},
	}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
	ans, _ := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "q", true)
	if !strings.Contains(textOf(ans), "Insiders bought") {
		t.Fatalf("legit insider fact was wrongly stripped: %q", textOf(ans))
	}
}

// TestAnswerETFGroundsNoFundamentals: in a GENERAL conversation, asking an ETF (empty fact
// sheet) for fundamentals must return a Go-OWNED descriptor ("… is an ETF … no company-level
// fundamentals") instead of a bare "No data." — so the model grounds its answer rather than
// inventing a launch year / coverage reason. The descriptor carries NO number and NO date.
func TestAnswerETFGroundsNoFundamentals(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{calls: []enrich.ChatToolCall{{ID: "c1", Name: "get_stock_facts", Arguments: `{"ticker":"DRAM","section":"fundamentals"}`}}},
		{content: "DRAM is an ETF, so it has no company fundamentals."},
	}}
	svc := NewService(llm, fakeFacts{research.FactSheet{}}, nil, "") // empty sheet for any ticker
	svc.SetSymbols(fakeSymbols{m: map[string]symbols.Symbol{"DRAM": {Ticker: "DRAM", Name: "Roundhill Memory ETF", ETF: true}}})
	if _, err := svc.Answer(context.Background(), "u", "", "en", nil, "show DRAM fundamentals", true); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	var toolMsg string
	for _, m := range llm.gotMessages[1] {
		if m.Role == "tool" {
			toolMsg = m.Content
		}
	}
	if !strings.Contains(toolMsg, "is an ETF") || !strings.Contains(toolMsg, "Roundhill Memory ETF") {
		t.Fatalf("ETF grounding missing from tool result: %q", toolMsg)
	}
	if strings.Contains(toolMsg, "No data") {
		t.Fatalf("bare 'No data' should have been replaced by the grounded descriptor: %q", toolMsg)
	}
	// The Go-owned descriptor must carry NO ungrounded year (the exact bug we're fixing).
	for _, yr := range []string{"2024", "2025", "2026", "newly", "launched", "coverage"} {
		if strings.Contains(strings.ToLower(toolMsg), yr) {
			t.Fatalf("descriptor leaked an ungrounded claim %q: %q", yr, toolMsg)
		}
	}
}

// TestAnswerETFEmptySectionGrounded: an ANCHORED ETF whose fact sheet has a technical section
// but no fundamentals section — get_facts("fundamentals") hits the empty-SECTION path, which
// must also ground in the ETF descriptor rather than the generic "no such section" line.
func TestAnswerETFEmptySectionGrounded(t *testing.T) {
	etfSheet := research.FactSheet{
		Ticker: "DRAM", Name: "Roundhill Memory ETF", AsOf: "2026-06-20", PriceLabel: "$77",
		Sections: []research.SectionFacts{{Key: "technical", TitleEN: "Technical", TitleZH: "技术面"}},
	}
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{calls: []enrich.ChatToolCall{{ID: "c1", Name: "get_facts", Arguments: `{"section":"fundamentals"}`}}},
		{content: "DRAM is an ETF — no company fundamentals; here's the technical picture."},
	}}
	svc := NewService(llm, fakeFacts{etfSheet}, nil, "")
	svc.SetSymbols(fakeSymbols{m: map[string]symbols.Symbol{"DRAM": {Ticker: "DRAM", Name: "Roundhill Memory ETF", ETF: true}}})
	if _, err := svc.Answer(context.Background(), "u", "DRAM", "en", nil, "what are DRAM's fundamentals?", true); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	var toolMsg string
	for _, m := range llm.gotMessages[1] {
		if m.Role == "tool" {
			toolMsg = m.Content
		}
	}
	if !strings.Contains(toolMsg, "is an ETF") {
		t.Fatalf("empty-section ETF grounding missing: %q", toolMsg)
	}
	if strings.Contains(toolMsg, "no such section") {
		t.Fatalf("generic 'no such section' should have been replaced by the ETF descriptor: %q", toolMsg)
	}
}

// TestAnswerNonETFEmptySectionStaysGeneric: a NON-ETF stock missing a section must NOT be
// mislabeled an ETF — the empty-section path keeps the generic "no such section" line.
func TestAnswerNonETFEmptySectionStaysGeneric(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{calls: []enrich.ChatToolCall{{ID: "c1", Name: "get_stock_facts", Arguments: `{"ticker":"AAPL","section":"fundamentals"}`}}},
		{content: "ok"},
	}}
	// Sheet has only a valuation section (no fundamentals), AAPL is NOT an ETF.
	sheet := research.FactSheet{Ticker: "AAPL", Name: "Apple", AsOf: "2026-06-20", Sections: []research.SectionFacts{{Key: "valuation", TitleEN: "Valuation"}}}
	svc := NewService(llm, fakeFacts{sheet}, nil, "")
	svc.SetSymbols(fakeSymbols{m: map[string]symbols.Symbol{"AAPL": {Ticker: "AAPL", Name: "Apple Inc.", ETF: false}}})
	if _, err := svc.Answer(context.Background(), "u", "", "en", nil, "AAPL fundamentals?", true); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	var toolMsg string
	for _, m := range llm.gotMessages[1] {
		if m.Role == "tool" {
			toolMsg = m.Content
		}
	}
	if !strings.Contains(toolMsg, "no such section") || strings.Contains(toolMsg, "is an ETF") {
		t.Fatalf("non-ETF empty section should stay generic, got: %q", toolMsg)
	}
}

// TestAnswerETFRefusesFundamentalsWidget: defense-in-depth — if the model tries to surface a
// fundamentals_table/valuation_table for an ETF, execTool must REFUSE it (an ETF has no
// company fundamentals → the table would render empty) and return the grounded descriptor
// instead of recording a widget block.
func TestAnswerETFRefusesFundamentalsWidget(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{calls: []enrich.ChatToolCall{{ID: "c1", Name: "surface_widget", Arguments: `{"type":"fundamentals_table","ticker":"DRAM"}`}}},
		{content: "DRAM is an ETF; here's its price chart instead."},
	}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "") // anchor AAPL sheet non-empty
	svc.SetSymbols(fakeSymbols{m: map[string]symbols.Symbol{"DRAM": {Ticker: "DRAM", Name: "Roundhill Memory ETF", ETF: true}}})
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "show DRAM's fundamentals table", true)
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	for _, b := range ans.Blocks {
		if b.Kind == "widget" {
			t.Fatalf("ETF fundamentals_table must be refused, not rendered: %+v", b)
		}
	}
	var toolMsg string
	for _, m := range llm.gotMessages[1] {
		if m.Role == "tool" {
			toolMsg = m.Content
		}
	}
	if !strings.Contains(toolMsg, "is an ETF") {
		t.Fatalf("ETF widget refusal should return the grounded descriptor: %q", toolMsg)
	}
}

func TestAnswerNotFoundAndDisabled(t *testing.T) {
	// Empty fact sheet → ErrNotFound.
	llm := &scriptedLLM{enabled: true}
	svc := NewService(llm, fakeFacts{research.FactSheet{}}, nil, "")
	if _, err := svc.Answer(context.Background(), "u", "ZZZZ", "en", nil, "q", true); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	// LLM disabled → ErrDisabled.
	off := NewService(&scriptedLLM{enabled: false}, fakeFacts{sampleSheet()}, nil, "")
	if _, err := off.Answer(context.Background(), "u", "AAPL", "en", nil, "q", true); err != enrich.ErrDisabled {
		t.Fatalf("err = %v, want ErrDisabled", err)
	}
}
