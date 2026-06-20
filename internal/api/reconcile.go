package api

import (
	"context"
	"time"

	"github.com/wombow-ai/tickwind/internal/billing"
	"github.com/wombow-ai/tickwind/internal/store"
)

// billingReconcileInitialDelay staggers the first reconcile run off startup.
const billingReconcileInitialDelay = 2 * time.Minute

// RunBillingReconciler periodically re-syncs stored entitlements to Stripe's
// AUTHORITATIVE subscription state — a safety backstop for a missed or out-of-order
// webhook (the webhook is the primary path; this only catches what it dropped). It NEVER
// changes billing; it only corrects OUR DB when it has drifted from Stripe, and writes
// only on genuine drift. Runs until ctx is cancelled; no-op when billing is disabled.
func (s *Server) RunBillingReconciler(ctx context.Context, every time.Duration) {
	if s.billing == nil || !s.billing.Enabled() {
		return
	}
	if every <= 0 {
		every = 6 * time.Hour
	}
	timer := time.NewTimer(billingReconcileInitialDelay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		checked, synced, skipped, err := s.reconcileSubscriptions(ctx)
		if err != nil {
			s.log.Warn("billing reconcile failed", "err", err)
		} else {
			s.log.Info("billing reconcile", "checked", checked, "synced", synced, "skipped_unbound", skipped)
		}
		timer.Reset(every)
	}
}

// reconcileSubscriptions runs two passes against Stripe's authoritative state:
//   - forward: list every Stripe subscription, pick the authoritative one per customer,
//     and write our row only when an entitlement-relevant field drifted;
//   - reverse: revoke any user we still mark Pro whose customer has NO subscription in
//     the list at all (a permanently-missed cancel).
//
// Returns (subsChecked, rowsSynced, customersSkippedUnbound, err). Unbound customers (no
// user↔customer binding yet) are skipped in the forward pass — the checkout webhook binds
// them; the reconciler never invents a binding. It NEVER changes Stripe, only our DB.
func (s *Server) reconcileSubscriptions(ctx context.Context) (checked, synced, skipped int, err error) {
	if s.billing == nil || !s.billing.Enabled() {
		return 0, 0, 0, nil
	}
	subs, err := s.billing.ListSubscriptions(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	checked = len(subs)

	// One authoritative subscription per customer. A customer can hold several (an old
	// canceled + a current active); prefer the entitlement-granting one so a stale
	// canceled row can never override a live active subscription.
	best := map[string]billing.Subscription{}
	for _, sub := range subs {
		if sub.Customer == "" || sub.ID == "" {
			continue
		}
		if cur, ok := best[sub.Customer]; !ok || moreAuthoritative(sub, cur) {
			best[sub.Customer] = sub
		}
	}

	// Forward pass: sync each customer Stripe still lists.
	for customer, sub := range best {
		existing, found, gerr := s.store.GetSubscriptionByCustomer(ctx, customer)
		if gerr != nil {
			s.log.Warn("billing reconcile: lookup failed", "customer", customer, "err", gerr)
			continue
		}
		if !found {
			skipped++ // no user bound to this customer yet; the checkout webhook binds it
			continue
		}
		target := existing
		applyStripeSub(&target, sub) // identical mapping the webhook uses (status → tier, etc.)
		if subscriptionDrifted(existing, target) {
			if uerr := s.store.UpsertSubscription(ctx, target); uerr != nil {
				s.log.Warn("billing reconcile: upsert failed", "user", existing.UserID, "err", uerr)
				continue
			}
			synced++
			s.log.Info("billing reconcile: corrected drift", "user", existing.UserID, "status", target.Status, "tier", target.Tier)
		}
	}

	// Reverse pass: revoke any user we STILL mark Pro whose Stripe customer has NO
	// subscription at all in the (verified-complete) list — a permanently-missed cancel
	// the webhook never delivered (Stripe's 3-day retry elapsed, or the canceled sub aged
	// out of the account). Safe only because ListSubscriptions REFUSES a truncated list,
	// so an absent customer genuinely has no subscription rather than being off-page.
	proRows, perr := s.store.ListProSubscriptions(ctx)
	if perr != nil {
		s.log.Warn("billing reconcile: list pro subscriptions failed (skipping reverse pass)", "err", perr)
		return checked, synced, skipped, nil
	}
	for _, row := range proRows {
		if row.StripeCustomerID == "" {
			continue // never bound to a Stripe customer — nothing to reconcile against
		}
		if _, ok := best[row.StripeCustomerID]; ok {
			continue // customer has a Stripe subscription — handled by the forward pass
		}
		row.Status = "canceled"
		row.Tier = tierFree
		if uerr := s.store.UpsertSubscription(ctx, row); uerr != nil {
			s.log.Warn("billing reconcile: revoke upsert failed", "user", row.UserID, "err", uerr)
			continue
		}
		synced++
		s.log.Info("billing reconcile: revoked (no Stripe subscription)", "user", row.UserID)
	}
	return checked, synced, skipped, nil
}

// moreAuthoritative reports whether subscription a should outrank b as the customer's
// representative: a Pro-granting status wins; ties break to the later current-period-end.
func moreAuthoritative(a, b billing.Subscription) bool {
	ra, rb := statusRank(a.Status), statusRank(b.Status)
	if ra != rb {
		return ra > rb
	}
	return a.CurrentPeriodEnd > b.CurrentPeriodEnd
}

func statusRank(status string) int {
	switch status {
	case "active", "trialing":
		return 2
	case "past_due":
		return 1
	default:
		return 0
	}
}

// subscriptionDrifted reports whether any entitlement-relevant field differs, so we write
// (and bump updated_at) ONLY when Stripe's state actually changed ours.
func subscriptionDrifted(a, b store.Subscription) bool {
	return a.StripeSubscriptionID != b.StripeSubscriptionID ||
		a.Status != b.Status ||
		a.Tier != b.Tier ||
		a.PriceID != b.PriceID ||
		a.Interval != b.Interval ||
		a.CancelAtPeriodEnd != b.CancelAtPeriodEnd ||
		!a.CurrentPeriodEnd.Equal(b.CurrentPeriodEnd)
}
