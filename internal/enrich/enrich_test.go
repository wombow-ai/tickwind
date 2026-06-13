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

func TestComposeReportNoop(t *testing.T) {
	got, err := Noop{}.ComposeReport(context.Background(), "anything", "zh")
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("err = %v, want ErrDisabled", err)
	}
	if got != nil {
		t.Fatalf("map = %v, want nil", got)
	}
}
