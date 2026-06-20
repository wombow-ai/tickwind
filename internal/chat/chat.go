// Package chat implements Product B — the personalized, ticker-scoped AI chat. A Pro
// user asks their OWN question; the model answers in prose while calling a CLOSED set of
// Go tools that (a) return pre-formatted, source-attributed facts and (b) surface preset
// widgets the frontend renders from the real store. The anti-hallucination contract is
// preserved exactly: the model never sees a raw number it could recompute, every number
// it may quote comes from a Go-formatted tool result, and a deterministic advice filter
// strips any investment-advice / price-target prose. This package is pure orchestration —
// the api layer owns the Pro gate, the per-user meter, persistence, and rate limiting.
package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/wombow-ai/tickwind/internal/enrich"
	"github.com/wombow-ai/tickwind/internal/research"
)

// maxToolIters bounds the tool-use loop per user turn (cost + latency backstop). After
// this many tool rounds the model is asked once more with NO tools, forcing a final
// prose answer.
const maxToolIters = 4

// widgetTypes is the CLOSED enum a surface_widget call may render. Any other value is
// rejected — the model cannot invent a widget, and a widget's numbers never enter its
// context (the tool returns only a "rendered: …" confirmation).
var widgetTypes = []string{
	"valuation_table", "fundamentals_table", "kline", "indicators",
	"flows_summary", "whales", "options", "insider",
}

// LLM is the narrow slice of enrich.Enricher this service needs (satisfied by the real
// enricher and by a fake in tests).
type LLM interface {
	Enabled() bool
	Chat(ctx context.Context, messages []enrich.ChatMessage, tools []enrich.ChatTool, model string) (string, []enrich.ChatToolCall, enrich.Usage, error)
}

// FactSource yields the Go-owned fact sheet for a ticker (the api's research service).
type FactSource interface {
	Report(ctx context.Context, ticker, lang string) research.FactSheet
}

// Service orchestrates one chat turn. model overrides the enricher's default chat model
// when non-empty (e.g. a Sonnet deep-dive turn); "" uses the configured chat default.
type Service struct {
	llm   LLM
	facts FactSource
	model string
}

// NewService builds a chat Service. model may be "" to use the enricher's chat default.
func NewService(llm LLM, facts FactSource, model string) *Service {
	return &Service{llm: llm, facts: facts, model: model}
}

// Enabled reports whether the underlying LLM is configured.
func (s *Service) Enabled() bool { return s.llm != nil && s.llm.Enabled() }

// Block is one ordered piece of an assistant answer: prose text or a rendered widget
// reference (the frontend draws the real widget from the store). Persisted as JSON.
type Block struct {
	Kind   string            `json:"kind"` // "text" | "widget"
	Text   string            `json:"text,omitempty"`
	Widget string            `json:"widget,omitempty"`
	Params map[string]string `json:"params,omitempty"`
}

// Answer is the assistant's reply: ordered blocks (prose then any surfaced widgets) plus
// the cumulative token usage for the turn (for cost telemetry).
type Answer struct {
	Blocks []Block      `json:"blocks"`
	Usage  enrich.Usage `json:"-"`
}

// ErrNotFound is returned when the ticker has no fact sheet (unknown / no data).
var ErrNotFound = errors.New("chat: no facts for ticker")

// Answer runs ONE user turn: it fetches the ticker's Go fact sheet, builds the firewall
// system prompt + per-ticker facts, threads the (already-windowed) history, runs the
// bounded tool loop, applies the deterministic advice post-filter, and returns the
// assistant's blocks + usage. history is prior turns as role/content (assistant prose
// only — no widget refs). It neither persists nor meters (the api owns that).
func (s *Service) Answer(ctx context.Context, ticker, lang string, history []enrich.ChatMessage, question string) (Answer, error) {
	if !s.Enabled() {
		return Answer{}, enrich.ErrDisabled
	}
	fs := s.facts.Report(ctx, ticker, lang)
	if len(fs.Sections) == 0 && fs.AsOf == "" {
		return Answer{}, ErrNotFound
	}
	material := research.Material(fs, lang)

	msgs := make([]enrich.ChatMessage, 0, len(history)+2)
	msgs = append(msgs, enrich.ChatMessage{Role: "system", Content: systemPrompt(ticker, lang, material)})
	msgs = append(msgs, history...)
	msgs = append(msgs, enrich.ChatMessage{Role: "user", Content: question})

	tools := toolSpecs(lang)
	var widgets []Block
	var total enrich.Usage

	for iter := 0; iter < maxToolIters; iter++ {
		content, calls, usage, err := s.llm.Chat(ctx, msgs, tools, s.model)
		addUsage(&total, usage)
		if err != nil {
			return Answer{}, err
		}
		if len(calls) == 0 {
			return s.finish(content, widgets, total, lang), nil
		}
		// Record the assistant's tool-call turn, then append each tool result.
		msgs = append(msgs, enrich.ChatMessage{Role: "assistant", Content: content, ToolCalls: calls})
		for _, c := range calls {
			msgs = append(msgs, enrich.ChatMessage{
				Role:       "tool",
				ToolCallID: c.ID,
				Content:    s.execTool(c, fs, lang, &widgets),
			})
		}
	}

	// Iteration cap hit: ask once more with NO tools so the model must answer in prose.
	content, _, usage, err := s.llm.Chat(ctx, msgs, nil, s.model)
	addUsage(&total, usage)
	if err != nil {
		return Answer{}, err
	}
	return s.finish(content, widgets, total, lang), nil
}

// finish assembles the final answer: the advice-filtered prose (or a redirect when the
// whole answer was stripped) followed by any widgets the model surfaced, in order.
func (s *Service) finish(prose string, widgets []Block, usage enrich.Usage, lang string) Answer {
	prose = filterAdvice(prose)
	if strings.TrimSpace(prose) == "" {
		prose = redirectNote(lang)
	}
	blocks := []Block{{Kind: "text", Text: prose}}
	blocks = append(blocks, widgets...)
	return Answer{Blocks: blocks, Usage: usage}
}

// execTool runs one closed tool against the Go fact sheet and returns its (string) result.
// surface_widget also records a widget block in widgets; its numbers never enter the
// model's context (the result is only a confirmation string).
func (s *Service) execTool(c enrich.ChatToolCall, fs research.FactSheet, lang string, widgets *[]Block) string {
	switch c.Name {
	case "get_facts":
		var args struct {
			Section string `json:"section"`
		}
		_ = json.Unmarshal([]byte(c.Arguments), &args)
		out := research.FactsForSection(fs, args.Section, lang)
		if out == "" {
			return "No such section. Valid sections: " + strings.Join(research.FactSectionKeys(), ", ")
		}
		return out
	case "surface_widget":
		var args struct {
			Type  string `json:"type"`
			Range string `json:"range"`
		}
		_ = json.Unmarshal([]byte(c.Arguments), &args)
		if !validWidget(args.Type) {
			return "Unknown widget type. Valid: " + strings.Join(widgetTypes, ", ")
		}
		params := map[string]string{}
		rng := normalizeRange(args.Range)
		if rng != "" {
			params["range"] = rng
		}
		*widgets = append(*widgets, Block{Kind: "widget", Widget: args.Type, Params: params})
		// Numbers NEVER enter the model context — only a confirmation.
		return "rendered: " + args.Type + " " + rng
	case "get_news_context":
		lines := research.NewsContext(fs)
		if len(lines) == 0 {
			return "No recent attributed news/community context."
		}
		return strings.Join(lines, "\n")
	default:
		return "Unknown tool."
	}
}

// validWidget reports whether t is in the closed widget enum.
func validWidget(t string) bool {
	for _, w := range widgetTypes {
		if w == t {
			return true
		}
	}
	return false
}

// normalizeRange clamps a chart range to the closed set {3M,1Y,5Y}; unknown → "1Y".
func normalizeRange(r string) string {
	switch strings.ToUpper(strings.TrimSpace(r)) {
	case "3M":
		return "3M"
	case "5Y":
		return "5Y"
	case "":
		return ""
	default:
		return "1Y"
	}
}

// filterAdvice drops any line that trips the deterministic advice / price-target guard
// (the same backstop the deep report runs over bull/bear points). A line-level filter
// keeps the rest of an otherwise-fine answer.
func filterAdvice(prose string) string {
	lines := strings.Split(prose, "\n")
	kept := lines[:0]
	for _, ln := range lines {
		if research.HasAdvice(ln) {
			continue
		}
		kept = append(kept, ln)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

// addUsage accumulates token usage across the tool loop.
func addUsage(dst *enrich.Usage, u enrich.Usage) {
	dst.PromptTokens += u.PromptTokens
	dst.CompletionTokens += u.CompletionTokens
	dst.TotalTokens += u.TotalTokens
	dst.CachedTokens += u.CachedTokens
}

// toolSpecs is the closed tool surface offered to the model (descriptions lang-aware).
func toolSpecs(lang string) []enrich.ChatTool {
	en := lang == "en"
	getFactsDesc := "返回本股票某板块经 Go 校验、带来源的事实(每个数字都有出处,你可以引用)。在陈述你尚未掌握的数字前先调用它。"
	widgetDesc := "在对话中内联渲染一个真实的 Tickwind 图表/表格(用户看到真实控件)。优先用它来\"展示\"数据,而不是罗列数字。你只会收到一个确认,不会拿到数据 —— 这是正常的。"
	newsDesc := "返回本股票近期带出处的新闻/社区背景(引用时注明来源,切勿当作事实或据此推导数字)。"
	if en {
		getFactsDesc = "Return Tickwind's Go-verified, source-attributed facts for one section of this stock (every number is sourced; you may quote these). Call it before stating any number you don't already have."
		widgetDesc = "Render a real Tickwind chart/table inline (the user sees the actual widget). PREFER showing a widget over reciting many numbers. You only get a confirmation back, not the data — that's expected."
		newsDesc = "Return recent attributed news/community context for this stock (quote with the source; never treat as fact or derive a number from it)."
	}
	return []enrich.ChatTool{
		{
			Name:        "get_facts",
			Description: getFactsDesc,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"section": map[string]any{
						"type":        "string",
						"enum":        research.FactSectionKeys(),
						"description": "Which section's facts to fetch.",
					},
				},
				"required": []string{"section"},
			},
		},
		{
			Name:        "surface_widget",
			Description: widgetDesc,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type": map[string]any{
						"type":        "string",
						"enum":        widgetTypes,
						"description": "Which preset widget to render.",
					},
					"range": map[string]any{
						"type":        "string",
						"enum":        []string{"3M", "1Y", "5Y"},
						"description": "Time range for chart widgets (kline/indicators).",
					},
				},
				"required": []string{"type"},
			},
		},
		{
			Name:        "get_news_context",
			Description: newsDesc,
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		},
	}
}

// redirectNote is shown when the advice filter stripped the entire answer (the model
// tried to give advice / a target). It states the no-advice stance plainly.
func redirectNote(lang string) string {
	if lang == "en" {
		return "Tickwind doesn't give price targets, fair-value estimates, or buy/sell advice. I can walk you through what the disclosed signals show — ask me about valuation, fundamentals, the technical picture, smart-money flows, or sentiment."
	}
	return "Tickwind 不提供目标价、估值结论或买卖建议。我可以带你看已披露信号说明了什么 —— 问我估值、基本面、技术面、资金面或情绪面都可以。"
}

// systemPrompt is the firewall: the absolute anti-hallucination + no-advice rules, the
// tool guide, and the per-ticker Go facts (so most questions need zero tool round-trips).
func systemPrompt(ticker, lang, material string) string {
	if lang == "en" {
		return "You are Tickwind's research assistant for " + ticker + ". You answer the user's questions about THIS stock only, grounded strictly in Tickwind's Go-verified facts.\n\n" +
			"ABSOLUTE RULES (never break):\n" +
			"1. NUMBERS: Every number, ratio, price, percentage, or date you state MUST come verbatim from the <facts> block below or from a get_facts tool result. NEVER invent, estimate, extrapolate, or compute a new number. If you don't have a figure, say so and offer to pull the relevant section — do not guess.\n" +
			"2. NO ADVICE: Never give investment advice, a price target, a fair-value estimate, or a buy/sell/hold recommendation. If asked (\"should I buy?\", \"what's it worth?\", \"price target?\"), refuse plainly and redirect to what the disclosed signals show. Stating an insider's buy or a congressional sale as a FACT is fine; recommending an action is not.\n" +
			"3. CONTEXT IS NOT FACT: News / community items (get_news_context) are attributed opinion/context — quote them WITH their source and never restate them as fact or derive a number from them.\n\n" +
			"TOOLS:\n" +
			"- get_facts(section): Tickwind's verified facts for one section (valuation, fundamentals, technical, flows, sentiment).\n" +
			"- surface_widget(type[, range]): render a real chart/table inline; prefer showing a widget over reciting many numbers.\n" +
			"- get_news_context(): recent attributed news/community context.\n\n" +
			"STYLE: concise, prose-first, plain language. The <facts> below often already answer the question — use it directly; reach for tools when they add value. Stay within what the data supports.\n\n" +
			"<facts>\n" + material + "\n</facts>"
	}
	return "你是 Tickwind 针对 " + ticker + " 的研究助手。你只回答关于这只股票的问题,且严格基于 Tickwind 经 Go 校验的事实。\n\n" +
		"绝对规则(不可违反):\n" +
		"1. 数字:你陈述的任何数字、比率、价格、百分比或日期,都必须逐字来自下方 <facts> 区块或某个 get_facts 工具结果。绝不臆造、估算、外推或自行计算新数字。没有某个数字就直说,并提出去取对应板块 —— 不要猜。\n" +
		"2. 不给建议:绝不给投资建议、目标价、估值结论或买入/卖出/持有建议。被问到(\"该买吗?\"\"值多少钱?\"\"目标价?\")时,明确拒绝,并转向已披露信号说明了什么。把内部人买入或国会议员卖出作为\"事实\"陈述可以;建议采取行动不行。\n" +
		"3. 背景不是事实:新闻/社区内容(get_news_context)是带出处的观点/背景 —— 引用时注明来源,切勿当作事实复述或据此推导数字。\n\n" +
		"工具:\n" +
		"- get_facts(section):某板块经校验的事实(估值 valuation、基本面 fundamentals、技术面 technical、资金面 flows、情绪面 sentiment)。\n" +
		"- surface_widget(type[, range]):内联渲染真实图表/表格;优先用控件展示,而非罗列数字。\n" +
		"- get_news_context():近期带出处的新闻/社区背景。\n\n" +
		"风格:简洁、以散文为主、平实。下方 <facts> 通常已能回答问题 —— 直接用;需要时再调工具。只说数据支持的内容。\n\n" +
		"<facts>\n" + material + "\n</facts>"
}
