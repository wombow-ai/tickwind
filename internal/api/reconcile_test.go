package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/billing"
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/store/memory"
)

// sub builds one Stripe subscription JSON object for the mock list response.
func sub(id, customer, status, price, interval string) string {
	return `{"id":"` + id + `","customer":"` + customer + `","status":"` + status + `",` +
		`"current_period_end":9999999999,"cancel_at_period_end":false,` +
		`"items":{"data":[{"price":{"id":"` + price + `","recurring":{"interval":"` + interval + `"}}}]}}`
}

// TestReconcileSubscriptions covers the safety-backstop reconciler: it corrects a stored
// tier that drifted from Stripe (a missed cancel and a missed activation), skips an
// unbound customer, leaves an already-correct row untouched, and — when a customer has
// several subscriptions — syncs to the authoritative (active) one, not a stale canceled.
func TestReconcileSubscriptions(t *testing.T) {
	body := `{"object":"list","has_more":false,"data":[` + strings.Join([]string{
		sub("sub_a", "cus_a", "canceled", "price_m", "month"),     // A: Stripe canceled (we think Pro)
		sub("sub_b", "cus_b", "active", "price_m", "month"),       // B: matches our row → no write
		sub("sub_c", "cus_c", "active", "price_m", "month"),       // C: no bound user → skip
		sub("sub_d_old", "cus_d", "canceled", "price_m", "month"), // D: stale canceled ...
		sub("sub_d_new", "cus_d", "active", "price_y", "year"),    // ... + live active (must win)
	}, ",") + `]}`
	stripe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/subscriptions" {
			io.WriteString(w, body)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer stripe.Close()

	st := memory.New()
	ctx := context.Background()
	// A: we wrongly think Pro (missed the cancel).
	_ = st.UpsertSubscription(ctx, store.Subscription{UserID: "userA", StripeCustomerID: "cus_a", StripeSubscriptionID: "sub_a", Status: "active", Tier: tierPro, PriceID: "price_m", Interval: "month"})
	// B: already correct (active/pro, matching plan) — includes the period end a
	// webhook-written row carries, so it must NOT register as drift.
	_ = st.UpsertSubscription(ctx, store.Subscription{UserID: "userB", StripeCustomerID: "cus_b", StripeSubscriptionID: "sub_b", Status: "active", Tier: tierPro, PriceID: "price_m", Interval: "month", CurrentPeriodEnd: time.Unix(9999999999, 0)})
	// D: we wrongly think free (missed the new activation).
	_ = st.UpsertSubscription(ctx, store.Subscription{UserID: "userD", StripeCustomerID: "cus_d", StripeSubscriptionID: "sub_d_old", Status: "canceled", Tier: tierFree})
	// E: we wrongly think Pro, but cus_e has NO subscription in Stripe at all (a
	// permanently-missed cancel) — the reverse pass must revoke it.
	_ = st.UpsertSubscription(ctx, store.Subscription{UserID: "userE", StripeCustomerID: "cus_e", StripeSubscriptionID: "sub_e", Status: "active", Tier: tierPro, PriceID: "price_m", Interval: "month", CurrentPeriodEnd: time.Unix(9999999999, 0)})

	s := &Server{
		store:   st,
		billing: billing.New(billing.Config{SecretKey: "sk_test_x", APIBaseURL: stripe.URL}),
		log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	checked, synced, skipped, err := s.reconcileSubscriptions(ctx)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if checked != 5 {
		t.Errorf("checked = %d, want 5", checked)
	}
	if synced != 3 { // A (→free, forward) + D (→pro, forward) + E (→free, reverse); B unchanged
		t.Errorf("synced = %d, want 3 (A + D + E)", synced)
	}
	if skipped != 1 { // cus_c unbound
		t.Errorf("skipped = %d, want 1 (unbound cus_c)", skipped)
	}

	if got := s.tierOf(ctx, "userA"); got != tierFree {
		t.Errorf("userA tier = %q, want free (Stripe canceled)", got)
	}
	if got := s.tierOf(ctx, "userB"); got != tierPro {
		t.Errorf("userB tier = %q, want pro (unchanged)", got)
	}
	if got := s.tierOf(ctx, "userD"); got != tierPro {
		t.Errorf("userD tier = %q, want pro (synced to the live active sub, not the stale canceled)", got)
	}
	// D synced to the authoritative active sub's id + annual plan.
	if d, _, _ := st.GetSubscription(ctx, "userD"); d.StripeSubscriptionID != "sub_d_new" || d.Interval != "year" {
		t.Errorf("userD not synced to the active sub: %+v", d)
	}
	// E revoked by the reverse pass (no Stripe subscription for its customer).
	if got := s.tierOf(ctx, "userE"); got != tierFree {
		t.Errorf("userE tier = %q, want free (no Stripe sub → reverse-pass revoke)", got)
	}
}

// TestReconcileNoBillingNoop confirms the reconciler is inert without a Stripe key.
func TestReconcileNoBillingNoop(t *testing.T) {
	s := &Server{store: memory.New(), billing: billing.New(billing.Config{}), log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	checked, synced, skipped, err := s.reconcileSubscriptions(context.Background())
	if err != nil || checked != 0 || synced != 0 || skipped != 0 {
		t.Fatalf("disabled billing should be a no-op, got checked=%d synced=%d skipped=%d err=%v", checked, synced, skipped, err)
	}
}
