package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/wombow-ai/tickwind/internal/billing"
	"github.com/wombow-ai/tickwind/internal/store/memory"
)

// TestHandleStripeEvent exercises the webhook event → entitlement mapping (the
// payment-correctness core) against a memory store: a checkout binds user↔customer,
// an active subscription grants Pro, deletion reverts to free, and an event for an
// unbound customer is tolerated (no error → 2xx).
func TestHandleStripeEvent(t *testing.T) {
	st := memory.New()
	s := &Server{store: st, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ctx := context.Background()
	const user, cust = "11111111-1111-1111-1111-111111111111", "cus_123"

	ev := func(id, typ, obj string) billing.Event {
		e := billing.Event{ID: id, Type: typ}
		e.Data.Object = json.RawMessage(obj)
		return e
	}

	// 1. checkout.session.completed binds the Supabase user to the Stripe customer
	// (tier stays free until a subscription event arrives).
	if err := s.handleStripeEvent(ctx, ev("evt_1", "checkout.session.completed",
		`{"id":"cs_1","client_reference_id":"`+user+`","customer":"`+cust+`"}`)); err != nil {
		t.Fatalf("checkout event: %v", err)
	}
	sub, ok, _ := st.GetSubscription(ctx, user)
	if !ok || sub.StripeCustomerID != cust {
		t.Fatalf("bind failed: sub=%+v ok=%v", sub, ok)
	}
	if got := s.tierOf(ctx, user); got != tierFree {
		t.Fatalf("tier before subscription = %q, want free", got)
	}

	// 2. customer.subscription.created (active) → Pro, with plan details synced.
	subObj := `{"id":"sub_1","customer":"` + cust + `","status":"active","current_period_end":9999999999,` +
		`"cancel_at_period_end":false,"items":{"data":[{"price":{"id":"price_m","recurring":{"interval":"month"}}}]}}`
	if err := s.handleStripeEvent(ctx, ev("evt_2", "customer.subscription.created", subObj)); err != nil {
		t.Fatalf("subscription created: %v", err)
	}
	if got := s.tierOf(ctx, user); got != tierPro {
		t.Fatalf("tier after active = %q, want pro", got)
	}
	sub, _, _ = st.GetSubscription(ctx, user)
	if sub.Status != "active" || sub.PriceID != "price_m" || sub.Interval != "month" || sub.StripeSubscriptionID != "sub_1" {
		t.Fatalf("subscription sync wrong: %+v", sub)
	}

	// 3. customer.subscription.deleted → free.
	if err := s.handleStripeEvent(ctx, ev("evt_3", "customer.subscription.deleted", subObj)); err != nil {
		t.Fatalf("subscription deleted: %v", err)
	}
	if got := s.tierOf(ctx, user); got != tierFree {
		t.Fatalf("tier after delete = %q, want free", got)
	}

	// 4. A subscription event for an unbound customer is tolerated (logged, no error).
	if err := s.handleStripeEvent(ctx, ev("evt_4", "customer.subscription.updated",
		`{"id":"sub_x","customer":"cus_unknown","status":"active"}`)); err != nil {
		t.Fatalf("unbound customer should not error: %v", err)
	}

	// 5. An ignored event type is a no-op (no error).
	if err := s.handleStripeEvent(ctx, ev("evt_5", "invoice.paid", `{}`)); err != nil {
		t.Fatalf("ignored event should not error: %v", err)
	}
}
