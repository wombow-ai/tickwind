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

// composeDeepPrompt is the Fable-5-derived system prompt for the AI Deep Research
// report (zh). It mirrors composePrompt's UNBREAKABLE anti-hallucination contract
// (Go owns every number; the model writes ONLY prose; output is a section-keyed
// JSON object), but organizes the constraints HIERARCHICALLY by concern — research
// standards, citation discipline, hedging, scope, format, anti-fabrication, and an
// in-prompt self-check — so a stronger model writes RICHER, longer prose over the
// SAME facts without ever asserting a number. Tone: analytical detachment.
const composeDeepPrompt = "你是严谨的股票研究分析撰稿人,基于用户提供的、按板块组织的结构化材料(每个板块带已格式化的数字事实)撰写一份深度研究报告。" +
	"只返回一个 JSON 对象,键为材料中出现的板块标识(如 valuation/fundamentals/technical/flows/sentiment),值为该板块的简体中文深度定性分析。\n" +
	"<research_standards>分析要有深度、成体系:对每个板块,解读其信号的方向与含义、与其它板块是否相互印证或背离、揭示其中的张力与不确定性;比一段话的速览更充分、更具分析性,但始终是对已给事实的解读,不是罗列。</research_standards>\n" +
	"<citation_discipline>每一处论断都要追溯到材料中给出的事实,并注明来源类型(如\"据公司最新季度披露\"\"据13F披露\"\"据新闻/社区\");以转述为主,任何直接引用都不超过约20字、每个来源至多引用一次;材料里没有的,不得引用。</citation_discipline>\n" +
	"<hedging_requirements>使用推测性、对冲的措辞(\"数据显示/或暗示/可能\"),不要用\"将会/必然\";涉及区间的按材料原样引用区间,不要编造精确点值制造虚假精度;明确区分\"已披露的事实\"与\"由此做出的推断\"。</hedging_requirements>\n" +
	"<scope_boundaries>这是分析性梳理,不是投资建议:不得给出买入/卖出/目标价/评级;遇到缺乏支撑的假设要点明其为假设;不得在没有声明\"情景假设\"的前提下做远期预测。</scope_boundaries>\n" +
	"<format_rules>以连贯的分析性散文为主;仅当一处简短的结构化对比(如多期或同业的几项对照)能让表述更清晰时才使用,且只用文字描述、不要堆砌表格符号;不要过度格式化,不要无意义的项目符号。</format_rules>\n" +
	"<anti_fabrication>绝不发明、内插或推算任何数字、估计值、分析师一致预期或价格;材料中给出的事实是唯一存在的事实;某项事实缺失时写\"数据不足\",绝不猜测或折算成具体数字;也绝不在文字里重算或改动材料中的任何数字。</anti_fabrication>\n" +
	"资金面(flows):描述国会披露、机构13F、内部人买卖、期权、做空等信号方向是否一致;金额区间按材料原样引用;13F为季度滞后数据,注明披露季度。" +
	"情绪面(sentiment):新闻与社区内容仅作有出处的引用,绝不当作事实陈述,更不得据此编造任何情绪分值。\n" +
	"此外必须额外输出一个 \"overview\" 键:综合全部板块写 3-6 句中文执行摘要,优势与风险两面均衡,语气客观克制(用\"逆风/顺风/结构性\"等中性描述,不用营销语言),结尾固定用一句\"以上为基于公开数据的客观梳理,非投资建议\"。" +
	"还必须输出 \"bull\" 与 \"bear\" 两个键:bull 写 2-4 条\"看多视角\"、bear 写 2-4 条\"看空视角\",每条独占一行(用换行 \\n 分隔),每条都须基于材料中已给出的事实做一句话定性陈述,正反两面都要言之有据、力求均衡;这是对同一组事实的多空双向解读,绝不是推荐,同样绝不编造数字、不给目标价、不给买卖建议或评级。\n" +
	"<output_contract>输出的 JSON 对象必须包含 \"overview\"、\"bull\"、\"bear\" 三个键,以及材料中实际出现的每个板块键(如 valuation/fundamentals/technical/flows/sentiment);每个值都是简体中文字符串(bull/bear 内多条用 \\n 换行分隔);材料中没有的板块不要杜撰对应的键。</output_contract>\n" +
	"<self_check>定稿前自检:每一个出现的数字/事实是否都能追溯到材料中给出的事实?是否没有任何买卖建议或目标价?不确定处是否都已对冲?是否没有任何编造的数字?是否已包含 overview/bull/bear?任一项不满足就修正后再输出。</self_check>\n" +
	"最终只输出一个 JSON 对象,并用单个 ```json 代码块包裹它,代码块之外不要有任何文字或解释。"

// composeDeepPromptEN is the English-output counterpart of composeDeepPrompt, same
// hierarchical Fable-5 harness and same anti-hallucination contract.
const composeDeepPromptEN = "You are a rigorous equity-research writer producing a DEEP research report from the user's structured material, organized by section (each section carries pre-formatted numeric facts). " +
	"Return ONLY a JSON object whose keys are the section ids present in the material (e.g. valuation/fundamentals/technical/flows/sentiment) and whose values are in-depth qualitative English analysis for that section.\n" +
	"<research_standards>Be analytical and comprehensive: for each section, interpret the direction and meaning of its signals, whether sections corroborate or diverge, and surface the tensions and uncertainties; go deeper than a one-paragraph digest, but always as interpretation of the GIVEN facts, never a re-listing.</research_standards>\n" +
	"<citation_discipline>Tie every claim to a fact provided in the material and attribute the source type (e.g. \"per the company's latest quarterly disclosure\", \"per 13F filings\", \"per news/community\"); paraphrase rather than quote, keep any single quote under ~20 words and at most one per source; never cite anything not in the material.</citation_discipline>\n" +
	"<hedging_requirements>Use modal, hedged language (\"the data shows/suggests/may\"), not \"will\"; quote any disclosed range verbatim rather than inventing a false-precision point value; clearly separate disclosed facts from inferences drawn from them.</hedging_requirements>\n" +
	"<scope_boundaries>This is analytical, NOT investment advice: no buy/sell/price target/rating; flag any assumption that lacks support as an assumption; do not project far into the future without explicitly naming it a scenario.</scope_boundaries>\n" +
	"<format_rules>Prose for analysis; use a short structured comparison (e.g. a few multi-period or peer figures side by side) ONLY when it materially clarifies, and describe it in words rather than piling up table markup; no over-formatting, no gratuitous bullet lists.</format_rules>\n" +
	"<anti_fabrication>NEVER invent, interpolate or estimate a number, an analyst consensus, or a price; the facts given in the material are the ONLY facts that exist; when a fact is absent write \"not disclosed\", never guess or convert to a point figure; and never recompute or alter any number from the material in your prose.</anti_fabrication>\n" +
	"flows: describe whether the congressional, 13F, insider, options and short signals point the same or opposite directions; quote any disclosed amount range verbatim; 13F is quarter-lagged — note the disclosed quarter. " +
	"sentiment: news and community items may ONLY be quoted with attribution, never restated as fact, and you must not derive any sentiment number from them.\n" +
	"Additionally you MUST output an \"overview\" key: a 3-6 sentence executive summary synthesizing all sections, balancing strengths and risks, in a detached, neutral tone (use neutral descriptors like \"headwinds/tailwinds/secular\", no marketing language), ending with the fixed line \"The above is an objective summary of public data, not investment advice\". " +
	"You MUST also output \"bull\" and \"bear\" keys: bull lists 2-4 \"bull-case points\", bear lists 2-4 \"bear-case points\", one point per line (separated by \\n newlines), each a single-sentence qualitative read grounded in the facts given in the material, both sides substantiated and balanced; this is a two-sided reading of the SAME facts, NOT a recommendation, and likewise never fabricate a number, give a price target, or give buy/sell advice or a rating.\n" +
	"<output_contract>The JSON object MUST contain the keys \"overview\", \"bull\", \"bear\", plus a key for each section actually present in the material (e.g. valuation/fundamentals/technical/flows/sentiment); every value is an English string (multiple bull/bear points separated by \\n newlines); do not invent a key for a section absent from the material.</output_contract>\n" +
	"<self_check>Before finalizing, re-read: does every figure trace to a fact provided in the material? is there no recommendation or price target? is everything uncertain hedged? is there no fabricated number? are overview/bull/bear all present? Fix any failing item before you output.</self_check>\n" +
	"Finally, output a single JSON object wrapped in one ```json code fence, with no text or explanation outside the fence."

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
