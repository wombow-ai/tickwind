package api

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/store/memory"
)

// TestChatQuotaGate_FreeAndPro: signed-in FREE users get a small message-count taste then an
// upgrade nudge (no meter); Pro users are gated on the cost-true TOKEN quota instead.
func TestChatQuotaGate_FreeAndPro(t *testing.T) {
	st := memory.New()
	ctx := context.Background()
	s := &Server{store: st, chatFreeLimit: 3, chatMonthlyTokenLimit: 1000, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	period := researchMonth()

	// FREE user (no subscription): allowed up to the small message cap, then blocked + upgrade note.
	for i := 0; i < 3; i++ {
		note, blocked, pro, _ := s.chatQuotaGate(ctx, "free", "en")
		if pro || blocked || note != "" {
			t.Fatalf("free turn %d: pro=%v blocked=%v note=%q; want free + not blocked", i, pro, blocked, note)
		}
		_ = st.IncrChatMsgUsed(ctx, "free", period)
	}
	note, blocked, pro, _ := s.chatQuotaGate(ctx, "free", "en")
	if pro || !blocked {
		t.Fatalf("free over-cap: pro=%v blocked=%v; want free + blocked", pro, blocked)
	}
	if !strings.Contains(strings.ToLower(note), "upgrade") {
		t.Fatalf("free block note should nudge upgrade: %q", note)
	}

	// PRO user: gated on TOKENS, not the small free message cap.
	_ = st.UpsertSubscription(ctx, store.Subscription{UserID: "pro", Status: "active", Tier: tierPro})
	for i := 0; i < 10; i++ { // many messages must NOT block a Pro user
		_ = st.IncrChatMsgUsed(ctx, "pro", period)
	}
	_, blocked, pro, used := s.chatQuotaGate(ctx, "pro", "en")
	if !pro || blocked {
		t.Fatalf("pro under token cap: pro=%v blocked=%v; want pro + not blocked", pro, blocked)
	}
	if used != 0 {
		t.Fatalf("pro pre-turn tokens = %d; want 0", used)
	}
	// Over the token cap → blocked, with the token count surfaced for the meter.
	_ = st.IncrChatTokensUsed(ctx, "pro", period, 1000)
	_, blocked, pro, used = s.chatQuotaGate(ctx, "pro", "en")
	if !pro || !blocked || used < 1000 {
		t.Fatalf("pro over token cap: pro=%v blocked=%v used=%d; want pro + blocked + used>=1000", pro, blocked, used)
	}
}
