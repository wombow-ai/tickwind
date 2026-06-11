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
	// Summarize returns a concise summary of text, or ErrDisabled when no LLM
	// is configured.
	Summarize(ctx context.Context, text string) (string, error)
	// TranslateTitles translates English news headlines to Simplified Chinese,
	// preserving order (result[i] is the translation of titles[i]). Returns
	// ErrDisabled when no LLM is configured.
	TranslateTitles(ctx context.Context, titles []string) ([]string, error)
	// Brief writes the Chinese pre-market briefing from structured material
	// (indices, movers, earnings, smart money). ErrDisabled when no LLM.
	Brief(ctx context.Context, material string) (string, error)
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

func (Noop) Summarize(context.Context, string) (string, error) {
	return "", ErrDisabled
}

func (Noop) TranslateTitles(context.Context, []string) ([]string, error) {
	return nil, ErrDisabled
}

func (Noop) Brief(context.Context, string) (string, error) {
	return "", ErrDisabled
}

// systemPrompt drives the per-stock digest. Chinese-first product → Chinese
// output. Structural anti-hallucination guardrails: only restate the supplied
// material, attribute the source type, and never produce advice/targets (also
// enforced by the UI disclaimer).
const systemPrompt = "你是股票信息速览助手。仅基于用户提供的新闻标题与社区帖子,用简体中文输出 3-5 条要点,每条以\"- \"开头。" +
	"内容涵盖:发生了什么、讨论的焦点、市场情绪倾向。要求:只陈述材料中出现的信息,在要点中注明来源类型(如\"据新闻\"\"据社区讨论\");" +
	"不要编造数字、事件或因果;严禁任何买卖建议、目标价或估值判断;语气中性客观。材料不足时输出更少条目;完全无实质内容时只输出\"暂无足够信息\"。"

type llm struct {
	http    *http.Client
	apiKey  string
	baseURL string
	model   string
}

func (l *llm) Enabled() bool { return true }

func (l *llm) Summarize(ctx context.Context, text string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model":       l.model,
		"temperature": 0.2,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
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

// Brief writes the daily Chinese pre-market briefing from structured material.
func (l *llm) Brief(ctx context.Context, material string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model":       l.model,
		"temperature": 0.3,
		"messages": []map[string]string{
			{"role": "system", "content": briefPrompt},
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
