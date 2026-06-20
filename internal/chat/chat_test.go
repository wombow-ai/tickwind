package chat

import (
	"context"
	"strings"
	"testing"

	"github.com/wombow-ai/tickwind/internal/enrich"
	"github.com/wombow-ai/tickwind/internal/research"
)

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
