// Package enrich is an optional, pluggable LLM enrichment layer (summaries,
// translation). It is disabled by default: when no LLM is configured a Noop is
// used and callers degrade gracefully. The real implementation speaks the
// OpenAI-compatible Chat Completions API, so it works with OpenAI, OpenRouter,
// or a local server — stdlib only, no SDK dependency.
package enrich

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ErrDisabled is returned by a disabled Enricher.
var ErrDisabled = errors.New("enrich: llm not configured")

// Enricher summarizes and translates text using an LLM.
type Enricher interface {
	// Enabled reports whether a real LLM backend is configured.
	Enabled() bool
	// Summarize returns a concise summary of text in the given language
	// ("zh"|"en"; anything else falls back to zh), or ErrDisabled when no LLM
	// is configured.
	Summarize(ctx context.Context, text, lang string) (string, error)
	// TranslateTitles translates English news headlines to Simplified Chinese,
	// preserving order (result[i] is the translation of titles[i]). Returns
	// ErrDisabled when no LLM is configured.
	TranslateTitles(ctx context.Context, titles []string) ([]string, error)
	// Brief writes the pre-market briefing from structured material (indices,
	// movers, earnings, smart money) in the given language ("zh"|"en"; anything
	// else falls back to zh). ErrDisabled when no LLM.
	Brief(ctx context.Context, material, lang string) (string, error)
	// ComposeReport writes per-section research prose from a pre-built material
	// string, returning a section-key→prose map (keys are the section keys present
	// in the material, e.g. "valuation"/"fundamentals"/"technical"). The prose is
	// qualitative only, in Simplified Chinese by default ("en" produces English):
	// material-only, never inventing or recomputing a number, no buy/sell/target/
	// valuation call, neutral, attributing source types, "数据不足" for a thin
	// section. Returns ErrDisabled when no LLM is configured.
	ComposeReport(ctx context.Context, material, lang string) (map[string]string, error)
	// ComposeDeepReport is the richer, Fable-5-harnessed sibling of ComposeReport:
	// it writes LONGER per-section research prose (a report, not a one-paragraph
	// digest) plus an executive "overview", over the SAME Go-owned facts, using a
	// (possibly stronger) model. The anti-hallucination contract is IDENTICAL: the
	// material carries only formatted strings, the model writes ONLY prose, and any
	// stray numeric key in the reply is ignored by the caller — a stronger model
	// writes richer prose, it never computes or asserts a number. Returns the same
	// section-key→prose map shape (and ErrDisabled when no LLM is configured).
	ComposeDeepReport(ctx context.Context, material, lang string) (map[string]string, error)
	// ExplainMove writes ONE short (1-2 sentence) hedged explanation of a notable
	// daily price move from a pre-built material string (the Go-computed move % +
	// direction + attributed evidence headlines). The move number and direction are
	// given in the material and MUST NOT be recomputed or altered; the model may
	// reference ONLY the supplied evidence headlines, must HEDGE ("可能与…有关"), must
	// NOT assert a definitive cause, invent a catalyst, or give any price target /
	// advice. Returns ErrDisabled when no LLM is configured.
	ExplainMove(ctx context.Context, material, lang string) (string, error)
	// SummarizeFiling writes a short (1-3 sentence) plain-language summary of an
	// 8-K material-event filing in the given language ("zh"|"en"; anything else
	// falls back to zh). The material is the Go-supplied source text of the filing
	// (the canonical item-code labels are owned by Go and given as context). The
	// model summarizes ONLY what the source text says happened: it must stay
	// factual, must NOT invent numbers/dates/names absent from the text, and must
	// NOT give any investment advice or price target. Returns ErrDisabled when no
	// LLM is configured (the caller then serves the filing with item labels only).
	SummarizeFiling(ctx context.Context, material, lang string) (string, error)
	// Chat runs ONE round-trip of the Product B personalized chat: it sends the message
	// history (system firewall + per-ticker Go facts + conversation + any tool results)
	// plus the closed tool surface to the (cheap) chat model, and returns the assistant's
	// prose content, any tool calls it requested, and token usage. It is a pure transport
	// — the CALLER owns the bounded tool loop, tool execution, the anti-hallucination
	// post-filter, persistence, and metering. model overrides the default chat model when
	// non-empty (e.g. a Sonnet deep-dive turn). Returns ErrDisabled when no LLM is set.
	Chat(ctx context.Context, messages []ChatMessage, tools []ChatTool, model string) (content string, toolCalls []ChatToolCall, usage Usage, err error)
}

// ChatMessage is one turn in a Product B conversation (OpenAI chat shape). Role is
// "system" | "user" | "assistant" | "tool". For an assistant turn that requested tools,
// ToolCalls is set (Content may be empty); for a tool result, Role=="tool", ToolCallID
// references the originating call, and Content is the Go-formatted result string.
type ChatMessage struct {
	Role       string
	Content    string
	ToolCallID string         // role=="tool": the id of the call this answers
	ToolCalls  []ChatToolCall // role=="assistant": the tool calls the model requested
}

// ChatToolCall is a function call the model requested. Arguments is the raw JSON string
// the model produced (the caller unmarshals it against the tool's schema).
type ChatToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// ChatTool is a function the model may call (OpenAI function-calling shape). Parameters
// is a JSON-Schema object describing the arguments; keep the surface tight + closed.
type ChatTool struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// Usage reports token accounting for a Chat round-trip, including prompt-cache reads
// (CachedTokens) so the caller can verify caching actually fires and track cost.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CachedTokens     int
}

// Config configures the LLM enricher. An empty APIKey yields a disabled Noop.
type Config struct {
	APIKey  string
	BaseURL string // OpenAI-compatible base; default https://api.openai.com/v1
	Model   string // default gpt-4o-mini
	// DeepModel is the (optionally stronger) model used ONLY by ComposeDeepReport.
	// When empty it falls back to Model, so the deep path costs/behaves exactly
	// like the normal compose until a stronger model id is configured (cost
	// control: the owner enables the pricier model via env when the paywall lands).
	DeepModel string
	// DeepBaseURL / DeepAPIKey route ComposeDeepReport to a SEPARATE provider from
	// the routine methods (cost-split: routine high-volume on the cheap default
	// provider, the premium deep report on another). Empty values fall back to
	// BaseURL / APIKey, so a single-provider setup is unchanged.
	DeepBaseURL string
	DeepAPIKey  string
	// ChatModel / ChatBaseURL / ChatAPIKey route the Product B personalized chat (the
	// Chat method) to its own provider/model — a CHEAP, high-frequency conversational
	// model (e.g. Claude Haiku) distinct from the flagship deep report. Each empty value
	// falls back to the DEEP equivalent (which itself falls back to the default), so the
	// chat path costs/behaves like the deep path until LLM_CHAT_MODEL is set.
	ChatModel   string
	ChatBaseURL string
	ChatAPIKey  string
}

// New returns a real Enricher when cfg.APIKey is set, otherwise a Noop.
func New(cfg Config) Enricher {
	if cfg.APIKey == "" {
		return Noop{}
	}
	base := cfg.BaseURL
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	model := cfg.Model
	if model == "" {
		model = "gpt-4o-mini"
	}
	// The deep-research compose uses a (possibly stronger) model; falling back to
	// the normal model keeps cost/behavior identical until LLM_DEEP_MODEL is set.
	deepModel := cfg.DeepModel
	if deepModel == "" {
		deepModel = model
	}
	// The deep compose may also use a SEPARATE provider (base URL + key); each
	// falls back to the default so a single-provider deployment is unchanged.
	deepBase := cfg.DeepBaseURL
	if deepBase == "" {
		deepBase = base
	}
	deepKey := cfg.DeepAPIKey
	if deepKey == "" {
		deepKey = cfg.APIKey
	}
	// The personalized chat (Product B) uses its own (cheap) model/provider; each value
	// falls back to the DEEP client (chat → deep → default), so it is unchanged until
	// LLM_CHAT_* is configured.
	chatModel := cfg.ChatModel
	if chatModel == "" {
		chatModel = deepModel
	}
	chatBase := cfg.ChatBaseURL
	if chatBase == "" {
		chatBase = deepBase
	}
	chatKey := cfg.ChatAPIKey
	if chatKey == "" {
		chatKey = deepKey
	}
	return &llm{
		// Generous ceiling so it never caps a legitimate call: the deep-research
		// compose (a premium Claude model, up to 6000 tokens) measured ~65s typical
		// and can approach ~110s at the ceiling. This MUST exceed the deep call's
		// context budget (api.llmDeepComposeTimeout=120s) so the per-call context
		// deadline — not this socket timeout — is the real bound. Routine calls stay
		// bounded tight by their own ~25s contexts, so the loose ceiling is harmless.
		http:        &http.Client{Timeout: 150 * time.Second},
		apiKey:      cfg.APIKey,
		baseURL:     strings.TrimRight(base, "/"),
		model:       model,
		deepModel:   deepModel,
		deepBaseURL: strings.TrimRight(deepBase, "/"),
		deepAPIKey:  deepKey,
		chatModel:   chatModel,
		chatBaseURL: strings.TrimRight(chatBase, "/"),
		chatAPIKey:  chatKey,
	}
}

// Noop is the disabled Enricher.
type Noop struct{}

func (Noop) Enabled() bool { return false }

func (Noop) Summarize(context.Context, string, string) (string, error) {
	return "", ErrDisabled
}

func (Noop) TranslateTitles(context.Context, []string) ([]string, error) {
	return nil, ErrDisabled
}

func (Noop) Brief(context.Context, string, string) (string, error) {
	return "", ErrDisabled
}

func (Noop) ComposeReport(context.Context, string, string) (map[string]string, error) {
	return nil, ErrDisabled
}

func (Noop) ComposeDeepReport(context.Context, string, string) (map[string]string, error) {
	return nil, ErrDisabled
}

func (Noop) ExplainMove(context.Context, string, string) (string, error) {
	return "", ErrDisabled
}

func (Noop) SummarizeFiling(context.Context, string, string) (string, error) {
	return "", ErrDisabled
}

func (Noop) Chat(context.Context, []ChatMessage, []ChatTool, string) (string, []ChatToolCall, Usage, error) {
	return "", nil, Usage{}, ErrDisabled
}

// systemPrompt drives the per-stock digest. Chinese-first product → Chinese
// output. Structural anti-hallucination guardrails: only restate the supplied
// material, attribute the source type, and never produce advice/targets (also
// enforced by the UI disclaimer).
const systemPrompt = "你是股票信息速览助手。仅基于用户提供的新闻标题与社区帖子,用简体中文输出 3-5 条要点,每条以\"- \"开头。" +
	"内容涵盖:发生了什么、讨论的焦点、市场情绪倾向。要求:只陈述材料中出现的信息,在要点中注明来源类型(如\"据新闻\"\"据社区讨论\");" +
	"不要编造数字、事件或因果;严禁任何买卖建议、目标价或估值判断;语气中性客观。材料不足时输出更少条目;完全无实质内容时只输出\"暂无足够信息\"。"

// systemPromptEN is the English-output counterpart, same guardrails. The product
// is Chinese-first, so zh is the default; en is served when the user's UI is in
// English.
const systemPromptEN = "You are a stock-info digest assistant. Based ONLY on the news headlines and community posts the user provides, output 3-5 bullet points in English, each starting with \"- \". " +
	"Cover: what happened, what the discussion focuses on, and the sentiment leaning. Requirements: state only information present in the material, and attribute the source type in each bullet (e.g. \"per news\", \"per community\"); " +
	"do not fabricate numbers, events or causation; absolutely no buy/sell advice, price targets or valuation calls; keep a neutral, objective tone. Output fewer bullets when material is thin; output only \"Not enough information yet\" when there is no substantive content."

// summarySystemPrompt picks the digest system prompt for a UI language.
func summarySystemPrompt(lang string) string {
	if lang == "en" {
		return systemPromptEN
	}
	return systemPrompt
}

type llm struct {
	http        *http.Client
	apiKey      string
	baseURL     string
	model       string
	deepModel   string // model for ComposeDeepReport (falls back to model)
	deepBaseURL string // base URL for ComposeDeepReport (falls back to baseURL)
	deepAPIKey  string // API key for ComposeDeepReport (falls back to apiKey)
	chatModel   string // default model for Chat / Product B (falls back to deepModel)
	chatBaseURL string // base URL for Chat (falls back to deepBaseURL)
	chatAPIKey  string // API key for Chat (falls back to deepAPIKey)
}

func (l *llm) Enabled() bool { return true }

func (l *llm) Summarize(ctx context.Context, text, lang string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model":       l.model,
		"temperature": 0.2,
		"messages": []map[string]string{
			{"role": "system", "content": summarySystemPrompt(lang)},
			{"role": "user", "content": text},
		},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+l.apiKey)

	resp, err := l.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("enrich: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("enrich: llm status %s", resp.Status)
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("enrich: decode: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", errors.New("enrich: empty llm response")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

// briefPrompt drives the daily pre-market briefing — one generation a day
// serves everyone. Same structural guardrails as the digest: material-only,
// no fabricated numbers, no advice.
const briefPrompt = "你是财经晨报编辑。仅基于用户提供的材料,写一篇 150-300 字的简体中文盘前简报," +
	"按【指数】【热点】【今日财报】【聪明钱】小节组织(材料缺某节就跳过该节)。" +
	"只引用材料中出现的数字与事实,不要编造;严禁任何买卖建议、目标价或预测;语气专业简洁。直接输出正文,不要前言。"

// briefPromptEN is the English-output counterpart (same guardrails). The
// material's section markers are Chinese (【指数】…) but carry numeric data;
// reorganize them under English headings (Indices / Movers / Earnings today /
// Smart money).
const briefPromptEN = "You are a financial pre-market briefing editor. Based ONLY on the material the user provides, write a 150-300 word English pre-market brief, " +
	"organized under the sections Indices / Movers / Earnings today / Smart money (skip a section when the material lacks it). " +
	"Cite only the numbers and facts present in the material; do not fabricate; absolutely no buy/sell advice, price targets or forecasts; professional, concise tone. Output the body directly with no preamble."

// briefSystemPrompt picks the briefing system prompt for a language.
func briefSystemPrompt(lang string) string {
	if lang == "en" {
		return briefPromptEN
	}
	return briefPrompt
}

// Brief writes the daily pre-market briefing from structured material, in the
// requested language ("zh"|"en").
func (l *llm) Brief(ctx context.Context, material, lang string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model":       l.model,
		"temperature": 0.3,
		"messages": []map[string]string{
			{"role": "system", "content": briefSystemPrompt(lang)},
			{"role": "user", "content": material},
		},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+l.apiKey)

	resp, err := l.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("enrich: brief request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("enrich: brief status %s", resp.Status)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("enrich: brief decode: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", errors.New("enrich: empty brief response")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

// explainMovePrompt drives the "why did this stock move today?" explainer — the
// LLM writes ONE short hedged sentence over the supplied evidence ONLY. The Go
// assembler owns the move % and direction (given in the material, NOT to be
// altered): the anti-hallucination contract. Hard guardrails: hedge, never assert
// a definitive cause, never invent a catalyst beyond the listed evidence, no
// price target / advice.
const explainMovePrompt = "你是股票异动解读助手。材料中已给出今日的涨跌幅与方向(由系统计算),以及若干条带来源类型的近期证据(新闻/公告/内部人交易)。" +
	"请用简体中文输出一句话(最多两句)的异动原因解读,必须遵守:" +
	"1)涨跌幅与方向只能照抄材料中的数字与方向,绝不改动、重算或编造;" +
	"2)只能引用材料中列出的证据条目,绝不编造材料以外的催化因素或事件,并在句中注明来源类型(如\"据新闻\"\"据公告\");" +
	"3)必须用推测性、对冲的措辞,如\"今日{涨/跌}X%,可能与[材料中的消息]有关\",绝不断言确定的因果关系;" +
	"4)严禁任何目标价、买卖建议或投资评级。" +
	"若材料中没有任何证据条目,则只输出\"今日{涨/跌}X%,暂无明确催化消息\"(用材料中的实际数字)。只输出这句解读,不要前言或解释。"

// explainMovePromptEN is the English-output counterpart, same guardrails. The
// product is Chinese-first (zh default); en is served when the UI is English.
const explainMovePromptEN = "You are a stock-move explainer. The material already gives today's percent change and direction (computed by the system) plus a few attributed recent evidence items (news / filings / insider trades). " +
	"Output ONE sentence (at most two) in English explaining the move, and you MUST: " +
	"1) copy the percent change and direction VERBATIM from the material — never alter, recompute or fabricate them; " +
	"2) reference ONLY the evidence items listed in the material — never invent a catalyst or event beyond them — and attribute the source type in the sentence (e.g. \"per news\", \"per filing\"); " +
	"3) use tentative, HEDGED wording such as \"Up/Down X% today, possibly related to [item from the material]\" — never assert a definitive cause; " +
	"4) absolutely no price target, buy/sell advice or rating. " +
	"If the material lists no evidence, output only \"Up/Down X% today; no clear catalyst\" (using the actual number from the material). Output only that sentence, no preamble."

// explainMoveSystemPrompt picks the move-explainer system prompt for a language.
func explainMoveSystemPrompt(lang string) string {
	if lang == "en" {
		return explainMovePromptEN
	}
	return explainMovePrompt
}

// ExplainMove writes one short, hedged explanation of a notable price move from
// the pre-built material string, in the requested language ("zh"|"en"). Cloned
// from Brief: the Go assembler owns the move number (it is in the material), the
// model only writes the hedged prose over the attributed evidence.
func (l *llm) ExplainMove(ctx context.Context, material, lang string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model":       l.model,
		"temperature": 0.2,
		"messages": []map[string]string{
			{"role": "system", "content": explainMoveSystemPrompt(lang)},
			{"role": "user", "content": material},
		},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+l.apiKey)

	resp, err := l.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("enrich: explain-move request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("enrich: explain-move status %s", resp.Status)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("enrich: explain-move decode: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", errors.New("enrich: empty explain-move response")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

// summarizeFilingPrompt drives the 8-K material-event summary — the LLM writes a
// short (1-3 sentence) plain-language summary of what the filing says happened.
// The Go assembler owns ALL the facts (form type, dates, accession URL, the
// parsed item codes AND their canonical labels — given as context); the model
// writes ONLY the prose summary over the supplied source text. Hard guardrails:
// summarize only what the source text states, never invent a number/date/name not
// in it, no investment advice / price target, and a factual neutral tone.
const summarizeFilingPrompt = "你是美股公司公告(SEC 8-K 重大事件报告)摘要助手。" +
	"材料中给出了该 8-K 的官方事项类别(由系统提供,不得改动)以及公告正文的纯文本节选。" +
	"请用简体中文写 1-3 句话的通俗摘要,说明这份公告实际披露了什么事情。必须遵守:" +
	"1)只根据材料中正文节选与给定事项类别陈述,绝不编造材料中没有的数字、日期、人名、金额或事件;" +
	"2)语气客观中性,不做任何买卖建议、目标价、估值判断或后市预测;" +
	"3)若正文节选信息过于稀薄、无法据此写出可靠摘要,只输出\"暂无足够信息\"。" +
	"只输出这段摘要本身,不要前言、不要罗列事项代码、不要代码块。"

// summarizeFilingPromptEN is the English-output counterpart, same guardrails. The
// product is Chinese-first (zh default); en is served when the UI is English.
const summarizeFilingPromptEN = "You are an SEC 8-K material-event filing summarizer. " +
	"The material gives the filing's official item categories (system-provided, not to be altered) and a plain-text excerpt of the filing body. " +
	"Write a 1-3 sentence plain-language summary in English of what the filing actually discloses. You MUST: " +
	"1) state only what the body excerpt and given item categories support — never invent a number, date, name, amount or event absent from the material; " +
	"2) keep a neutral, factual tone — no buy/sell advice, price target, valuation call or forecast; " +
	"3) output only \"Not enough information\" when the excerpt is too thin to summarize reliably. " +
	"Output only the summary itself — no preamble, no listing of item codes, no code fences."

// summarizeFilingSystemPrompt picks the filing-summary system prompt for a language.
func summarizeFilingSystemPrompt(lang string) string {
	if lang == "en" {
		return summarizeFilingPromptEN
	}
	return summarizeFilingPrompt
}

// SummarizeFiling writes a short plain-language summary of an 8-K filing from the
// pre-built material string (item-category context + body excerpt), in the
// requested language ("zh"|"en"). Cloned from ExplainMove: the Go assembler owns
// every fact (form/dates/URL/item labels), the model only writes the prose over
// the supplied source text.
func (l *llm) SummarizeFiling(ctx context.Context, material, lang string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model":       l.model,
		"temperature": 0.2,
		"messages": []map[string]string{
			{"role": "system", "content": summarizeFilingSystemPrompt(lang)},
			{"role": "user", "content": material},
		},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+l.apiKey)

	resp, err := l.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("enrich: summarize-filing request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("enrich: summarize-filing status %s", resp.Status)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("enrich: summarize-filing decode: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", errors.New("enrich: empty summarize-filing response")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

// composePrompt drives the per-stock research report — the LLM writes ONLY
// qualitative prose, never a number. The Go assembler owns every figure (the
// anti-hallucination contract): the model receives a section-keyed material
// string and returns a JSON object whose keys are the section keys present in
// the material and whose values are the prose for that section.
const composePrompt = "你是股票研报撰稿助手。用户提供按板块组织的结构化材料(每个板块带数字事实)。" +
	"只返回一个 JSON 对象,键为材料中出现的板块标识(如 valuation/fundamentals/technical/flows/sentiment),值为对应板块的简体中文定性分析文字。" +
	"严格要求:只依据材料中已给出的数字与事实撰写,绝不自行计算、推断或编造任何数字;不要在文字里重复罗列原始数字,而是做定性解读(偏高/偏低、增长/下滑、超买/超卖、资金流入/流出、关注上升/下降等)。" +
	"资金面(flows):描述国会披露、机构13F、内部人买入、期权、做空等信号方向是否一致;金额区间按材料原样引用,绝不折算成具体数字;13F为季度滞后数据,需注明披露季度。" +
	"情绪面(sentiment):新闻与社区内容(标有\"背景材料/据新闻/据社区讨论\")仅作有出处的引用,绝不当作事实陈述,更不得据此编造任何情绪分值;若有市场恐惧贪婪指数仅作大盘背景。" +
	"严禁任何买卖建议、目标价、估值结论或预测,严禁\"跟着某议员买\"之类表述;注明信息来源类型;语气中性客观。某板块材料过于单薄时,该板块的值写\"数据不足\"。" +
	"此外必须额外输出一个 \"overview\" 键:综合以上所有板块写 3-5 句中文总览,优势与风险两面均衡,结尾用一句\"以上为基于公开数据的客观梳理,非投资建议\";同样绝不编造数字、不给目标价、不给买卖建议。" +
	"还必须输出 \"bull\" 与 \"bear\" 两个键:bull 写 2-4 条\"看多视角\"、bear 写 2-4 条\"看空视角\",每条独占一行(用换行 \\n 分隔),每条都须基于材料中已给出的事实做一句话定性陈述,正反两面都要言之有据、力求均衡;这是对同一组事实的多空双向解读,绝不是推荐——同样绝不编造数字、不给目标价、不给买卖建议或评级。" +
	"只输出该 JSON 对象,不要解释或代码块。"

// composePromptEN is the English-output counterpart, same guardrails. The
// product is Chinese-first (zh default); en is served when the UI is English.
const composePromptEN = "You are a stock research-report writer. The user provides structured material organized by section (each section carries numeric facts). " +
	"Return ONLY a JSON object whose keys are the section ids present in the material (e.g. valuation/fundamentals/technical/flows/sentiment) and whose values are qualitative English analysis prose for that section. " +
	"Strict requirements: write only from the numbers and facts already given in the material; never compute, infer or fabricate any number, and do not merely re-list the raw numbers — give a qualitative read (high/low, growing/shrinking, overbought/oversold, inflow/outflow, attention rising/falling, etc.). " +
	"flows: describe whether the congressional, 13F, insider-buy, options and short signals point the same or opposite directions; quote any disclosed amount range VERBATIM, never converting it to a point figure; 13F is quarter-lagged data — note the disclosed quarter. " +
	"sentiment: news and community items (marked \"attributed context / per news / per community discussion\") may ONLY be quoted with attribution, never restated as fact, and you must NOT derive any sentiment number from them; a market Fear & Greed reading, if present, is broad-market backdrop only. " +
	"Absolutely no buy/sell advice, price targets, valuation calls or forecasts, and no \"follow member X's trade\" framing; attribute the source type; keep a neutral, objective tone. When a section's material is too thin, set that section's value to \"Not enough data\". " +
	"Additionally you MUST output an \"overview\" key: a 3-5 sentence balanced synthesis across all sections (both strengths and risks), ending with one line \"The above is an objective summary of public data, not investment advice\"; likewise never fabricate a number, give a price target, or give buy/sell advice. " +
	"You MUST also output \"bull\" and \"bear\" keys: bull lists 2-4 \"bull-case points\", bear lists 2-4 \"bear-case points\", one point per line (separated by \\n newlines), each a single-sentence qualitative read grounded in the facts given in the material, with both sides substantiated and balanced; this is a two-sided reading of the SAME facts, NOT a recommendation — likewise never fabricate a number, give a price target, or give buy/sell advice or a rating. " +
	"Output only the JSON object, no explanation or code fences."

// composeSystemPrompt picks the report-composition system prompt for a language.
func composeSystemPrompt(lang string) string {
	if lang == "en" {
		return composePromptEN
	}
	return composePrompt
}

// ComposeReport writes per-section research prose from a pre-built material
// string and returns the section-key→prose map. Cloned from Brief, but asks for
// a JSON object via response_format (the TranslateTitles idiom) so one call
// fills every section; the reply is parsed tolerant of code fences.
func (l *llm) ComposeReport(ctx context.Context, material, lang string) (map[string]string, error) {
	body, err := json.Marshal(map[string]any{
		"model":           l.model,
		"temperature":     0.3,
		"response_format": map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "system", "content": composeSystemPrompt(lang)},
			{"role": "user", "content": material},
		},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+l.apiKey)

	resp, err := l.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("enrich: compose request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("enrich: compose status %s", resp.Status)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("enrich: compose decode: %w", err)
	}
	if len(out.Choices) == 0 {
		return nil, errors.New("enrich: empty compose response")
	}
	return parseSectionProse(out.Choices[0].Message.Content)
}

// composeDeepMaxTokens is the longer output ceiling for the deep research report
// — a multi-section report with an executive overview needs more room than the
// one-paragraph digest. Generous, but bounded so a runaway reply can't blow the
// token budget.
const composeDeepMaxTokens = 6000

// composeDeepPrompt is the system prompt for the AI Deep Research report (zh),
// refined 2026-06-19 applying prompt-engineering craft distilled from the leaked
// Claude Fable 5 system prompt + Anthropic best practice (prose-first / minimal
// formatting, a ~15-char one-per-source citation rule, a given-facts-vs-inference
// firewall, an in-prompt self-check) — authored via a multi-candidate workflow and
// adversarially reviewed for the UNBREAKABLE contract. It opens with a three-rule
// preamble (numbers belong to the material; no advice/price-direction ever; news &
// community are attributed context only), then hierarchical concern sections, then
// a strict output contract the Go parser consumes (single ```json fence; keys =
// overview + present-section bare-lowercase tokens + bull/bear; overview ends with
// the fixed disclaimer). Go still owns every number; the model writes ONLY prose.
const composeDeepPrompt = "你是一位严谨、中立的股票研究撰稿人,正在为「Tickwind」深度研究功能撰写一份面向中文读者的美股深度定性报告。你收到的全部事实,都已由系统的 Go 代码预先计算、预先格式化为一段按板块组织的「材料」字符串(板块可能包括 valuation 估值、fundamentals 基本面、technical 技术面、flows 资金面、sentiment 情绪面),且每项都标注了来源类型。你的唯一职责,是基于这些既定事实,用克制、冷静、专业的中文散文写出有深度的定性解读——读起来应像一份一流人类分析师的研究备忘录。你绝不产生任何新的数字。\n\n请先理解三条不可动摇的铁律,它们高于以下一切其它要求。\n\n第一,数字只属于材料。系统已算好并格式化了每一个数字、比率、市值、区间、价格与分析师一致预期;材料中给出的事实,是唯一存在的事实。你绝不发明、计算、内插、估算、折算,也绝不以「换一种说法」把任何数字、比率、一致预期或价格重述为新事实。不得做任何加减乘除,不得换算单位,不得把同比/环比反推成绝对值,不得把几个给定数字合成出材料里没有的新数字,也绝不在散文里重算或改动材料里已有的任何数字——原样转述即可。已披露的区间(如某笔期权交易「1,001-15,000 美元」)必须原样引用,绝不折算成单一点位数值而制造虚假精度。\n\n第二,绝无投资建议。这是客观梳理,不是荐股:任何时候都不得给出买入/卖出/增持/减持意见、目标价、估值结论、价格预测或评级,也不得出现「值得买」「应回避」之类的措辞或暗示。同样不得使用前瞻性的价格方向措辞,如「上行/下行空间」「仍有上涨/回调空间」「估值偏贵故应卖出」等——这些都属于变相的方向判断。你只描述与解读「已披露信号」本身的方向与含义(信号偏强还是偏弱、彼此印证还是背离),从不预判股价走向,也从不告诉读者该如何行动。(Go 有确定性兜底,会丢弃任何含「目标价」「强烈买入」「建议买」等字样的多空要点——所以请让每条要点保持纯描述性。)\n\n第三,新闻与社区内容只能作为带出处的背景。材料中标注为背景的新闻、社区讨论、市场「恐惧与贪婪」读数等,只能以「据新闻」「据社区讨论」这样带出处的方式引用,绝不当作既成事实陈述,更不得据此推导出任何情绪数值或其它数字。市场「恐惧与贪婪」读数仅代表大盘整体氛围,与个股无关。\n\n<既定事实与推断的分离>\n这是本提示词的灵魂。请在心里、也在措辞上,把两类内容严格分开:\n- 既定事实:材料里逐字给出的内容。你可以陈述它,并解读它的方向与含义。\n- 你的推断:你基于既定事实得出的解读。每一处推断都必须用情态/软化语气标注,让读者一眼就能区分「数据说了什么」与「你认为这意味着什么」。\n推断只能建立在既定事实之上,绝不能引入材料之外的任何数字或论断。一句话里若同时含事实与推断,请让事实部分可追溯到材料,推断部分带上「或显示」「可能」「往往意味着」这类标记。任何无法追溯到某个给定事实的量化说法,都不得出现。\n</既定事实与推断的分离>\n\n<深度与跨板块综合>\n你的价值在于解读,而非把事实重新罗列一遍——要比一段式速览更充分、更具洞察,但始终是对既定事实的解读。对每个板块:解读其信号的方向与含义(用顺风、逆风、结构性、周期性等中性描述词);指出板块内部与板块之间是相互印证还是彼此背离,并把其中的张力与不确定性摆到台面上。\n</深度与跨板块综合>\n\n<散文优先>\n以流畅的分析性散文写作,自然成段,适合中文读者通顺阅读。避免要点符号、小标题、编号列表与表格;唯有当一处简短的结构化对比(如几个跨期或同业数字的并列)能真正提升清晰度时方可例外,且应极为罕见,并尽量用文字描述而非堆砌表格符号;即便如此,对比中的每一个数字也必须原样取自材料、逐字呈现,绝不因并列而推算、合成或反推出材料里没有的新数字。不做任何无谓的格式化。让论证在句子中自然推进,如同人写就的研究正文。\n</散文优先>\n\n<引用纪律>\n每一处论断都要系于材料中的某个给定事实,并标明其来源类型(如「据估值数据」「据基本面数据」「据 13F 披露」「据 SEC Form 4」「据新闻」「据社区讨论」)。优先转述,少直接引用;如确需直接引用,任何一处不超过约 15 个字,且每个来源至多一处直接引用。绝不引用材料里没有的任何内容。当某事实缺失时,明言「数据不足」或「未披露」,绝不猜测、不插值、不反推、不估算。\n</引用纪律>\n\n<对缺失与不足数据的处理>\n- 板块缺失:材料里没有出现的板块,绝不为它编造一个键,也绝不补写其内容。\n- 单点缺失:板块在但某项数据缺失时,在相关句子里点明「该项未披露/数据不足」,不要绕过,也不要用邻近数字顶替。\n- 不要因为「数据不足」就沉默带过——指出这一缺口本身就是有价值的解读,但解读到此为止,绝不向前猜测。\n</对缺失与不足数据的处理>\n\n<资金面板块>\n在 flows 板块,专门说明国会交易、机构 13F、内部人买卖、期权、做空等信号彼此指向「同向」还是「相反」;金额区间按材料原样引用。13F 数据滞后一个季度——务必标明材料中给出的披露季度。\n</资金面板块>\n\n<对冲与语气>\n保持冷静、中立、分析性的语气。使用情态化的对冲语言(数据显示/或暗示/可能/往往);绝不使用「将会」「必然」「一定会」这类确定性断言。始终把「已披露的事实」与「你的推断」分开陈述。使用中性描述词(逆风、顺风、结构性),不用任何营销式或煽动性的措辞;且「顺风/逆风/结构性/周期性」描述的是所披露的业务或信号状况本身,绝非对该股票的买卖判断或股价方向暗示。\n</对冲与语气>\n\n<定稿前自检>\n输出前在心里逐条过一遍,任何一条不过就改到过为止:\n1. 文中每一个数字、比率、价格、一致预期,是否都逐字出现在材料里(而非由材料中的数字算出、折算、合成或反推而来)?有没有任何一个是我自己算出来、凑出来或推出来的?\n2. 是否完全没有投资建议、目标价、买卖/评级措辞或暗示?\n3. 每条推断是否都带了软化/情态标注,且只建立在既定事实之上?\n4. 缺失数据是否都明说「数据不足/未披露」,而非猜测或顶替?\n5. 区间是否原样引用、未被折算?新闻/社区是否仅作带出处的背景、未被改写为事实、未被用来推导数字?\n6. 是否只为材料里实际出现的板块生成了键,没有凭空多出一个键?\n7. overview、bull、bear 是否齐备?overview 的最后一句是否正是那句固定免责声明?\n</定稿前自检>\n\n接下来说明输出要求,Go 解析器会逐字消费你的回复,必须完全吻合,请严格遵守。\n\n你只输出一个 JSON 对象,用单个 ```json 围栏代码块包裹,围栏之外不得有任何字符、解释或思考过程。它的键恰好由以下三部分构成,缺一不可,也不得多出:\n\n其一,overview 键:综合全部板块写一段 3 到 6 句的中文执行级综述,优势与风险两面均衡,语气中立克制;这一段的最后一句,必须原样固定为:以上为基于公开数据的客观梳理,非投资建议\n\n其二,为材料中实际出现的每一个板块各输出一个键。判定某板块「出现」的标准:它的标识(valuation、fundamentals、technical、flows、sentiment 之一)在材料里作为一个板块出现过。键名必须就是这个小写英文标识本身(例如 valuation),绝不可用「估值」这类中文板块名作键。每个值是该板块的简体中文深度定性分析,且必须是非空的实质散文——即便该板块数据稀薄,也要写出对所给内容的解读并点明缺口,绝不留空字符串。材料里没有出现的板块,绝不杜撰对应的键。\n\n其三,bull 键与 bear 键:bull 写 2 到 4 条看多视角,bear 写 2 到 4 条看空视角;每条独占一行,在同一个字符串值内用真正的换行把多条分隔开(不要写出反斜杠 n 之类的字面记号)。每一条都必须是扎根于某个给定事实的一句话定性陈述,正反两面都言之有据、彼此均衡——这是对同一组事实的双向解读,绝不是推荐,既不预判股价方向,也绝不编造数字、不给目标价、不给买卖建议或评级。\n\n每个值都是一个简体中文字符串。再次强调:不要为材料中缺席的板块发明键,也不要遗漏 overview、bull、bear 三者中的任何一个。"

// composeDeepPromptEN is the English-output counterpart of composeDeepPrompt — the
// same refined harness, three-rule preamble, given-facts-vs-inference firewall, and
// strict output contract, in English for the en UI.
const composeDeepPromptEN = "You are a rigorous, neutral equity research writer for Tickwind's Deep Research feature, composing the qualitative narrative of a deep US-stock report for a Chinese-first product. Every fact you receive has been pre-computed and pre-formatted by the system's Go code into one section-organized material string (sections may include valuation, fundamentals, technical, flows, sentiment), each item tagged with its source type. Your sole job is to interpret those given facts in depth, in restrained, detached, professional prose that reads like a first-rate human analyst note. You never produce a new number of any kind.\n\nThree unbreakable rules come first and override everything else below.\n\nFirst, the numbers belong to the material alone. The system has already computed and formatted every number, ratio, market cap, range, price, and analyst consensus; the facts given in the material are the only facts that exist. You never invent, compute, interpolate, estimate, convert, or restate-as-new any number, ratio, consensus, or price. Do no arithmetic, convert no units, never back-calculate an absolute value from a year-over-year or quarter-over-quarter figure, never synthesize a new number the material lacks out of several given ones, and never recompute or alter any number already in the material — carry it across verbatim. A disclosed range (for example an options trade of 1,001 to 15,000 dollars) is quoted exactly as given and never collapsed into a single point figure that would manufacture false precision.\n\nSecond, no investment advice, ever. This is an objective summary, not a stock pick: no buy or sell or add or trim view, no price target, no valuation verdict, no price forecast, and no rating, at any time, nor wording or hints like worth buying or should avoid. Likewise use no forward price-direction framing such as upside/downside, room to run, more downside ahead, or expensive-so-sell — these are veiled directional calls. You describe and interpret the direction and meaning of the DISCLOSED SIGNAL itself (whether a signal reads strong or weak, corroborating or diverging); you never predict where the share price will go, and you never tell the reader what to do. (Go has a deterministic backstop that drops any bull or bear point containing target-price, strong-buy, or recommend-buy language — so keep every point purely descriptive.)\n\nThird, news and community items are attributed context only. Anything the material tags as context (news, community discussion, a market Fear and Greed reading) may be referenced only with attribution (per news, per community discussion), is never restated as established fact, and is never used to derive a sentiment number or any other figure. A market Fear and Greed reading is broad-market backdrop only and says nothing about this specific stock.\n\n<given_facts_vs_inference>\nThis is the soul of this prompt. Hold two categories strictly apart, in your mind and in your wording:\n- Given facts: what the material states verbatim. You may state it and interpret its direction and meaning.\n- Your inferences: the reading you derive from given facts. Every inference must be marked with modal or hedged language, so the reader can tell at a glance what the data says from what you think it means.\nAn inference may rest only on given facts; it may never introduce a number or claim from outside the material. If a sentence carries both a fact and an inference, keep the fact traceable to the material and tag the inference with phrases like the data suggests, may, or often points to. Any quantitative statement that cannot be traced to a specific given fact must not appear.\n</given_facts_vs_inference>\n\n<depth_and_cross_section_synthesis>\nYour value is interpretation, not a re-listing of the facts — go meaningfully deeper than a one-paragraph digest, yet stay always within interpretation of the given facts. For each section, interpret the direction and meaning of its signals (neutral descriptors: headwind, tailwind, secular, cyclical); say whether signals corroborate or diverge, within a section and across sections; and surface the tensions and uncertainties between them.\n</depth_and_cross_section_synthesis>\n\n<prose_first>\nWrite in flowing analytical prose, in natural paragraphs that read smoothly. Avoid bullet points, headers, numbered lists, and tables; permit a brief structured comparison (a few multi-period or peer figures set side by side) only when it genuinely sharpens clarity, let that be rare, and describe it in words rather than piling up table markup; even then, every figure in the comparison must be taken verbatim from the material — never derive, synthesize, or back-calculate a new number just because figures sit side by side. No gratuitous formatting. Let the argument advance through sentences, the way a human-written research body reads.\n</prose_first>\n\n<citation_discipline>\nTie every claim to a specific given fact and attribute its source type (per valuation data, per fundamentals data, per 13F filings, per SEC Form 4, per news, per community discussion). Prefer paraphrase over direct quotation; when a direct quote is truly needed, keep it under roughly 15 words and use at most one direct quote per source. Never cite anything not present in the material. When a fact is absent, say so plainly (data insufficient / not disclosed) and never guess, interpolate, back-calculate, or estimate.\n</citation_discipline>\n\n<handling_absent_or_insufficient_data>\n- Missing section: never invent a key for a section absent from the material, and never write content for it.\n- Missing data point: when a section is present but a figure is absent, name the gap in the relevant sentence (this item is not disclosed / data insufficient); do not route around it and do not substitute a nearby number.\n- Do not stay silent on a gap — naming the gap is itself a valuable reading — but the reading stops there; do not guess forward.\n</handling_absent_or_insufficient_data>\n\n<flows_section>\nIn the flows section, state whether the congressional, 13F, insider, options, and short-interest signals point the SAME way or OPPOSITE ways; quote any disclosed amount range verbatim. The 13F data is quarter-lagged, so always note the disclosed quarter given in the material.\n</flows_section>\n\n<hedging_and_tone>\nKeep the tone detached, neutral, and analytical throughout. Use modal, hedged language (the data shows, may suggest, appears to, often points to); never assert will or must as certainties. Always state disclosed facts and the inferences you draw from them as separate things. Use neutral descriptors (headwinds, tailwinds, secular, cyclical); reach for no marketing or promotional language. These descriptors characterize the disclosed business or signal condition itself, never a buy/sell judgment on the stock or a hint about where its price is headed.\n</hedging_and_tone>\n\n<self_check>\nRun through every item silently before finalizing; fix until all pass:\n1. Does every figure, ratio, price, and consensus in the text appear VERBATIM in the material (rather than being computed, converted, synthesized, or back-calculated from the material's numbers)? Is there any that I worked out, combined, or inferred myself?\n2. Is there zero investment advice, price target, valuation verdict, or buy/sell/rating wording or hint?\n3. Is every inference hedged and grounded only in given facts?\n4. Is every absent fact stated as data insufficient / not disclosed, rather than guessed or substituted?\n5. Are ranges quoted verbatim, not converted? Are news and community items kept as attributed context only — not restated as fact, not used to derive a number?\n6. Did I create a key only for sections actually present in the material, with no key conjured for an absent section?\n7. Are overview, bull, and bear all present, and is the overview's last sentence exactly the fixed disclaimer?\n</self_check>\n\nNow the output requirements. A Go parser consumes your reply verbatim, so it must match exactly; follow them precisely.\n\nYou output ONE JSON object wrapped in a SINGLE ```json fenced code block, with nothing — no text, explanation, or reasoning — outside the fence. Its keys consist of exactly the following three parts, with none missing and none added:\n\nOne, an overview key: a 3-to-6-sentence executive synthesis across all sections, balancing strengths and risks in a detached, neutral tone; its final sentence must be exactly, word for word: The above is an objective summary of public data, not investment advice\n\nTwo, one key for each section actually present in the material. A section counts as present when its identifier (one of valuation, fundamentals, technical, flows, sentiment) appears as a section in the material. The key must be that bare lowercase English token exactly (for example valuation) — never a translated or display name. Its value is the in-depth qualitative English analysis for that section, and must be non-empty substantive prose even when the section's data is sparse (interpret what is given and name the gap; never emit an empty string). Never invent a key for a section absent from the material.\n\nThree, a bull key and a bear key: bull gives 2 to 4 bull-case points, bear gives 2 to 4 bear-case points; put each point on its own line within the same string value, separated by a real line break (do not write a literal backslash-n token). Each point is a single-sentence qualitative read grounded in a given fact, both sides substantiated and balanced — a two-sided reading of the SAME facts, not a recommendation; it predicts no price direction and likewise never fabricates a number, gives a price target, or gives buy or sell advice or a rating.\n\nEach value is an English string. Again: do not invent a key for a section absent from the material, and do not omit any of overview, bull, or bear."

// composeDeepSystemPrompt picks the deep-report system prompt for a language.
func composeDeepSystemPrompt(lang string) string {
	if lang == "en" {
		return composeDeepPromptEN
	}
	return composeDeepPrompt
}

// ComposeDeepReport writes richer per-section research prose (plus an executive
// overview) from the pre-built material string, returning the section-key→prose
// map. It is the deep sibling of ComposeReport with the IDENTICAL anti-hallucination
// contract — prose-only output, stray numeric keys ignored by the caller — but
// differs in: (1) the stronger deepModel; (2) a possibly SEPARATE provider
// (deepBaseURL/deepAPIKey — the cost-split: routine on the cheap default provider,
// the flagship deep report on a premium one); (3) a larger token budget; (4) the
// Claude-idiomatic hierarchical system prompt; and (5) NO response_format — Anthropic
// (and reasoning models) reject OpenAI's json_object, so the prompt asks for a single
// fenced ```json block and the hardened parser (parseSectionProse) recovers it.
// Returns ErrDisabled via Noop when no LLM is configured.
func (l *llm) ComposeDeepReport(ctx context.Context, material, lang string) (map[string]string, error) {
	body, err := json.Marshal(map[string]any{
		"model":       l.deepModel,
		"temperature": 0.3,
		"max_tokens":  composeDeepMaxTokens,
		"messages": []map[string]string{
			{"role": "system", "content": composeDeepSystemPrompt(lang)},
			{"role": "user", "content": material},
		},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.deepBaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+l.deepAPIKey)

	resp, err := l.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("enrich: compose-deep request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("enrich: compose-deep status %s", resp.Status)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("enrich: compose-deep decode: %w", err)
	}
	if len(out.Choices) == 0 {
		return nil, errors.New("enrich: empty compose-deep response")
	}
	return parseSectionProse(out.Choices[0].Message.Content)
}

// chatMaxTokens caps a single conversational turn. A grounded answer over pre-formatted
// facts is short; this bounds cost and latency (the meter + per-user cap bound the rest).
const chatMaxTokens = 1500

// Chat — see the interface doc. One round-trip to the chat client (chatBaseURL/chatAPIKey),
// default chatModel unless model overrides. Sends messages (already including the system
// firewall + per-ticker facts) and the closed tool surface; returns assistant content,
// requested tool calls, and usage. NO response_format (Anthropic/tool-calling reject the
// OpenAI json_object), low temperature for grounded answers.
func (l *llm) Chat(ctx context.Context, messages []ChatMessage, tools []ChatTool, model string) (string, []ChatToolCall, Usage, error) {
	chosen := model
	if chosen == "" {
		chosen = l.chatModel
	}

	msgs := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		mm := map[string]any{"role": m.Role, "content": m.Content}
		if m.Role == "tool" {
			mm["tool_call_id"] = m.ToolCallID
		}
		if len(m.ToolCalls) > 0 {
			tc := make([]map[string]any, len(m.ToolCalls))
			for i, c := range m.ToolCalls {
				tc[i] = map[string]any{
					"id":   c.ID,
					"type": "function",
					"function": map[string]any{
						"name":      c.Name,
						"arguments": c.Arguments,
					},
				}
			}
			mm["tool_calls"] = tc
		}
		msgs = append(msgs, mm)
	}

	reqBody := map[string]any{
		"model":       chosen,
		"temperature": 0.2,
		"max_tokens":  chatMaxTokens,
		"messages":    msgs,
	}
	if len(tools) > 0 {
		specs := make([]map[string]any, len(tools))
		for i, t := range tools {
			specs[i] = map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.Parameters,
				},
			}
		}
		reqBody["tools"] = specs
		reqBody["tool_choice"] = "auto"
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", nil, Usage{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.chatBaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", nil, Usage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+l.chatAPIKey)

	resp, err := l.http.Do(req)
	if err != nil {
		return "", nil, Usage{}, fmt.Errorf("enrich: chat request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, Usage{}, fmt.Errorf("enrich: chat status %s", resp.Status)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			TotalTokens         int `json:"total_tokens"`
			PromptTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", nil, Usage{}, fmt.Errorf("enrich: chat decode: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", nil, Usage{}, errors.New("enrich: empty chat response")
	}
	usage := Usage{
		PromptTokens:     out.Usage.PromptTokens,
		CompletionTokens: out.Usage.CompletionTokens,
		TotalTokens:      out.Usage.TotalTokens,
		CachedTokens:     out.Usage.PromptTokensDetails.CachedTokens,
	}
	msg := out.Choices[0].Message
	var calls []ChatToolCall
	for _, c := range msg.ToolCalls {
		calls = append(calls, ChatToolCall{ID: c.ID, Name: c.Function.Name, Arguments: c.Function.Arguments})
	}
	return msg.Content, calls, usage, nil
}

// parseSectionProse maps the model's section-keyed JSON reply back onto a
// map[string]string. It is hardened for the deep path's stronger / reasoning
// models: it strips a leading <think>…</think> reasoning block (DeepSeek-R1 et al.)
// and any Markdown code fence, then parses the object; failing that it scans for
// balanced top-level {…} spans and tries them from LAST to first (a reasoning
// model's final answer object follows its scratch work). A value that arrives as a
// JSON array of strings (a model returning bull/bear as a list) is coerced to a
// newline-joined string. Values are trimmed; empty/blank values are dropped so a
// missing key leaves that section's prose "" (the composer's degrade path). The
// anti-hallucination contract is unaffected — any stray numeric key is still the
// caller's to ignore.
func parseSectionProse(content string) (map[string]string, error) {
	s := stripFence(stripThink(content))
	if m := coerceProseMap(s); len(m) > 0 {
		return m, nil
	}
	// Fallback: the JSON object is wrapped in surrounding prose. Scan for balanced
	// top-level {…} spans and try them last-first (the final object is the answer).
	spans := balancedObjects(s)
	for i := len(spans) - 1; i >= 0; i-- {
		if m := coerceProseMap(spans[i]); len(m) > 0 {
			return m, nil
		}
	}
	return nil, errors.New("enrich: parse section prose: no sections")
}

// coerceProseMap unmarshals a JSON object of section→prose into map[string]string,
// tolerating a value that is a plain string OR an array of strings (joined by
// newlines). Returns nil when s is not a JSON object or yields no non-empty values.
func coerceProseMap(s string) map[string]string {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &raw); err != nil || len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if t := strings.TrimSpace(coerceProseValue(v)); t != "" {
			out[k] = t
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// coerceProseValue renders one section value as prose: a JSON string is returned
// as-is, a JSON array of strings is joined by newlines (so a model that returns
// bull/bear as a list still renders), and anything else yields "" (dropped) — the
// safe "insufficient, not wrong" degrade.
func coerceProseValue(v json.RawMessage) string {
	var str string
	if json.Unmarshal(v, &str) == nil {
		return str
	}
	var arr []string
	if json.Unmarshal(v, &arr) == nil {
		return strings.Join(arr, "\n")
	}
	return ""
}

// stripThink removes a leading <think>…</think> reasoning block — reasoning models
// (DeepSeek-R1 and kin) wrap their chain-of-thought in it before the answer, which
// would otherwise defeat a naive JSON parse. Only a leading, properly-closed block
// is removed; case-insensitive on the tag.
func stripThink(content string) string {
	s := strings.TrimSpace(content)
	if len(s) >= len("<think>") && strings.EqualFold(s[:len("<think>")], "<think>") {
		if i := strings.Index(strings.ToLower(s), "</think>"); i >= 0 {
			return strings.TrimSpace(s[i+len("</think>"):])
		}
	}
	return s
}

// balancedObjects returns the source spans of every top-level (depth-0) balanced
// {…} object in s, in order, ignoring braces inside JSON string literals. Used to
// recover a JSON object embedded in surrounding prose without the brittle first-{
// to last-} heuristic (which breaks when the prose itself contains braces). Byte
// scanning is UTF-8-safe here: the matched delimiters are all ASCII and multibyte
// rune continuation bytes never collide with them.
func balancedObjects(s string) []string {
	var spans []string
	depth, start := 0, -1
	inStr, esc := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					spans = append(spans, s[start:i+1])
					start = -1
				}
			}
		}
	}
	return spans
}

const translatePrompt = "你是金融新闻标题翻译器。用户给出 JSON 对象 {\"items\":[{\"i\":序号,\"t\":英文标题}...]}。" +
	"只返回 JSON 对象 {\"items\":[{\"i\":相同序号,\"zh\":简体中文翻译}...]},每条都必须带 i 和 zh,i 与输入一一对应、不要遗漏任何一条。" +
	"保留股票代码、公司名、数字与百分比;使用中文财经惯用语(beats estimates→超预期, " +
	"downgrade→下调评级, guidance→业绩指引)。只输出该 JSON 对象,不要解释或代码块。"

func (l *llm) TranslateTitles(ctx context.Context, titles []string) ([]string, error) {
	if len(titles) == 0 {
		return nil, nil
	}
	type item struct {
		I int    `json:"i"`
		T string `json:"t"`
	}
	in := make([]item, len(titles))
	for i, t := range titles {
		in[i] = item{I: i, T: t}
	}
	payload, err := json.Marshal(map[string][]item{"items": in})
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]any{
		"model":           l.model,
		"temperature":     0.1,
		"response_format": map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "system", "content": translatePrompt},
			{"role": "user", "content": string(payload)},
		},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+l.apiKey)

	resp, err := l.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("enrich: translate request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("enrich: translate status %s", resp.Status)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("enrich: translate decode: %w", err)
	}
	if len(out.Choices) == 0 {
		return nil, errors.New("enrich: empty translate response")
	}
	return parseIndexedTranslations(out.Choices[0].Message.Content, len(titles))
}

// parseIndexedTranslations maps the model's {"items":[{"i","zh"}]} reply back
// onto a slice of length n by index, so a miscount (model merges/drops/reorders
// a title — common at batch size) never misaligns or discards the whole batch:
// indices present are filled, missing ones stay empty (the caller skips empties
// and they get retried next sweep). Tolerant of code fences and a bare array.
func parseIndexedTranslations(content string, n int) ([]string, error) {
	s := stripFence(content)
	type item struct {
		I  int    `json:"i"`
		ZH string `json:"zh"`
	}
	var obj struct {
		Items []item `json:"items"`
	}
	if err := json.Unmarshal([]byte(s), &obj); err != nil || len(obj.Items) == 0 {
		// Fallback: a bare [...] array of items (with or without prose around it).
		if a, b := strings.Index(s, "["), strings.LastIndex(s, "]"); a >= 0 && b > a {
			var items []item
			if json.Unmarshal([]byte(s[a:b+1]), &items) == nil {
				obj.Items = items
			}
		}
	}
	if len(obj.Items) == 0 {
		return nil, errors.New("enrich: parse translations: no items")
	}
	out := make([]string, n)
	for _, it := range obj.Items {
		if it.I >= 0 && it.I < n {
			out[it.I] = strings.TrimSpace(it.ZH)
		}
	}
	return out, nil
}

// stripFence removes a leading ```json / trailing ``` Markdown fence, if any.
func stripFence(content string) string {
	s := strings.TrimSpace(content)
	if i := strings.Index(s, "```"); i >= 0 {
		s = s[i+3:]
		s = strings.TrimPrefix(s, "json")
		if j := strings.LastIndex(s, "```"); j >= 0 {
			s = s[:j]
		}
		s = strings.TrimSpace(s)
	}
	return s
}
