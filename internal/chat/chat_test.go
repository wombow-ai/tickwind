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
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "what's the P/E?", true, "")
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if textOf(ans) != "Apple trades at 31.2x earnings." {
		t.Fatalf("text = %q", textOf(ans))
	}
	if len(ans.Blocks) != 1 {
		t.Fatalf("want 1 block, got %d", len(ans.Blocks))
	}
	// The system prompt must carry the per-ticker facts (grounding) + the number-discipline rule.
	sys := llm.gotMessages[0][0]
	if sys.Role != "system" || !strings.Contains(sys.Content, "31.2x") || !strings.Contains(sys.Content, "never invent") {
		t.Fatalf("system prompt missing facts or grounding rule: %q", sys.Content)
	}
}

func TestAnswerGetFactsToolRoundTrip(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{calls: []enrich.ChatToolCall{{ID: "c1", Name: "get_facts", Arguments: `{"section":"valuation"}`}}},
		{content: "Per the valuation facts, P/E is 31.2x."},
	}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "pull valuation", true, "")
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
	if _, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "who holds it?", true, ""); err != nil {
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
	var steps []Step
	ans, err := svc.AnswerStream(context.Background(), "u", "AAPL", "en", nil, "what's the P/E?", true, "",
		func(tok string) { streamed.WriteString(tok) },
		func(st Step) { steps = append(steps, st) })
	if err != nil {
		t.Fatalf("AnswerStream: %v", err)
	}
	if textOf(ans) != "Apple trades at 31.2x earnings." {
		t.Fatalf("answer text = %q", textOf(ans))
	}
	if streamed.String() != "Apple trades at 31.2x earnings." {
		t.Fatalf("streamed tokens = %q, want the final answer only (no tokens for the tool turn)", streamed.String())
	}
	// The get_facts tool action is surfaced as one execution-chain step (before it runs).
	if len(steps) != 1 || steps[0].Kind != "facts" {
		t.Fatalf("steps = %+v, want exactly one facts step", steps)
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
	if _, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "any news?", true, ""); err != nil {
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
	if _, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "hi", true, ""); err != nil {
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
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "show me the chart", true, "")
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
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "show me RSI over the last year", true, "")
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
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "what's AAPL's seasonality", true, "")
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
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "is AAPL beating the market", true, "")
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
		{"fundamentals+valuation collapse (same card)", []Block{wg("fundamentals_table", "AAPL"), wg("valuation_table", "AAPL")}, []string{"fundamentals_table"}},
		{"fundamentals+valuation diff tickers both kept", []Block{wg("fundamentals_table", "AAPL"), wg("valuation_table", "MSFT")}, []string{"fundamentals_table", "valuation_table"}},
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
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "how does AAPL move on earnings", true, "")
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
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "how's AAPL's value and quality vs the market", true, "")
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
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "chart the foobar indicator", true, "")
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
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "junk", true, "")
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
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "loop forever", true, "")
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

// TestAnswerShipsAdvice: chat is a full advisor — a buy/sell view + a price target ship
// verbatim (the old deterministic strip is GONE).
func TestAnswerShipsAdvice(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{content: "My price target is $250 and I'd lean buy."},
	}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
	ans, _ := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "should I buy?", true, "")
	got := strings.ToLower(textOf(ans))
	if !strings.Contains(got, "price target") || !strings.Contains(got, "$250") || !strings.Contains(got, "buy") {
		t.Fatalf("advice should ship verbatim, got %q", textOf(ans))
	}
}

// TestAnswerKeepsLegitInsiderFact: a bare buy/sell describing a FACT ships unchanged (nothing
// strips it anymore; the structural number-grounding is the live contract).
func TestAnswerKeepsLegitInsiderFact(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{content: "Insiders bought 12,000 shares last quarter, per Form 4."},
	}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
	ans, _ := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "q", true, "")
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
	if _, err := svc.Answer(context.Background(), "u", "", "en", nil, "show DRAM fundamentals", true, ""); err != nil {
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
	if _, err := svc.Answer(context.Background(), "u", "DRAM", "en", nil, "what are DRAM's fundamentals?", true, ""); err != nil {
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
	if _, err := svc.Answer(context.Background(), "u", "", "en", nil, "AAPL fundamentals?", true, ""); err != nil {
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
	ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "show DRAM's fundamentals table", true, "")
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
	if _, err := svc.Answer(context.Background(), "u", "ZZZZ", "en", nil, "q", true, ""); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	// LLM disabled → ErrDisabled.
	off := NewService(&scriptedLLM{enabled: false}, fakeFacts{sampleSheet()}, nil, "")
	if _, err := off.Answer(context.Background(), "u", "AAPL", "en", nil, "q", true, ""); err != enrich.ErrDisabled {
		t.Fatalf("err = %v, want ErrDisabled", err)
	}
}

// TestStepFor asserts each tool maps to its execution-chain step kind (the label is Go-owned;
// an unknown tool yields no step, and a get_stock_facts with no ticker falls back to the anchor).
func TestStepFor(t *testing.T) {
	tests := []struct {
		name, toolName, args, wantKind string
		ok                             bool
		wantInLabel                    string
	}{
		{"get_facts", "get_facts", `{"section":"valuation"}`, "facts", true, "AAPL valuation"},
		{"get_stock_facts other", "get_stock_facts", `{"ticker":"msft","section":"fundamentals"}`, "facts", true, "MSFT fundamentals"},
		{"get_stock_facts default anchor", "get_stock_facts", `{"section":"technical"}`, "facts", true, "AAPL"},
		{"news", "get_news_context", `{}`, "news", true, "news"},
		{"web shows query", "search_web", `{"query":"NVDA earnings"}`, "web", true, "Searching the web for NVDA earnings"},
		{"web newline collapsed", "search_web", `{"query":"line one\nline two"}`, "web", true, "Searching the web for line one line two"},
		{"widget", "surface_widget", `{"type":"kline"}`, "widget", true, "chart"},
		{"watchlist", "get_watchlist", `{}`, "watchlist", true, "watchlist"},
		{"unknown", "frobnicate", `{}`, "", false, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st, ok := stepFor(enrich.ChatToolCall{Name: tc.toolName, Arguments: tc.args}, "AAPL", "en")
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if !tc.ok {
				return
			}
			if st.Kind != tc.wantKind {
				t.Errorf("kind = %q, want %q", st.Kind, tc.wantKind)
			}
			if !strings.Contains(st.Label, tc.wantInLabel) {
				t.Errorf("label = %q, want to contain %q", st.Label, tc.wantInLabel)
			}
			// The web step's query echo must NEVER contain a raw newline (layout-safety).
			if strings.ContainsAny(st.Label, "\n\r") {
				t.Errorf("step label has a raw newline: %q", st.Label)
			}
		})
	}
	// A long query is rune-truncated with an ellipsis so the chain row stays bounded.
	long := strings.Repeat("alpha ", 30)
	st, _ := stepFor(enrich.ChatToolCall{Name: "search_web", Arguments: `{"query":"` + long + `"}`}, "AAPL", "en")
	if !strings.HasSuffix(st.Label, "…") || len([]rune(st.Label)) > 80 {
		t.Errorf("long web query not truncated: %q (%d runes)", st.Label, len([]rune(st.Label)))
	}
}

// TestBackstopWidget covers the deterministic auto-surface: it fires for exactly one stock's
// topical facts, picks the highest-priority section's widget, refuses the fundamentals family
// for an ETF (kline allowed), and skips multi-ticker / redirect / already-surfaced / non-topical.
func TestBackstopWidget(t *testing.T) {
	svc := NewService(&scriptedLLM{enabled: true}, fakeFacts{sampleSheet()}, nil, "")
	svc.SetSymbols(fakeSymbols{m: map[string]symbols.Symbol{
		"DRAM": {Ticker: "DRAM", Name: "Roundhill Memory ETF", ETF: true},
		"AAPL": {Ticker: "AAPL", Name: "Apple Inc.", ETF: false},
	}})

	t.Run("fundamentals surfaces fundamentals_table", func(t *testing.T) {
		ts := &turnState{}
		ts.recordTopical("AAPL", "fundamentals")
		b, ok := svc.backstopWidget(ts, "en", "Apple is profitable.")
		if !ok || b.Widget != "fundamentals_table" || b.Params["ticker"] != "AAPL" {
			t.Fatalf("got %+v ok=%v, want fundamentals_table/AAPL", b, ok)
		}
		if len(b.Params) != 1 {
			t.Errorf("backstop widget carries %d params, want only ticker", len(b.Params))
		}
	})
	t.Run("valuation outranks fundamentals", func(t *testing.T) {
		ts := &turnState{}
		ts.recordTopical("AAPL", "fundamentals")
		ts.recordTopical("AAPL", "valuation")
		b, ok := svc.backstopWidget(ts, "en", "x")
		if !ok || b.Widget != "valuation_table" {
			t.Fatalf("got %+v ok=%v, want valuation_table (higher priority)", b, ok)
		}
	})
	t.Run("technical surfaces kline", func(t *testing.T) {
		ts := &turnState{}
		ts.recordTopical("AAPL", "technical")
		b, ok := svc.backstopWidget(ts, "en", "x")
		if !ok || b.Widget != "kline" {
			t.Fatalf("got %+v ok=%v, want kline", b, ok)
		}
	})
	t.Run("ETF refuses fundamentals family", func(t *testing.T) {
		ts := &turnState{}
		ts.recordTopical("DRAM", "fundamentals")
		if b, ok := svc.backstopWidget(ts, "en", "x"); ok {
			t.Fatalf("ETF should not get a fundamentals widget, got %+v", b)
		}
	})
	t.Run("ETF still gets a price chart", func(t *testing.T) {
		ts := &turnState{}
		ts.recordTopical("DRAM", "technical")
		if b, ok := svc.backstopWidget(ts, "en", "x"); !ok || b.Widget != "kline" {
			t.Fatalf("ETF kline should be allowed, got %+v ok=%v", b, ok)
		}
	})
	t.Run("multi-ticker aborts", func(t *testing.T) {
		ts := &turnState{}
		ts.recordTopical("AAPL", "valuation")
		ts.recordTopical("MSFT", "fundamentals")
		if _, ok := svc.backstopWidget(ts, "en", "x"); ok {
			t.Fatal("a 2-stock comparison must not auto-surface a widget")
		}
	})
	t.Run("already-surfaced aborts", func(t *testing.T) {
		ts := &turnState{Widgets: []Block{{Kind: "widget", Widget: "kline"}}}
		ts.recordTopical("AAPL", "valuation")
		if _, ok := svc.backstopWidget(ts, "en", "x"); ok {
			t.Fatal("must not add a widget when the model already surfaced one")
		}
	})
	t.Run("redirect prose aborts", func(t *testing.T) {
		ts := &turnState{}
		ts.recordTopical("AAPL", "valuation")
		if _, ok := svc.backstopWidget(ts, "en", redirectNote("en")); ok {
			t.Fatal("must not decorate a stripped-to-redirect answer")
		}
	})
	t.Run("no topical facts aborts", func(t *testing.T) {
		ts := &turnState{}
		ts.recordTopical("AAPL", "sentiment") // non-topical → not recorded
		if _, ok := svc.backstopWidget(ts, "en", "x"); ok {
			t.Fatal("a sentiment-only / general turn must not auto-surface a widget")
		}
	})
}

// TestSystemPromptModes asserts the mode dial changes DEPTH only: every mode keeps the factual
// guard (rule 1 + GROUNDING); explore adds the fuller two-sided/your-call appendix while focused
// gets the tight appendix; there is NO no-advice boundary anywhere (it was removed).
func TestSystemPromptModes(t *testing.T) {
	// The Auto/Focused/Explore toggle was removed → mode is INERT: every mode value produces the
	// SAME prompt, and no explore/focused depth appendix remains.
	base := systemPrompt("AAPL", "en", "facts", false, false, true, "")
	for _, mode := range []string{"", "focused", "explore"} {
		if got := systemPrompt("AAPL", "en", "facts", false, false, true, mode); got != base {
			t.Errorf("mode %q changed the prompt — mode must be inert", mode)
		}
	}
	if strings.Contains(base, "EXPLORE turn") || strings.Contains(base, "FOCUSED turn") {
		t.Error("the explore/focused depth appendix must be gone")
	}
	// RULE ZERO (resourceful tool use, never refuse without searching) leads, and is hasWeb-gated.
	noWeb := systemPrompt("AAPL", "en", "facts", false, false, false, "")
	if !strings.Contains(base, "RULE ZERO") || !strings.Contains(noWeb, "RULE ZERO") {
		t.Error("RULE ZERO must be present in every prompt")
	}
	if !strings.Contains(base, "search_web") {
		t.Error("the hasWeb prompt must point the model at search_web")
	}
	if strings.Contains(noWeb, "search_web") {
		t.Error("the keyless prompt must NOT promise search_web")
	}
	// Factual-number grounding survives the minimal rewrite.
	if !strings.Contains(base, "never invent") {
		t.Error("the factual-number grounding rule must survive the rewrite")
	}
	// zh parity: the resourcefulness rule ships in zh too.
	if !strings.Contains(systemPrompt("AAPL", "zh", "facts", false, false, true, ""), "首要规则") {
		t.Error("zh prompt missing the resourcefulness rule (首要规则)")
	}
}

// TestAdviceShipsEveryMode locks that the mode dial never strips advice: a buy/sell view ships
// verbatim in every mode (mode is a depth/length dial only, it never reaches finish).
func TestAdviceShipsEveryMode(t *testing.T) {
	const adviceLine = "On balance the setup looks attractive here — a compelling entry, and I'd lean buy."
	for _, mode := range []string{"explore", "focused", ""} {
		llm := &scriptedLLM{enabled: true, replies: []reply{{content: adviceLine}}}
		svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
		ans, err := svc.Answer(context.Background(), "u", "AAPL", "en", nil, "is it a buy?", true, mode)
		if err != nil {
			t.Fatalf("mode %q: %v", mode, err)
		}
		if txt := textOf(ans); !strings.Contains(txt, "compelling entry") || !strings.Contains(txt, "lean buy") {
			t.Errorf("mode %q: advice must ship verbatim (depth-only dial): %q", mode, txt)
		}
	}
}

// TestAnswerStreamTrace asserts the streaming path PERSISTS the execution chain as a leading,
// display-only "trace" block (so reloaded history can show what the assistant did).
func TestAnswerStreamTrace(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{calls: []enrich.ChatToolCall{{ID: "c1", Name: "get_facts", Arguments: `{"section":"valuation"}`}}},
		{content: "P/E is 31.2x."},
	}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
	ans, err := svc.AnswerStream(context.Background(), "u", "AAPL", "en", nil, "valuation?", true, "", nil, nil)
	if err != nil {
		t.Fatalf("AnswerStream: %v", err)
	}
	if len(ans.Blocks) == 0 || ans.Blocks[0].Kind != "trace" || len(ans.Blocks[0].Steps) != 1 {
		t.Fatalf("want a leading trace block with 1 step, got %+v", ans.Blocks)
	}
	if ans.Blocks[0].Steps[0].Kind != "facts" {
		t.Errorf("trace step kind = %q, want facts", ans.Blocks[0].Steps[0].Kind)
	}
	// A direct (no-tool) answer has NO trace block.
	llm2 := &scriptedLLM{enabled: true, replies: []reply{{content: "hi"}}}
	svc2 := NewService(llm2, fakeFacts{sampleSheet()}, nil, "")
	ans2, _ := svc2.AnswerStream(context.Background(), "u", "AAPL", "en", nil, "hi", true, "", nil, nil)
	for _, b := range ans2.Blocks {
		if b.Kind == "trace" {
			t.Error("a no-tool answer must not carry a trace block")
		}
	}
}

// TestAnswerStreamInterleaved locks the persisted INTERLEAVE: when the model narrates BEFORE its
// tool call (a preamble), the stored blocks are [narration][trace][text] in order — so a reloaded
// turn shows the Claude-style interleave, not a trace collapsed at the top. The preamble is kind
// "narration" (display-only); only the final answer is "text".
func TestAnswerStreamInterleaved(t *testing.T) {
	llm := &scriptedLLM{enabled: true, replies: []reply{
		{content: "Let me pull that.", calls: []enrich.ChatToolCall{{ID: "c1", Name: "get_facts", Arguments: `{"section":"valuation"}`}}},
		{content: "P/E is 31.2x."},
	}}
	svc := NewService(llm, fakeFacts{sampleSheet()}, nil, "")
	ans, err := svc.AnswerStream(context.Background(), "u", "AAPL", "en", nil, "valuation?", true, "", nil, nil)
	if err != nil {
		t.Fatalf("AnswerStream: %v", err)
	}
	if len(ans.Blocks) < 3 || ans.Blocks[0].Kind != "narration" || ans.Blocks[1].Kind != "trace" || ans.Blocks[2].Kind != "text" {
		t.Fatalf("want [narration][trace][text] in order, got %+v", ans.Blocks)
	}
	if ans.Blocks[0].Text != "Let me pull that." {
		t.Errorf("narration text = %q, want the preamble", ans.Blocks[0].Text)
	}
	if ans.Blocks[2].Text != "P/E is 31.2x." {
		t.Errorf("answer text = %q, want the final answer", ans.Blocks[2].Text)
	}
}
