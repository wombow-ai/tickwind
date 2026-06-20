package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wombow-ai/tickwind/internal/billing"
	"github.com/wombow-ai/tickwind/internal/store"
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
	// Idempotency: Stripe delivers at-least-once + out of order. PROCESS-then-record:
	// check (read-only) whether the id was already handled; if so, ack 200 (dup). Only
	// after handleStripeEvent SUCCEEDS do we record it as seen. This way a transient
	// store failure during handling leaves the id unrecorded → the 5xx makes Stripe
	// retry and the retry genuinely reprocesses (the old record-FIRST order would mark
	// it seen, then a handling failure was permanently lost — a paid grant/revoke gone).
	// INVARIANT: this read-precheck is an optimization, NOT an atomic gate — concurrent
	// deliveries of the same event can both pass it and both process. That stays correct
	// ONLY because every handler is an absolute-state UpsertSubscription (re-applying the
	// same event converges to the same row). A future read-modify-increment handler would
	// need its own atomicity, not this precheck.
	seen, err := s.store.StripeEventSeen(r.Context(), ev.ID)
	if err != nil {
		s.log.Error("stripe webhook: idempotency read failed", "event", ev.ID, "err", err)
		w.WriteHeader(http.StatusInternalServerError) // 5xx → Stripe retries
		return
	}
	if seen {
		w.WriteHeader(http.StatusOK) // already processed
		return
	}
	if err := s.handleStripeEvent(r.Context(), ev); err != nil {
		s.log.Error("stripe webhook: handle failed", "type", ev.Type, "event", ev.ID, "err", err)
		w.WriteHeader(http.StatusInternalServerError) // 5xx → Stripe retries (NOT recorded → reprocesses)
		return
	}
	if _, err := s.store.MarkStripeEventSeen(r.Context(), ev.ID, ev.Type); err != nil {
		// Processed OK but failed to record the idempotency ledger. Return 5xx so Stripe
		// RETRIES until the event is durably recorded: the retry reprocesses (absolute-state
		// UpsertSubscription → harmless) and re-marks. Leaving it unrecorded-but-processed
		// would let a later re-delivery replay it OUT OF ORDER against newer state (re-grant
		// a canceled user, or revoke a re-subscribed one).
		s.log.Error("stripe webhook: processed but idempotency write failed — 5xx to force re-record", "event", ev.ID, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
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
		// Bind the Supabase user ↔ Stripe customer.
		sub, _, err := s.store.GetSubscription(ctx, cs.ClientReferenceID)
		if err != nil {
			return err
		}
		sub.UserID = cs.ClientReferenceID
		sub.StripeCustomerID = cs.Customer
		if sub.Tier == "" {
			sub.Tier = tierFree
		}
		// Recover entitlement from the session's own subscription. A customer.subscription.*
		// event can arrive BEFORE this checkout (Stripe does not order them) and gets dropped
		// (no user↔customer binding existed yet) — which would strand a paying user on free
		// until some later, unguaranteed event. Re-pull the authoritative subscription state
		// here so the grant never depends on event ordering.
		// Best-effort entitlement recovery: re-pull the authoritative subscription so an
		// out-of-order subscription.* event that was dropped (before this binding existed)
		// can't strand a paying user on free. A recovery failure must NEVER block the
		// user↔customer BINDING itself — on any error we log + skip recovery and still bind
		// (as free; a later subscription.* event or the reconciliation backstop grants Pro),
		// rather than 5xx-looping with the user left unbound. Customer-match is a
		// defense-in-depth guard (never grant Pro from a subscription that isn't this
		// session's customer, even though a verified webhook already guarantees the linkage).
		if cs.Subscription != "" && s.billing != nil && s.billing.Enabled() {
			if ss, err := s.billing.GetSubscription(ctx, cs.Subscription); err != nil {
				s.log.Warn("stripe webhook: checkout entitlement re-pull failed — binding only", "checkout", cs.ID, "subscription", cs.Subscription, "err", err)
			} else if ss.Customer == "" || ss.Customer == cs.Customer {
				applyStripeSub(&sub, ss)
			} else {
				s.log.Warn("stripe webhook: checkout subscription customer mismatch — skipping recovery", "checkout", cs.ID, "subscription", cs.Subscription, "sub_customer", ss.Customer, "session_customer", cs.Customer)
			}
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
			// We can't map customer → user here; ack 2xx and let the checkout handler recover
			// the entitlement by re-pulling cs.Subscription (above), rather than 5xx-looping.
			s.log.Warn("stripe webhook: subscription event for unbound customer", "customer", ss.Customer, "type", ev.Type)
			return nil
		}
		applyStripeSub(&existing, ss)
		if ev.Type == "customer.subscription.deleted" {
			existing.Status = "canceled"
			existing.Tier = tierFree
		}
		return s.store.UpsertSubscription(ctx, existing)
	}
	return nil // ignored event type
}

// applyStripeSub copies a Stripe subscription's authoritative state (status → tier, plan,
// period, cancel flag) onto our stored row. Shared by the checkout-recovery path and the
// subscription.* sync so both derive entitlement identically.
func applyStripeSub(target *store.Subscription, ss billing.Subscription) {
	target.StripeSubscriptionID = ss.ID
	target.Status = ss.Status
	target.Tier = subTier(ss.Status)
	target.PriceID = ss.PriceID()
	target.Interval = ss.Interval()
	target.CancelAtPeriodEnd = ss.CancelAtPeriodEnd
	if pe := ss.PeriodEnd(); pe > 0 {
		target.CurrentPeriodEnd = time.Unix(pe, 0)
	}
}

// validStripeCustomer reports whether id is a real Stripe customer id (cus_…). Guards
// the checkout/portal handlers against manual/admin-grant placeholders (e.g.
// "manual_admin_grant") that would 400 the Stripe API.
func validStripeCustomer(id string) bool { return strings.HasPrefix(id, "cus_") }

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
	// Reuse the user's existing Stripe customer ONLY if it's a real Stripe id (cus_…).
	// A manual/admin-grant placeholder (or any non-Stripe value) must NOT be passed to
	// Stripe — it 400s "No such customer" and the checkout 502s. Pass "" so Stripe
	// creates a fresh customer; the webhook then binds it.
	customerID := ""
	if sub, found, _ := s.store.GetSubscription(r.Context(), u.ID); found && validStripeCustomer(sub.StripeCustomerID) {
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
	if err != nil || !found || !validStripeCustomer(sub.StripeCustomerID) {
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
