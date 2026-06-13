package telegram

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient returns a client pointed at srv with a non-empty token so it is
// Enabled, plus the parsed request the handler captured.
func newTestClient(srv *httptest.Server, token, chat string) *Client {
	c := New(token, chat, srv.Client())
	c.baseURL = srv.URL
	return c
}

func TestSendMessageSuccess(t *testing.T) {
	var gotPath, gotChat, gotText, gotParse, gotPreview, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		_ = r.ParseForm()
		gotChat = r.PostForm.Get("chat_id")
		gotText = r.PostForm.Get("text")
		gotParse = r.PostForm.Get("parse_mode")
		gotPreview = r.PostForm.Get("disable_web_page_preview")
		_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":4242}}`)
	}))
	defer srv.Close()

	c := newTestClient(srv, "TOKEN123", "@tickwind")
	id, err := c.SendMessage(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if id != 4242 {
		t.Errorf("message_id = %d, want 4242", id)
	}
	if gotPath != "/botTOKEN123/sendMessage" {
		t.Errorf("path = %q, want /botTOKEN123/sendMessage", gotPath)
	}
	if gotCT != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q", gotCT)
	}
	if gotChat != "@tickwind" {
		t.Errorf("chat_id = %q, want @tickwind (default)", gotChat)
	}
	if gotText != "hello world" {
		t.Errorf("text = %q", gotText)
	}
	if gotParse != "" {
		t.Errorf("parse_mode = %q, want empty by default", gotParse)
	}
	if gotPreview != "" {
		t.Errorf("disable_web_page_preview = %q, want empty by default", gotPreview)
	}
}

func TestSendMessageOptionsPassthrough(t *testing.T) {
	var gotChat, gotParse, gotPreview string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotChat = r.PostForm.Get("chat_id")
		gotParse = r.PostForm.Get("parse_mode")
		gotPreview = r.PostForm.Get("disable_web_page_preview")
		_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":7}}`)
	}))
	defer srv.Close()

	c := newTestClient(srv, "T", "@default")
	id, err := c.SendMessage(context.Background(), "<b>hi</b>",
		WithChat("-100123"), WithHTML(), WithoutPreview())
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if id != 7 {
		t.Errorf("message_id = %d, want 7", id)
	}
	if gotChat != "-100123" {
		t.Errorf("chat_id = %q, want -100123 (WithChat override)", gotChat)
	}
	if gotParse != "HTML" {
		t.Errorf("parse_mode = %q, want HTML", gotParse)
	}
	if gotPreview != "true" {
		t.Errorf("disable_web_page_preview = %q, want true", gotPreview)
	}
}

func TestSendPhotoSuccess(t *testing.T) {
	var gotPath, gotPhoto, gotCaption, gotParse string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotPhoto = r.PostForm.Get("photo")
		gotCaption = r.PostForm.Get("caption")
		gotParse = r.PostForm.Get("parse_mode")
		_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":99}}`)
	}))
	defer srv.Close()

	c := newTestClient(srv, "TK", "@tickwind")
	id, err := c.SendPhoto(context.Background(),
		"https://tickwind.com/og/AAPL.png", "<b>AAPL</b> up 3%", WithHTML())
	if err != nil {
		t.Fatalf("SendPhoto: %v", err)
	}
	if id != 99 {
		t.Errorf("message_id = %d, want 99", id)
	}
	if gotPath != "/botTK/sendPhoto" {
		t.Errorf("path = %q, want /botTK/sendPhoto", gotPath)
	}
	if gotPhoto != "https://tickwind.com/og/AAPL.png" {
		t.Errorf("photo = %q", gotPhoto)
	}
	if gotCaption != "<b>AAPL</b> up 3%" {
		t.Errorf("caption = %q", gotCaption)
	}
	if gotParse != "HTML" {
		t.Errorf("parse_mode = %q, want HTML", gotParse)
	}
}

func TestSendOKFalseReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`)
	}))
	defer srv.Close()

	c := newTestClient(srv, "TK", "@nope")
	_, err := c.SendMessage(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error on ok:false, got nil")
	}
	if !strings.Contains(err.Error(), "chat not found") {
		t.Errorf("error %q should carry Telegram description", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not *APIError", err)
	}
	if apiErr.Code != 400 {
		t.Errorf("APIError.Code = %d, want 400", apiErr.Code)
	}
}

func TestSendRateLimitReturnsRateLimitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 30","parameters":{"retry_after":30}}`)
	}))
	defer srv.Close()

	c := newTestClient(srv, "TK", "@tickwind")
	_, err := c.SendMessage(context.Background(), "spam")
	if err == nil {
		t.Fatal("expected error on 429, got nil")
	}
	var rl *RateLimitError
	if !errors.As(err, &rl) {
		t.Fatalf("error %v is not *RateLimitError", err)
	}
	if rl.RetryAfter != 30 {
		t.Errorf("RetryAfter = %d, want 30", rl.RetryAfter)
	}
}

func TestDisabledClientIsNoOp(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":1}}`)
	}))
	defer srv.Close()

	c := newTestClient(srv, "", "@tickwind") // empty token => disabled
	if c.Enabled() {
		t.Fatal("Enabled() = true for empty token, want false")
	}

	id, err := c.SendMessage(context.Background(), "should not send")
	if err != nil || id != 0 {
		t.Errorf("SendMessage no-op = (%d,%v), want (0,nil)", id, err)
	}
	id, err = c.SendPhoto(context.Background(), "https://x/y.png", "cap")
	if err != nil || id != 0 {
		t.Errorf("SendPhoto no-op = (%d,%v), want (0,nil)", id, err)
	}
	if hits != 0 {
		t.Errorf("server hits = %d, want 0 (disabled client must not call out)", hits)
	}
}

func TestEnabledReflectsToken(t *testing.T) {
	if New("", "@c", nil).Enabled() {
		t.Error("empty token should be disabled")
	}
	if New("  ", "@c", nil).Enabled() {
		t.Error("whitespace-only token should be disabled")
	}
	if !New("abc", "@c", nil).Enabled() {
		t.Error("non-empty token should be enabled")
	}
}

func TestNilHTTPClientGetsDefault(t *testing.T) {
	c := New("tok", "@c", nil)
	if c.http == nil {
		t.Fatal("nil *http.Client should be replaced with a default")
	}
	if c.http.Timeout == 0 {
		t.Error("default *http.Client should have a non-zero timeout")
	}
}

func TestEscapeHTML(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"a < b & c > d", "a &lt; b &amp; c &gt; d"},
		{"S&P 500", "S&amp;P 500"},
		{`"quoted"`, `"quoted"`}, // quotes left untouched
	}
	for _, tc := range cases {
		if got := EscapeHTML(tc.in); got != tc.want {
			t.Errorf("EscapeHTML(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNon200NonJSONIncludesStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "<html>502 Bad Gateway</html>")
	}))
	defer srv.Close()

	c := newTestClient(srv, "TK", "@tickwind")
	_, err := c.SendMessage(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error on non-JSON 502, got nil")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("error %q should mention the HTTP status", err)
	}
}
