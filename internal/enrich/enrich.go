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
		http:    &http.Client{Timeout: 30 * time.Second},
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

const systemPrompt = "You are a concise financial assistant. Summarize the " +
	"following stock news and social posts in 3-5 short bullet points covering " +
	"what changed and why it might matter. Be factual and neutral; this is not " +
	"investment advice."

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

const translatePrompt = "你是金融新闻标题翻译器。用户给出一个 JSON 对象 {\"titles\":[英文标题...]}。" +
	"只返回一个 JSON 对象 {\"titles\":[简体中文翻译...]},数组等长且顺序一致。" +
	"保留股票代码、公司名、数字与百分比;使用中文财经惯用语(beats estimates→超预期, " +
	"downgrade→下调评级, guidance→业绩指引)。只输出该 JSON 对象,不要解释或代码块。"

func (l *llm) TranslateTitles(ctx context.Context, titles []string) ([]string, error) {
	if len(titles) == 0 {
		return nil, nil
	}
	payload, err := json.Marshal(map[string][]string{"titles": titles})
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
	return parseTitleArray(out.Choices[0].Message.Content, len(titles))
}

// parseTitleArray parses the model's reply into exactly `want` titles, tolerant
// of the shapes models actually emit: a {"titles":[...]} object, a bare [...]
// array, or either wrapped in a Markdown code fence / surrounded by prose. The
// strict length check guards against ever writing misaligned translations.
func parseTitleArray(content string, want int) ([]string, error) {
	s := strings.TrimSpace(content)
	if i := strings.Index(s, "```"); i >= 0 { // strip a leading ```json fence
		s = s[i+3:]
		s = strings.TrimPrefix(s, "json")
		if j := strings.LastIndex(s, "```"); j >= 0 {
			s = s[:j]
		}
		s = strings.TrimSpace(s)
	}

	// Preferred: the {"titles":[...]} object we asked for.
	var obj struct {
		Titles []string `json:"titles"`
	}
	if err := json.Unmarshal([]byte(s), &obj); err == nil && len(obj.Titles) > 0 {
		return checkLen(obj.Titles, want)
	}

	// Fallbacks: a bare array, or an array embedded in surrounding prose
	// (slice from the first '[' to the last ']').
	arr := s
	if !strings.HasPrefix(arr, "[") {
		if a, b := strings.Index(s, "["), strings.LastIndex(s, "]"); a >= 0 && b > a {
			arr = s[a : b+1]
		}
	}
	var titles []string
	if err := json.Unmarshal([]byte(arr), &titles); err != nil {
		return nil, fmt.Errorf("enrich: parse translations: %w", err)
	}
	return checkLen(titles, want)
}

func checkLen(titles []string, want int) ([]string, error) {
	if len(titles) != want {
		return nil, fmt.Errorf("enrich: got %d translations, want %d", len(titles), want)
	}
	return titles, nil
}
