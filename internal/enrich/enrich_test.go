package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseIndexedTranslations(t *testing.T) {
	// The requested {"items":[{i,zh}]} object, in order.
	got, err := parseIndexedTranslations(`{"items":[{"i":0,"zh":"英伟达超预期"},{"i":1,"zh":"苹果回购创纪录"}]}`, 2)
	if err != nil || got[0] != "英伟达超预期" || got[1] != "苹果回购创纪录" {
		t.Fatalf("object: got=%v err=%v", got, err)
	}

	// Fenced + reordered: index decides position, not array order.
	got, err = parseIndexedTranslations("```json\n{\"items\":[{\"i\":1,\"zh\":\"乙\"},{\"i\":0,\"zh\":\"甲\"}]}\n```", 2)
	if err != nil || got[0] != "甲" || got[1] != "乙" {
		t.Fatalf("fenced/reordered: got=%v err=%v", got, err)
	}

	// Miscount: model dropped index 1 of 3 → present ones fill, missing stays
	// empty (retried next sweep), NEVER misaligns or discards the batch.
	got, err = parseIndexedTranslations(`{"items":[{"i":0,"zh":"甲"},{"i":2,"zh":"丙"}]}`, 3)
	if err != nil {
		t.Fatalf("miscount err: %v", err)
	}
	if got[0] != "甲" || got[1] != "" || got[2] != "丙" {
		t.Fatalf("miscount: got=%v, want [甲, '', 丙]", got)
	}

	// Bare array of items (model ignored the object wrapper).
	got, err = parseIndexedTranslations(`[{"i":0,"zh":"只此一条"}]`, 1)
	if err != nil || got[0] != "只此一条" {
		t.Fatalf("bare array: got=%v err=%v", got, err)
	}

	// Out-of-range index is ignored (no panic), others still applied.
	got, err = parseIndexedTranslations(`{"items":[{"i":9,"zh":"越界"},{"i":0,"zh":"有效"}]}`, 2)
	if err != nil || got[0] != "有效" || got[1] != "" {
		t.Fatalf("out-of-range: got=%v err=%v", got, err)
	}

	// Total garbage → error (sweep retries).
	if _, err = parseIndexedTranslations(`抱歉无法完成`, 2); err == nil {
		t.Fatal("non-JSON should error")
	}
}

func TestParseSectionProse(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    map[string]string
		wantErr bool
	}{
		{
			name:    "keyed object",
			content: `{"valuation":"估值偏高","fundamentals":"营收增长","technical":"位于均线上方"}`,
			want:    map[string]string{"valuation": "估值偏高", "fundamentals": "营收增长", "technical": "位于均线上方"},
		},
		{
			name:    "fenced reply still parses",
			content: "```json\n{\"valuation\":\"估值合理\"}\n```",
			want:    map[string]string{"valuation": "估值合理"},
		},
		{
			name:    "object embedded in surrounding prose",
			content: "好的,结果如下:{\"technical\":\"超买\"} 完毕",
			want:    map[string]string{"technical": "超买"},
		},
		{
			name:    "blank values dropped, others kept",
			content: `{"valuation":"  ","fundamentals":"  净利润为正  "}`,
			want:    map[string]string{"fundamentals": "净利润为正"},
		},
		{
			name:    "leading <think> block stripped (reasoning model)",
			content: "<think>let me reason about {this} carefully</think>\n```json\n{\"valuation\":\"估值偏高\"}\n```",
			want:    map[string]string{"valuation": "估值偏高"},
		},
		{
			name:    "array value coerced to newline-joined string",
			content: `{"bull":["现金流稳健","回购加速"],"valuation":"中性"}`,
			want:    map[string]string{"bull": "现金流稳健\n回购加速", "valuation": "中性"},
		},
		{
			name:    "prose with braces then the real object - last balanced wins",
			content: "分析(含{花括号}干扰)如下:{\"technical\":\"超买\"}",
			want:    map[string]string{"technical": "超买"},
		},
		{
			name:    "no sections is an error",
			content: `抱歉无法完成`,
			wantErr: true,
		},
		{
			name:    "all-empty values is an error",
			content: `{"valuation":"","fundamentals":"   "}`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSectionProse(tt.content)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("len: got %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Fatalf("key %q: got %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

// chatReply serves an OpenAI-compatible /chat/completions reply whose single
// choice's message content is the given string, so the *llm parsing path can be
// exercised end-to-end against an httptest server.
func chatReply(t *testing.T, content string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization header = %q, want Bearer test-key", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": content}},
			},
		})
	}))
}

func TestComposeReport(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    map[string]string
	}{
		{
			name:    "keyed JSON object",
			content: `{"valuation":"估值偏高","fundamentals":"盈利改善"}`,
			want:    map[string]string{"valuation": "估值偏高", "fundamentals": "盈利改善"},
		},
		{
			name:    "fenced reply still parses",
			content: "```json\n{\"technical\":\"位于均线下方\"}\n```",
			want:    map[string]string{"technical": "位于均线下方"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := chatReply(t, tt.content)
			defer srv.Close()

			enr := New(Config{APIKey: "test-key", BaseURL: srv.URL, Model: "test-model"})
			if !enr.Enabled() {
				t.Fatal("enricher should be enabled with an API key")
			}
			got, err := enr.ComposeReport(context.Background(), "valuation: PE 31.2x\nfundamentals: revenue up", "zh")
			if err != nil {
				t.Fatalf("ComposeReport: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("len: got %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Fatalf("key %q: got %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

// TestComposeDeepReport asserts the deep compose parses the section-keyed reply
// (sharing parseSectionProse, here via a fenced ```json block — the deep prompt's
// requested format), uses the deep model + a larger token budget, and sends NO
// response_format (Anthropic and reasoning models reject OpenAI's json_object; the
// prompt + hardened parser carry the contract instead).
func TestComposeDeepReport(t *testing.T) {
	var gotModel string
	var gotMaxTokens float64
	var hasFormat bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model          string         `json:"model"`
			MaxTokens      float64        `json:"max_tokens"`
			ResponseFormat map[string]any `json:"response_format"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotModel = req.Model
		gotMaxTokens = req.MaxTokens
		hasFormat = req.ResponseFormat != nil
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "```json\n{\"valuation\":\"估值偏高,需结合行业背景看待。\",\"overview\":\"综合梳理。以上为基于公开数据的客观梳理,非投资建议。\"}\n```"}},
			},
		})
	}))
	defer srv.Close()

	// DeepModel set → the deep compose must use it (cost control: stronger model
	// only when configured).
	enr := New(Config{APIKey: "test-key", BaseURL: srv.URL, Model: "normal-model", DeepModel: "strong-model"})
	got, err := enr.ComposeDeepReport(context.Background(), "valuation: PE 31.2x", "zh")
	if err != nil {
		t.Fatalf("ComposeDeepReport: %v", err)
	}
	if got["valuation"] == "" || got["overview"] == "" {
		t.Fatalf("missing prose: %v", got)
	}
	if gotModel != "strong-model" {
		t.Errorf("model = %q, want the deep model", gotModel)
	}
	if gotMaxTokens != composeDeepMaxTokens {
		t.Errorf("max_tokens = %v, want %d", gotMaxTokens, composeDeepMaxTokens)
	}
	if hasFormat {
		t.Error("deep compose must NOT send response_format (Claude/reasoning-safe)")
	}
}

// TestComposeDeepReportSeparateProvider asserts the cost-split routing: when
// DeepBaseURL/DeepAPIKey are set, ComposeDeepReport hits the DEEP provider (its URL
// + key + model) while everything else stays on the default provider.
func TestComposeDeepReportSeparateProvider(t *testing.T) {
	var deepAuth, defaultAuth string
	deepSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deepAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": `{"valuation":"deep prose"}`}}},
		})
	}))
	defer deepSrv.Close()
	defaultSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defaultAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": `{"valuation":"routine prose"}`}}},
		})
	}))
	defer defaultSrv.Close()

	enr := New(Config{
		APIKey: "default-key", BaseURL: defaultSrv.URL, Model: "default-model",
		DeepAPIKey: "deep-key", DeepBaseURL: deepSrv.URL, DeepModel: "deep-model",
	})

	got, err := enr.ComposeDeepReport(context.Background(), "valuation: PE 31.2x", "zh")
	if err != nil {
		t.Fatalf("ComposeDeepReport: %v", err)
	}
	if got["valuation"] != "deep prose" {
		t.Fatalf("deep report hit the wrong provider: %v", got)
	}
	if deepAuth != "Bearer deep-key" {
		t.Errorf("deep Authorization = %q, want Bearer deep-key", deepAuth)
	}
	if defaultAuth != "" {
		t.Errorf("default provider should not have been called by the deep compose; got auth %q", defaultAuth)
	}

	// A routine method still uses the default provider/key.
	if _, err := enr.ComposeReport(context.Background(), "valuation: PE 31.2x", "zh"); err != nil {
		t.Fatalf("ComposeReport: %v", err)
	}
	if defaultAuth != "Bearer default-key" {
		t.Errorf("routine Authorization = %q, want Bearer default-key", defaultAuth)
	}
}

// TestComposeDeepReportFallbackModel asserts an empty DeepModel falls back to the
// normal Model — ZERO behavior change until LLM_DEEP_MODEL is set.
func TestComposeDeepReportFallbackModel(t *testing.T) {
	var gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotModel = req.Model
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": `{"valuation":"x"}`}}},
		})
	}))
	defer srv.Close()

	enr := New(Config{APIKey: "test-key", BaseURL: srv.URL, Model: "normal-model"}) // no DeepModel
	if _, err := enr.ComposeDeepReport(context.Background(), "valuation: PE 31.2x", "zh"); err != nil {
		t.Fatalf("ComposeDeepReport: %v", err)
	}
	if gotModel != "normal-model" {
		t.Errorf("model = %q, want fallback to the normal model", gotModel)
	}
}

func TestComposeDeepReportNoop(t *testing.T) {
	got, err := Noop{}.ComposeDeepReport(context.Background(), "anything", "zh")
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("err = %v, want ErrDisabled", err)
	}
	if got != nil {
		t.Fatalf("got = %v, want nil", got)
	}
}

// TestChatToolCallsAndUsage exercises the Product B chat round-trip: a tools array is
// sent, a tool_calls response is parsed back, usage (incl. prompt-cache reads) is
// surfaced, and the assistant/tool round-trip messages marshal correctly.
func TestChatToolCallsAndUsage(t *testing.T) {
	var gotModel string
	var gotTools int
	var gotRoles []string
	var gotToolCallID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model    string `json:"model"`
			Tools    []any  `json:"tools"`
			Messages []struct {
				Role       string `json:"role"`
				ToolCallID string `json:"tool_call_id"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotModel = req.Model
		gotTools = len(req.Tools)
		for _, m := range req.Messages {
			gotRoles = append(gotRoles, m.Role)
			if m.Role == "tool" {
				gotToolCallID = m.ToolCallID
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": "",
					"tool_calls": []map[string]any{{
						"id": "call_1", "type": "function",
						"function": map[string]any{"name": "get_facts", "arguments": `{"section":"valuation"}`},
					}},
				},
			}},
			"usage": map[string]any{
				"prompt_tokens": 1200, "completion_tokens": 20, "total_tokens": 1220,
				"prompt_tokens_details": map[string]any{"cached_tokens": 1000},
			},
		})
	}))
	defer srv.Close()

	enr := New(Config{APIKey: "k", BaseURL: srv.URL, Model: "m", ChatModel: "chat-model"})
	tools := []ChatTool{{Name: "get_facts", Description: "d", Parameters: map[string]any{"type": "object"}}}
	history := []ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "walk me through valuation"},
		{Role: "assistant", ToolCalls: []ChatToolCall{{ID: "call_0", Name: "get_facts", Arguments: `{"section":"flows"}`}}},
		{Role: "tool", ToolCallID: "call_0", Content: "- Net flow: +$1.2M"},
	}
	content, calls, usage, err := enr.Chat(context.Background(), history, tools, "")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if gotModel != "chat-model" {
		t.Errorf("model = %q, want chat-model", gotModel)
	}
	if gotTools != 1 {
		t.Errorf("tools sent = %d, want 1", gotTools)
	}
	if gotToolCallID != "call_0" {
		t.Errorf("tool message tool_call_id = %q, want call_0 (round-trip marshaling)", gotToolCallID)
	}
	if content != "" || len(calls) != 1 {
		t.Fatalf("want 1 tool call + empty content, got content=%q calls=%v", content, calls)
	}
	if calls[0].Name != "get_facts" || calls[0].Arguments != `{"section":"valuation"}` {
		t.Errorf("tool call = %+v", calls[0])
	}
	if usage.PromptTokens != 1200 || usage.CompletionTokens != 20 || usage.CachedTokens != 1000 {
		t.Errorf("usage = %+v, want prompt 1200 / completion 20 / cached 1000", usage)
	}
}

// TestChatModelOverrideAndFallback: an explicit model arg wins; an empty ChatModel falls
// back to the deep model (chat → deep → default).
func TestChatModelOverrideAndFallback(t *testing.T) {
	var gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotModel = req.Model
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "ok"}}},
		})
	}))
	defer srv.Close()

	// No ChatModel/ChatBaseURL → chat falls back to the deep client, which falls back to
	// the default model + base URL.
	enr := New(Config{APIKey: "k", BaseURL: srv.URL, Model: "default-model"})
	if _, _, _, err := enr.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, nil, ""); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if gotModel != "default-model" {
		t.Errorf("fallback model = %q, want default-model (chat→deep→default)", gotModel)
	}
	// An explicit model arg overrides the configured chat model.
	if _, _, _, err := enr.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, nil, "sonnet-deepdive"); err != nil {
		t.Fatalf("Chat override: %v", err)
	}
	if gotModel != "sonnet-deepdive" {
		t.Errorf("override model = %q, want sonnet-deepdive", gotModel)
	}
}

func TestChatNoop(t *testing.T) {
	content, calls, usage, err := Noop{}.Chat(context.Background(), nil, nil, "")
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("err = %v, want ErrDisabled", err)
	}
	if content != "" || calls != nil || (usage != Usage{}) {
		t.Fatalf("Noop.Chat should return zero values, got %q %v %+v", content, calls, usage)
	}
}

func TestComposeReportNoop(t *testing.T) {
	got, err := Noop{}.ComposeReport(context.Background(), "anything", "zh")
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("err = %v, want ErrDisabled", err)
	}
	if got != nil {
		t.Fatalf("map = %v, want nil", got)
	}
}
