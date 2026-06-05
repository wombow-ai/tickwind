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

// Enricher summarizes text using an LLM.
type Enricher interface {
	// Enabled reports whether a real LLM backend is configured.
	Enabled() bool
	// Summarize returns a concise summary of text, or ErrDisabled when no LLM
	// is configured.
	Summarize(ctx context.Context, text string) (string, error)
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
