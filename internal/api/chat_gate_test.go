package api

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/store/memory"
)

// TestChatQuotaGate_FreeAndPro: both tiers are TOKEN-based — Pro on a per-MONTH budget (with
// the sidebar meter), signed-in FREE users on a small per-WEEK token taste (no meter), then
// an upgrade nudge. Verifies the period + limit each tier is gated on.
func TestChatQuotaGate_FreeAndPro(t *testing.T) {
	st := memory.New()
	ctx := context.Background()
	s := &Server{store: st, chatFreeWeeklyTokens: 3000, chatMonthlyTokenLimit: 1000, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	week, month := researchWeek(), researchMonth()

	// FREE user (no subscription): gated on the WEEKLY token bucket.
	note, blocked, pro, period, _, limit := s.chatQuotaGate(ctx, "free", "en")
	if pro || blocked || period != week || limit != 3000 {
		t.Fatalf("free under cap: pro=%v blocked=%v period=%q limit=%d; want free, weekly, 3000", pro, blocked, period, limit)
	}
	_ = st.IncrChatTokensUsed(ctx, "free", week, 3000) // hit the weekly cap
	note, blocked, pro, _, _, _ = s.chatQuotaGate(ctx, "free", "en")
	if pro || !blocked {
		t.Fatalf("free over weekly cap: pro=%v blocked=%v; want free + blocked", pro, blocked)
	}
	if !strings.Contains(strings.ToLower(note), "upgrade") {
		t.Fatalf("free block note should nudge upgrade: %q", note)
	}

	// PRO user: gated on the MONTHLY token budget — the free user's weekly bucket is separate.
	_ = st.UpsertSubscription(ctx, store.Subscription{UserID: "pro", Status: "active", Tier: tierPro})
	_, blocked, pro, period, used, limit := s.chatQuotaGate(ctx, "pro", "en")
	if !pro || blocked || period != month || limit != 1000 || used != 0 {
		t.Fatalf("pro under cap: pro=%v blocked=%v period=%q limit=%d used=%d; want pro, monthly, 1000, 0", pro, blocked, period, limit, used)
	}
	_ = st.IncrChatTokensUsed(ctx, "pro", month, 1000) // hit the monthly cap
	_, blocked, pro, _, used, _ = s.chatQuotaGate(ctx, "pro", "en")
	if !pro || !blocked || used < 1000 {
		t.Fatalf("pro over token cap: pro=%v blocked=%v used=%d; want pro + blocked + used>=1000", pro, blocked, used)
	}
}

// TestQuotaResetHelpers locks the quota-window reset timestamps the chat usage meter exposes:
// the monthly reset is the 1st of a future month; the weekly reset is a future Monday within 7d.
func TestQuotaResetHelpers(t *testing.T) {
	now := time.Now()
	m := nextMonthResetET()
	if m.Day() != 1 {
		t.Errorf("monthly reset not on the 1st: %v", m)
	}
	if !m.After(now) {
		t.Errorf("monthly reset not in the future: %v", m)
	}
	w := nextWeekResetET()
	if w.Weekday() != time.Monday {
		t.Errorf("weekly reset not a Monday: %v", w)
	}
	if !w.After(now) || w.After(now.Add(7*24*time.Hour+time.Hour)) {
		t.Errorf("weekly reset not within the next 7 days: %v (now %v)", w, now)
	}
}
