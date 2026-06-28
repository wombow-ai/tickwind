package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/wombow-ai/tickwind/internal/auth"
	"github.com/wombow-ai/tickwind/internal/store"
)

// funnelEvents / funnelSurfaces are the CLOSED enums the public event endpoint accepts. Anything
// else is dropped silently (a 204, never stored) so the table can't be bloated with arbitrary
// strings by an abuser hitting the public POST. The per-IP rate limiter bounds volume further.
var funnelEvents = map[string]bool{
	"paywall_view":        true, // a user hit a Pro wall (surface = which one)
	"pro_view":            true, // the /pro page was viewed
	"checkout_started":    true, // clicked Subscribe → Stripe checkout
	"subscription_active": true, // a subscription went active (fired server-side from the webhook)
}
var funnelSurfaces = map[string]bool{
	"": true, "deep_research": true, "indicators": true, "screener": true,
	"backtest": true, "chat": true, "pro_page": true, "checkout": true, "webhook": true,
}

// postEvent records ONE first-party conversion-funnel event (POST /v1/event {event, surface, lang}).
// Public + fire-and-forget: anon visitors matter for the funnel, so it never requires auth and
// always 204s fast (a store error is swallowed — analytics must never break the UI). The user is
// attached + their current tier stamped when a valid bearer is present.
func (s *Server) postEvent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Event   string `json:"event"`
		Surface string `json:"surface"`
		Lang    string `json:"lang"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !funnelEvents[req.Event] { // unknown event → drop silently, don't store
		w.WriteHeader(http.StatusNoContent)
		return
	}
	surface := req.Surface
	if !funnelSurfaces[surface] {
		surface = ""
	}
	ev := store.FunnelEvent{Event: req.Event, Surface: surface}
	if req.Lang == "zh" {
		ev.Lang = "zh"
	} else {
		ev.Lang = "en"
	}
	if u, ok := auth.UserFrom(r.Context()); ok && u.ID != "" {
		ev.UserID = u.ID
		ev.Tier = s.tierOf(r.Context(), u.ID)
	}
	_ = s.store.SaveFunnelEvent(r.Context(), ev) // best-effort; never fail the client
	w.WriteHeader(http.StatusNoContent)
}

// getFunnel returns the conversion-funnel aggregate (GET /v1/admin/funnel?days=30) — admin-gated.
func (s *Server) getFunnel(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if !s.isAdmin(u) {
		writeJSON(w, http.StatusForbidden, errBody("admin only"))
		return
	}
	days := 30
	if d, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("days"))); err == nil && d > 0 {
		days = d
	}
	if days > 365 {
		days = 365
	}
	stats, err := s.store.FunnelSummary(r.Context(), days)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("funnel summary failed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": days, "stats": stats})
}
