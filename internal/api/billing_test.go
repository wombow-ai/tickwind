package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/billing"
	"github.com/wombow-ai/tickwind/internal/store"
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

// TestCheckoutRecoversEntitlement covers the out-of-order webhook fix: when
// checkout.session.completed carries a subscription id but no subscription.* event has
// bound the user yet (the dropped-event case), the checkout handler re-pulls the
// authoritative subscription from Stripe and grants Pro — instead of stranding a paying
// user on free.
func TestCheckoutRecoversEntitlement(t *testing.T) {
	stripe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/subscriptions/") {
			io.WriteString(w, `{"id":"sub_9","customer":"cus_9","status":"active","current_period_end":9999999999,`+
				`"cancel_at_period_end":false,"items":{"data":[{"price":{"id":"price_y","recurring":{"interval":"year"}}}]}}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer stripe.Close()

	st := memory.New()
	bsvc := billing.New(billing.Config{SecretKey: "sk_test_x", APIBaseURL: stripe.URL})
	s := &Server{store: st, log: slog.New(slog.NewTextHandler(io.Discard, nil)), billing: bsvc}
	ctx := context.Background()
	const user = "22222222-2222-2222-2222-222222222222"

	ev := billing.Event{ID: "evt_c", Type: "checkout.session.completed"}
	ev.Data.Object = json.RawMessage(`{"id":"cs_9","client_reference_id":"` + user + `","customer":"cus_9","subscription":"sub_9"}`)
	if err := s.handleStripeEvent(ctx, ev); err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if got := s.tierOf(ctx, user); got != tierPro {
		t.Fatalf("tier = %q, want pro (entitlement recovered from cs.Subscription)", got)
	}
	sub, _, _ := st.GetSubscription(ctx, user)
	if sub.Status != "active" || sub.PriceID != "price_y" || sub.Interval != "year" || sub.StripeSubscriptionID != "sub_9" {
		t.Fatalf("recovered subscription not synced: %+v", sub)
	}
}

// TestCheckoutBindsEvenIfRecoveryFails guards the (c) regression fix: if the entitlement
// re-pull from Stripe fails, the user↔customer BINDING must still persist (the user is
// bound as free, awaiting a later subscription.* event) — a recovery failure must never
// 5xx-loop and leave a paying user unbound.
func TestCheckoutBindsEvenIfRecoveryFails(t *testing.T) {
	stripe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError) // GET /v1/subscriptions/{id} fails
	}))
	defer stripe.Close()

	st := memory.New()
	bsvc := billing.New(billing.Config{SecretKey: "sk_test_x", APIBaseURL: stripe.URL})
	s := &Server{store: st, log: slog.New(slog.NewTextHandler(io.Discard, nil)), billing: bsvc}
	ctx := context.Background()
	const user = "44444444-4444-4444-4444-444444444444"

	ev := billing.Event{ID: "evt_cf", Type: "checkout.session.completed"}
	ev.Data.Object = json.RawMessage(`{"id":"cs_f","client_reference_id":"` + user + `","customer":"cus_f","subscription":"sub_f"}`)
	if err := s.handleStripeEvent(ctx, ev); err != nil {
		t.Fatalf("checkout must not error when recovery fails: %v", err)
	}
	sub, ok, _ := st.GetSubscription(ctx, user)
	if !ok || sub.StripeCustomerID != "cus_f" {
		t.Fatalf("binding lost when recovery failed: sub=%+v ok=%v", sub, ok)
	}
	if got := s.tierOf(ctx, user); got != tierFree {
		t.Fatalf("tier = %q, want free (recovery failed → awaits later subscription event)", got)
	}
}

// TestValidStripeCustomer guards the checkout/portal fix: only a real cus_ id may be
// reused as the Stripe customer; a manual/admin-grant placeholder must be rejected (it
// would 400 the Stripe API and 502 the checkout).
func TestValidStripeCustomer(t *testing.T) {
	for _, c := range []struct {
		id   string
		want bool
	}{
		{"cus_abc123", true},
		{"manual_admin_grant", false},
		{"", false},
		{"sub_xyz", false},
	} {
		if got := validStripeCustomer(c.id); got != c.want {
			t.Errorf("validStripeCustomer(%q) = %v, want %v", c.id, got, c.want)
		}
	}
}

// failUpsertStore makes UpsertSubscription fail on demand (to simulate a transient DB
// error during webhook processing).
type failUpsertStore struct {
	*memory.Store
	fail bool
}

func (f *failUpsertStore) UpsertSubscription(ctx context.Context, sub store.Subscription) error {
	if f.fail {
		return errors.New("transient db error")
	}
	return f.Store.UpsertSubscription(ctx, sub)
}

func signWebhook(secret, payload string) string {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "." + payload))
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

// TestWebhookReprocessesAfterTransientFailure covers the high-severity ordering fix: a
// handler failure must NOT record the event as seen, so Stripe's retry genuinely
// reprocesses it (the old record-FIRST order permanently lost the grant/revoke).
func TestWebhookReprocessesAfterTransientFailure(t *testing.T) {
	base := memory.New()
	fs := &failUpsertStore{Store: base}
	const secret = "whsec_test"
	bsvc := billing.New(billing.Config{SecretKey: "sk_test_x", WebhookSecret: secret})
	s := &Server{store: fs, log: slog.New(slog.NewTextHandler(io.Discard, nil)), billing: bsvc}
	ctx := context.Background()
	const user, cust = "33333333-3333-3333-3333-333333333333", "cus_t"
	if err := base.UpsertSubscription(ctx, store.Subscription{UserID: user, StripeCustomerID: cust, Tier: tierFree}); err != nil {
		t.Fatal(err)
	}

	payload := `{"id":"evt_s","type":"customer.subscription.created","data":{"object":{"id":"sub_1","customer":"` + cust +
		`","status":"active","current_period_end":9999999999,"items":{"data":[{"price":{"id":"price_m","recurring":{"interval":"month"}}}]}}}}`
	sig := signWebhook(secret, payload)
	post := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/stripe/webhook", strings.NewReader(payload))
		req.Header.Set("Stripe-Signature", sig)
		s.stripeWebhook(rec, req)
		return rec
	}

	// 1. First delivery — UpsertSubscription fails → 5xx, event NOT recorded.
	fs.fail = true
	if rec := post(); rec.Code != http.StatusInternalServerError {
		t.Fatalf("first delivery: want 500, got %d", rec.Code)
	}
	if seen, _ := base.StripeEventSeen(ctx, "evt_s"); seen {
		t.Fatal("event recorded despite handler failure → a retry would skip it (the bug)")
	}
	if got := s.tierOf(ctx, user); got != tierFree {
		t.Fatalf("tier after failed delivery = %q, want still free", got)
	}

	// 2. Stripe retries — now succeeds → processed + recorded + Pro granted.
	fs.fail = false
	if rec := post(); rec.Code != http.StatusOK {
		t.Fatalf("retry: want 200, got %d", rec.Code)
	}
	if got := s.tierOf(ctx, user); got != tierPro {
		t.Fatalf("retry should grant pro, tier = %q", got)
	}

	// 3. A duplicate re-delivery is skipped (idempotent ack, no reprocess needed).
	if rec := post(); rec.Code != http.StatusOK {
		t.Fatalf("duplicate: want 200, got %d", rec.Code)
	}
}
