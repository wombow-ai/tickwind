# Tickwind — Strategy Research, Round 2 (2026-06)

Second parallel-subagent round, on non-feature angles (round 1 = product features, in
`future-features-2026-06.md`). Agents: monetization · growth · positioning · **engineering** ·
legal. Full per-agent transcripts live in the session task outputs; this is the synthesis.

---

## ⭐ Synthesis — the whole board points one way

All five angles **converge on a single strategy**: the **SEC/EDGAR public-domain "follow-the-money" +
AI layer** is simultaneously the **moat** (positioning), the **paid tier** (monetization — needs NO
quote license), the **legally-safest lane** (R1/R2), and **SEO content** (growth). And the **bilingual,
multi-market (US+HK+TW) research-only** angle is the **wedge** — newly *timely* because a 2026 CSRC
crackdown is detaching Chinese investors from brokers (Futu/Tiger/Longbridge) but not from their
research need, and Tickwind is research-only by design.

**Recommended path (in order):**
1. **Finish the Financials feature** (in progress) — first slice of gov-data depth.
2. **Ship Alerts** — the #1 retention mechanic, runs on owned data, anchors the paid tier.
3. **Observability + nightly backups, this week ($0)** — Sentry + slog + UptimeRobot + Healthchecks +
   `pg_dump`→R2. The corpus (the moat) currently has **zero backups**; the system fails silently today.
4. **SEO trio** — full-universe sitemap + JSON-LD + hreflang (cheap, compounding; the SSR pages already
   exist) → unlocks the **bilingual SEO moat** (HK/TW search Google, underserved in zh).
5. **Pre-monetization gates** (owner): Privacy Policy + ToS · finish DMCA (agent + on-site page +
   repeat-infringer policy + `ADMIN_USER_IDS`) · a securities-counsel consult on the publisher posture ·
   budget **Vercel Pro $20/mo** (Hobby prohibits commercial use).
6. **Then monetize** the GREEN gov-data + AI layer via **LemonSqueezy** (Plus $9.99 / Pro $24.99) —
   quotes stay free+delayed; license a redistribution-clean quote feed (Tiingo→Databento) when revenue
   justifies it.

**Owner decisions this surfaces:** (a) commit to the bilingual-global-Chinese-investor wedge as the
brand? (b) target timing to start charging? (c) the counsel consult + ToS/Privacy + Vercel Pro are
owner actions that gate the paywall.

---

## 🛠 Engineering / scale / reliability roadmap

Ranked by value/effort for a solo dev; **#1–#12 are all $0** (free tiers).

**Do this week (turns "fails silently" → "tells you when it breaks"):**
1. **Sentry** error tracking (Go + Next SDK, free 5k err/mo) — wrap the mux + `defer sentry.Recover()` in **every ingestor goroutine** (today a goroutine panic = silent data gap). = backlog item ⑧.
2. **`log/slog` JSON** structured logging (stdlib, no dep) — per-ingestor/request context.
3. **`/healthz` → real readiness** (ping both DBs + scheduler heartbeat) + **UptimeRobot** (free) alert.
4. **Healthchecks.io** dead-man's-switch per ingestor (freshness SLOs: quotes<2m, news<30m, opp/guru<3h, pruner<7h) — alerts when a goroutine wedges.
5. **Nightly `pg_dump` → Cloudflare R2/B2** (free) — ⚠️ **Supabase free tier has NO automated backups** and the corpus is the moat; currently **zero backups**. Highest-value DR action.

**Then ($0):** govulncheck + gosec + Dependabot in CI; Prometheus `/metrics` → Grafana Cloud free; **global per-IP rate-limit** (esp. the unthrottled on-demand `IngestOne` trigger — abuse vector) + Cloudflare WAF managed ruleset; **edge-cache public reads** (`Cache-Control`/`ETag` on /hot,/topics,/opportunities,/news → Cloudflare CDN offloads the VPS — cheapest scale lever); auto-deploy on green CI; CORS allowlist (not `*`); test gaps (ingest scheduler, Split partial-failure, auth alg-confusion negative cases, pruner-protects-substack, opportunity ranker edges); BRIN/composite indexes now, TimescaleDB hypertables when rows hit tens of millions (>2000× faster retention DELETEs — helps the pruner).

**⚠️ Costs money — flag for the commercialization phase (NOT eng-optional):**
- **Vercel Hobby PROHIBITS commercial/revenue use** → must move to **Pro ($20/mo)** the moment Tickwind charges. Also Hobby's 100GB/1M-invocation cap **pauses the deploy with no warning** (app goes offline). Single most important non-eng infra item at launch.
- **Supabase Pro (~$25/mo)** when PITR / no-pause / larger DB needed (free projects pause after ~1wk inactivity — continuous ingest keeps it warm; Healthchecks #4 is the early warning). Until then nightly pg_dump→R2 is the $0 substitute.
- Second VPS / LB (~$5–20/mo) only when the single-VPS SPOF matters; the architecture (durable Market store + cheap-rebuild User store) already makes "make the API stateless + IaC the box" a near-term $0 resilience win.

**Scale bottleneck order:** provider API rate limits (first ceiling — mitigated by batching/caching/single-flight) → read egress (→ edge cache) → SSE fan-out (tens of thousands of conns) → co-located Postgres (split off first).

---

## 💰 Monetization & subscription strategy
_(agent running — pending)_

## 📈 Growth, SEO & distribution
_(agent running — pending)_

## 🎯 Positioning, differentiation & moat

**Recommended wedge:** *"The bilingual (zh/en), multi-market research terminal for globally-minded
Chinese-speaking investors — US + HK + TW, primary-source-first, **no brokerage attached**."*

**Timely tailwind (agent-researched — verify before betting the brand):** a **May 2026 CSRC
crackdown** on illegal cross-border brokerage (Futu/Tiger/Longbridge — ~RMB 2.26B/~US$333M proposed
fines; ~1M mainland US-stock accounts entering a 2-year sell-only wind-down). It severs *trading*
but not the *research/follow* need. Every Chinese-facing incumbent here is a **broker** (the thing
now under fire); Tickwind is **research/data-only** (owner's hard constraint "never touch a funded
account" = perfect fit). Converts a crowded-looking space into a timely opening.

**The unoccupied intersection (4-way AND):** multi-market (US+HK+TW) × true bilingual zh/en ×
primary-source + alt-data depth × research-only. No incumbent sits in all four — Futu/moomoo = broker
(+ no SEC pipeline, now toxic for the mainland segment); Koyfin/Fiscal.ai = English-only, intl
filings "on the way"; Xueqiu = China-bound, no primary-source rigor, no English.

- **Wedge B "follow-the-money / alt-data"** (insider/congress/13F/short-interest) = **feature pillar
  + the paywall**, NOT the brand (category saturated — Unusual Whales/Quiver/Capitol Trades; data is
  public-domain/commoditized; can't out-feature them). Unique angle = fusing it onto HK/TW + bilingual.
- **Wedge C "best unified per-stock page"** = execution standard, not a wedge (everyone claims it).
- **Moat = the stack:** (1) the **multi-market × bilingual integration itself** (integration capital
  incumbents won't build — brokers can't, US terminals won't localize) — strongest; (2) the
  multi-source aggregation + EDGAR/Form-4 backbone; (3) GREEN immutable data-history accrual
  (compounds); (4) bilingual community network effect (latent, post-traffic); (5) $0 cost structure
  (outlasts VC-backed entrants in a niche).
- **Persona "The Bridge Investor (查理)":** Chinese-speaking, globally mobile, cross-market book
  (US + China ADRs + HK incl. Tencent/Zhipu/MiniMax + some TW), reads zh+en, tired of switching
  Xueqiu/Yahoo/SEC/broker. **The owner IS this persona** → maximal founder-market fit.
- **One-liner:** *"Tickwind is the bilingual research terminal for the global Chinese investor —
  US, Hong Kong & Taiwan, primary sources first, 中文 and English on one page. We track the money;
  we don't touch yours."*
- **Honest risks:** the **HK leg is the shakiest** (gray Yahoo quotes = redistribution-RED; HK
  filings still blocked) → de-risk HK before charging; niche TAM unproven (n=1 = owner); stay clearly
  on the *research* side of the CSRC line (don't market "help mainland residents trade offshore").

## 📈 Growth, SEO & distribution ($0)

**Strategy:** make every ticker a bilingual, schema-rich SSR page **in the sitemap** → wrap each in a
shareable **OG card** → seed in communities → capture returns with **alerts/PWA** → measure with
privacy analytics. The SSR pages already exist, so the long tail just needs to be made discoverable.

- **P0 (highest ROI, S–M, biggest win/hour):** ① expand `sitemap.ts` from "popular" to the **full
  ingested symbol universe** + `lastmod` from data freshness (segment <50k/file); ② **JSON-LD** per
  stock page (`Corporation` w/ `tickerSymbol:"NASDAQ:AAPL"`, `Dataset`/`FinancialProduct` for
  quote/financials, `BreadcrumbList`, `FAQPage`) — conditional-render only when fields are populated
  (partial markup = zero lift); ③ **hreflang** cluster (`en`, `zh-Hant` HK/TW, `zh-Hans`, `x-default`),
  self-referencing, 200-only URLs.
- **P1:** dynamic **OG share-cards** (`@vercel/og`; **EOD/delayed data only** — live-quote
  redistribution is RED); **bilingual content depth** (localized blurb + "什么是$TICKER/财报解读" +
  FAQ, ≥500 unique words/page); **Alerts** (web-push — the #1 retention mechanic, own data); **privacy
  analytics** (self-host **Umami** on the VPS = $0, no consent banner).
- **Bilingual SEO moat:** HK/TW search **Google, not Baidu** → win with standard SEO + correct
  hreflang, no ICP license. Map keywords by **intent** (财报/股价/美股/港股/台股), don't machine-translate;
  owner = native speaker = in-house edge. English-quality, Google-indexed, deep bilingual per-stock
  content is genuinely thin among incumbents.
- **Distribution = artifacts, not links:** seed OG cards on StockTwits ($TICKER), FinTwit/X, Reddit
  (value-first), **Taiwan: Dcard 美股/股票 + PTT Stock 板**, **Xueqiu (residential/China egress)**, own
  Discord/Telegram. Content loop: pages rank → shares → backlinks → rankings; UGC adds freshness.
- **PWA:** manifest + SW + web-push (iOS 16.4+ needs Add-to-Home-Screen; ~16% opt-in — free additive).
- **North-star:** weekly returning users with ≥1 watchlist item. Tooling: Google Search Console (free)
  + self-hosted Umami.

## ⚖️ Legal / compliance risk register
_(informational, not legal advice.)_

- **R1 — RIA / advice line (likelihood Med→High once paid · impact CRITICAL · GATING):** stay a
  **publisher, not an investment adviser** (Advisers Act §202(a)(11)(D); *Lowe v. SEC* 3-part test —
  **impersonal · bona-fide/disinterested · regular circulation**). All AI/signal/board output must be
  **descriptive of public sources, never personalized or prescriptive** — hard-bar price targets,
  buy/sell/hold ratings, position sizing, and "for you" personalization. The "what changed" brief =
  neutral info about *followed* tickers, NOT a recommendation engine. Run digests on a **fixed
  schedule**, not event-triggered "act now" tips. SEC robo-adviser guidance: a *tailored* automated
  tool IS an adviser; the FPL "inanimate tool applying objective criteria" line is the safe side.
  Disclaimers are necessary but **NOT sufficient** — protection is structural. **→ a short
  securities-counsel consult before charging for any AI/signal feature is the highest-ROI legal spend.**
- **R2 — market-data redistribution (High × High):** engineering gate — **no Alpaca/Yahoo quotes
  to paying users** until a redistribution-licensed vendor; monetize the GREEN SEC/public-domain lane;
  upgrade Finnhub to a commercial plan before charging; FINRA short-interest = display-only + "Source:
  FINRA" (no bulk redistribution without a FINRA agreement).
- **R3 — UGC §230 + DMCA (Med→High):** §230 shields UGC torts and good-faith moderation doesn't waive
  it (keep moderation neutral; don't let AI *adopt* user claims). DMCA §512 needs: finish the **agent
  registration** + **on-site DMCA page** + a **repeat-infringer termination policy** (the most-fumbled
  §512 requirement) + set `ADMIN_USER_IDS`.
- **R4 — privacy (Low→Med):** **publish a Privacy Policy + ToS now** — Stripe AND Google OAuth require
  it regardless of CCPA/GDPR thresholds (Tickwind is under both today); document IP-capture purpose +
  retention (tie to the pruner); add a consent banner only if analytics are added; avoid accidental
  GDPR triggers (EUR pricing / EU-targeted analytics).
- **R5:** FTC endorsement (keep Guru rail non-commercial; clearly label any future affiliate/sponsored)
  · trademark (nominative use only, no logo-as-branding) · ADA/WCAG 2.1 AA (fix Aurora contrast,
  add chart/price-tag text alternatives for screen readers, label icon-only buttons, a11y statement).
- **Two gating items before the paywall flips:** (1) R1 securities-counsel sign-off on the
  publisher-not-adviser posture + disclaimers; (2) R2 engineering gate (no licensed live quotes to
  paying users). R3 (DMCA) + R4 (ToS/Privacy) are launch-blockers but largely self-serve.

## 💰 Monetization & subscription strategy

- **You can charge NOW without solving the quote blocker** — gate AI (filing summaries/diffs/digests),
  **alerts**, the gov-data "follow-the-money" suite (Congress/13F/short-interest), and the screener;
  keep live quotes **FREE + delayed** and never behind the paywall. This unblocks monetization on the
  GREEN/legally-safe layer immediately.
- **Pricing (proposed):** **Free** (the SEO funnel — all stock pages, delayed quotes, capped
  watchlist/notes, 3–5 AI summaries/mo) → **Plus $9.99/mo ($89/yr):** alerts, unlimited
  watchlists + AI summaries/digests + daily brief, full fundamentals/indicators, >3yr K-line →
  **Pro $24.99/mo ($239/yr):** Congress/13F/FINRA suite, filing red-flag scanner, RAG, screener,
  API; real-time quotes only once licensed. Undercuts Koyfin ($39) / Unusual Whales ($48).
- **Quote licensing (cheapest legal path, staged):** now = free+delayed, never paywalled → at first
  revenue = **Tiingo redistribution license** (IEX-derived, no exchange fees, flat-rate, sales-quoted —
  likely low-triple-digits/mo) → at scale = **Databento Equities Basic ($825/mo flat, cleanest rights)**.
  AVOID Nasdaq/SIP direct ($1,500+/mo + ~$76/sub). ⚠️ **15-min-delayed SIP is NOT the cheap unlock —
  IEX-derived is.** HK live quotes are HKEX-license-gated → keep HK free+delayed, never paywall.
- **Payments — ⚠️ Stripe is NOT available in Taiwan:** use a **Merchant-of-Record (LemonSqueezy,
  ~5% + $0.50, now Stripe-owned)** from day 1 — it's the legal seller, remits global VAT/tax, and
  sidesteps the Taiwan issue. Migrate to Stripe-direct (+ a US LLC) only at ~$250k ARR. Map subscription
  → a per-user `entitlement` flag via webhook; gate paid endpoints server-side (mirror `requireUser`).
- **Revenue scenarios:** conservative ~50 paid ≈ $0.3–0.5k net/mo · **base ~300 paid ≈ $3.7k net MRR**
  (self-funds the clean-rights quote upgrade) · optimistic ~1,500 paid ≈ $17k+/mo. Margin engine =
  gov-data + AI (near-zero marginal cost via content-hash caching); quotes are a cost center to minimize,
  never the headline paid feature until rights are clean.
