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
	// ExplainMove writes ONE short (1-2 sentence) hedged explanation of a notable
	// daily price move from a pre-built material string (the Go-computed move % +
	// direction + attributed evidence headlines). The move number and direction are
	// given in the material and MUST NOT be recomputed or altered; the model may
	// reference ONLY the supplied evidence headlines, must HEDGE ("可能与…有关"), must
	// NOT assert a definitive cause, invent a catalyst, or give any price target /
	// advice. Returns ErrDisabled when no LLM is configured.
	ExplainMove(ctx context.Context, material, lang string) (string, error)
}

// Config configures the LLM enricher. An empty APIKey yields a disabled Noop.
type Config struct {
	APIKey  string
	BaseURL string // OpenAI-compatible base; default https://api.openai.com/v1
	Model   string // default gpt-4o-mini
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
	return &llm{
		// Generous ceiling: batch translation streams many tokens and some
		// OpenRouter providers are slow; per-call context keeps tighter bounds.
		http:    &http.Client{Timeout: 90 * time.Second},
		apiKey:  cfg.APIKey,
		baseURL: strings.TrimRight(base, "/"),
		model:   model,
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

func (Noop) ExplainMove(context.Context, string, string) (string, error) {
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
	http    *http.Client
	apiKey  string
	baseURL string
	model   string
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

// parseSectionProse maps the model's section-keyed JSON reply back onto a
// map[string]string. Tolerant of Markdown code fences (stripFence) and of an
// object wrapped in surrounding prose ({...} extracted by brace span), mirroring
// parseIndexedTranslations. Values are trimmed; empty/blank values are dropped so
// a missing key leaves that section's prose "" (the composer's degrade path).
func parseSectionProse(content string) (map[string]string, error) {
	s := stripFence(content)
	var m map[string]string
	if err := json.Unmarshal([]byte(s), &m); err != nil || len(m) == 0 {
		// Fallback: an object embedded in surrounding prose.
		if a, b := strings.Index(s, "{"), strings.LastIndex(s, "}"); a >= 0 && b > a {
			_ = json.Unmarshal([]byte(s[a:b+1]), &m)
		}
	}
	if len(m) == 0 {
		return nil, errors.New("enrich: parse section prose: no sections")
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if t := strings.TrimSpace(v); t != "" {
			out[k] = t
		}
	}
	if len(out) == 0 {
		return nil, errors.New("enrich: parse section prose: all sections empty")
	}
	return out, nil
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
