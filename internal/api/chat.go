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

// chatTurnTimeout bounds the WHOLE chat turn (the ≤5 sequential LLM round-trips of the
// tool loop). Every other AI surface wraps its LLM call in a deadline; chat was the only
// unbounded one — without this a stalled upstream could hold a goroutine + LLM slot for
// ~5×150s. 90s covers a normal multi-tool turn (Haiku) with headroom; past it the handler
// returns a clean "try again" rather than hanging.
const chatTurnTimeout = 90 * time.Second

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

// SetChatTokenLimit sets the per-Pro-user monthly TOKEN soft-cap (<=0 ignored).
func (s *Server) SetChatTokenLimit(n int) {
	if n > 0 {
		s.chatMonthlyTokenLimit = n
	}
}

// SetChatFreeWeeklyTokens sets the per-FREE-user WEEKLY token soft-cap (<=0 ignored).
func (s *Server) SetChatFreeWeeklyTokens(n int) {
	if n > 0 {
		s.chatFreeWeeklyTokens = n
	}
}

// chatQuotaGate enforces the per-user chat quota — TIER-AWARE, both sides now TOKEN-based.
// Pro users are gated on the per-MONTH token limit (and get the sidebar meter); signed-in
// FREE users get a small per-WEEK token taste (chatFreeWeeklyTokens) and NO meter (the UI
// hides it). It returns the friendly note to send when over, whether to block, whether the
// user is Pro, the quota PERIOD (so the caller increments the right bucket), the pre-turn
// token count, and the limit (so the caller can report the post-turn meter for Pro).
func (s *Server) chatQuotaGate(ctx context.Context, userID, lang string) (note string, blocked, pro bool, period string, usedTokens, limit int) {
	pro = s.tierOf(ctx, userID) == tierPro
	if pro {
		period, limit = researchMonth(), s.chatMonthlyTokenLimit
	} else {
		period, limit = researchWeek(), s.chatFreeWeeklyTokens // weekly reset for the free taste
	}
	usedTokens, _ = s.store.GetChatTokensUsed(ctx, userID, period)
	if usedTokens >= limit {
		if pro {
			return chatLimitNote(lang), true, true, period, usedTokens, limit
		}
		return chatFreeLimitNote(lang), true, false, period, usedTokens, limit
	}
	return "", false, pro, period, usedTokens, limit
}

// chatReady checks the chat is enabled and the user is within the burst throttle (NOT a
// Pro gate — free users may chat with a small per-month quota, enforced per-turn). It
// writes the appropriate response + returns false on any failure.
func (s *Server) chatReady(w http.ResponseWriter, r *http.Request, u auth.User) bool {
	if s.chatSvc == nil || !s.chatSvc.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, errBody("chat unavailable"))
		return false
	}
	// NB: NO Pro gate here — signed-in FREE users may chat with a small per-month quota
	// (enforced per-turn by chatQuotaGate). requireUser still gates anonymous visitors.
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

// postConvChatStream is the SSE variant of postConvChat (POST /v1/conversations/{id}/chat/stream):
// same auth/Pro gate + conversation lookup, then streams the turn token-by-token.
func (s *Server) postConvChatStream(w http.ResponseWriter, r *http.Request) {
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
	s.chatTurnStream(w, r, u, conv.ID, conv.AnchorTicker)
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

	note, blocked, pro, period, usedTokens, limit := s.chatQuotaGate(r.Context(), u.ID, lang)
	if blocked {
		resp := map[string]any{
			"blocks":        []chat.Block{{Kind: "text", Text: note}},
			"limit_reached": true,
			"upgrade":       !pro,
			"disclaimer":    chatDisclaimer(lang),
		}
		if pro { // free users get NO meter (the UI hides their quota)
			resp["meter"] = map[string]int{"used": usedTokens, "limit": limit}
		}
		writeJSON(w, http.StatusOK, resp)
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
	ctx, cancel := context.WithTimeout(r.Context(), chatTurnTimeout)
	defer cancel()
	ans, err := s.chatSvc.Answer(ctx, u.ID, anchorTicker, lang, llmHist, msg, s.chatPersonalDataAllowed(ctx, u.ID))
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
	// Auto-title a GENERAL conversation from its first user message, so the hub sidebar
	// shows a meaningful name instead of the default "New chat". First message only
	// (len(hist)==0); stock conversations are already named by their ticker.
	if anchorTicker == "" && len(hist) == 0 {
		if title := deriveChatTitle(msg); title != "" {
			_ = s.store.RenameConversation(r.Context(), u.ID, convID, title)
		}
	}
	if err := s.store.IncrChatMsgUsed(r.Context(), u.ID, period); err != nil {
		s.log.Debug("chat meter incr failed (non-fatal)", "user", u.ID, "err", err)
	}
	if err := s.store.IncrChatTokensUsed(r.Context(), u.ID, period, ans.Usage.TotalTokens); err != nil {
		s.log.Debug("chat token meter incr failed (non-fatal)", "user", u.ID, "err", err)
	}
	s.chatMu.Lock()
	s.chatDayCount++
	s.chatMu.Unlock()
	s.log.Info("chat", "ticker", anchorTicker, "conv", convID, "user", u.ID,
		"prompt_tokens", ans.Usage.PromptTokens, "cached_tokens", ans.Usage.CachedTokens, "completion_tokens", ans.Usage.CompletionTokens)

	resp := map[string]any{"blocks": ans.Blocks, "disclaimer": chatDisclaimer(lang)}
	if pro { // Pro sees the token meter; free users' quota is hidden in the UI
		resp["meter"] = map[string]int{"used": usedTokens + ans.Usage.TotalTokens, "limit": limit}
	}
	writeJSON(w, http.StatusOK, resp)
}

// getChatUsage returns the signed-in user's current monthly chat token usage, so the hub
// can show the quota bar immediately on load (the per-turn meter only arrives with a reply).
func (s *Server) getChatUsage(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	used, _ := s.store.GetChatTokensUsed(r.Context(), u.ID, researchMonth())
	writeJSON(w, http.StatusOK, map[string]int{"used": used, "limit": s.chatMonthlyTokenLimit})
}

// chatTurnStream is the SSE (streaming) variant of chatTurn: it streams the final answer's
// content tokens as they generate (a "token" event each), then sends a terminal "done" event
// carrying the AUTHORITATIVE advice-filtered blocks + meter (the client reconciles the
// streamed text with these — the anti-hallucination contract is unchanged: Go owns every
// number, finish() ran the advice filter). Same gating + persistence + metering as chatTurn.
func (s *Server) chatTurnStream(w http.ResponseWriter, r *http.Request, u auth.User, convID, anchorTicker string) {
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

	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering so tokens flush live
	send := func(payload any) {
		b, _ := json.Marshal(payload)
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(b)
		_, _ = w.Write([]byte("\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}

	note, blocked, pro, period, usedTokens, limit := s.chatQuotaGate(r.Context(), u.ID, lang)
	if blocked {
		done := map[string]any{"type": "done", "blocks": []chat.Block{{Kind: "text", Text: note}}, "limit_reached": true, "upgrade": !pro, "disclaimer": chatDisclaimer(lang)}
		if pro {
			done["meter"] = map[string]int{"used": usedTokens, "limit": limit}
		}
		send(done)
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
		send(map[string]any{"type": "done", "blocks": []chat.Block{{Kind: "text", Text: chatBusyNote(lang)}}, "busy": true})
		return
	}
	hist, _ := s.store.ListChatMessages(r.Context(), convID, maxChatHistory)
	llmHist := storeMsgsToLLM(hist)
	if estTokens(llmHist) > maxThreadHistoryTokens {
		send(map[string]any{"type": "done", "blocks": []chat.Block{{Kind: "text", Text: chatThreadFullNote(lang)}}, "thread_full": true})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), chatTurnTimeout)
	defer cancel()
	ans, err := s.chatSvc.AnswerStream(ctx, u.ID, anchorTicker, lang, llmHist, msg, s.chatPersonalDataAllowed(ctx, u.ID), func(tok string) {
		send(map[string]any{"type": "token", "text": tok})
	})
	if err != nil {
		s.log.Debug("chat stream failed", "conv", convID, "err", err)
		send(map[string]any{"type": "error"})
		return
	}

	blob, _ := json.Marshal(ans.Blocks)
	_ = s.store.AppendChatMessage(r.Context(), store.ChatMessage{ConversationID: convID, UserID: u.ID, Ticker: anchorTicker, Role: "user", Content: msg})
	_ = s.store.AppendChatMessage(r.Context(), store.ChatMessage{ConversationID: convID, UserID: u.ID, Ticker: anchorTicker, Role: "assistant", Content: string(blob)})
	if anchorTicker == "" && len(hist) == 0 {
		if title := deriveChatTitle(msg); title != "" {
			_ = s.store.RenameConversation(r.Context(), u.ID, convID, title)
		}
	}
	if err := s.store.IncrChatMsgUsed(r.Context(), u.ID, period); err != nil {
		s.log.Debug("chat meter incr failed (non-fatal)", "user", u.ID, "err", err)
	}
	if err := s.store.IncrChatTokensUsed(r.Context(), u.ID, period, ans.Usage.TotalTokens); err != nil {
		s.log.Debug("chat token meter incr failed (non-fatal)", "user", u.ID, "err", err)
	}
	s.chatMu.Lock()
	s.chatDayCount++
	s.chatMu.Unlock()
	s.log.Info("chat", "ticker", anchorTicker, "conv", convID, "user", u.ID, "stream", true,
		"prompt_tokens", ans.Usage.PromptTokens, "cached_tokens", ans.Usage.CachedTokens, "completion_tokens", ans.Usage.CompletionTokens)

	done := map[string]any{"type": "done", "blocks": ans.Blocks, "disclaimer": chatDisclaimer(lang)}
	if pro {
		done["meter"] = map[string]int{"used": usedTokens + ans.Usage.TotalTokens, "limit": limit}
	}
	send(done)
}

// deriveChatTitle makes a short conversation title from the first user message: the
// first non-empty line, trimmed, capped at ~48 runes (rune-safe for CJK).
func deriveChatTitle(msg string) string {
	title := strings.TrimSpace(msg)
	if i := strings.IndexAny(title, "\r\n"); i >= 0 {
		title = strings.TrimSpace(title[:i])
	}
	if r := []rune(title); len(r) > 48 {
		title = strings.TrimSpace(string(r[:48])) + "…"
	}
	return title
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

// chatPersonalDataAllowed reads the user's privacy pref: the chat reads their own
// watchlist/holdings/notes UNLESS they opted out (prefs.chat_personal_data == false).
// Defaults ON (absent pref / read error → true) — reading your own data in your own chat.
func (s *Server) chatPersonalDataAllowed(ctx context.Context, userID string) bool {
	blob, found, err := s.store.GetPrefs(ctx, userID)
	if err != nil || !found {
		return true
	}
	var p struct {
		ChatPersonalData *bool `json:"chat_personal_data"`
	}
	if json.Unmarshal(blob, &p) != nil || p.ChatPersonalData == nil {
		return true
	}
	return *p.ChatPersonalData
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

// chatFreeLimitNote is shown to a signed-in FREE user who has used up their small monthly
// taste of the chat — a friendly nudge to upgrade (the meter itself is hidden for free users).
func chatFreeLimitNote(lang string) string {
	if lang == "zh" {
		return "你已用完本周的免费 AI 对话额度,下周一重置。升级 Tickwind Pro 即可畅聊,并解锁完整 AI 深度研报、指标与提醒。"
	}
	return "You've used up this week's free AI chat (it resets Monday). Upgrade to Tickwind Pro for unlimited chat — plus the full AI deep-research reports, indicators, and alerts."
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
