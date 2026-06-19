package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/wombow-ai/tickwind/internal/billing"
)

// SetBilling injects the Stripe billing service post-New. nil-safe: with no service
// (or a keyless one) every billing/webhook handler serves 404, so the surface is
// inert until Stripe is configured — exactly like the current keyless production.
func (s *Server) SetBilling(svc *billing.Service) { s.billing = svc }

// billingLive reports whether the Stripe billing surface is configured + enabled.
func (s *Server) billingLive() bool { return s.billing != nil && s.billing.Enabled() }

// maxWebhookBody caps the webhook payload we read (Stripe events are small).
const maxWebhookBody = 1 << 20 // 1 MiB

// stripeWebhook ingests Stripe webhook events. It reads the RAW body (before any
// parse) to verify the signature, dedups via the idempotency ledger, then applies
// the event to the subscriptions table. Status semantics for Stripe's retry: 400 on
// an unverifiable/garbage request (don't retry); 5xx on an internal failure (Stripe
// retries for ~3 days); 2xx on success OR a duplicate OR an ignored event type.
func (s *Server) stripeWebhook(w http.ResponseWriter, r *http.Request) {
	if s.billing == nil || !s.billing.WebhookEnabled() {
		writeJSON(w, http.StatusNotFound, errBody("not found"))
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBody))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("read body"))
		return
	}
	if err := s.billing.VerifyWebhook(body, r.Header.Get("Stripe-Signature"), time.Now()); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("bad signature"))
		return
	}
	ev, err := billing.ParseEvent(body)
	if err != nil || ev.ID == "" {
		writeJSON(w, http.StatusBadRequest, errBody("parse event"))
		return
	}
	// Idempotency: Stripe delivers at-least-once + out of order. Record-first; a
	// duplicate is acknowledged 200 without reprocessing.
	fresh, err := s.store.MarkStripeEventSeen(r.Context(), ev.ID, ev.Type)
	if err != nil {
		s.log.Error("stripe webhook: idempotency write failed", "event", ev.ID, "err", err)
		w.WriteHeader(http.StatusInternalServerError) // 5xx → Stripe retries
		return
	}
	if !fresh {
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := s.handleStripeEvent(r.Context(), ev); err != nil {
		s.log.Error("stripe webhook: handle failed", "type", ev.Type, "event", ev.ID, "err", err)
		w.WriteHeader(http.StatusInternalServerError) // 5xx → Stripe retries
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleStripeEvent applies a verified event to the subscriptions table. Unknown
// event types are ignored (nil → 2xx). Returns an error only on an internal/store
// failure (so the webhook 5xxes and Stripe retries).
func (s *Server) handleStripeEvent(ctx context.Context, ev billing.Event) error {
	switch ev.Type {
	case "checkout.session.completed":
		var cs billing.CheckoutSession
		if err := json.Unmarshal(ev.Data.Object, &cs); err != nil {
			return err
		}
		if cs.ClientReferenceID == "" || cs.Customer == "" {
			return nil // nothing to bind
		}
		// Bind the Supabase user ↔ Stripe customer. Preserve any tier already set by
		// an out-of-order subscription.* event; the subscription.* sync sets tier.
		sub, _, err := s.store.GetSubscription(ctx, cs.ClientReferenceID)
		if err != nil {
			return err
		}
		sub.UserID = cs.ClientReferenceID
		sub.StripeCustomerID = cs.Customer
		if sub.Tier == "" {
			sub.Tier = tierFree
		}
		return s.store.UpsertSubscription(ctx, sub)

	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted":
		var ss billing.Subscription
		if err := json.Unmarshal(ev.Data.Object, &ss); err != nil {
			return err
		}
		existing, ok, err := s.store.GetSubscriptionByCustomer(ctx, ss.Customer)
		if err != nil {
			return err
		}
		if !ok {
			// No user bound yet (subscription.* arrived before checkout.session.completed).
			// We can't map customer → user; ack 2xx (checkout.completed will bind, and a
			// later subscription.updated re-syncs) rather than 5xx-loop forever.
			s.log.Warn("stripe webhook: subscription event for unbound customer", "customer", ss.Customer, "type", ev.Type)
			return nil
		}
		existing.StripeSubscriptionID = ss.ID
		existing.Status = ss.Status
		existing.Tier = subTier(ss.Status)
		existing.PriceID = ss.PriceID()
		existing.Interval = ss.Interval()
		existing.CancelAtPeriodEnd = ss.CancelAtPeriodEnd
		if ss.CurrentPeriodEnd > 0 {
			existing.CurrentPeriodEnd = time.Unix(ss.CurrentPeriodEnd, 0)
		}
		if ev.Type == "customer.subscription.deleted" {
			existing.Status = "canceled"
			existing.Tier = tierFree
		}
		return s.store.UpsertSubscription(ctx, existing)
	}
	return nil // ignored event type
}

// subTier derives the entitlement tier from a Stripe subscription status.
func subTier(status string) string {
	switch status {
	case "active", "trialing":
		return tierPro
	default:
		return tierFree
	}
}

// billingCheckout (POST /v1/billing/checkout?interval=month|year) starts a Stripe
// Checkout session for the logged-in user and returns its hosted URL.
func (s *Server) billingCheckout(w http.ResponseWriter, r *http.Request) {
	if !s.billingLive() {
		writeJSON(w, http.StatusNotFound, errBody("not found"))
		return
	}
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	interval := r.URL.Query().Get("interval")
	if interval == "" {
		interval = "month"
	}
	if s.billing.PriceID(interval) == "" {
		writeJSON(w, http.StatusBadRequest, errBody("unknown plan"))
		return
	}
	// Reuse the user's existing Stripe customer when known (avoids duplicate customers).
	customerID := ""
	if sub, found, _ := s.store.GetSubscription(r.Context(), u.ID); found {
		customerID = sub.StripeCustomerID
	}
	url, err := s.billing.Checkout(r.Context(), u.ID, customerID, interval)
	if err != nil {
		s.log.Error("billing: checkout failed", "user", u.ID, "err", err)
		writeJSON(w, http.StatusBadGateway, errBody("checkout unavailable"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

// billingPortal (POST /v1/billing/portal) opens the Stripe Billing Portal for the
// logged-in user to manage/cancel — requires an existing Stripe customer.
func (s *Server) billingPortal(w http.ResponseWriter, r *http.Request) {
	if !s.billingLive() {
		writeJSON(w, http.StatusNotFound, errBody("not found"))
		return
	}
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	sub, found, err := s.store.GetSubscription(r.Context(), u.ID)
	if err != nil || !found || sub.StripeCustomerID == "" {
		writeJSON(w, http.StatusConflict, errBody("no subscription to manage"))
		return
	}
	url, err := s.billing.Portal(r.Context(), sub.StripeCustomerID)
	if err != nil {
		s.log.Error("billing: portal failed", "user", u.ID, "err", err)
		writeJSON(w, http.StatusBadGateway, errBody("portal unavailable"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

// billingMe (GET /v1/billing/me) reports the logged-in user's entitlement.
func (s *Server) billingMe(w http.ResponseWriter, r *http.Request) {
	if !s.billingLive() {
		writeJSON(w, http.StatusNotFound, errBody("not found"))
		return
	}
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	resp := map[string]any{"tier": s.tierOf(r.Context(), u.ID)}
	if sub, found, _ := s.store.GetSubscription(r.Context(), u.ID); found {
		resp["cancel_at_period_end"] = sub.CancelAtPeriodEnd
		if !sub.CurrentPeriodEnd.IsZero() {
			resp["current_period_end"] = sub.CurrentPeriodEnd.UTC().Format(time.RFC3339)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}
