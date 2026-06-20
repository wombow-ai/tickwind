package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/wombow-ai/tickwind/internal/auth"
	"github.com/wombow-ai/tickwind/internal/chat"
	"github.com/wombow-ai/tickwind/internal/enrich"
	"github.com/wombow-ai/tickwind/internal/store"
)

// chatDailyCap bounds Product B chat generations per ET day across ALL users — a
// catastrophic-cost backstop on top of the per-user monthly meter + per-user throttle.
// A cached Haiku turn is ~$0.005, so 500/day caps worst-case spend at ~$2.5/day.
const chatDailyCap = 500

// maxChatHistory caps how many messages of a thread are loaded — generous so a normal
// thread is sent in FULL (a stable, growing prefix is what lets Anthropic prompt-cache
// the conversation; a front-sliding window would shift the prefix and break the cache).
// The real bound is maxThreadHistoryTokens below.
const maxChatHistory = 400

// maxThreadHistoryTokens caps a single thread's replayed context (estimated). Past it the
// turn soft-degrades with a "start a new conversation" note — bounding per-turn cost +
// latency (this is the token-based limit, on top of the monthly message meter). ~9000
// tokens ≈ a couple dozen turns; a fresh thread (the reset) starts cheap again.
const maxThreadHistoryTokens = 9000

// estTokens is a cheap (chars/4) token estimate for the thread-budget check.
func estTokens(msgs []enrich.ChatMessage) int {
	chars := 0
	for _, m := range msgs {
		chars += len(m.Content)
	}
	return chars / 4
}

// SetChat injects the Product B chat engine (nil → the chat endpoint 503s).
func (s *Server) SetChat(svc *chat.Service) { s.chatSvc = svc }

// SetChatLimit sets the per-Pro-user monthly message soft-cap (<=0 ignored).
func (s *Server) SetChatLimit(n int) {
	if n > 0 {
		s.chatMonthlyLimit = n
	}
}

// chatReady checks the chat is enabled, the user is Pro, and within the burst throttle;
// it writes the appropriate response + returns false on any failure.
func (s *Server) chatReady(w http.ResponseWriter, r *http.Request, u auth.User) bool {
	if s.chatSvc == nil || !s.chatSvc.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, errBody("chat unavailable"))
		return false
	}
	if s.tierOf(r.Context(), u.ID) != tierPro {
		writeJSON(w, http.StatusPaymentRequired, map[string]any{"error": "Tickwind Pro required", "upgrade": true})
		return false
	}
	if !s.chatRL.allow(u.ID) {
		writeJSON(w, http.StatusTooManyRequests, errBody("too many messages — give it a moment"))
		return false
	}
	return true
}

// postChat handles a chat turn for a STOCK conversation: POST /v1/stocks/{ticker}/chat
// (backward-compat). It resolves the per-(user,ticker) conversation, then runs the turn.
func (s *Server) postChat(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if !s.chatReady(w, r, u) {
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	if ticker == "" {
		writeJSON(w, http.StatusBadRequest, errBody("missing ticker"))
		return
	}
	convID, err := s.store.GetOrCreateStockConversation(r.Context(), u.ID, ticker)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("conversation error"))
		return
	}
	s.chatTurn(w, r, u, convID, ticker)
}

// postConvChat handles a chat turn for an explicit conversation: POST
// /v1/conversations/{id}/chat. Ownership is enforced (404 if not the user's).
func (s *Server) postConvChat(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if !s.chatReady(w, r, u) {
		return
	}
	conv, found, err := s.store.GetConversation(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, errBody("conversation not found"))
		return
	}
	s.chatTurn(w, r, u, conv.ID, conv.AnchorTicker)
}

// chatTurn runs the shared chat turn for a resolved conversation: decode + meter + global
// cap + token-budget cap + answer + persist + charge. anchorTicker grounds the answer (""
// = general/cross-stock, handled from C3). The user is already Pro-gated + throttled.
func (s *Server) chatTurn(w http.ResponseWriter, r *http.Request, u auth.User, convID, anchorTicker string) {
	var req struct {
		Message string `json:"message"`
		Lang    string `json:"lang"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("bad request"))
		return
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		writeJSON(w, http.StatusBadRequest, errBody("empty message"))
		return
	}
	if len(msg) > 2000 {
		msg = msg[:2000]
	}
	lang := req.Lang
	if lang != "zh" {
		lang = "en"
	}

	period := researchMonth()
	used, _ := s.store.GetChatMsgUsed(r.Context(), u.ID, period)
	if used >= s.chatMonthlyLimit {
		writeJSON(w, http.StatusOK, map[string]any{
			"blocks":        []chat.Block{{Kind: "text", Text: chatLimitNote(lang)}},
			"limit_reached": true,
			"meter":         map[string]int{"used": used, "limit": s.chatMonthlyLimit},
			"disclaimer":    chatDisclaimer(lang),
		})
		return
	}
	day := summaryDay()
	s.chatMu.Lock()
	if s.chatDayDate != day {
		s.chatDayDate = day
		s.chatDayCount = 0
	}
	over := s.chatDayCount >= chatDailyCap
	s.chatMu.Unlock()
	if over {
		writeJSON(w, http.StatusOK, map[string]any{
			"blocks": []chat.Block{{Kind: "text", Text: chatBusyNote(lang)}},
			"busy":   true,
		})
		return
	}

	hist, _ := s.store.ListChatMessages(r.Context(), convID, maxChatHistory)
	llmHist := storeMsgsToLLM(hist)
	if estTokens(llmHist) > maxThreadHistoryTokens {
		writeJSON(w, http.StatusOK, map[string]any{
			"blocks":      []chat.Block{{Kind: "text", Text: chatThreadFullNote(lang)}},
			"thread_full": true,
		})
		return
	}
	ans, err := s.chatSvc.Answer(r.Context(), anchorTicker, lang, llmHist, msg)
	if err != nil {
		switch {
		case errors.Is(err, chat.ErrNotFound):
			writeJSON(w, http.StatusNotFound, errBody("no data for "+anchorTicker))
		case errors.Is(err, enrich.ErrDisabled):
			writeJSON(w, http.StatusServiceUnavailable, errBody("chat unavailable"))
		default:
			s.log.Debug("chat answer failed", "conv", convID, "err", err)
			writeJSON(w, http.StatusBadGateway, errBody("chat failed — try again"))
		}
		return
	}

	blob, _ := json.Marshal(ans.Blocks)
	_ = s.store.AppendChatMessage(r.Context(), store.ChatMessage{ConversationID: convID, UserID: u.ID, Ticker: anchorTicker, Role: "user", Content: msg})
	_ = s.store.AppendChatMessage(r.Context(), store.ChatMessage{ConversationID: convID, UserID: u.ID, Ticker: anchorTicker, Role: "assistant", Content: string(blob)})
	if err := s.store.IncrChatMsgUsed(r.Context(), u.ID, period); err != nil {
		s.log.Debug("chat meter incr failed (non-fatal)", "user", u.ID, "err", err)
	}
	s.chatMu.Lock()
	s.chatDayCount++
	s.chatMu.Unlock()
	s.log.Info("chat", "ticker", anchorTicker, "conv", convID, "user", u.ID,
		"prompt_tokens", ans.Usage.PromptTokens, "cached_tokens", ans.Usage.CachedTokens, "completion_tokens", ans.Usage.CompletionTokens)

	writeJSON(w, http.StatusOK, map[string]any{
		"blocks":     ans.Blocks,
		"disclaimer": chatDisclaimer(lang),
		"meter":      map[string]int{"used": used + 1, "limit": s.chatMonthlyLimit},
	})
}

// findStockConv returns the user's existing conversation for a ticker WITHOUT creating
// one (used by read/reset paths so merely viewing a stock chat doesn't litter the list).
func (s *Server) findStockConv(ctx context.Context, userID, ticker string) string {
	convs, _ := s.store.ListConversations(ctx, userID)
	for _, c := range convs {
		if c.AnchorTicker == ticker {
			return c.ID
		}
	}
	return ""
}

// writeChatMessages renders a message slice to the wire shape (assistant → parsed blocks).
func writeChatMessages(w http.ResponseWriter, msgs []store.ChatMessage) {
	type wireMsg struct {
		Role   string       `json:"role"`
		Blocks []chat.Block `json:"blocks,omitempty"`
		Text   string       `json:"text,omitempty"`
		At     time.Time    `json:"at"`
	}
	out := make([]wireMsg, 0, len(msgs))
	for _, m := range msgs {
		wm := wireMsg{Role: m.Role, At: m.CreatedAt}
		if m.Role == "assistant" {
			var blocks []chat.Block
			if json.Unmarshal([]byte(m.Content), &blocks) == nil {
				wm.Blocks = blocks
			} else {
				wm.Text = m.Content
			}
		} else {
			wm.Text = m.Content
		}
		out = append(out, wm)
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": out})
}

// getChatHistory returns the user's stock thread (GET /v1/stocks/{ticker}/chat) — the
// user's own data, no Pro gate. Empty when the stock conversation doesn't exist yet.
func (s *Server) getChatHistory(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	convID := s.findStockConv(r.Context(), u.ID, ticker)
	if convID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"messages": []any{}})
		return
	}
	msgs, err := s.store.ListChatMessages(r.Context(), convID, 100)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeChatMessages(w, msgs)
}

// getConvHistory returns an explicit conversation's messages (GET
// /v1/conversations/{id}/chat). Ownership enforced (404 otherwise).
func (s *Server) getConvHistory(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	conv, found, err := s.store.GetConversation(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, errBody("conversation not found"))
		return
	}
	msgs, err := s.store.ListChatMessages(r.Context(), conv.ID, 100)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeChatMessages(w, msgs)
}

// deleteChat clears the user's stock thread (DELETE /v1/stocks/{ticker}/chat — the "new
// conversation" reset). No-op when the conversation doesn't exist.
func (s *Server) deleteChat(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	if convID := s.findStockConv(r.Context(), u.ID, ticker); convID != "" {
		if err := s.store.ClearChatMessages(r.Context(), convID); err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// convWire is the conversation list/detail wire shape.
type convWire struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	AnchorTicker string    `json:"anchor_ticker,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// getConversations lists the user's conversations (GET /v1/conversations), newest first.
func (s *Server) getConversations(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	convs, err := s.store.ListConversations(r.Context(), u.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	out := make([]convWire, 0, len(convs))
	for _, c := range convs {
		out = append(out, convWire{ID: c.ID, Title: c.Title, AnchorTicker: c.AnchorTicker, UpdatedAt: c.UpdatedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": out})
}

// postConversation creates a new conversation (POST /v1/conversations {title?,
// anchor_ticker?}). Pro-gated (it's a chat feature). For a stock anchor it reuses the
// existing per-(user,ticker) conversation.
func (s *Server) postConversation(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if !s.chatReady(w, r, u) {
		return
	}
	var req struct {
		Title        string `json:"title"`
		AnchorTicker string `json:"anchor_ticker"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req)
	anchor := strings.ToUpper(strings.TrimSpace(req.AnchorTicker))
	var id string
	var err error
	if anchor != "" {
		id, err = s.store.GetOrCreateStockConversation(r.Context(), u.ID, anchor)
	} else {
		title := strings.TrimSpace(req.Title)
		if title == "" {
			title = "New chat"
		}
		id, err = s.store.CreateConversation(r.Context(), u.ID, title, "")
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	conv, _, _ := s.store.GetConversation(r.Context(), u.ID, id)
	writeJSON(w, http.StatusOK, convWire{ID: conv.ID, Title: conv.Title, AnchorTicker: conv.AnchorTicker, UpdatedAt: conv.UpdatedAt})
}

// patchConversation renames a conversation (PATCH /v1/conversations/{id} {title}).
func (s *Server) patchConversation(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("bad request"))
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeJSON(w, http.StatusBadRequest, errBody("empty title"))
		return
	}
	if len(title) > 120 {
		title = title[:120]
	}
	if err := s.store.RenameConversation(r.Context(), u.ID, r.PathValue("id"), title); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// deleteConversation removes a conversation + its messages (DELETE /v1/conversations/{id}).
func (s *Server) deleteConversation(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteConversation(r.Context(), u.ID, r.PathValue("id")); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// storeMsgsToLLM maps persisted messages to the model history: user messages pass through
// as text; assistant messages contribute only their PROSE (widget refs are dropped — the
// model doesn't need them and they'd waste tokens). Empty messages are skipped.
func storeMsgsToLLM(msgs []store.ChatMessage) []enrich.ChatMessage {
	out := make([]enrich.ChatMessage, 0, len(msgs))
	for _, m := range msgs {
		content := m.Content
		if m.Role == "assistant" {
			content = assistantProse(m.Content)
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		out = append(out, enrich.ChatMessage{Role: m.Role, Content: content})
	}
	return out
}

// assistantProse extracts the joined text-block prose from a stored assistant message
// (JSON-encoded blocks); falls back to the raw content if it isn't block JSON.
func assistantProse(content string) string {
	var blocks []chat.Block
	if err := json.Unmarshal([]byte(content), &blocks); err != nil {
		return content
	}
	var b strings.Builder
	for _, bl := range blocks {
		if bl.Kind == "text" && bl.Text != "" {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(bl.Text)
		}
	}
	return b.String()
}

func chatLimitNote(lang string) string {
	if lang == "zh" {
		return "本月对话额度已用完,下月初重置。完整深度研报仍可查看。"
	}
	return "You've reached this month's chat limit — it resets next month. Your full deep reports are still available."
}

func chatBusyNote(lang string) string {
	if lang == "zh" {
		return "AI 对话暂时繁忙,请稍后再试。"
	}
	return "Chat is briefly at capacity — please try again shortly."
}

func chatThreadFullNote(lang string) string {
	if lang == "zh" {
		return "这个对话已经很长了 —— 点「新对话」开个新的,保持清晰也更省 token。"
	}
	return "This conversation is getting long — start a new one to keep it sharp (and cheaper)."
}

func chatDisclaimer(lang string) string {
	if lang == "zh" {
		return "AI 生成 · 数字来自公开数据并标注来源 · 非投资建议"
	}
	return "AI-generated · figures sourced from public data · not investment advice"
}
