# Stripe activation runbook (Phase 1 → live)

Phase 1 plumbing is deployed and **inert**: with no Stripe env set, the billing/
webhook endpoints 404 and nothing is written. This runbook activates it. Do **TEST
MODE first** (`sk_test_…`); switch to live keys only at the Phase 2 go-live.

## 1. Stripe Dashboard (test mode toggle ON)

1. **Product + Prices** — create one Product "Tickwind Pro" with two recurring
   Prices: **$12.99 / month** and **$99 / year**. Copy each `price_…` id.
2. **Webhook endpoint** — Developers → Webhooks → Add endpoint:
   - URL: `https://api.tickwind.com/v1/stripe/webhook`
   - Events: `checkout.session.completed`, `customer.subscription.created`,
     `customer.subscription.updated`, `customer.subscription.deleted`.
   - Copy the **Signing secret** (`whsec_…`).
3. **Billing Portal** — Settings → Billing → Customer portal → enable (gives
   cancel / plan-switch / update-card / invoices for free, no code).
4. **Secret key** — Developers → API keys → copy the **Secret key** (`sk_test_…`).

## 2. VPS `.env` (secrets, never git)

Set these four on `/root/tickwind/.env` (paste the ids/secrets from step 1):

```
STRIPE_SECRET_KEY=sk_test_…
STRIPE_WEBHOOK_SECRET=whsec_…
STRIPE_PRICE_MONTHLY=price_…
STRIPE_PRICE_ANNUAL=price_…
```

Then recreate the api container so it re-reads `.env`:
`cd /root/tickwind && docker compose up -d --force-recreate api`

(The deploy script doesn't touch `.env`, so these persist across deploys.)

## 3. Verify activation

- `docker compose logs api | grep billing` → `billing enabled=true webhook_enabled=true`.
- `GET https://api.tickwind.com/v1/billing/me` (with a logged-in Bearer token) → 200
  `{"tier":"free"}` (was 404 when inert).
- Stripe CLI end-to-end (test mode):
  `stripe listen --forward-to https://api.tickwind.com/v1/stripe/webhook`
  then `stripe trigger checkout.session.completed` → the webhook should 200 and a
  `subscriptions` row should appear.

## What's already built (Phase 1, dark)

Two-client-safe entitlements: `subscriptions` + `stripe_events` tables (durable),
the stdlib Stripe client (no SDK) with HMAC webhook-signature verification + replay
guard + idempotency ledger, `tierOf(user)→pro|free` (active/trialing→pro, errors→
free), and the 4 endpoints (webhook / checkout / portal / me). All inert until the
keys above are set.

## NOT yet built (Phase 2 — needs owner go; user-facing + real money)

- Server-side truncation of the deep report for free users (the actual paywall).
- Frontend `/pro` pricing page, `useEntitlement`, the "unlock → upgrade" CTA.
- Switch to **live** Stripe keys.

See `docs/monetization-plan.md` for the full plan + the locked decisions.
