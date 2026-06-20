package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/wombow-ai/tickwind/internal/chat"
	"github.com/wombow-ai/tickwind/internal/enrich"
	"github.com/wombow-ai/tickwind/internal/store"
)

// chatDailyCap bounds Product B chat generations per ET day across ALL users — a
// catastrophic-cost backstop on top of the per-user monthly meter + per-user throttle.
// A cached Haiku turn is ~$0.005, so 500/day caps worst-case spend at ~$2.5/day.
const chatDailyCap = 500

// maxChatHistory is how many recent messages (≈ turns×2) of a thread are replayed into
// the model context each turn (bounds input tokens; older turns drop off).
const maxChatHistory = 20

// SetChat injects the Product B chat engine (nil → the chat endpoint 503s).
func (s *Server) SetChat(svc *chat.Service) { s.chatSvc = svc }

// SetChatLimit sets the per-Pro-user monthly message soft-cap (<=0 ignored).
func (s *Server) SetChatLimit(n int) {
	if n > 0 {
		s.chatMonthlyLimit = n
	}
}

// postChat handles one Product B chat turn: POST /v1/stocks/{ticker}/chat with
// {message, lang?}. Pro-gated, per-user throttled, monthly-metered, and bounded by a
// global daily cap. On success it persists both turns, charges the meter, and returns
// the assistant's ordered blocks (prose + surfaced widgets) + a meter readout.
func (s *Server) postChat(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if s.chatSvc == nil || !s.chatSvc.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, errBody("chat unavailable"))
		return
	}
	// Pro gate — Product B is a whole-feature Pro unlock (the only real cost center).
	if s.tierOf(r.Context(), u.ID) != tierPro {
		writeJSON(w, http.StatusPaymentRequired, map[string]any{"error": "Tickwind Pro required", "upgrade": true})
		return
	}
	// Per-user burst throttle (atop the monthly meter).
	if !s.chatRL.allow(u.ID) {
		writeJSON(w, http.StatusTooManyRequests, errBody("too many messages — give it a moment"))
		return
	}

	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	if ticker == "" {
		writeJSON(w, http.StatusBadRequest, errBody("missing ticker"))
		return
	}
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
		lang = "en" // English-first default
	}

	// Per-user monthly meter (read fails OPEN → 0). Over → soft-degrade, not an error.
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
	// Global per-ET-day backstop.
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

	// Windowed history (most recent ~10 turns, oldest→newest).
	hist, _ := s.store.ListChatMessages(r.Context(), u.ID, ticker, maxChatHistory)
	ans, err := s.chatSvc.Answer(r.Context(), ticker, lang, storeMsgsToLLM(hist), msg)
	if err != nil {
		switch {
		case errors.Is(err, chat.ErrNotFound):
			writeJSON(w, http.StatusNotFound, errBody("no data for "+ticker))
		case errors.Is(err, enrich.ErrDisabled):
			writeJSON(w, http.StatusServiceUnavailable, errBody("chat unavailable"))
		default:
			s.log.Debug("chat answer failed", "ticker", ticker, "err", err)
			writeJSON(w, http.StatusBadGateway, errBody("chat failed — try again"))
		}
		return
	}

	// Success: persist BOTH turns, charge the meter, count the global slot.
	blob, _ := json.Marshal(ans.Blocks)
	_ = s.store.AppendChatMessage(r.Context(), store.ChatMessage{UserID: u.ID, Ticker: ticker, Role: "user", Content: msg})
	_ = s.store.AppendChatMessage(r.Context(), store.ChatMessage{UserID: u.ID, Ticker: ticker, Role: "assistant", Content: string(blob)})
	if err := s.store.IncrChatMsgUsed(r.Context(), u.ID, period); err != nil {
		s.log.Debug("chat meter incr failed (non-fatal)", "user", u.ID, "err", err)
	}
	s.chatMu.Lock()
	s.chatDayCount++
	s.chatMu.Unlock()
	s.log.Info("chat", "ticker", ticker, "user", u.ID,
		"prompt_tokens", ans.Usage.PromptTokens, "cached_tokens", ans.Usage.CachedTokens, "completion_tokens", ans.Usage.CompletionTokens)

	writeJSON(w, http.StatusOK, map[string]any{
		"blocks":     ans.Blocks,
		"disclaimer": chatDisclaimer(lang),
		"meter":      map[string]int{"used": used + 1, "limit": s.chatMonthlyLimit},
	})
}

// getChatHistory returns the user's persisted thread for a ticker (GET
// /v1/stocks/{ticker}/chat). The thread is the user's own data — no Pro gate (a
// non-subscriber simply has none). Assistant messages are returned as parsed blocks.
func (s *Server) getChatHistory(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	msgs, err := s.store.ListChatMessages(r.Context(), u.ID, ticker, 100)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
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

func chatDisclaimer(lang string) string {
	if lang == "zh" {
		return "AI 生成 · 数字来自公开数据并标注来源 · 非投资建议"
	}
	return "AI-generated · figures sourced from public data · not investment advice"
}
