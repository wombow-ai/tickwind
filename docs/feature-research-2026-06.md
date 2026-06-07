# Feature & risk research — 2026-06 (decisions)

Four parallel research streams. These are the DECISIONS to build against; full
reasoning was in the research pass.

## 1. Private notes (个股 + 日历) — DECIDED, build
- **One `notes` table** (per-user → `Split.User` local DB), columns:
  `id text PK ("note:"+rand), user_id uuid, ticker text NULL, note_date date NULL,
  body text, pinned bool, created_at, updated_at timestamptz`. Indexes:
  (user_id,ticker,created_at DESC), (user_id,note_date) WHERE note_date NOT NULL,
  (user_id,created_at DESC). A note is stock-scoped, date-scoped, both, or neither.
- **`store.Note` + 4 store methods** (SaveNote, ListNotes(NoteFilter{UserID,Ticker,From,To,Limit}),
  UpdateNote(userID,id,*body,*pinned)→found, DeleteNote(userID,id)→found). Ownership
  enforced in the query (`WHERE user_id=$1`); not-yours → 404 (don't leak existence).
  Mirror clips: memory map `notes[userID][id]`, postgres upsert/ON CONFLICT, Split→User.
- **API** (all requireUser): `POST /v1/notes`, `GET /v1/notes` (?ticker= OR ?from=&to= OR all,
  pinned-first), `PATCH /v1/notes/{id}` (body/pinned only in v1), `DELETE /v1/notes/{id}`.
  Add CORS `PATCH,PUT`. Add `patchJson` to web/src/lib/api.ts (none today).
- **UX**: a "Notes" tab on the stock detail page (auth-only, like "Saved links" — textarea
  compose + FeedList + a new `{kind:'note'}` arm in TimelineItem) + a standalone `/notes`
  page (chronological, pinned-first; ticker chips deep-link to /stock/[ticker]; TopNav
  "Notes" link + `nav.notes` i18n en/zh). **v1** = backend + stock tab + /notes list.
  **v1.1** = month-grid calendar view (frontend-only over the existing ?from=&to= endpoint),
  re-tagging, markdown.

## 2. Comments (个股 + 综合评论区) — DECIDED: GO, with safeguards
US law: **Section 230 shields us as a neutral host; pre-publication moderation is NOT
legally required.** Publisher's exclusion (Lowe v. SEC + 2024 Seeking Alpha dismissal)
keeps us a non-adviser as long as content is impersonal + we don't tout. **MUST-HAVE before
launch**: ToS + Community Guidelines (**18+**, ban manipulation/touting/illegal/IP/harassment);
a prominent **"not investment advice / user opinions / not endorsed"** disclaimer on every
comment area + signup; **neutral-host posture** (no staff stock picks, no paid promotion of a
ticker, delete-only moderation — never rewrite a post); **report/abuse button**; **admin
takedown + ban** + repeat-infringer practice; **DMCA designated agent** (register w/ Copyright
Office ~$6, renew every 3y — OWNER ACTION; required if images/long paste allowed); **rate-limit
+ anti-spam**; **store user_id + IP + timestamp** per comment (+ edit/delete logs); **privacy
policy** covering it. NICE-TO-HAVE: profanity filter, pump-spam heuristics, reputation/trust
levels, first-poster throttle. EU DSA: micro/small-enterprise exempt (Art. 19) at our size;
UK OSA minimal. Watch-out: a future **paid AI** that GENERATES stock commentary is likely OUR
speech (not §230-shielded) — wall it off + get securities counsel before shipping AI picks.

## 3. K-line indicators — DECIDED
- **First batch (精不在多)**: **MA (SMA 5/10/20/60, the "N日线", overlay) + MACD(12,26,9, pane) +
  RSI(14, pane) + Volume (pane)**. Defer EMA-standalone + Bollinger to batch 2.
- **Formula nuances** (get these right or values won't match TradingView/StockCharts):
  SMA emits null for first N-1; EMA k=2/(N+1), **seed = SMA of first N**; MACD line starts at
  idx 25, signal = EMA9 over the *compacted* non-null MACD values (starts ~idx 33); **RSI uses
  Wilder smoothing** (avg=(prev*13+cur)/14, NOT simple SMA), first avg = SMA of first 14, RSI
  path-dependent → compute over FULL history then slice; Bollinger σ = **population** stddev (÷N,
  not N-1).
- **Library: TradingView `lightweight-charts` v5 (Apache-2.0)** — native multi-pane candlesticks,
  `subscribeClick` for clickable, **keep `attributionLogo:true`** (satisfies the Apache NOTICE
  link requirement). Client-only (`'use client'` + useEffect + chart.remove() cleanup; no SSR).
- **Compute client-side** in TS from `GET /v1/stocks/{ticker}/bars`, over the full fetched
  history (≥250 bars ideal for RSI convergence) then slice to the visible window. No backend
  change. Add a loading skeleton for the chart.

## 4. Data-source commercialization risk — IMPORTANT (for paid/AI later)
The dominant risk is **redistributing market QUOTES/bars to paying end users.**
- 🟢 **GREEN** (safe for paid, $0): **SEC EDGAR** (public domain; <10rps + UA), **Bluesky**
  (public posts), **TWSE/TPEx** (OGDL — commercial OK, **must attribute** "Source: TWSE/TPEx").
- 🟡 **YELLOW** (fixable): **Finnhub** (free tier = personal; buy commercial/redistribution plan
  before charging, or drop news), **ApeWisdom** (no written license + upstream Reddit terms; get
  email OK, show only aggregate counts, keep fallback), **Substack RSS** (headline + short excerpt
  + link-back only; never full/paywalled text).
- 🔴 **RED** (must replace before charging $): **Alpaca** (ToS: "you cannot redistribute Alpaca
  API data"; free = personal/non-commercial; even Pro ≠ redistribution) — **our #1 exposure**;
  **Yahoo** HK chart scrape (ToS bans commercial/automated); **StockTwits** (API registrations
  closed; raw-post extraction barred → use their embed widget or drop); **Xueqiu** (unlicensed +
  soft-blocked → drop).
- **The one fix before monetizing**: re-architect the **quote layer** (US Alpaca + HK Yahoo) onto
  a **redistribution-licensed vendor** (Twelve Data / EODHD / Intrinio, ~$50–250/mo at startup
  scale) and default the UI to **15-min delayed** quotes (real-time SIP to end users also needs
  exchange display agreements + per-user pro/non-pro fees). Confirm exact license wording w/ each
  vendor's sales before flipping subscriptions on.
