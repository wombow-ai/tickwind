// Package telegram is a tiny, self-contained client for the Telegram Bot API,
// limited to the handful of send-only methods Tickwind needs to broadcast to a
// public channel (e.g. @tickwind): sendMessage and sendPhoto. It deliberately
// does NOT poll getUpdates — this bot only speaks, it never listens.
//
// The client is dependency-injection friendly: the bot token, the default
// destination chat, and the *http.Client are all passed to New, and the API
// host lives in a configurable baseURL field so tests can point it at an
// httptest server. Nothing here reads the environment; main is expected to read
// TELEGRAM_BOT_TOKEN / TELEGRAM_CHANNEL and hand them in.
//
// When constructed with an empty token the client is disabled: Enabled reports
// false and every send becomes a no-op returning (0, nil), so callers can wire
// it unconditionally and let configuration decide whether messages actually go
// out.
//
// Telegram API shape: https://api.telegram.org/bot<token>/<method>, form-encoded
// request, JSON response of the form {"ok":bool,"result":{...},
// "description":string,"error_code":int,"parameters":{"retry_after":int}}.
package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// defaultBaseURL is the public Telegram Bot API host. Override Client.baseURL in
// tests to redirect requests at an httptest server.
const defaultBaseURL = "https://api.telegram.org"

// defaultTimeout bounds requests when New is called with a nil *http.Client.
const defaultTimeout = 15 * time.Second

// Client sends messages to Telegram on behalf of a single bot. It is safe for
// concurrent use: all fields are read-only after construction and *http.Client
// is itself concurrency-safe.
type Client struct {
	http        *http.Client
	token       string // bot token; empty disables the client
	defaultChat string // chat used when no WithChat option is given
	baseURL     string // API host, default defaultBaseURL; overridable in tests
}

// New returns a Client for the given bot token and default destination chat.
//
// The chat may be a numeric ID ("-1001234567890") or a public @username
// ("@tickwind"); the latter only works for public channels/groups the bot can
// post to. If hc is nil a default *http.Client with a sane timeout is used.
//
// An empty token yields a disabled client (see Enabled): all sends become
// no-ops. This lets callers construct the client unconditionally and gate real
// delivery purely on whether a token was configured.
func New(token, defaultChat string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: defaultTimeout}
	}
	return &Client{
		http:        hc,
		token:       strings.TrimSpace(token),
		defaultChat: strings.TrimSpace(defaultChat),
		baseURL:     defaultBaseURL,
	}
}

// Enabled reports whether the client has a bot token and will actually attempt
// delivery. A disabled client treats every send as a successful no-op.
func (c *Client) Enabled() bool { return c.token != "" }

// Option customizes a single send. Options are applied left to right.
type Option func(*params)

// params accumulates the form fields for one send call.
type params struct {
	chat           string
	parseMode      string
	disablePreview bool
}

// WithChat overrides the destination chat for this send (numeric ID or
// @username). When unset the client's default chat is used.
func WithChat(chat string) Option {
	return func(p *params) { p.chat = strings.TrimSpace(chat) }
}

// WithHTML sets parse_mode=HTML so Telegram renders HTML markup in the text or
// caption. Note that with HTML mode the literal characters < > & must be
// escaped as &lt; &gt; &amp; in any non-markup content; see EscapeHTML.
func WithHTML() Option {
	return func(p *params) { p.parseMode = "HTML" }
}

// WithoutPreview sets disable_web_page_preview=true so Telegram does not render
// a link preview card for URLs in a sendMessage text.
func WithoutPreview() Option {
	return func(p *params) { p.disablePreview = true }
}

// SendMessage sends text to a chat via the sendMessage method and returns the
// resulting Telegram message_id. The default chat is used unless WithChat is
// given. With a disabled client (empty token) this is a no-op returning
// (0, nil).
//
// Errors: a non-200 status or an {"ok":false} body yields an error carrying
// Telegram's description; a 429 (too many requests) yields a *RateLimitError
// exposing RetryAfter, which can be detected with errors.As.
func (c *Client) SendMessage(ctx context.Context, text string, opts ...Option) (int, error) {
	if !c.Enabled() {
		return 0, nil
	}
	p := c.resolve(opts)

	form := url.Values{}
	form.Set("chat_id", p.chat)
	form.Set("text", text)
	if p.parseMode != "" {
		form.Set("parse_mode", p.parseMode)
	}
	if p.disablePreview {
		form.Set("disable_web_page_preview", "true")
	}
	return c.call(ctx, "sendMessage", form)
}

// SendPhoto sends a photo by URL via the sendPhoto method and returns the
// resulting message_id. photoURL must be publicly reachable: Telegram fetches it
// server-side (used for Tickwind's OG image cards), so localhost or
// authenticated URLs will not work. caption is optional and, when WithHTML is
// passed, may contain HTML markup. The default chat is used unless WithChat is
// given. With a disabled client this is a no-op returning (0, nil).
//
// Errors behave as in SendMessage, including *RateLimitError on 429.
func (c *Client) SendPhoto(ctx context.Context, photoURL, caption string, opts ...Option) (int, error) {
	if !c.Enabled() {
		return 0, nil
	}
	p := c.resolve(opts)

	form := url.Values{}
	form.Set("chat_id", p.chat)
	form.Set("photo", photoURL)
	if caption != "" {
		form.Set("caption", caption)
	}
	if p.parseMode != "" {
		form.Set("parse_mode", p.parseMode)
	}
	return c.call(ctx, "sendPhoto", form)
}

// resolve folds the options into a params, defaulting the chat to the client's
// configured destination.
func (c *Client) resolve(opts []Option) params {
	p := params{chat: c.defaultChat}
	for _, opt := range opts {
		if opt != nil {
			opt(&p)
		}
	}
	return p
}

// apiResponse mirrors the envelope every Bot API method returns. Result is left
// raw and decoded only on success.
type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
	Parameters  *struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// call POSTs a form-encoded request to the named Bot API method and returns the
// message_id from a successful result.
func (c *Client) call(ctx context.Context, method string, form url.Values) (int, error) {
	endpoint := c.baseURL + "/bot" + c.token + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, fmt.Errorf("telegram: build %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("telegram: %s: %w", method, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("telegram: %s: read response: %w", method, err)
	}

	var api apiResponse
	// Telegram returns JSON for both success and documented errors; a decode
	// failure means something non-conforming (proxy/HTML error page), so report
	// the status and a snippet rather than a confusing decode error.
	if jsonErr := json.Unmarshal(body, &api); jsonErr != nil {
		if resp.StatusCode != http.StatusOK {
			return 0, fmt.Errorf("telegram: %s: %s: %s", method, resp.Status, snippet(body))
		}
		return 0, fmt.Errorf("telegram: %s: decode response: %w", method, jsonErr)
	}

	if resp.StatusCode == http.StatusTooManyRequests || api.ErrorCode == http.StatusTooManyRequests {
		retry := 0
		if api.Parameters != nil {
			retry = api.Parameters.RetryAfter
		}
		return 0, &RateLimitError{RetryAfter: retry, Description: api.Description}
	}

	if !api.OK {
		desc := api.Description
		if desc == "" {
			desc = resp.Status
		}
		return 0, fmt.Errorf("telegram: %s: %w", method, &APIError{Code: api.ErrorCode, Description: desc})
	}

	var result struct {
		MessageID int `json:"message_id"`
	}
	if err := json.Unmarshal(api.Result, &result); err != nil {
		return 0, fmt.Errorf("telegram: %s: decode result: %w", method, err)
	}
	return result.MessageID, nil
}

// APIError is returned when Telegram replies with {"ok":false}. It carries the
// Bot API error_code and description.
type APIError struct {
	Code        int
	Description string
}

func (e *APIError) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("api error %d: %s", e.Code, e.Description)
	}
	return "api error: " + e.Description
}

// RateLimitError is returned when Telegram rate-limits a send (HTTP 429 /
// error_code 429). RetryAfter is the number of seconds Telegram asks the caller
// to wait before retrying (0 if unspecified). Detect it with errors.As.
type RateLimitError struct {
	RetryAfter  int
	Description string
}

func (e *RateLimitError) Error() string {
	desc := e.Description
	if desc == "" {
		desc = "too many requests"
	}
	return fmt.Sprintf("telegram: rate limited: %s (retry after %ds)", desc, e.RetryAfter)
}

// EscapeHTML escapes the characters that are special in Telegram's HTML
// parse_mode (& < >), so arbitrary text can be safely embedded between HTML
// tags when using WithHTML. It intentionally does not touch quotes, which are
// only significant inside tag attributes.
func EscapeHTML(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	).Replace(s)
}

// snippet returns a short, single-line excerpt of a response body for use in
// error messages, so a stray HTML/proxy error page does not flood logs.
func snippet(b []byte) string {
	const max = 200
	s := strings.TrimSpace(string(b))
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

// compile-time assurances that our error types satisfy the error interface.
var (
	_ error = (*APIError)(nil)
	_ error = (*RateLimitError)(nil)
)
