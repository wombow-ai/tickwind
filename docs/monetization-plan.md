<!-- Authored 2026-06-19/20 via the tickwind-monetization-research workflow (4 research angles + synthesis), grounded in the owner monetization vision. STATUS: design only — do NOT build the paywall until the owner opens the gate. -->

# Tickwind Product + Monetization Plan

> **OWNER DECISIONS LOCKED (2026-06-20):** anonymous = **crawlable overview teaser** (gate depth, not existence) · price **$12.99/mo · $99/yr** · **NO reverse trial** (simple Free/Pro two tiers; free = teaser + a few generations/month) · **Phase 0 GO** (pre-paywall polish; Phases 1-3 await a further go-ahead). Free-gen allowance + Product-B default model (Haiku) follow the §8 recommendations unless changed.
>
> **Phase 0 progress:** the stale "1 report/day" copy (dict.ts deep.gate/quota, en+zh) is corrected to monthly — done. Remaining Phase-0 polish: an anti-hallucination trust line on the report; leave one mega-cap report fully open as an SEO/demo asset; make the overview teaser prominent.

*Synthesis of four research memos (market/pricing, conversational design, Stripe/entitlements, paywall UX). Standing constraints kept central throughout: the **anti-hallucination contract** (Go owns every number; the LLM writes prose only over pre-formatted facts; no advice/targets/ratings) and **$0-baseline cost discipline**. Note: CLAUDE.md records monetization as owner-deferred — this is the build-ready design for when the owner opens that gate, per the owner's monetization brief.*

---

## 1. The two-product model

Tickwind ships **one trust moat, two product surfaces** over a single shared fact substrate. Both consume the *same* Go-owned `FactSheet`; the difference is shape and cost profile.

**Product A — Deep Research (the current structured report).**
A comprehensive, fixed-section report (估值/基本面/技术面/资金面/情绪面/概览 + bull/bear). It stays **SHARED + cached per (ticker, ET-month, lang)** — one generation amortizes across every viewer, so the marginal cost of an additional *viewer* is ≈ $0. What changes is that **viewing** becomes the gate (today only *generation* is gated). This is the "大而全" baseline: rigorous, but the same for everyone.

**Product B — Personalized AI Research (new, conversational).**
A **ticker-scoped chat thread** where a Pro user brings their *own* question, and the LLM answers in prose while calling Go tools that (a) return pre-formatted facts and (b) surface **preset chart/data widgets inline** (the existing `KLineChart`, `FundamentalsCard`, flows/whales/options cards). This is the "spark/inspiration/personalized" layer Product A lacks. Unlike A, B is **per-user and uncacheable** — every message is a fresh paid LLM call. That cost asymmetry is the entire reason B is whole-feature Pro-gated and metered.

**How they relate — one funnel, one substrate:**

| | **A — Deep Research** | **B — Personalized AI Research** |
|---|---|---|
| Shape | Fixed-section report | Open conversational thread |
| Personalization | None (same for all) | The user's own questions/angle |
| Caching | **Shared** per (ticker, ET-month, lang) | **Per-user, per-message** — not shareable |
| Cost driver | Generation (rare, ~$0.05) | Per-message (frequent) — the metering target |
| Gate | **Viewing** gated (anon/free/Pro) | **Whole feature** Pro-only + metered |
| Reuses | `Assemble`/`ComposeDeepReport`/`research` | **A's fact sheet as chat context** + the same widgets |

The clean journey: anon sees A's teaser → signs up free → reads more of A → upgrades to Pro for full A → opens B to *interrogate* it ("you said the flows diverge — show me"). **B literally consumes A's fact sheet as grounding context**, so the two share one data substrate and one anti-hallucination harness. Both feed the branded-PDF/share-image export loop (§7).

---

## 2. Tiers & entitlements

Three viewer states, one paid tier. Every gate is **enforced server-side** (client `tier` is display-only).

| Lever | **Anonymous** | **Free (logged-in)** | **Pro** |
|---|---|---|---|
| Header / price / disclaimers / citations | ✓ (never gated) | ✓ | ✓ |
| **Product A — Executive overview** | ✓ prose teaser (crawlable) | ✓ overview + **§1 full** | ✓ full |
| Product A — sections §2–6 | locked index (titles only) | locked (server-omitted) + "Unlock" CTA | ✓ |
| Product A — bull/bear verdict | ✗ | ✗ | ✓ |
| **Generate a NEW stock's report** | ✗ | **2 / month** | unlimited (fair-use soft cap ~30/day) |
| Regenerate for freshness (`?fresh=1`) | ✗ | ✗ | ✓ |
| Model quality | — | Sonnet 4.6 (shared cache) | **Opus 4.8 on regen** |
| PDF / branded share-image export | ✗ | ✗ | ✓ |
| **Product B — conversational thread** | ✗ (sees upsell) | ✗ (sees upsell) | ✓ **~150 msgs/mo** (soft-capped) |
| Product B — "deep-dive" answer | — | — | Sonnet 4.6 toggle (Opus on top plan, later) |

**Mapping the owner's 4 Pro unlocks:** (a) more/unlimited generations → Free 2/mo, Pro unlimited; (b) on-demand regenerate → Pro-only `?fresh=1`; (c) Opus 4.8 quality → Pro regen runs Opus, and is the B "deep-dive" ceiling; (d) deeper reports / more sections / export → Pro unlocks §2–6 + bull/bear + PDF/share export. **The view-gate** is the three Product-A states above.

**Conversion mechanic — reverse trial (no card):** every new free signup gets **7 days of full Pro automatically, no credit card**. They experience full A + B + export, form the reference point, then auto-downgrade to Free. Loss-aversion at downgrade converts strongly, collects no card (no dark pattern), and AI-native products on reverse trials convert at the high end (6–20%). This is the single highest-leverage lever in the memos.

---

## 3. Recommended pricing

**Pro: $12.99/mo or $99/yr** (annual = ~$8.25/mo, **~36% off**). Annual is the default/recommended toggle; monthly is the trust on-ramp and the anchor.

**Free limits:** unlimited *viewing* of A teasers (≈$0 to serve, it's the funnel); **2 new-stock generations/month**; **0 Product B** (upsell only, except during the reverse trial).

**Justification:**
- **Conversion-optimal band.** Consumer AI-research clusters at $10–20/mo. $12.99 sits dead-center, decisively undercutting Seeking Alpha ($299/yr), Fiscal.ai ($39/mo), Koyfin ($39/mo), and even Simply Wall St ($120/yr) — while offering conversational AI SWS lacks.
- **Annual-first** improves cash flow for a solo dev and cuts Stripe fee drag (~$3.96/yr in fixed fees across 12 monthly charges vs $0.30 once). The aggressive ~36% annual discount (vs the 16.7% industry norm) is justified because retention + cash flow matter most for a young, single-owner product, and discounts are the most effective honest enticement.
- **Margin is enormous, the risk is *under*-charging.** Shared A reports are near-zero marginal cost; B is capped (§5). True marginal cost ≈ $0.50–0.80/Pro/mo worst case → **~95%+ gross margin** even after Stripe. The constraint isn't cost — it's perceived value and the LLM budget floor (don't price below ~$8/mo-equivalent; too cheap signals low value and starves the model budget).
- **The 2/mo free generation** (raised from today's 1) is the deliberate funnel investment: 1/mo is so tight a free user can't form a "I made this" habit. Worst case ≈ $0.10/free-user/mo at Sonnet; most free generations hit the shared cache (≈$0).

*Pricing fork to settle:* the two memos differ slightly — **$15/$129** (market memo) vs **$12.99/$99** (UX memo). Recommend **$12.99/$99**: the lower point maximizes conversion for a new brand and the margin headroom makes the ~$30/yr difference immaterial. Flagged in §8.

**Defer:** a $39/mo "Max" tier (unlimited Opus B turns) until power-user demand actually proves out.

---

## 4. The gating / teaser UX (anon / free / Pro)

Three *distinct* asks — never conflate them (the #1 funnel mistake).

**Anonymous → free ("why log in").** Render the **executive-overview prose** + a **locked section index** (titles visible, bodies not). This proves a rigorous, source-cited report exists — and stays **crawlable** (critical: the vision's "anon sees nothing" would suppress the pSEO funnel and the share-image loop; the resolution is anon sees the *teaser*, gated only on depth). CTA framed **"Read free"** (not "Sign up"), primary button = **Google OAuth** (already wired). **Capture intent:** deep-link back to the exact `/stock/NVDA/research` after signup — never dump on a dashboard.

**Free → Pro ("why upgrade").** The upgrade moment is **at the section wall, in context**, after the user read the full §1 on a stock they care about — the highest-intent moment, not a cold pricing page. Secondary triggers, each naming a *specific* benefit: hitting the 2-generation cap ("Pro = unlimited"); clicking **Regenerate** (freshness); clicking **Export** (the share/PDF gate); clicking **"Ask your own question"** (the Product B entry). Instrument which trigger converts.

**Pro → full.** Full A (all sections + bull/bear), regenerate, Opus, export, and Product B.

**Hard technical rule (security + ethics):** locked sections must be **omitted server-side** from the free payload (`prose`/`facts` stripped, replaced by `{"locked": true}` sentinels). **Never ship full prose to the client and hide it with CSS** — that's both a trivially-bypassed fake gate (paywall-remover scripts) and a dark-pattern-adjacent fake. Since reports are shared/cached, a leaked full report is a real risk; truncate in Go.

**No-dark-pattern guardrails (owner's explicit constraint):** the teaser must be *genuinely useful* (real overview + one full section, Seeking-Alpha-style), not a 2-line tease; the overview must not give away then hide the verdict; "Load more" states the price plainly; annual savings shown honestly; one-click cancel via Stripe Portal with access through the paid period; **never paywall the disclaimer or the source citations** (the thing that protects the user); the DeepSeek→Opus quality tiering is disclosed honestly ("Free uses a fast model; Pro uses Opus 4.8"). The anti-hallucination trust story ("AI writes the analysis; it never invents a number — every figure sourced") is the conversion asset — surface it on the wall and the pricing page; don't undercut it with manipulative UX.

---

## 5. Product B design — UX + architecture + cost controls

### UX
- New route `app/[locale]/(main)/stock/[ticker]/chat/page.tsx` (login + Pro gated), entry button on the Research tab beside the existing `DeepEntry` funnel. **Ticker-scoped** (always one stock — bounds the fact universe and keeps the system context cacheable; multi-ticker compare is a deliberate v2).
- Each assistant turn is an ordered list of **blocks**: `prose` (Markdown interpretation) and `widget` (a preset card rendered by the frontend from a Go-supplied payload). Example — *"is NVDA expensive vs its history and is smart money still buying?"* streams: prose on P/E → `valuation_table` widget → prose on flows → `kline 1Y` + `flows_summary` widgets. **Widgets are the existing React components** — B builds no new chart code.
- **Suggested-prompt chips** ("Walk me through valuation vs peers/history", "What are the bear-case risks?", "Summarize the smart-money signals") lower the free-text hallucination variance — they map to existing fact sections so the model almost always has Go facts to ground on.
- **SSE token streaming** (the backend already runs `internal/stream`) so prose streams live and widgets pop in as tool results resolve; fall back to the proven `DeepResearchView` poll pattern if SSE-per-chat is too much for v1.
- Mandatory disclaimer on every thread; **Pro can export a thread** as a branded PDF (reuse `ExportPdfButton` + `@media print`) — a personalized, watermarked, shareable artifact = the viral hook.

### Architecture (anti-hallucination preserved exactly)
The entire model context — system prompt, per-ticker fact context, **and every tool result** — is a Go-formatted string. The widget mechanism closes the number channel: the model cannot draw a chart with invented points; it can only emit `surface_widget("kline","1Y")` and **Go renders it from the real candle store.**

- **Tool surface** (OpenAI-compatible function calling over the existing OpenRouter `enrich` client — same endpoint + a `tools` array, no new SDK). Keep it **tight and closed**:
  - `get_facts(section)` → the verbatim `buildMaterial` `Label: Value [source]` block for a section.
  - `surface_widget(type, params)` → renders a preset card; `type` is a **closed enum** (`valuation_table`, `fundamentals_table`, `kline`, `indicators`, `flows_summary`, `whales`, `options`, `insider`), `params` constrained (e.g. `range ∈ {3M,1Y,5Y}`). Critically, it returns only "rendered: kline 1Y" — **the widget's numbers never enter the model's context** (protects the contract + saves tokens).
  - `get_news_context(topic?)` → attributed, explicitly non-numeric backdrop ("do not derive a number from this").
- **Per-turn loop:** (1) static cacheable system prompt = the distilled `composeDeepPrompt` firewall (three-rule preamble, given-facts-vs-inference firewall, citation discipline, advice ban) + tool-use rules ("to state any number you MUST have it from `get_facts`; to show a chart call `surface_widget`; never assert a number a tool didn't return"); (2) the per-ticker `buildMaterial(fs)` string (cacheable — most questions then need *zero* tool round-trips); (3) conversation history (after the cache breakpoint); (4) standard tool loop, **capped at ≤4 iterations**.
- **Prompt caching** aggressively (`cache_control` via OpenRouter): frozen system prompt + per-ticker fact context before the breakpoint, history after. Reads ~0.1×, 5-min writes ~1.25× — this is what makes a Haiku turn cost ~$0.001. **Verify caching actually fires** (`cache_read_input_tokens > 0`); a silent invalidator (a timestamp in the system prompt, non-deterministic tool-JSON ordering) would 10× cost without warning.
- **State** in the cheap/losable **User store** via `store.Split`: `chat_thread` + `chat_message` (message = role + ordered blocks JSON). The model is stateless; Go sends a **windowed** history (last ~10 turns) each turn to bound input tokens. *(Note: this diverges from billing state in §6, which goes to the durable Market store — chat history is rebuildable, billing is not.)*
- **Model routing** — extend `enrich` to a third env-driven "chat" target (`LLM_CHAT_MODEL`/`_BASE_URL`/`_API_KEY`, defaulting to the deep client if unset, matching the `LLM_DEEP_*` idiom): default turn → **Claude Haiku 4.5** ($1/$5); in-thread "deep dive" → **Sonnet 4.6** ($3/$15); top-plan ceiling → **Opus 4.8**. Never hardcoded. A new `enrich.Chat(ctx, ticker, history, tools, model)` returns `(blocks, toolCalls, usage)`; the `hasAdvice` advice-guard runs as a deterministic post-filter over every emitted prose block.

### Cost controls (B is the margin landmine — these are mandatory, not optional)
A cached Haiku turn ≈ **$0.001–0.004/message** (~$0.003–0.008 cold); a Sonnet deep-dive ≈ 3–5× that. Therefore:
- **Pro-only** (caps the eligible population to payers).
- **Per-user monthly message meter** (~150–200/mo) cloning `deep_research_quota` (ET-month, `GetDeepQuotaUsed`/`IncrDeepQuotaUsed` shape) → **≤ ~$0.80/Pro/mo** worst case. Soft-degrade ("monthly chat limit reached, resets {date}"), not a hard error.
- **Global daily backstop cap** (reuse `researchDailyCap`) so even a quota-read failure can't run away.
- **Per-user rate limit** (tighter bucket on the existing `internal/ratelimit`, ~1 msg/3s) against scripted floods.
- **Tool-iteration cap (≤4)** + **output `max_tokens` cap (~800)** + **per-call timeout** (chat ~30s, deep-dive ~60s, wired like `llmDeepComposeTimeout`).
- **Charge-on-success metering:** increment the meter only when a real assistant turn was produced (mirror `composeDeepBackground`'s charge-once-refund-on-fail); a failed turn never burns quota. Watch the OpenRouter balance with telemetry — the team has already hit the $5 exhaustion wall once.

---

## 6. Stripe + Supabase entitlements — the concrete build

**Load-bearing decisions:** webhook hosted on the **Go backend** (`POST /v1/stripe/webhook`, one source of truth — no second runtime); add **`github.com/stripe/stripe-go/v82`** (the one place to break stdlib-first — signature verification is security-critical); entitlement source of truth = a **local DB row** webhook-synced (O(1) hot-path read, not a Stripe API call per gate check); link via **`client_reference_id` = the Supabase user UUID** (= the JWT `sub` = `auth.User.ID`).

**Stripe Dashboard (test mode first):** one Product "Tickwind Pro"; two recurring Prices (`price_monthly`, `price_annual`); server-created Checkout Sessions (mode=subscription); the **Customer Portal** enabled (gives cancel / plan-switch-with-proration / update-card / invoices / dunning for free — zero custom code); a webhook endpoint with its signing secret. Four secrets to the VPS `.env` only (never git): `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`, `STRIPE_PRICE_MONTHLY`, `STRIPE_PRICE_ANNUAL`. No monthly Stripe fee → fits $0-baseline; defer Stripe Tax (+0.5%) given self-managed tax.

**Schema** (add to `internal/store/postgres/schema.sql`, idempotent) — routed to the **durable Market store** via `Split` (billing is *not* cheap-to-rebuild like watchlist/notes):
- `subscriptions` (PK `user_id` uuid; `stripe_customer_id`, `stripe_subscription_id`, raw `status`, derived `tier`, `price_id`, `interval`, `current_period_end`, `cancel_at_period_end`, `updated_at`).
- `stripe_events` (PK `event_id`) — the idempotency ledger (Stripe delivers at-least-once + out of order).

Four store methods mirroring the quota shape: `GetSubscription`, `UpsertSubscriptionByCustomer`, `SetSubscriptionCustomer`, `MarkStripeEventSeen` (INSERT … ON CONFLICT DO NOTHING → first-time bool). Memory-store impls for tests.

**Endpoints** on the existing mux:
- `POST /v1/billing/checkout` (requireUser, `?interval=` → price id, reuse customer if known) → `{url}`.
- `POST /v1/billing/portal` (requireUser) → `{url}`.
- `GET /v1/billing/me` (requireUser) → `{tier, current_period_end, cancel_at_period_end}`.
- `POST /v1/stripe/webhook` — **no auth, signature-verified**: read the **raw** body (`io.LimitReader`, before any JSON parse), `webhook.ConstructEvent`, dedup via `MarkStripeEventSeen`, then handle. Event set: `checkout.session.completed` (bind user↔customer), `customer.subscription.{created,updated,deleted}` (**re-derive the whole row from the self-describing Subscription object** → order-independent), `invoice.paid`/`invoice.payment_failed` (refresh period/status). Status map: `active`/`trialing`→Pro; everything else→free.

**Gating helper** next to `requireUser`: `tierOf(ctx, u) string`. **Fails OPEN to 'free'** for the *viewing/quota* gates (a DB hiccup never hard-locks a paying user out of the free experience, never wrongly grants Pro). *Note the one nuance between memos:* for the **whole-feature** Product-B gate, prefer **fail-closed** (an unknown subscription state must NOT grant an expensive Pro-only feature). `current_period_end` + small leeway covers the renewal↔`invoice.paid` gap.

**Wire-up:** Product A's `getResearchDeep` truncates server-side for `tierOf=="free"` and bypasses the `deep_research_quota` check + honors `?fresh=1`/`?model=opus` for Pro. Product B's chat endpoint is `tierOf=="pro"` else 402/403 with an upgrade hint. **Webhook path exempt from the per-IP rate limiter** and outside `requireUser`.

**Security must-dos:** **fail startup if `STRIPE_WEBHOOK_SECRET` is empty** (CVE-2026-41432: stripe-go computes HMAC with an empty key → publicly-forgeable signatures → unlimited quota fraud); return **5xx on internal failure** so Stripe retries (up to ~3 days), **2xx** only on success/duplicate; keep `tier` in the **DB, not the JWT** (so a downgrade is instant, not waiting for token refresh); a `GET /v1/stripe/sync?customer=` admin reconciliation path covers a webhook missed during an SSH-flake/deploy window.

**Frontend (minimal):** `createCheckout(interval)`/`createPortal()`/`getEntitlement()` in `api.ts` (existing `getToken()` Bearer pattern; checkout/portal just `window.location.assign(url)`); a small `useEntitlement()` context (drives the "Load more → Upgrade" CTA + Pro badge); locale-prefixed `/en|/zh/pro` page (monthly/annual toggle) + `/pro/success` (re-fetch entitlement) + a "Manage subscription" Portal button in Settings.

---

## 7. Phased rollout (each phase shippable, lowest-risk first)

**Phase 0 — pre-paywall polish (no gating, ships dark).** Fix the **stale "1 report/day" copy** in `dict.ts` (lines ~202–207, 945–966) — the backend is already monthly; correct to monthly now regardless. Leave one **mega-cap report fully open** (e.g. AAPL) as an evergreen demo + SEO asset + best sales page. Make the executive-overview teaser genuinely valuable. Surface the anti-hallucination trust line on the report. *Risk: none — pure copy/content.*

**Phase 1 — Stripe plumbing (no UI gating, ships dark).** Add `stripe-go`, the two tables + four store methods (Market-routed), the webhook handler, the three billing endpoints, `tierOf`. Verify end-to-end with the **Stripe CLI in test mode** (`stripe listen --forward-to … && stripe trigger checkout.session.completed`). No gating changes yet. *Risk: low — additive, no user-visible change; the security checklist (empty-secret fail-start, raw-body verify, idempotency, 5xx-on-error) is the focus.*

**Phase 2 — Product A view-gating + go-live.** Server-side truncation for free users in `getResearchDeep`; Pro bypasses quota + `?fresh=1` + Opus regen; the three viewer states; the reverse-trial grant; frontend teaser CTA + `/pro` + Portal + `useEntitlement`. Switch Stripe to **live keys**, launch the paywall. *Risk: medium — this is the revenue moment; the load-bearing correctness is "truncate in Go, not CSS" and the fail-open/closed split. Adversarially verify a free session cannot retrieve gated prose over the wire.*

**Phase 3 — Product B.** (a) backend `enrich.Chat` tool-calling primitive + the three-tool service, unit-tested with a fake LLM; (b) the chat endpoint + `chat_thread`/`chat_message` state + the message meter + global cap + per-user rate limit + charge-on-success (Pro-gated); (c) the frontend thread UI reusing existing widgets + prompt chips + SSE/poll; (d) the Pro gate + funnel entry; (e) deep-dive toggle + branded thread export; (f) **a dedicated adversarial-review hardening pass** for the wider free-form hallucination surface + prompt-cache + cost telemetry verification. *Risk: highest (cost + hallucination surface) — which is exactly why it ships last, only after the entitlement infra is proven by A.*

---

## 8. Open decisions for the owner + key risks

**Real forks (need an owner call):**
1. **Price point: $12.99/$99 vs $15/$129.** Recommend **$12.99/$99** (max conversion for a new brand; margin makes the delta immaterial). *Owner picks.*
2. **Anonymous teaser vs total lockout.** The vision says anon sees *nothing* of A; every memo argues anon must see the **crawlable overview teaser + branded share image** or the pSEO/i18n funnel and the viral loop are suppressed. **Strong recommendation: gate depth, not existence.** *Owner confirms.*
3. **Reverse trial (7-day full Pro, no card) — yes/no.** Highest-leverage conversion lever in the research, no dark pattern. Recommend **yes**. *Owner confirms.*
4. **Free generation allowance: keep 1/mo or raise to 2/mo.** Recommend **2** (habit formation; ≈$0.10/user/mo worst case, mostly cached). *Owner picks.*
5. **Product B default model: Haiku 4.5 vs Sonnet 4.6.** Recommend **Haiku default + Sonnet deep-dive toggle** (cost). *Owner picks — affects per-message cost ~3–5×.*
6. **A $39 "Max" tier — defer until power users emerge.** Recommend **defer**. *No action now.*

**Key risks:**
- **Monetization is owner-deferred (CLAUDE.md).** Build nothing until the owner opens the gate; this is the design, not a green light.
- **Product B cost blow-up (highest).** Per-user, uncacheable — structurally opposite to A. The full mitigation stack (Pro gate + monthly meter + global cap + rate limit + iteration/output caps + prompt caching + charge-on-success + balance telemetry) is mandatory. Verify caching fires.
- **Truncation leak.** Shared/cached reports + a CSS-only gate = a one-line bypass and a leaked full report. **Gate server-side** — the single most important correctness rule in Phase 2.
- **Webhook security.** Empty-secret forgery (CVE-2026-41432 → fail-start), raw-body verify, idempotency ledger, 5xx-on-error retries — all non-negotiable.
- **Wider free-form hallucination surface in B.** Open questions ("what's the fair value?", "should I buy?") have no Go fact. Mitigations: closed tool set + closed widget enum + the system-prompt firewall + the deterministic `hasAdvice` post-filter + a refusal/redirect pattern ("Tickwind doesn't give targets/advice — here's what the disclosed signals show"). Needs the dedicated adversarial review (the team's proven multi-finder workflow) before B launch.
- **Free/redistribution-safe sources only.** The standing constraint stays in force for every paid feature — no gray quote sources (the accepted free-IEX-over-Yahoo trade-off), no web-search augmentation in B v1 (breaches both "Go owns every number" and the free-sources rule).
- **Scope creep in B.** v1 stays single-ticker, closed-tool, Go-facts-only. Multi-ticker compare, portfolio chat, web search = explicit v2+.
- **Operational:** Vercel ~100-deploy/day cap (batch frontend commits), RackNerd SSH flakiness (background+poll deploys), and the ~3s cold Cloudflare-Tunnel reset (a cheap frontend retry-once on B's first cold message) — all already in the team's playbook.

---

**Bottom line:** one trust moat (Go-owned facts, no hallucinated numbers — the thing no consumer comparable can match), expressed as a **shared cached report whose *viewing* you gate** (near-pure margin) and a **per-user conversational layer you Pro-gate-and-meter** (the only real cost center). Price at **$12.99/$99 annual-default** in the proven conversion band, convert via a **genuinely-useful teaser + a no-card reverse trial** (no dark patterns), build entitlements **once** (Stripe webhook → durable DB row → `tierOf`), and ship **lowest-risk-first**: copy polish → Stripe plumbing dark → Product A view-gate go-live → Product B last, behind the proven infra and a hallucination-hardening review.
