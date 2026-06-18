# Tickwind — AI Memory & Project Context

> This file is the **persistent AI memory**. Update it every iteration with new
> decisions, state, and conventions so any future session can resume cold.

## What it is
A personal, web-based command center for global stocks (US / HK / KR): all-session
real-time prices (incl. overnight), company announcements/filings, news, and
social-media chatter — unified per stock. **Engineering-first**; LLM is an optional,
feature-flagged plugin, never on the critical path. Web only.

## Owner / infra
- GitHub: `wombow-ai/tickwind`. EDGAR User-Agent / contact email: `inverael@gmail.com`.
- Domain `tickwind.com` (on Cloudflare). **Frontend → Vercel** (auto-deploys on push
  to `main`, Root Directory `web/`). **Backend → RackNerd VPS `root@104.168.38.21`**
  (4 GB / 3 vCPU / 60 GB; **migrated 2026-06-10** from the old 1 GB box `104.168.46.15`,
  which is now a STOPPED cold standby — `docker compose start` there to roll back),
  reached at `api.tickwind.com` via a **Cloudflare Tunnel**. **~$60/year (owner-funded upgrade).**
- **⚠️ Vercel Hobby limits (learned 2026-06-09/10): ~100 deploys/day.** A fast `/loop`
  that pushes 1–2 commits/tick can EXHAUST it → new pushes stop deploying (site keeps
  serving the last good deploy; new routes 404) with no obvious error. Symptom seen: a
  new page 404 for 15+ min while older pages stay 200. **Diagnosis that proves it's
  Vercel-side, not code:** `cd web && npm ci && npm run build` (clean, == Vercel) — if it
  succeeds and emits the route, the code is fine and the owner must check the Vercel
  dashboard (usage/build logs). **Mitigations:** ONE commit/push per tick (batch
  feature+ROADMAP); avoid ROADMAP-only pushes that still trigger a rebuild; quota resets
  at UTC midnight. Backend (SSH) deploys are independent of this.
- **Deploy flow** (backend): `rsync` the changed Go source (`internal/`, `cmd/`) to
  `/root/tickwind/` using `ssh -i ~/.ssh/tickwind_deploy`, then
  `ssh … 'cd /root/tickwind && docker compose up -d --build api'`. The VPS holds the
  source via rsync (NOT git — note macOS `._*` artifacts) + a local Postgres + the
  durable Supabase as `MARKET_DATABASE_URL` (split store). `.env` on the VPS is live —
  never overwrite it (rsync excludes it). Verify via `https://api.tickwind.com/healthz`.
  **SSH from here can be flaky** (long rsync connections drop mid-transfer; rapid
  reconnects trip sshd throttling → "Connection closed by remote host"). Mitigations,
  proven 2026-06-08: run the rebuild **DETACHED** so a mid-build SSH drop can't kill it —
  `ssh … 'cd /root/tickwind && nohup docker compose up -d --build api > /tmp/tw_build.log
  2>&1 & echo started'` — then **verify via PUBLIC curl** (e.g. `/v1/search?q=DRAM`), no
  SSH needed. rsync is idempotent → just retry it on a drop; use single quick SSH commands
  and don't spin/reconnect rapidly (that worsens the throttle).
- **MOST RESILIENT deploy (proven 2026-06-09, use when SSH transfers drop): the box pulls source
  from the PUBLIC GitHub repo itself via ONE short SSH command** (no slow rsync stream from the Mac):
  `ssh -o IPQoS=none … root@VPS 'cd /root/tickwind && nohup sh -c "curl -sL https://github.com/wombow-ai/tickwind/archive/refs/heads/main.tar.gz -o /tmp/tw.tgz && tar xzf /tmp/tw.tgz -C /tmp && cp -r /tmp/tickwind-main/internal/* internal/ && cp -r /tmp/tickwind-main/cmd/* cmd/ && cp /tmp/tickwind-main/go.{mod,sum} . && docker compose up -d --build api" > /tmp/deploy.log 2>&1 & echo DEPLOY_LAUNCHED'`
  **CRITICAL (proven 2026-06-09): background the ENTIRE script via `nohup sh -c "…" & echo` so the SSH
  command returns SUB-SECOND.** The flaky link drops connections held open more than a few seconds
  (e.g. during the remote curl/tar/cp), so the older form that ran curl+tar+cp inline (backgrounding
  only the build) got dropped; a sub-second launch survives. `IPQoS=none` also helps on some paths.
  (commit+push first so GitHub has the code.) Verify via PUBLIC curl (`/v1/universe` count, `/healthz`,
  `/v1/holdings`→401). **Never copies `.env`** (gitignored). Repo `wombow-ai/tickwind` is public.
- **Deploy script lives at `/root/deploy-ptr.sh` (persistent), NOT `/tmp`.** ⚠️ 2026-06-14: `/tmp/deploy-ptr.sh`
  was swept by systemd-tmpfiles, so `sh /tmp/deploy-ptr.sh` printed `cannot open … No such file` yet the
  wrapping `& echo DEPLOY_LAUNCHED` STILL returned exit 0 → several "successful" deploys were silent no-ops,
  caught only because the NEW route 404'd on public curl while `/healthz`+old routes stayed 200. Recreated it in
  `/root/` (the tarball-pull script above). **Deploy = `ssh -i ~/.ssh/tickwind_deploy -o IdentitiesOnly=yes …
  root@VPS '(nohup sh /root/deploy-ptr.sh > /tmp/deploy.log 2>&1 &) && echo DEPLOY_LAUNCHED'`.** A bare `ssh`
  (no `-i`) offers id_rsa and fails `Permission denied`. **ALWAYS verify the new code is live via public curl of
  the NEW route — never trust DEPLOY_LAUNCHED.**
- **VPS infra (1GB RAM!) — root-caused 2026-06-09:** a `docker build` (Go compile) can exhaust RAM+
  swap → the OOM killer kills NEW sshd sessions ("Accepted publickey … session opened" then the
  client sees "Connection closed by remote host"), which also drops rsync/tar mid-stream and trips
  **fail2ban** (it banned both the Mac's IP and the owner's → total SSH lockout; recover via the
  panel's **VNC** console → `fail2ban-client unban --all`). **Fixes applied + persistent:** added a
  **1G swapfile** (`/swapfile2`, in fstab; total swap ~4G → OOM gone, normal SSH stable again);
  **whitelisted the deploy IP** `154.29.158.47` in `/etc/fail2ban/jail.d/tickwind-ignore.conf`;
  `docker system prune -af` after builds to reclaim disk (a rebuild can push `/` from ~40%→90%).
  **SSH still drops intermittently — use SINGLE, SPACED attempts; do NOT rapid-retry in a tight loop**
  (a burst of reconnects re-trips sshd MaxStartups / fail2ban → "Connection closed by remote host"). One
  attempt per `/loop` tick (spaced ~60s) is the reliable pattern; if a deploy attempt drops, defer to the
  next tick rather than hammering. If fully locked out again, owner unbans via the panel **VNC** console.
- **brapi.dev API key** (Brazil B3, feature #38) provided by owner 2026-06-09, stored in the **VPS `.env`**
  as `BRAPI_API_KEY` (NOT in git — repo is public). Read it from the VPS env when building the BR adapter.
- **DB backups (2026-06-12):** the **local user Postgres** (watchlist/notes/holdings/clips/alerts) is dumped
  daily at **04:30** by `/root/backup.sh` (cron) → `gzip` to `/root/backups/user-TS.sql.gz`, newest 7 kept.
  The script auto-detects the postgres container + reads creds from its env. The **market** corpus lives on
  managed Supabase (backed up there), so only the user DB needs local dumps. **SSH transfer note:** held-open
  / `cat|ssh` stdin transfers DROP on this box — push small files via **base64 embedded in the command**
  (`B64=$(base64<f|tr -d '\n'); ssh host "echo '$B64'|base64 -d > /path"`); long ops nohup-backgrounded.
- **v7 unlocks (2026-06-13, owner-provided; secrets live ONLY in the VPS `.env`, never git):**
  `TELEGRAM_BOT_TOKEN` (for the planned morning-briefing/alert Telegram push) and `RESIDENTIAL_PROXY_URL`
  (dataimpulse `http://…@gw.dataimpulse.com:823`, to reach datacenter-IP-blocked sources: HKEXnews, Xueqiu,
  Nasdaq IPO API) are appended to the VPS `.env`. **Google OAuth** provider is configured in Supabase; the
  frontend flag now defaults ON (`GOOGLE_OAUTH_ENABLED = NEXT_PUBLIC_GOOGLE_OAUTH !== '0'`). KRX key still
  pending (Korea market deferred).

## Owner habits & preferences (keep this current — context gets compacted)
- **Workflow**: drives development via `/loop` (autonomous, self-paced). Each iteration =
  one verified increment → commit (directly to `main`, solo dev) → deploy → schedule next.
  Wants **parallel subagents** for research/dev ("不要你一个人干") — research/design agents
  are reliable; for code, build it myself or fall back if a code agent socket-fails.
- **Communicates in Chinese**; wants concise, scannable progress. **"你拍板" = trust my
  judgment** on design/style/architecture — surface only genuine product decisions, decide the
  rest. Don't over-ask.
- **Verify before commit** (hard gate): `go build ./... && go vet ./... && gofmt -l .` (empty)
  + relevant `go test`; frontend `npm run build`. Then deploy + live-verify.
- **Quality bar**: "**精不在多**" — precision/correctness over quantity (e.g. ship few, correct
  indicators). Engineering-first; LLM optional/off the critical path.
- **Commercial intent**: $0 now. **MONETIZATION DEFERRED (owner, 2026-06): do NOT build any
  paid/monetization work yet** — no pricing/tiers, payment infra (Stripe/LemonSqueezy), quote-
  redistribution licensing, paywalls, or subscription gating. **Everything else on the roadmap is
  greenlit** to build autonomously (Financials, Alerts, gov-data "follow-the-money" suite, AI
  filing summaries, SEO, observability/backups, polish, HK/markets). Keep features free + quotes
  delayed. Still mind commercialization risk PROACTIVELY for *future* paid plans — esp.
  **market-quote redistribution** (Alpaca + Yahoo are RED; see `docs/feature-research-2026-06.md`)
  — but that's a later gate, not now. Default to free/redistribution-safe sources; the owner
  **will explicitly override** for specific cases (e.g. HK gray Yahoo) — honor the override + flag.
- **Security**: do NOT rotate secrets / VPS password / Supabase JWT — owner-driven before launch;
  keys handed over are for staging/use. Never touch a funded brokerage account.
- **Memory discipline**: update `CLAUDE.md` + `ROADMAP.md` + `docs/` every iteration so a
  compacted/cold session resumes correctly.

## Stack
- Backend: **Go 1.26**, stdlib-first. Module `github.com/wombow-ai/tickwind`.
- Storage behind the `store.Store` interface: `memory` (dev) + `postgres`
  (TimescaleDB + pgvector) on the server.
- Frontend: **Next.js 16** (App Router, **SSR**, TypeScript, Tailwind v4) +
  **Supabase Auth** (`@supabase/ssr`), deploy target **Vercel**. "Aurora" design.
- Later: Python **Futu** sidecar (HK/US realtime), **LLM** enrichment plugin.

## Key decisions (do not re-litigate)
- v1 is **US-first**. Data only from **free, redistribution-safe / public** sources:
  SEC EDGAR (filings), Alpaca/Finnhub (US prices incl. overnight), Reddit/StockTwits
  (social). **Multi-market** (2026-06): **TW live** (TWSE/TPEx EOD, keyless);
  **HK prices live** via Yahoo delayed quotes — an **owner-authorized "gray" source**
  (HK exchange quotes are licence-gated; `internal/yahoo`, isolated + documented) for
  the 3 names the owner follows (Tencent `0700.HK`, Zhipu/"Knowledge Atlas" `2513.HK`,
  MiniMax `0100.HK`); **HK filings via HKEXnews — now FEASIBLE (2026-06-18 probe), owner-gated build:** the old
  "`prefix.do` (code→stockId) empty / datacenter-IP-gated" blocker was a STALE-PARAM artifact — with `lang=EN`
  (uppercase, not `lang=en`) it returns the full JSONP (Tencent `0700`→stockId 7609); `titleSearchServlet` takes
  that stockId, and the static HKEX `ListOfSecurities.xlsx` downloads as a fallback. Buildable filings-only
  (mirrors `TWAdapter`) — recorded + recommended in `docs/hk-kr-watchlist-feasibility.md`, awaiting the owner's
  greenlight (deferred roadmap item + a product call → surfaced, not auto-built). **KR DEFERRED** (KRX prices +
  OpenDART filings code-ready + inert; owner's KRX-site access is blocked — they'll
  supply the free KRX key later).
- **Never touch a funded brokerage account from code** — market-data only; if a broker
  API is ever needed, use an unfunded/paper key + isolation (user's explicit concern).
- LLM stays optional / behind a flag.

## Conventions
- **Go**: Google Go Style Guide + Effective Go. Doc comments on every exported
  identifier; wrap errors with `%w`; keep `go vet` + `gofmt` clean; table-driven tests.
- **TS/React**: Google TypeScript Style Guide; ESLint clean.
- **Ingestion**: each source is a small client package `internal/<source>` wired into
  the ingest scheduler.
- **Reuse OSS**: prefer proven patterns from mature projects (OpenBB, edgartools,
  OpenStock, pgx examples) over inventing.
- **Commits**: conventional (`feat:`/`chore:`/`fix:`…); end body with the
  `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>` trailer.
- **Verify before commit**: `go build ./... && go vet ./... && gofmt -l .` (empty);
  frontend `npm run build`.

## Run
- Backend dev (no infra): `make run` → http://localhost:8080 (memory store, live EDGAR).
- Full stack (server): `docker compose up -d --build`.

## Current state (update each iteration)
- **Session status (2026-06-14):** dev driven by an autonomous `/loop` (multi-subagent workflows).
  **v8 重点收费路线 R1 + R2 P0 shipped + live-verified this session:** **R1 indicator engine** —
  catalog `/v1/indicators` (282 stock-applicable, dataset-driven) + `/indicators` page; per-stock
  `/v1/stocks/{t}/indicators` (28 P0 = 19 computed + 2 market-context + 7 crypto-unsupported) +
  `IndicatorsPanel` on StockView. **R2 AI deep-research report** — `internal/research` anti-hallucination
  fact-sheet (Go owns every number, LLM writes prose only via `enrich.ComposeReport`, LLM-off → 200
  data-only) at `GET /v1/stocks/{t}/research` + public Research tab (3 sections: 估值/基本面/技术面).
  Owner directives still in force: **monetization deferred** (R1/R2 free, only cache+daily-cap plumbing),
  **web-push deferred**. See `ROADMAP.md` (v8 section) for the live status + backlog.
- **Shipped + LIVE-verified 2026-06-15 (v8 no-confirm batch, owner away ~10-min /loop):**
  **① AI Deep Research COMPLETE** (increments 1–3 all live): anti-hallucination harness
  (`enrich.ComposeDeepReport`, Fable-5-modeled sections, structural number-safety via
  `parseSectionProse`) → login + **1 deep report / user / day** SITE-WIDE quota
  (`deep_research_quota` table; anon→401, over-quota→429, charged only on real prose) →
  report view `/[locale]/stock/[ticker]/research` (noindex,follow + AI-Digest top-right entry
  button). LIVE: `/en/stock/AAPL/research` 200, title "Apple Inc. · AI Deep Research". **Only the
  paywall (increment 4) is parked for the owner** (`docs/owner-confirm.md` #1/#2).
  **② LLM compose per-call timeout** (commit 88eb75c, `internal/api/api.go`): each AI compose
  (research/summary/movement/material-events, incl. depth=deep) is wrapped at the handler boundary
  in `context.WithTimeout` (**25s** normal / **60s** deep); the enrich methods build requests via
  `http.NewRequestWithContext`, so the deadline cancels the REAL outbound HTTP call — an uncached
  request degrades to data-only FAST instead of the enrich client's ~90s ceiling. **LIVE-verified:
  uncached SOFI/research returned data-only 200 at 27s (= the 25s bound + overhead), vs the old
  ~90s.** All four refund paths fire on timeout (research `refundGlobalCap`, movement/material
  `…DayCount--`, summary `refundCap`); the deep per-user quota is charged only on real prose so a
  timeout never burns it; `getMaterialEvents` re-runs facts-only if the deadline fires during the
  EDGAR fetch (a timeout can't 404 a valid ticker); `getSummary` degrades to a 200 empty-digest
  (genuine upstream errors still 502). Anti-hallucination tests byte-identical.
  **③ Soft sign-in gate on the per-stock IndicatorsPanel** (commit 940555d, web growth nudge):
  anon sees the first 5 indicator rows + a gentle "sign in to see all N" CTA (`LocalLink`,
  locale-prefixed); logged-in unchanged (full set + customize picker). Pure CLIENT-SIDE view layer
  (the panel fetches client-side → **zero crawl impact**); the public pSEO `/indicators` +
  `/indicators/[id]` LIBRARY pages are untouched + stay crawlable (LIVE: `/en/indicators` 200).
  **④ Opportunity-board us-gaap shares fallback** (commit e864dce, data-coverage; LIVE-verified):
  `refreshShares` now falls back to `us-gaap:CommonStockSharesOutstanding` for CIKs the canonical
  `dei:EntityCommonStockSharesOutstanding` frame leaves unresolved (dei stays canonical, fallback
  never overrides), behind a 450-day staleness guard + 0/1-share plausibility guard at the frame
  layer — widening the board without admitting a wrong cap (insufficient-not-wrong). Cap band/
  MinBuyValue/ranking + keep-last-good untouched. **LIVE: board 4 → 13 rows, ALL caps in-band
  ($334M–$1.68B); startup log `refreshed shares ciks=5796 via_fallback=216` = 216 dei-less CIKs
  resolved via the us-gaap fallback.**
- **Known pre-existing issue found 2026-06-15 (cold-ticker research intermittent ~3s empty-reply):**
  an UNCACHED first request to a cold ticker's on-demand research INTERMITTENTLY returns an empty
  reply / reset at **~exactly 3.0s** (curl exit 52 `CURLE_GOT_NOTHING` on HTTP/1.1, exit 16
  `CURLE_HTTP2` on HTTP/2), no CF error headers, no origin panic; an immediate retry succeeds. Not
  all cold tickers hit it (DKNG cold→200@9.2s, SOFI cold→200@27s succeeded; ZS/DDOG/RBLX cold→000@3s).
  **Root-caused to the Cloudflare Tunnel hop, NOT a code bug** (Go has no `WriteTimeout`, no `3*time.
  Second` literal, no panic; a CF edge 524 would carry `cf-ray` — these don't; `cloudflared` is a
  TOKEN tunnel = ingress/timeouts in the CF Zero-Trust dashboard, not a local config). NOT caused by
  the ② LLM-timeout change (the reset is in the cold fact-sheet ASSEMBLY, before the 25s/60s compose).
  **Practical impact LOW** for deep research (reached from an already-warmed /stock page). Mitigations
  (deferred): (a) frontend retry-once on a network/empty-reply error in ResearchReport/
  DeepResearchView (cheap, next up); (b) CF dashboard tunnel HTTP-timeout tuning (owner); (c) async
  report generation. In `docs/owner-confirm.md` #5 + memory `tickwind-cold-research-3s-reset`.
- **Shipped + LIVE-verified 2026-06-15 (capital-flows data correctness audit — `9b1fb2c`):** an
  adversarial Workflow audit (6 finders, one per subsystem feeding the AI research report 资金面
  section, each finding refuted by an independent skeptic) found **7 confirmed correctness bugs** on
  numbers Go computes and the report presents as authoritative; **FINRA short-volume, short-interest,
  and insider Form 4 audited CLEAN**; 2 findings correctly refuted. All 7 fixed: **options/cboe** —
  (a) `MaxPain` was non-deterministic on tie strikes (Go map iteration) → now sorts strikes ascending
  + lower-strike tie-break (deterministic; pain formula unchanged); (b) `MaxPain` emitted a degenerate
  magnet from a 1-strike sparse expiry → now requires ≥`minMaxPainStrikes`(3) distinct OI-bearing
  strikes else returns 0 (insufficient-not-wrong). **13F** — (c) PRN-only (bond) positions (Shares=0)
  were tagged "新建仓/new" every quarter → now classify by Value delta when shares are 0; (d) the
  "持仓机构数" holder count silently undercounted (only top-15 positions indexed) → reverse index now
  walks every position, only the rendered list stays top-15-capped (weight % still uses the full
  portfolio denominator); (e) aggregate count as-of used the largest holder's quarter → now the oldest.
  **congress** — (f) the PTR ticker chip had NO symbols-universe validation (every sibling money-flow
  path validates) → now validates extracted tickers against the US symbols universe (wired from main
  like the guru rail, nil-safe; residual real-ticker collisions like `(ON)` need asset-type
  disambiguation, deferred); (g) `wrappedAmountHigh` could adopt a narrative $ figure as the range high
  bound → now skips sub-rows + rejects high<low. Full combined `go build/vet/gofmt/test ./cmd/...
  ./internal/...` green; anti-hallucination contract intact. **LIVE no-regression verified on AAPL**
  (congress `$1,001-$15,000` band correct, whales Buffett 22% 维持/Dalio 加仓/Li Lu 维持 with
  2026-03-31 as-of, short 50.6%/3.38 dtc; options `/v1/stocks/AAPL/options` max_pain 292.5 sane =
  cboe fix happy-path-clean). Bonus: opp board grew 13→**25** rows as more us-gaap-fallback sweeps ran.
- **Known pre-existing gap found 2026-06-15 (research 资金面 silently omits OPTIONS for most tickers):**
  the research fact sheet is cached per (ticker, ET-day, lang), and the options block reads
  `OptionsCache.Cached` (cache-only, by design — never block assemble on a multi-MB Cboe fetch). But
  `OptionsCache.c.cache` (per-ticker views) is populated ONLY by an on-demand `Options()` call (a
  `/v1/stocks/{t}/options` or similar hit); **`scanUnusual` fetches each mega-cap's chain every 30m but
  only builds the unusual *board* — it never populates `c.cache`**. So unless someone hit `/options`
  for a ticker within 15m (optionsTTL) before its research report was first assembled that day, the
  report's options block is absent — and the per-day report cache then freezes that options-less sheet
  for the ET-day (confirmed: AAPL's 3 fetches shared one `generated_at`, assembled cold post-restart;
  `/options` showed max_pain 292.5 the whole time). NOT a regression from the audit fixes (cboe works).
  **FIXED + LIVE-verified (commit 3b0cb3d):** extracted the chain→`OptionsView` build into a pure
  `viewFromChain` helper (no fetch); `compute()` delegates to it (byte-identical), and `scanUnusual`
  reuses its SINGLE existing chain fetch to also build the view and write `c.cache[tk]` (same map/
  optionsEntry/TTL the on-demand path uses) — **no second Cboe pull**. The ~40 scanned mega-caps are
  now continuously warm, so their reports reliably include options; the post-restart cold window
  shrinks to the first scan (~40s). Non-scan thin names stay on-demand-only. **LIVE: AAPL report now
  shows max pain $292.50 + p/c 0.63/0.71; TSLA $400; NVDA $205 — all with Cboe ~15min-delayed
  attribution** (was: options block entirely absent). cboe MaxPain + the unusual board untouched.
  (Deploy SSH dropped mid-launch but the nohup'd script completed; verified container restart +
  DEPLOY_DONE before confirming the business result — no double build.)
- **Shipped + LIVE-verified 2026-06-15 (indicators-engine correctness audit — `f583b4e`):** an
  adversarial Workflow audit (7 family finders → independent-skeptic verify) of the ~161-indicator
  compute engine found **6 confirmed formula bugs**; the **oscillator family and the fundamental
  Group-0 ratios audited CLEAN**; 0 refuted. All 6 fixed: **technical** — KAMA was reseeded at a raw
  close `period` bars from the end and iterated only `period` steps (discarding ~250 bars of the IIR
  recursion, ~2.4% wrong in ranging markets) → now seeds once + iterates full history like its sibling
  MAs; KVO (Klinger) used signed raw volume (≈OBV) → now the canonical Volume Force
  `vol·|2(dm/cm)−1|·trend·100` with cumulative-measurement reset on trend flip; Parabolic SAR clamped
  against 1 prior bar and tested reversal on the un-clamped SAR → now Wilder's 2-prior-bar bound applied
  BEFORE the penetration test. **fundamental** — the EV/debt family fell back to
  `us-gaap:LiabilitiesNoncurrent` (TOTAL non-current liabilities) when a debt tag was absent, inflating
  EV/net-gearing/ROIC/EV-multiples → dropped that fallback to genuine debt tags only (absent → 0 →
  insufficient-not-wrong; also removes the silent concept-flip behind lt-debt-ratio); lt-debt-ratio
  catalog text corrected to "long-term debt / (debt + equity)" (code already computed the canonical
  debt-to-cap); Piotroski F-score point 5 (ΔLEVER) compared raw LTD dollars → now grades the LTD/
  TotalAssets *ratio* change per Piotroski (2000), fixing an off-by-one F-score (all-or-nothing
  preserved). Combined `go build/vet/gofmt/test ./cmd/... ./internal/...` green; load-bearing tests
  per fix. **LIVE-verified no-regression on AAPL** (all 6 present + status ok; KVO −168724→−23.9M,
  KAMA 305.31→303.32, SAR 312.84→313.89 = new formulas live; AAPL EV/lt-debt unchanged [explicit debt
  tag], Piotroski 7). **→ Two adversarial data audits this session (capital-flows + indicators) = 13
  real bugs fixed + 5 subsystems/families certified clean; the flagship report's data layer is
  hardened.** Anti-hallucination contract intact throughout.
- **Shipped + LIVE-verified 2026-06-15 (fundamentals-XBRL extraction audit — `e1eca41`, HIGH-sev fix):**
  a 3rd adversarial Workflow audit (5 finders: income stmt / balance sheet / cash flow / shares-dei /
  helper-semantics) of the extraction layer feeding the paid report + fundamentals panel + ~100
  indicators found **1 HIGH-severity bug**; the other 4 families audited CLEAN; 0 refuted. **Bug:
  prior-year selection (`annualForFY`) matched SEC companyfacts' report-context `fy` field, not the
  period's actual end-date year.** A 10-K embeds its 2-3 prior years as comparative columns, all
  re-stamped with the FILING's `fy` + one shared `filed` date, so all matched the target year and the
  oldest (SEC sorts ascending by end-date) won deterministically → every prior-year flow resolved to
  the WRONG fiscal year. Live repro: Apple FY2025 `revenue_prior` = FY2022 ($394.3B) instead of FY2024
  ($391.0B) — corrupting RevenuePrior/NetIncomePrior/EPSPrior/GrossProfitPrior/OperatingIncomePrior +
  same-FY COGS → every YoY growth, dROA, Piotroski delta, gross margin, turnover. **Fix:**
  `annualForEndYear(endYr)` keyed on the period's own end-date year (`endYear` helper) + the
  End-then-Filed tie-break used by `latestAnnual`/`latestInstant`; 7 callers pass `endYear(primary)-1`
  (prior) / `endYear(revPt)` (same-FY COGS). Primary/latest values (sorted by End) untouched; regression
  test reproduces the multi-`fy`-context collision (fails pre-fix, passes post-fix). **LIVE-verified:
  AAPL revenue_prior 394328→391035, NI_prior 99803→93736, revenue-growth 5.54%→6.43%, earnings-growth
  12.23%→19.50% (all now vs FY2024); asset-growth unchanged 12.03% [balance-sheet-derived via
  priorInstant = correctly out of scope]; MSFT prior FY2024 correct.** Corrects YoY/growth/Piotroski for
  ALL filers. **→ THREE adversarial data audits this session (capital-flows + indicators +
  fundamentals-XBRL) = 14 real bugs fixed (7+6+1) + 9 subsystems/families certified clean; the paid
  flagship's data layer is thoroughly hardened. Audit phase closed — pivoting to feature/SEO/UX.**
- **Shipped + LIVE-verified 2026-06-15 (pSEO A-Z `/stocks` directory — `0729777`, first post-audit pivot):**
  a crawlable stock directory — `/stocks` hub (A-Z index) + `/stocks/[letter]` pages (26×2 locales)
  listing the quote-bearing tickers per letter, each linking to `/stock/{ticker}` — aids Google crawl
  discovery + internal linking for the ~6,695 quote-bearing pages + a browse-all-stocks UX. Cloned the
  proven pSEO shape (topic/[key] hub + screen/[preset] list): generateStaticParams letters×locales
  (+54 static pages → 972 total), per-locale generateMetadata + langAlternates, single-locale render,
  BreadcrumbList + CollectionPage/ItemList JSON-LD with locale-prefixed item URLs (0 bare-path leak),
  LocalLink, ISR, noindex-thin guard (<3 tickers or empty/error → noindex,follow; fail-open). Uses
  `quoteBearingTickers()` (not the 16k /v1/symbols set → no thin/dead links); no per-ticker name fetch.
  Added to the sitemap (+54) + a Footer + TopNav More entry point. **LIVE: /en/stocks + /en/stocks/{a,b,x}
  + /zh/stocks all 200; /en/stocks/a carries 173 distinct /stock/{ticker} links; /zh/stocks single-locale
  (`<html lang=zh>` + 美股代码大全); sitemap +54.**
- **Investigated + deferred 2026-06-15 (dual-class total market cap):** BRK.A/BRK.B show `market_cap=
  insufficient` (the stale-shares guard correctly zeroes the 2011-frozen undimensioned count).
  Investigate-first verdict: **companyfacts (the only XBRL source the app calls) has NO dimensional
  data and NO current BRK share count** (only the 2011 freeze; frames API 404s on member paths) — the
  per-class current shares live only in raw inline-XBRL instance docs the app doesn't fetch. GOOGL/GOOG
  are ALREADY correct (companyfacts current aggregate 12.116B × class price ≈ $4.37T; verified GOOGL
  quote $360.87 is the real live price, not a 2x bug). So only "no-current-aggregate" dual-class filers
  (BRK-class) are affected. A correct fix needs a NEW raw-XBRL fetch+parse pipeline (FilingSummary →
  cover instance → `StatementClassOfStockAxis`-dimensioned shares + scale + TradingSymbol/
  NoTradingSymbolFlag join, excluding bond rows) + a proxy-price rule for non-traded classes — bespoke,
  per-issuer, low generality, for a few high-profile names. Math checks out ($1.066T BRK) but **deferred
  to owner** (`insufficient` is honest; ROI low). See `docs/owner-confirm.md` #6. Don't re-investigate.
- **Shipped + LIVE-verified 2026-06-15 (owner returned — problem batch):** ① **rate-cut odds removed
  from the homepage** (`5d46f22`) → relocated to `/calendar/macro` (its documented home, below the
  events timeline). ② **new-IPO price** (`9b4ca63`): `BarCache.LatestQuote` now falls back to the
  latest REAL daily candle close (`source="daily"`, `session="closed"`, PrevClose==Price so no phantom
  change, never fabricated) when the live snapshot + Yahoo consolidated overlay are both empty — fixes
  brand-new IPOs (e.g. SPCX) showing no price on the cards while the K-line had it (free Alpaca=IEX-only,
  a thin new listing has no IEX print yet). LIVE: SPCX $173.95. ③ **movement explainer language**
  (`9b4ca63`): "Why's it moving?" showed Chinese even in EN mode — the movement CALLER built the LLM
  user-prompt material + the canned data-only line + the Go evidence titles/source labels in Chinese
  ONLY (biasing the model to answer Chinese under the EN system prompt). Now built per `lang`
  (`Assemble`/`Report`/`Material` thread lang). Audited every other backend LLM-text path
  (summary/my-digest/research/material-events/briefing) — all already respected lang; movement was the
  only one. Anti-hallucination contract unchanged. ④ **per-IP rate limiter** (`c7eae11`,
  `internal/ratelimit`): token bucket (default 300 rpm / 60 burst, env `RATELIMIT_RPM`/`RATELIMIT_BURST`),
  client IP from **CF-Connecting-IP** (behind the CF Tunnel), exempt `/healthz` + `/v1/stream`, fail-open,
  429+Retry-After, sharded + idle-sweeper, wired in `cmd/server/main.go` (not api.go). Added because
  requests jumped to ~40-50k from ~20 users (scrapers). **LIVE-verified SAFE: a 45-req page-load burst →
  all 200 (legit unaffected); a 150-req burst → 106×429 (bot throttled); /healthz 200 throughout.**
  ⑤ **Material events + Insider activity moved lower** on StockView (`bb3ae06`, `#material-events`/
  `#insider-activity` anchors preserved for research-citation deep-links). The frontend zh/en audit found
  NO other hardcoded-Chinese-in-EN bugs (i18n otherwise solid). All deployed + verified.
- **Shipped + LIVE-verified 2026-06-15/16 (owner returned, GO on the dev plan):** ⓐ **"Possibly related"
  Chinese-in-EN fix** (`6a83a44`): `movement.gatherEvidence` always used `n.HeadlineZH` regardless of
  lang → now picks the original `Headline` for en (HeadlineZH is the zh translation). (Same
  HeadlineZH-first pattern exists in `research/sentiment.go corpusContext`, but that's LLM *input*, not
  user-visible — left for a later lang-through-Assemble pass.) ⓑ **AI Deep Research = ASYNC + MONTHLY
  quota** (`fcd004f` backend + `3eddd3a` frontend): the deep report (`?depth=deep`) was synchronous
  (blocked 10-60s). Now returns the data-only fact sheet INSTANTLY with `prose_status` ∈ ready /
  generating / quota_exhausted / llm_disabled; a DETACHED background goroutine (`context.Background()` +
  60s timeout, its own `cloneFactSheet` to fix an in-place-prose data race) composes the prose, caches
  it, and increments the quota EXACTLY ONCE on success; atomic single-flight (one gen per
  ticker/ET-month/lang) + global-cap refund on empty/panic. Quota changed day→**ET-month** (1 report/
  user/month, `DEEP_RESEARCH_MONTHLY_LIMIT`); over-quota is a graceful data-only 200 (`quota_exhausted`),
  not a hard 429. Frontend `DeepResearchView` polls ~4s (cap ~25/~100s) on `generating`, shows a loading
  affordance / monthly-limit note, and is **backward-compatible** (absent prose_status or prose-present
  → done, no poll → safe deploy window). Normal (non-deep) /research stays synchronous + unchanged.
  **Model**: `LLM_DEEP_MODEL=deepseek/deepseek-r1` set in the VPS `.env` (owner-approved; deep path uses
  R1, stronger; falls back to LLM_MODEL=V3-0324-paid if unset). **LIVE-verified**: async backend deployed
  (DEPLOY_DONE 16:15, `deep_monthly_limit=1`); no sync-path regression (PLTR /research llm=true / ready /
  6 sections; LLM healthy — morning briefing generated); anon-deep→401; frontend routes 200; per-IP rate
  limiter actively throttling bots (`rate limit exceeded ip=…` in logs); healthz ok after the R1
  recreate. Authed async/quota/R1 flow is owner-visual (login → report opens instantly, prose fills in).
  **NOTE: `deepseek-chat-v3-0324` is PAID not free (owner clarified, $5 topped up) — intermittent
  data-only is transient throttle/timeout/cap; the system degrades correctly (insufficient-not-wrong).**
- **Shipped + LIVE 2026-06-16 (plan item D — multi-tab stock page, `df00170`):** `StockView` split into
  **Overview** (price/quote header always visible; then Why's-it-moving, fundamentals, AI summary,
  K-line, indicators, pulse) + **Details** (earnings/short, material events, insider, congress, whales,
  options, + the existing per-section tab bar: news/discussion/comments/research/notes/alerts/holdings).
  Both panels stay mounted via `hidden` (not unmount) so every `scroll-mt-20` anchor is in the DOM and
  research-citation deep-links resolve across tabs (an `ANCHOR_TAB` map + a mount/hashchange effect
  auto-switches to the owning tab then scrolls). i18n `stock.tabOverview`/`tabDetails` (en+zh). Routes
  /en|/zh/stock/AAPL 200. **Plan status (owner GO 2026-06-16): A✅ D✅ done+verified; C = app-layer
  per-IP rate limiter ✅ live + CF-dashboard edge rules are the owner's action; B (Yahoo removal)
  awaits the owner's HK-quotes decision (Claude rec: keep Yahoo+HK until near the deferred paywall).**
- **Shipped 2026-06-17 (owner decided: remove Yahoo now; investigated a slowdown):** **B — Yahoo removed**
  (`782163d`, LIVE-verified: `/v1/indices` now empty → frontend uses the keyless Alpaca ETF proxies
  SPY/DIA/QQQ; US prices intact via Alpaca + the daily-candle fallback; HK 0700/2513/0100 + Hang Seng
  drop; F&G re-weights around the removed VIX). A follow-up guard (`bcfb6f8`, **LIVE-verified 2026-06-17**)
  makes `getQuote` drop a lingering `source="yahoo"` stored quote (HK was still serving a stale Yahoo
  price post-removal) → HK falls to "—" (`0700.HK` now 404s "no quote yet"), US re-resolves to Alpaca
  (AAPL source=alpaca $298.21). **Slowdown (owner: site was slow/stuck ~6-7h ago) — diagnosed, NOT a fix yet:**
  root cause = **LLM (DeepSeek V3 / OpenRouter) ~12s upstream latency** in that window → the AI page
  endpoints `/summary` + `/movement` (+ `/research`) each took ~11.9s while the data endpoints stayed
  <1s; the box was healthy (load ~0, no OOM, no restart, normal ~3000 req/hr volume = NOT a bot surge,
  zero rate-limit hits in-window; `/v1/stream` SSE durations are normal long-lived connections). Proposed
  fix (awaiting owner GO): make `/summary` + `/movement` async like the deep research (instant data-only
  + bg prose + poll) so the page never blocks on the LLM. **C — owner is configuring the CF edge rules
  (WAF rate-limit /v1/* + Bot Fight Mode) themselves.** ⚠️SSH note: this stretch's rapid deploys/diagnostics
  throttled sshd (repeated 255 ack-drops); deploys still LAND despite the dropped ack (nohup sub-second),
  but back off + space attempts, and use a VPS-background-to-file + cat poll for heavy log scans (inline
  long scans drop the link).
- **Shipped + LIVE-verified 2026-06-17 (cont. — owner's 3 UI/i18n requirements + the Yahoo-quote guard):**
  **Req1 — flattened the stock-page tabs** (`331d5d4`, web): removed the "Details" wrapper sub-tab and
  promoted/consolidated its children so the top-level tabs are now **Overview / Research / Filings & Money /
  News & Discussion / My** (auth-gated) — same-category panels merged into one tab each. All panels stay
  mounted via `hidden` (not unmount) so every `scroll-mt-20` anchor stays in the DOM; the `ANCHOR_TAB` map
  (#short/#material-events/#insider-activity/#congress/#whales/#options→'Money', etc.) + a mount/hashchange
  effect auto-switch to the owning tab then scroll, so research-citation deep-links still resolve across tabs.
  New i18n keys stock.tabResearch/tabMoney/tabNews/tabMy. **Req2 — Deep Research "Export PDF"** (`331d5d4`,
  web): a dependency-free `ExportPdfButton` toggles a `tw-print-research` body class + `window.print()`
  (cleans up on `afterprint`); a scoped `@media print { body.tw-print-research … }` block in `globals.css`
  prints just the report. Image export **skipped** (would need a heavy html-to-canvas dep, out of proportion
  to the ask — owner said "简单的话可以做,不简单就算了"). **Req3 — Chinese-leaking-into-EN swept** (`b7727bf`,
  Go): owner flagged "Short Trend = 上升 / rising" + "Market Fear & Greed = 56 (贪婪)" under an `en` report;
  root cause = several Go-owned fact VALUES were hardcoded Chinese regardless of lang. Added a
  `pickLang(lang,en,zh)` helper (`internal/research/format.go`) and made them language-aware: flows
  `shortTrend` (上升/下降/横盘→rising/falling/flat), `tradeTypeLabel` (congress buy/sell), `changeTagLabel`
  (13F new/add/trim/exit), sentiment F&G label (`pickLang(lang,res.Label,res.LabelZh)` → "42 (Fear)" not
  "贪婪") + the social-mentions prior-value note, the loss-maker P/E "亏损"→"loss", the 3 source consts
  (srcCongress/srcThirteenF/srcInsiderSEC de-Chinesed → "House/Senate PTR"/"SEC 13F"/"SEC Form 4"), and the
  **disclaimers** in research + movement + materialevents (each `const Disclaimer`→`DisclaimerZH/EN` + a
  `disclaimerFor(lang)`/`pickLang`). Anti-hallucination contract intact (Go still owns every number; only
  label STRINGS became lang-aware); tests referencing the renamed const fixed; combined `go build/vet/gofmt/
  test ./cmd/... ./internal/...` green + web `tsc` clean. **+ Yahoo-quote guard now LIVE** (`bcfb6f8`, rode
  this Go deploy). **C (CF edge) configured by the owner:** Cloudflare rate-limit = **50 req / IP / 10s**
  (= 300/min, the only granularity the dashboard offers — matches the app-layer 300-rpm limiter). **LIVE-
  verified** (DEPLOY_DONE 16:10:03Z): `/v1/stocks/AAPL/research?lang=en` → 200, disclaimer "AI-generated ·
  figures from public data · not investment advice", **zero CJK in any fact value**, F&G "42 (Fear)"; guard
  → `0700.HK` 404 "no quote yet" (HK "—") while AAPL $298.21 source=alpaca; healthz ok; `/en/stock/AAPL` 200
  (flattened tabs + Export-PDF — the visual layout is owner-visual). **Still queued (awaiting owner's explicit
  "做"): make `/summary` + `/movement` async** (instant data-only + bg prose + poll, reusing the deep-research
  pattern) — the proposed root-cause fix for the LLM-latency page slowdown the owner reported.
- **Shipped 2026-06-17 (cont.² — systematic Chinese-in-EN audit, the owner's "复查" request):** the owner's
  hunch ("感觉发生好几次") was right. An adversarial Workflow audit (5 finders by subsystem → an independent
  skeptic refutes each candidate) swept every CJK literal across backend + frontend and found a whole class
  the Stage-2 home-card fix had missed: **the Open Graph / Twitter SHARE-CARD images rendered Chinese under
  `/en`** (9 candidates → **5 confirmed**, 4 refuted). `ogImageMeta(...)` was called with hardcoded Chinese
  eyebrow/title/subtitle + no `lang` on (a) the **root `layout.tsx` default** (openGraph + twitter — inherited
  by EVERY page without its own OG: ~6,000 `/en/stock/[ticker]` + /en/hot|news|discussion|congress|earnings|…)
  and (b) **4 static section pages** (smart-money / unusual / opportunities / calendar-ipo). The `/api/og`
  route renders title/eyebrow/subtitle VERBATIM (and deliberately fetches a Noto Sans SC subset), so an EN
  user's link previews on X/Telegram/Slack/微信 + the "save image" card were fully Chinese; with no `lang` the
  chrome badge also defaulted to "中文美股数据台". **Fix (`<this commit>`):** per-locale `lang: loc` + zh/en
  branch on the 4 section pages (matching the established home/topic/fund/indicators/congress pattern; commit
  `d7d83e8`), and the root default flipped to **English** (it sits above `[locale]`, the langAlternates x-default — per
  the owner's "single-value defaults to English" principle). A grep of ALL 15 `ogImageMeta` call sites confirms
  the other 10 were already locale-correct — no misses, the audit was complete for this class. Web build green
  (1024 static pages, TS clean). **LIVE-verified** (Vercel `d7d83e8`): the og:image on /en/{smart-money,
  opportunities,unusual,calendar-ipo,stock/AAPL,hot} now carries `lang=en` + an English title with zero CJK
  (stock/AAPL + hot confirm the root-default inheritance is English across the ~6,000 inheriting pages), while
  /zh/smart-money correctly keeps its Chinese card (国会山股神…) — no over-correction. The browser-tab `<title>`
  on these pages was already EN (LocalizedTitle) — only the OG image surface had been overlooked. **→ This closes the Chinese-in-EN class for the session (3 rounds:
  "Possibly related"/movement caller → research/movement/material labels+disclaimers → OG share cards).**
- **Shipped + LIVE-verified 2026-06-17 (cont.³ — ASYNC `/summary` + `/movement`, the slowdown root-cause fix;
  owner "继续开发" → started the queued #1):** the owner-reported "site slow/stuck" was diagnosed earlier as
  LLM (~12s) latency BLOCKING the AI page endpoints. Both are now ASYNC, mirroring the proven deep-research
  pattern (`getResearchDeep`): the handler returns INSTANTLY with a `prose_status`, and the LLM call moves to a
  DETACHED background goroutine. **`getSummary`** (commit `96c149b`, `internal/api/api.go`): cache/store hit →
  `ready`; miss → empty + `generating` + a `composeSummaryBackground` goroutine (single-flight via `sumInflight`,
  cap reserved-then-refunded-on-failure, `context.Background()`+timeout, caches AND persists on success); over-cap
  → 200 `quota_exhausted` (was a hard 429); 503 only when the LLM is disabled (unchanged). **`getMovement`**:
  returns the cheap canned data-only line INSTANTLY with `generating` and composes the ONE hedged LLM sentence in
  an `explainMovementBackground` goroutine, caching the upgrade; sub-threshold / LLM-off → terminal `ready`;
  over-cap → canned + `quota_exhausted`. Both add `prose_status` (ready|generating|llm_disabled|quota_exhausted)
  to the wire shape (`writeMovement`/`writeSummary`); the Go-owned numbers/evidence are unchanged + served
  instantly. INVARIANTS mirror `getResearchDeep` (one bg gen per (ticker,ET-day,lang), no double-charge, detached
  ctx, inflight gate always cleared). **Frontend** (`web/`, opus agent + my adversarial review): `AISummaryCard`
  + `MovementCard` poll every 4s (cap 25) while `generating` (mirroring `DeepResearchView`); MovementCard shows
  the canned line immediately and silently upgrades to the LLM sentence; **backward-compatible** (absent
  `prose_status` → the old synchronous behavior, no poll). Tests (`movement_test`, `summary_persistence_test`)
  made async/poll-aware (first response = generating; await the bg gen via a poll / the cap counter; fake counters
  atomic). Full `go build/vet/gofmt/test ./cmd/... ./internal/... -race` green + web build (1056 pages) clean.
  **LIVE-verified** (DEPLOY_LAUNCHED, `prose_status` present = new code serving): `/summary` AAPL **1.6s** (store
  hit, ready, 1265-char digest) · NVDA/SOFI **1.2–1.5s** (cold → instant empty `generating`) → a follow-up poll
  showed NVDA/SOFI flipped to **`ready` with real 1233/1412-char digests** (the bg goroutine completed + cached —
  the full async loop closes); `/movement` AAPL/TSLA/NVDA **0.97–1.4s** (all sub-threshold today → `ready`,
  card hides). **~1–1.6s vs the old ~12s block — the slowdown is root-cause fixed.** healthz ok; `/en/stock/AAPL`
  200. The movement significant-move `generating→canned→LLM-upgrade` path wasn't exercised live (no >5% mover at
  the time) but is unit-tested and uses the identical mechanism the summary path proved end-to-end.
- **Shipped 2026-06-14 (owner batch + greenlit follow-ups, all live-verified):** R2 now has all **6
  sections** (估值/基本面/技术面/资金面/情绪面/概览) + a **two-sided 看多/看空 (bull/bear)** reading on the
  overview (one ComposeReport call gains `bull`/`bear` keys; a deterministic Go advice-guard strips any
  point that slips into targets/buy-sell — chosen over an LLM self-critique; Go still owns every number).
  **Selectable per-stock indicators** (Phase A picker + Increment 1 Group-0 + **Increment 2 Groups 1/2/4**):
  per-stock fundamental set 19→**157 emitted ids** (148 ok for AAPL); new XBRL fields + ~39 ratios
  (margins/turnover/working-capital/EV/debt), anti-fabrication preserved; a `usd` unit renders FCF/EV
  compact ("$4.5T"). **Signed-in cloud prefs** `GET/PUT /v1/me/prefs` (opaque JSON blob, User store via
  Split, 8KB cap, shallow-merge) — IndicatorsPanel: server selection wins over localStorage + migrate-up.
  **Guru-watch staleness fix** (real cause was the `$cashtag`-only gate, NOT an IP block): rail = latest
  KOL posts newest-first, ≤2/author, chips only for universe-validated cashtag/exchange tickers (bare
  parentheticals dropped — acronym collisions). **Portfolio** now shows **当日盈亏 (day P&L)** + per-row
  today's-move + allocation %. **Already-shipped (verified this session, were stale in the roadmap):**
  Alerts Center (TopNav bell + `/me?tab=alerts` global list, triggered/active split + re-arm) and Chinese
  search (苹果→AAPL via the curated alias table + CJK routing).
- **Shipped 2026-06-14 (cont. — fast-cadence ~2-min loop, all live-verified):**
  **Indicators Increment 3 — composite scores**: `fundamental.altman-z-score` (bankruptcy Z; +RetainedEarnings
  field) + `fundamental.piotroski-f-score` (9-pt quality; +4 prior-FY fields) → **159 emitted ids**;
  all-or-nothing anti-fabrication (any missing input → insufficient, never a partial score). AAPL Z=12.09
  (safe) / F=7 verified. Beneish-M deliberately unsupported. **F&G index** now feeds **breadth** (advancers/
  decliners from the universe price cache) + **social heat** (hot-list mention momentum) → up to 5/6 components
  (new-highs/lows deferred — no 52w range in the universe snapshot); refresh changed 24h→**hourly** (intraday
  signals + boot-race fix; breadth populates within ~1h of an undisturbed box). **AI movement explainer**
  `GET /v1/stocks/{t}/movement` — move-triggered (|chg|>=5% → significant), Go owns change_pct, LLM writes ONE
  hedged Chinese sentence (`enrich.ExplainMove`) over attributed evidence (news/filings/insider), canned
  data-only fallback; `internal/movement` + research-pattern cache/cap/setter + StockView MovementCard.
  **52-week high/low** on the stock header (`getBars` year_high/low from the candle cache + a range bar with
  the price marker). **K-line crosshair OHLC legend** (hover readout). **US Treasury yield-curve macro strip**
  `GET /v1/macro` (`internal/treasury`, keyless Treasury.gov, 2Y/10Y + 2s10s recession signal, server-driven
  12h ingestor, home MacroStrip; tenors matched by header-name, spread null unless both legs present).
  **Crypto market-mood strip** `GET /v1/crypto` (`internal/cryptofg`, keyless alternative.me Fear & Greed +
  best-effort CoinGecko BTC/ETH spot, server-driven ingestor, home CryptoStrip). **Market beta + 1-year TSR**
  (Phase C risk/return): `fundamental.beta` (vs SPY, date-aligned daily-return covariance/variance, ≥60 pairs +
  var>0) + `fundamental.tsr` (total shareholder return %, price appreciation + dividends over ~1y, ≥240
  candles) → **161 emitted ids**; AAPL beta 1.20 / TSR 46.9% live-verified. **8-K material-event filings**
  `GET /v1/stocks/{t}/material-events` (`internal/materialevents` + `internal/edgar/material_events.go`) — a
  company's recent current-report filings (≤120 days, ≤10, newest-first). **Go owns** the canonical item-code→
  bilingual-label map (33 standard 8-K codes — 1.01 material agreement, 2.02 earnings, 5.02 officer changes,
  5.07 vote results, 9.01 exhibits, …; unknown codes → generic `Item X.XX`, never fabricated), form/dates/
  accession URL. **LLM writes only** a short factual per-filing summary over the primary-doc text (HTML→text,
  ~7k-rune cap); degrades to labels-only when LLM off / over daily cap / source too thin — never invents facts,
  never on the critical path. Per-ticker/ET-day/lang in-memory cache + daily LLM-report cap; on-demand
  server-driven refresh; StockView `FilingsCard` (bilingual, SEC EDGAR attribution + as-of). v2: EX-99.1
  exhibit fetch when the primary doc is thin. **LIVE-verified** (AAPL: 2.02 earnings + 9.01 exhibits, Chinese
  AI summary + disclaimer). **Insider Activity timeline** `GET /v1/stocks/{t}/insider-activity`
  (`internal/insideractivity` + `internal/edgar/insider_activity.go`) — a company's recent Form 4 open-market
  **buys AND sells** (≤25 within 90 days, newest-first), each `{type, owner, role, shares, price, value=
  shares×price, date, planned_10b5_1, accession_url}`. **Go owns every number** (pure structured data, NO LLM);
  the buy half of `sec.ParseForm4` is untouched (strictly additive `Sells`/`Sale` + `Date` + `Planned10b5_1`,
  so the Opportunity buy board is unaffected). **10b5-1 planned-sale flag**: document-level `<aff10b5One>`
  (the post-2023 SEC checkbox) is primary, a boundary-guarded footnote scan (`10b5-1` not followed by a digit,
  so "10b5-10" can't false-positive) is the pre-2023 backstop — never guessed, default false. Per-ticker/ET-day
  cache + single-flight; StockView `InsiderActivityCard` (green BUY / red SELL, 10b5-1 tag, SEC source + 2-day
  filing-delay note). Footnote-only-priced lines (weighted-avg, no `<value>`) are dropped, not fabricated.
  Shipped after a 5-dimension adversarial review (3 low/nit findings fixed: edgar.Client now self-throttles
  ≥120 ms/req like `sec.Client`, so the ≤25-filing sweep stays under SEC's 10 req/s — also hardens material-
  events). v2: derivative-table option exercises (code M). **LIVE-verified** (AAPL: 13 sells / 0 buys,
  net −$111.7 M; every value = shares×price; 10b5-1 correct — Levinson Director discretionary `false`,
  Borders/Parekh/O'Brien officer plan sells `true`).
- **Shipped 2026-06-14 (pSEO Stage 1 — path-based `/zh` `/en` i18n, the long-deferred hreflang unblock):**
  migrated the whole web app from client-side `?lang=` to **path-based locale URLs** (`/en/...`, `/zh/...`).
  The decisive move: reimplement `useT`/`useLang` internals to read the locale from a React **Context** fed by
  the `[locale]` route segment — so all **~592 `tr()` call sites stayed unchanged** and SSR now renders the
  correct language (no English-only first paint). `(main)`/`(auth)`/`designs` moved under `app/[locale]/`;
  `<html lang>` + providers live in `app/[locale]/layout.tsx`, root `layout.tsx` is a pass-through;
  `web/src/proxy.ts` middleware detects locale (cookie `tw-lang` → `Accept-Language` `zh*` → default `en`) and
  **308-redirects** bare paths (static-asset extension allowlist so dotted tickers like `BRK.B`/`0700.HK` still
  localize, not 404); `<LocalLink>` prefixes internal links (48 `next/link` imports swapped); the language
  toggle now `router.push`es the locale-swapped path (preserving query+hash). SEO is path-based: `langAlternates`
  emits per-locale canonical + `{en, zh-CN, x-default}` hreflang, `sitemap.ts` emits both `/en` and `/zh` per
  URL, `robots.ts` covers both; `generateStaticParams` is locale×slug (**632 static pages**, incl. indicators
  282×2). Added `app/not-found.tsx` (own `<html>`/`<body>`) + `app/[locale]/not-found.tsx`. Shipped after a
  5-dimension adversarial review (12 raised → **10 confirmed, 0 blockers**; the high [5 section pages
  canonicalizing to the homepage + no hreflang] + both medium [404 pages rendering html/body-less] + the lows
  all fixed and re-verified in built HTML). Default locale `en` (x-default); `zh` via Accept-Language/cookie.
  **LIVE-verified** on prod: both locales 200, bare paths 308 (incl. dotted tickers `BRK.B`→`/en/stock/BRK.B`),
  `/en/hot` canonical=`/en/hot` + en/zh-CN/x-default hreflang, `/zh` serves `<html lang="zh">` + Chinese,
  `/en/zzzz` 404 with valid `<html lang>`, sitemap emits both locales (2210 `/en` + 1326 `/zh` locs).
- **Shipped 2026-06-14 (pSEO Stage 2 — single-locale rendering):** the localized pSEO Server Components
  (home, guide hub + `[slug]`, indicators hub + `[id]`, fund/`[slug]`, congress/member/`[slug]`) now render
  ONLY the active locale's content (chosen from the `[locale]` route segment) instead of dual-rendering both
  languages behind the `[data-i18n]` CSS-hide — so `/en` and `/zh` ship **genuinely distinct single-language
  HTML** (verified: `/zh/guide` Chinese body only, `/en/guide` English only, per-locale `<title>`). JSON-LD
  (guide FAQPage q/a, breadcrumb labels), OG image (`lang` + active-locale eyebrow/title) and the tab title
  (now per-locale via `generateMetadata`, retiring `LocalizedTitle` on these pages) are all locale-correct.
  Shipped after a focused 2-dimension adversarial review (4 low/nit findings, 0 high; fixed: home per-locale
  OG card [was Chinese-only for both], fund + congress-member breadcrumb JSON-LD `item` URLs now locale-
  prefixed to match canonical). Left as a known data limitation: the indicator `definition`/`formula` catalog
  has no `_zh` field, so `/zh/indicators/[id]` DefinedTerm `description` stays English (name IS localized).
  Meta `keywords`/`description` intentionally stay mixed (deferred).
- **Shipped 2026-06-14 (pSEO Stage 3①+② — /stock scale-up):** **①** `GET /v1/symbols` (Go, additive
  `SymbolSearcher.AllUSTickers()` → `symbols.Cache.Get().USTickers()`) enumerates the full US symbols index —
  **LIVE count 16,118** (incl. AAPL + dotted BRK.B; `?limit=`; nil-safe). **②** `/stock/[ticker]` scaled with
  **three thin-content guards**: (a) `generateStaticParams` pre-renders ONLY the popular subset (`POPULAR_TICKERS`
  ∪ hot/surging/wsb ∪ opportunities, **130 pages = 65×2 locales**) — build stays bounded (762 pages, ~26 s) —
  everything else stays dynamic ISR (`revalidate=600`); (b) the **sitemap** lists only QUOTE-BEARING tickers
  (via `getScreen`, which drops price≤0) — **530 `/stock` URLs (265/locale)**, NOT the 16,118 (the ~9,400
  quote-less names are excluded); (c) per-page **noindex-when-thin** (`robots:{index:false,follow:true}` only
  when a ticker has NO quote AND NO fundamentals, **fail-open** — only a definitive 404 counts, so transient
  errors never deindex a real page). Shared `web/src/lib/pseo.ts` keeps the page + sitemap DRY. **Known limiter:**
  the backend `getScreen` hard-caps `limit` at 200, so the sitemap expansion is currently ~265/locale; reaching
  the full ~6,695 quote-bearing universe needs a Go follow-up (a price-universe ticker endpoint or raising the
  cap).
- **Shipped 2026-06-14 (pSEO Stage 3③ — quote-universe sitemap scale-up):** **`GET /v1/universe/symbols`** (Go,
  `universe.Cache.Tickers()` → the price-universe snapshot keys, LIVE **count 6,695** = matches `/v1/universe`,
  a strict subset of `/v1/symbols`' 16,118) lifts the `/v1/screen` 200-cap. The sitemap's `quoteBearingTickers`
  now sources it, so `/stock` sitemap URLs jumped **530 → 6,000** (3,000 tickers × 2 locales; `MAX_STOCK_URLS`
  measured at 3,000 for a young domain, popular-first union so mega-caps are never sliced; `generateStaticParams`
  still the popular ~130 only, build bounded; 2.8 MB sitemap). **⚠️ Discovered:** the Alpaca quote-universe
  EXCLUDES S&P mega-caps (AAPL/MSFT/NVDA absent from `/v1/universe/symbols` + `/v1/screen`, though `/v1/stocks/
  AAPL/quote` works on-demand) — a pre-existing data quirk that also means the **screener can't surface mega-caps**
  (flagged as a separate task to root-cause; pSEO sitemap unaffected — mega-caps come via the popular union).
- **Shipped 2026-06-14 (pSEO Stage 3④ — /screen/[preset] landing pages):** a new `/screen/[preset]` family —
  **9 curated screener landing pages × 2 locales = 18 prerendered** (`top-gainers`, `top-losers`,
  `penny-movers`, `penny-losers`, `small-cap-breakouts`, `big-decliners`, `premarket-movers`,
  `afterhours-movers`, `overnight-movers`) for high-intent queries ("美股今日涨幅榜 / Top Gaining US Stocks
  Today"). Each runs a fixed `getScreen` query (params verified against the Go handler: `sort` ∈ change/price
  asc·desc, `session` ∈ pre/regular/post/overnight; **no volume sort → "most-active" dropped**) and renders a
  ranked, internally-linked table into `/stock/{t}`. Clones the proven pSEO shape: `generateStaticParams`
  (preset×locale), per-locale `generateMetadata` + `langAlternates`, single-locale render, ItemList +
  BreadcrumbList JSON-LD (locale-prefixed `item` URLs), `revalidate=600`, graceful empty-state (session presets
  are empty off-hours, ISR refills). `web/src/lib/presets.ts` config + a preset cross-link hub on `/screen`.
  Built-HTML verified: single-locale, canonical+3 hreflang, 0 bare-path JSON-LD leak; sitemap +18. Note: the
  preset universe inherits the mega-cap exclusion (movers are mid/small-cap — fine for gainers/losers intent).
- **Shipped 2026-06-14 (pSEO Stage 3⑤ — /topic static; pSEO COMPLETE):** upgraded `/topic/[key]` from a
  thin `?label=` client wrapper into a static pSEO Server Component — `generateStaticParams` from `getTopics()`
  (8 live topics × 2 = **16 prerendered**, ISR `revalidate=1800` + dynamicParams for new themes), per-locale
  `generateMetadata` + `langAlternates`, single-locale crawlable render (verbatim news-derived label + localized
  chrome + a related-tickers grid linking into `/stock/{t}` + topic-scoped headlines via `getNewsBatch(…,topic)`),
  CollectionPage/ItemList/BreadcrumbList JSON-LD (locale-prefixed), `notFound()`+noindex for empty topics.
  Deleted the now-orphaned `TopicPage.tsx` client component. Built-HTML verified single-locale + canonical/3
  hreflang + 0 bare-path leak; sitemap +16. **→ pSEO COMPLETE (Stages 1–3, all LIVE-verified):** path-based
  `/zh`+`/en` i18n (hreflang unblocked) · single-locale distinct HTML · ~6,000 `/stock` + 564 indicator + 18
  screen-preset + 16 topic + guides/funds/congress/sections = **a few thousand indexable pages with valid
  hreflang**, up from a single client-toggle URL. Each stage shipped via adversarial review + built-HTML + live
  verification. **Optional follow-ups (deferred, owner's call):** A-Z `/stocks` directory from `/v1/symbols`;
  lift `MAX_STOCK_URLS` 3,000→~6,695 as crawl authority grows.
- **Fixed 2026-06-14 (screener/universe mega-cap exclusion — poison-batch bug):** the price universe + screener
  were silently missing **all S&P mega-caps** (AAPL/MSFT/NVDA/TSLA/GOOGL/AMZN/META…) plus ~3.7k other names.
  Root cause (live-reproduced against the prod Alpaca account): the SEC symbol directory writes class shares
  with a **hyphen** (`BRK-B`), but Alpaca wants a **dot** (`BRK.B`); `alpaca.SnapshotQuotes` batches 100/req and
  the SEC feed **front-loads the mega-caps into batch 0 alongside `BRK-B`** → Alpaca **400s the WHOLE 100-symbol
  batch** (`invalid symbol: BRK-B`) → the old `if !200 { continue }` silently dropped all 100 (every mega-cap).
  540 hyphenated tickers poisoned 38/105 batches. **Fix** (`internal/alpaca/alpaca.go`, no feed change — stays
  free `iex`): `NormalizeSymbol` maps the last hyphen class-suffix to a dot (`BRK-B`→`BRK.B`, foreign dot-form
  like `0700.HK` + plain tickers untouched), responses keyed by the canonical dot form; plus **recursive
  bisection** on HTTP 400 so a future bad symbol can only drop itself, never wipe a batch. `SnapshotQuotes`/
  `Snapshots` both hardened; partial-map-on-error preserved (only errors if NOTHING priced — not a regression).
  Live harness: priced **6,695 → 7,280** (+585), all mega-caps now `source=alpaca`. Verify post-deploy after the
  next universe sweep (`UNIVERSE_SWEEP_EVERY`, ~5 min): `/v1/universe/symbols` + `/v1/screen` contain AAPL/NVDA.
- **Fixed 2026-06-14 (data-integrity audit — 4 silent-data-loss fixes):** a 5-dimension adversarial audit
  (8 raised → 5 confirmed, 3 rejected as latent-not-manifesting) found more bugs of the poison-batch *class*
  (plausible-but-empty, no error). Fixed: **(HIGH) recent-IPO fundamentals** — SEC's companyfacts `cik` field is
  a number for old filers but a zero-padded STRING for newer ones; `factsResp.CIK int` failed the whole strict
  decode → ALL fundamentals + 78 fundamental indicators silently 404'd for RDDT/ARM/CART/CRWV/CAVA/RBRK… (the
  growing recent-IPO cohort); the field was unused → dropped it. **(HIGH) class/preferred share split-brain** —
  EDGAR keys class shares with a hyphen (`BRK-B`) but the app's canonical form is the dot (`BRK.B`, used by the
  universe/aliases/sitemap), so `/stock/BRK.B` (which the sitemap *publishes*) silently had a quote but NO
  fundamentals/filings/material-events/insider/research for ~539 class shares incl. mega-cap BRK.B. Fix: ONE
  shared `symbols.Canonical` (dot form; `alpaca.NormalizeSymbol` now delegates — identical logic, universe
  unaffected), SEC `symbols.FetchUS` canonicalizes (collapses the duplicate search hit, dedup keeps the CIK),
  and `edgar.lookup` retries the dot↔hyphen variants so `BRK.B`→CIK resolves for all 5 EDGAR consumers.
  **(LOW)** symbols refresh now folds last-good Nasdaq-Trader symbols on outage (was wholesale-clobbering ETFs)
  + a <50% shrink guard. **(LOW)** social-body LLM context truncated by rune not byte (was garbling CJK).
  gofmt/build/vet/-race all clean; new tests (string-cik fixture, `Canonical` mapping, class-share CIK round-trip,
  dedup-keeps-CIK). Process restart on deploy reloads the edgar tickerMap + symbols index, so a normal redeploy
  suffices. Verify live: `/v1/stocks/BRK.B/fundamentals` + `/v1/stocks/RDDT/fundamentals` → 200 w/ data.
- **Fixed 2026-06-14 (stale shares → wrong market cap):** verifying the class-share fix surfaced that BRK.B's
  `market_cap` read ~$460M (Berkshire is ~$1T). Root cause: `dei:EntityCommonStockSharesOutstanding` froze for a
  COHORT of older issuers — Berkshire's undimensioned concept has 7 points ending **2011** (941,481); `latestInstant`
  returned that 14-yr-stale count → 941,481 × $489 ≈ $460M. Same cohort: HEICO (2015), Bio-Rad (2010), Comcast
  (2009), Ford (2011); Paramount/Fox even carry garbage (0/1). (GOOGL/META/Lennar have NO undimensioned concept →
  fall to a current us-gaap fallback → already correct.) **Fix** (`internal/edgar/fundamentals.go`, anti-hallucination:
  insufficient-not-wrong): a clock-free recency guard — `extractFundamentals` records `SharesAsOf` (the chosen
  instant's End) and, if it's >`sharesStaleAfterDays=450` (~15 mo) older than `latestFinancialEnd` (newest of
  revenue/NI/equity/assets in the same payload), ZEROES `Shares`/`SharesPrior` so every shares-dependent output
  cascades to insufficient via the existing `Shares<=0` guard — `market_cap`, P/B, P/S, EV (+ ev/sales·fcf·ebitda),
  bvps/sps/fcfps/cfps/dps/capex-share, buyback/dividend yield, forward-P/E, Altman-Z. P/E (EPS-based), revenue,
  net income, equity are UNTOUCHED. Never nulls a current filer (they restate shares every 10-Q, ≤~1 quarter behind);
  no financial anchor → no-op (can't prove stale). Test `TestExtractFundamentals_StaleSharesGuard` (BRK-stale→zeroed
  but revenue kept / fresh→kept / no-anchor→kept). **Deferred:** correct dual-class total market cap (per-class
  dimensioned shares × per-class prices) + a sanity floor for recent garbage values (Paramount/Fox 0/1).
- **Ops (2026-06-14):** the new 4 GB VPS lacked the old box's fail2ban deploy-IP whitelist → a burst of
  deploy connects banned `154.29.158.47`; fixed durably via `/etc/fail2ban/jail.d/tickwind-ignore.conf`
  (owner VNC). The ssh unit on this box is **`ssh`, NOT `sshd`**. Box has 2 G swap + healthy RAM (not OOM).
- Phase 0 ✅ · Phase 1 ✅ · Phase 2 ✅ (prices REST + SSE live stream + frontend
  live price + Finnhub news; all auto-disable without keys). Alpaca prices
  LIVE-VERIFIED end-to-end with paper keys (local `.env`, gitignored). Finnhub
  news also LIVE-VERIFIED (real AAPL headlines). Phase 3: StockTwits social ✅
  (live-verified, no key) via `internal/stocktwits` → `Post` store →
  `/v1/stocks/{ticker}/social` + frontend `SocialFeed` (Discussion section).
  Social is now multi-source via `ingest.SocialSource` (Name + Posts) — **5
  post-based sources** in `cmd/server/main.go`'s `social` slice, each a small
  `internal/<src>` client with a `New()` + `_test.go`. The scheduler calls every
  source per ticker and `SaveSocial`s each batch; the store **merges by id**, so
  sources coexist (e.g. StockTwits + Tickertick = 60 posts for AAPL, verified).
  Sources: **StockTwits** + **Tickertick** are keyless & always on (Tickertick =
  free UGC/analysis links via `api.tickertick.com/feed?q=(and z:<t> ...)`,
  live-verified). **Reddit** rewritten to OAuth (`oauth.reddit.com`, password
  grant, UA `tickwind:com.tickwind.ingest:0.1`; the old public `.json` 403'd from
  datacenter IPs; the keyless `.rss` route is *also* 429-blocked from datacenter
  IPs — verified from the VPS — so only OAuth works server-side). **Reddit is NOT
  pursued** (owner's call, 2026-06): commercially restricted + its signal is
  already covered by ApeWisdom (buzz) + Tickertick (`T:ugc` Reddit links); the
  `internal/reddit` client stays in code, disabled by default. **Bluesky**
  `app.bsky.feed.searchPosts` (AT Protocol; session cached + 401-retry) is **LIVE**
  (creds in the VPS `.env`; ~30 finance posts/ticker, merged into the feed). **Xueqiu
  (雪球)** unofficial JSON, keyless (mints its own cookie via `/hq`), but datacenter
  IPs get soft-blocked (HTTP 200 empty body → 0 posts, no error — helps from
  residential/China egress). Each disabled/blocked source degrades to 0 posts, so
  the feed is robust. **Numeric signals** (a different shape from posts): a
  per-ticker `store.Signal` "pulse" (one row per (ticker, source); buzz facet =
  mentions/rank/upvotes/24h-deltas, sentiment facet = score/label/sample) via
  `ingest.SignalSource` (BULK — one call covers many tickers, run once per cycle
  after the per-ticker passes; `SaveSignals`/`ListSignals` upsert by (ticker,
  source), routed to the Market DB). Sources: **ApeWisdom** (`internal/apewisdom`,
  keyless — Reddit/WSB mention momentum; scans ≤3 leaderboard pages, stops when
  all wanted found) + **Alpha Vantage** (`internal/alphavantage`, NEWS_SENTIMENT,
  relevance-weighted per-ticker sentiment; free tier 25/day → the client
  self-budgets with a daily cap + ≥90-min refresh + cache, and a rate-limit reply
  marks the day spent; off without `ALPHAVANTAGE_API_KEY`). Served at
  `GET /v1/stocks/{ticker}/signals`; frontend `PulseBar` shows a buzz chip + a
  sentiment chip on the detail page (hidden when empty). **Trending hot list**: a
  market-wide `store.HotStock` leaderboard (one snapshot, replaced wholesale each
  cycle) built from ApeWisdom's top-40 via `ingest.HotSource` (the same apewisdom
  client doubles as SignalSource + HotSource). Heat = mentions × (1 +
  clamp(24h-growth, 0, 2)) — volume × momentum, computed/ranked in
  `ingest.rankHotList`+`heatScore` (unit-tested); served at `GET /v1/hot`, shown
  at `/hot` (`HotList`, TopNav "Hot") with mentions + Δ% (no opaque score). The
  hotlist pg table is replaced in a tx (clear+insert). **WSB board** ranks by 24h
  leaderboard rank-climb (`rank_24h_ago−rank`), NOT mention delta — ApeWisdom
  mention counts are an intraday accumulation so deltas read uniformly negative
  ("all declining"); rank is normalised → a real green/red mix (`rankClimb` +
  `RankPrev`, unit-tested). **Retention pruner** ✅ (`store.Pruner` +
  `internal/ingest/prune.go`, own goroutine off the request path, `PRUNE_EVERY`=6h):
  tiered DELETEs — news 60d/hot120, social 30d/hot90 (NEVER `source=substack`, the
  大V/Serenity rail), filings 730d, insider 90d, seen_form4 60d, + per-ticker caps
  500; hot-list tickers keep the longer window; Split forwards to the Market store;
  memory+postgres impls, tested. Env: `RETAIN_*`/`PROTECT_SOCIAL_SOURCES`/`CAP_*`.
  Clipper inbox ✅
  (`POST /v1/stocks/{ticker}/clip` → title fetch → `clip` Post; frontend paste box
  + Saved-links section). Phase 3 done. Phase 4 started: persisted editable
  watchlist ✅ (`/v1/watchlist` CRUD; scheduler + poller read it live, seeded from
  WATCHLIST; frontend add/remove board). Next in Phase 4: HK/KR FilingSource (needs
  DART key + HKEXnews scraping — deferred), optional LLM plugin, auth + polish.
- **热点话题条 (Hot Topics)** ✅: `internal/topics` — a curated keyword dictionary
  over ingested news, ranked by recency×momentum (generic-bucket demotion); atomic
  `topics.Cache` → `GET /v1/topics`, with a `?topic=` filter on `/v1/news`
  (`topics.Match`). Frontend `TopicsStrip` on the home hub.
- **机会榜 (Opportunity board)** ✅ LIVE: small-cap US stocks with **insider open-
  market buying** (SEC Form 4, code P). `internal/sec` (throttled EDGAR client:
  daily Form-4 index, `FetchForm4`, `ParseForm4` keeps only code P, dei
  shares-outstanding frames; dei `val` decoded as float64) → `store.InsiderBuy`
  (`SaveInsiderBuys`/`RecentInsiderBuys`, Market DB) → `internal/opportunity` (pure
  `Recompute`: gate market cap $300M–$2.5B = dei shares × Alpaca price, MinBuyValue
  $25k, rank by distinct buyers then $value; `ValidTicker`; atomic `Cache`), driven
  by `internal/ingest/opportunity.go` (`OpportunityIngestor`, own goroutine: sweeps
  the daily Form-4 index skipping seen accessions, backfills
  `OPPORTUNITY_BACKFILL_DAYS`, 2h ticker; needs Alpaca for prices). Market caps via
  `alpaca.Snapshots` (bulk ≤100/req, resilient — skips bad batches). Served
  `GET /v1/opportunities`; frontend `OpportunityBoard` at `/opportunities` (TopNav
  "Opportunities") — evidence-first cards ("3 insiders bought $1.2M", top buyers,
  "View SEC filing"), muted (no green-hero), on-card disclaimers. **Persisted
  seen-set** ✅ (no re-sweep on redeploy): processed Form-4 accessions are stored
  in the durable Market DB (`seen_form4` table, routed via Split; `MarkForm4Seen`
  upserts, `SeenForm4Since` loads on startup over backfill+7d/≥40d, pruned 60d).
  `OpportunityIngestor.loadSeen` seeds the in-memory set on boot — verified live
  (a restart logged `loaded seen form-4 count=3362`, board recomputed immediately).
- **大V / Guru-watch rail** ✅ LIVE: newsletter-cadence opinions from curated finance
  KOLs, anchored to tickers. `internal/substack` (public-RSS client + curated
  `Feeds` incl. **Serenity** `aleabitoreddit.substack.com/feed`; extracts cashtag
  tickers minus a stoplist; teaser only, never the full/paywalled body) →
  `internal/guru` (`Rank`: keep stock-anchored posts, dedupe by URL, newest-first,
  cap; atomic `Cache`), driven by `internal/ingest/guru.go` (`GuruIngestor`, own
  goroutine, 2h, key-free). Served `GET /v1/gurus`; frontend `GuruRail` under the
  board on `/opportunities` (author badge, $-chips deep-linking to the stock,
  "Source" link, "third-party opinions — not advice"). X/Twitter live tweets are
  NOT used (API blocked, $5k/mo) — newsletters are the proxy.
- **Home hub** = info-source entry (`HomeHub`): a live Markets strip + `TopicsStrip`,
  then **Boards & signals** (Hot stocks · Opportunity · Guru-watch) over **Feeds**
  (News · Discussion) — each module previews real items and links to its full page.
- **User features (2026-06, all live)**:
  - **私人笔记 / Notes** ✅ — per-user private notes (stock- and/or date-scoped).
    `store.Note` + `/v1/notes` (POST/GET/PATCH/DELETE, requireUser, ownership in the
    query → 404 not 403) routed to the **User** store via Split; frontend `NotesPanel`
    (compose + pinned-first list + pin/delete) on a StockView "Notes" tab + a `/notes`
    page. **Calendar view** ✅ (`NotesCalendar`): month grid over the existing
    `?from=&to=`, **compact cells** + a **two-column layout on `lg`** (grid + a sticky
    day-detail panel; defaults to today so the panel is never empty), with major
    **Events overlaid** as dots (reuses `getEvents`). `/notes` widens to `max-w-4xl`
    in calendar view.
  - **评论区 / Comments** ✅ — PUBLIC per-stock + global-board comments (§230 neutral
    host). `store.Comment` + `/v1/comments` (GET public; POST/DELETE/`{id}/report`
    auth) routed to the **Market** (durable) store; **safeguards**: per-user rate-limit
    (10/10min), report+flag, **soft-delete** (author-or-admin), admin takedown via
    `ADMIN_USER_IDS` env (matched by Supabase **UUID or email**, case-insensitive,
    via `Server.isAdmin`), IP+author+ts captured for moderation; author = email
    local-part (email/uid never exposed). Frontend `CommentsPanel` on a public
    StockView "Comments" tab + a `/community` page, with a "not investment advice"
    disclaimer. `ADMIN_USER_IDS` ✅ SET on the VPS (`allalphaplus@gmail.com`, via SSH).
    Owner TODO: finish DMCA agent registration (in progress — copyright.gov login error LG22,
    owner emailed their support) + add an on-site `/dmca` notice page before launch.
  - **K线 / K-line** ✅ — `store.Candle` + `alpaca.DailyOHLC` + `BarCache.DailyCandles`
    (≈260-bar cache) → `GET /v1/stocks/{ticker}/candles`; `web/src/lib/indicators.ts`
    (sma/ema/macd/rsi/bollinger, canonical formulas: SMA-seeded EMA, **Wilder** RSI,
    population-σ Bollinger; null warmup; compute over full history then slice);
    `KLineChart` (TradingView **lightweight-charts v5**, Apache-2.0, keep
    `attributionLogo`) — candles + MA5/10/20/60 + Volume/MACD/RSI panes, client-only.
    A **BOLL** legend chip toggles a dashed Bollinger (20,2σ) upper/lower envelope on
    the price pane (off by default; middle band = SMA20 = the MA20 line).
  - **财务信息 / Fundamentals** ✅ — free **SEC XBRL** (no quote
    license needed → safe for a future paid tier). `edgar.Fundamentals(ticker)` pulls companyfacts
    → latest-FY revenue/net-income/diluted-EPS + shares + equity (tag-priority; **weighted-avg
    shares fallback** for multi-class issuers like MSTR that omit dei shares). `ingest.FundamentalsCache`
    (24h + 1h-neg). `GET /v1/stocks/{t}/fundamentals` (`FundamentalsSource` in api) computes
    **market cap** (price×shares), **P/E** (price÷EPS, null for losses → 亏损/—), **P/B** from the
    live quote (polled, else on-demand). **Frontend `FundamentalsCard`** on StockView (compact
    6-cell grid 市值/市盈率/营收/净利润 + EPS/P/B, period chip, P/E→亏损 for losses, `fmtCompactUSD`
    T/B/M, hides on 404; i18n `fund.*`). ✅ **COMPLETE & live-verified** (AAPL $4.5T/PE41, MSTR $40.8B/PE—).
  - **提醒 / Alerts** ✅ v1 — per-user price/event alerts. `store.Alert`
    {ticker,kind,threshold,active,triggered_at} + `/v1/alerts` CRUD (requireUser, Split→User) +
    StockView auth-only "Alerts" tab (kinds: price_above/price_below/pct_move/new_filing).
    `ingest.AlertEvaluator` goroutine (every 2m): ListActiveAlerts → latest quote (BarCache) /
    latest filing → MarkAlertTriggered; frontend shows an in-app "triggered" badge. Memory+postgres
    +Split, `alerts` table, unit-tested, deployed. **web-push deferred** (owner; iOS needs a PWA).
  - **SEO** ✅ — `app/sitemap.ts` = popular ∪ live-board tickers (ISR, real-data only, ~60+ URLs);
    `/stock/[ticker]` SSR emits JSON-LD (`Corporation` + `BreadcrumbList` + financials `Dataset`) +
    canonical + company-name title (server-fetched security+fundamentals, ISR 10m). ⚠️ hreflang /
    bilingual SEO **deferred** — needs URL-level i18n (`?lang=` or `/zh|/en`); single-URL client
    toggle can't do valid hreflang.
- **On-demand collection** ✅ — `getStock` 404 for a REAL symbol (validated vs the
  symbol directory) fires `IngestOne` (fixes the "$MU all-empty" bug). `IngestOne` is
  **single-flight** (sync.Map per ticker → exactly one init collection). Frontend polls
  ~90s while collecting.
- **Commercialization risk** (for paid/AI later): see `docs/feature-research-2026-06.md`
  — **Alpaca + Yahoo quote redistribution is RED** (must move to a redistribution-
  licensed vendor before charging); SEC/Bluesky/TWSE green; Finnhub/ApeWisdom/Substack
  yellow.
- Frontend live price: `web/src/lib/useQuotes.ts` (one shared EventSource for all
  cards) + `PriceTag`/`SessionBadge`; shows "—" gracefully when `/quote` 404s.
- News: `internal/finnhub` → `News` store → `GET /v1/stocks/{ticker}/news`,
  ingested on the scheduler (needs `FINNHUB_TOKEN`); frontend `NewsTimeline`.
- API `?limit=` parsing is shared via `queryLimit` in `internal/api`.
- Prices: Alpaca REST **snapshot** (`/v2/stocks/{t}/snapshot`) → one call gives the
  latest all-session trade (feed-aware ET session classifier) **plus `prevDailyBar`
  close = `Quote.PrevClose`** (the day's change reference). `Quote` in store →
  `GET /v1/stocks/{ticker}/quote`. Poller auto-disables when `ALPACA_API_KEY/SECRET`
  are unset. Postgres `quotes.prev_close` column (idempotent `ADD COLUMN IF NOT
  EXISTS`); `GetQuote` `COALESCE(prev_close,0)`. Verified e2e locally.
  **Extended-hours freshness fallback (2026-06-12, fixes "RDW frozen at 17.09
  in post-market"):** the on-demand `BarCache.LatestQuote` overlays a fresher
  consolidated print when the IEX trade is stale (>5min) — thin names go
  hours between IEX prints. This fallback was Finnhub `/quote`, but Finnhub's
  **free `/quote` freezes `c`/`t` at the 16:00 ET close in pre/post-market** →
  showed the stale close labeled "post". Now uses **`yahoo.Consolidated`**
  (`yahoo.ExtendedQuote` reads the `includePrePost` 1-min chart series, last
  non-null print = the real pre/post price; Yahoo's `meta.regularMarketPrice`
  also freezes, so MUST use the series). `overlayConsolidated` labels source
  `"yahoo"`; the extended-hours prev_close anchoring (→ Alpaca daily-bar
  regular close) is unchanged → no phantom day-change. Yahoo = owner-authorized
  gray source (also HK), keyless, ~15min delayed + labeled, free display only,
  replace before any paid tier. **Note:** only the **on-demand** path (most of
  the market) has this; the **poller** (popular ∪ watchlist set) is still
  Alpaca-only — liquid names have IEX after-hours prints so it's minor, but a
  thin *watchlisted* name can still freeze post-market (backlog: wire the same
  Yahoo fallback into `PricePoller`).
- Live push: `GET /v1/stream` = Server-Sent Events via `internal/stream.Hub`
  (chose SSE over WebSocket — one-way, stdlib-only). Poller publishes each quote;
  handler sends an initial `: connected` so headers flush immediately.
- Frontend lives in `web/` (Next 16, src-dir layout): `src/app` (pages),
  `src/components`, `src/lib`. Static export to `web/out`. Detail page is
  `/stock?ticker=XYZ` (query param, no dynamic route — keeps export simple).
- Backend packages: `internal/{config,store,store/memory,store/postgres,edgar,alpaca,ingest,api}`.

## LLM enrichment (optional)
- `internal/enrich`: `Enricher` interface + `Noop` (disabled) + OpenAI-compatible
  HTTP impl (stdlib). `enrich.New(Config)` returns Noop when `LLM_API_KEY` is empty.
- `GET /v1/stocks/{ticker}/summary` summarizes recent news+social; returns 503 when
  disabled. Set `LLM_API_KEY` (+ optional `LLM_BASE_URL`, `LLM_MODEL`) to enable.
- Stays off the critical path (per the engineering-first requirement).

## Multi-tenant + auth (商用)
- `internal/auth`: stdlib verify of Supabase JWTs. **Dispatches on `alg`:
  `ES256` → verified against the project's JWKS public keys (Supabase signs user
  tokens with asymmetric ECC keys now — this is what real logins use), `HS256` →
  legacy shared secret. Each alg uses its own key type, so no alg confusion.**
  JWKS fetched from `SUPABASE_URL/auth/v1/.well-known/jwks.json` (cached, refetch
  on unknown kid, rate-limited). `Middleware` attaches the user when a valid
  bearer token is present (does NOT reject anon — handlers gate via `requireUser`).
  Config: `SUPABASE_URL` (ES256, required for login) + optional
  `SUPABASE_JWT_SECRET` (HS256). Tested incl. real ES256 via a test JWKS.
- Data split: **shared/global** (securities, filings, quotes, news, social =
  public market data) vs **per-user** (watchlist + private clips, keyed by the
  JWT `sub` UUID). Public stock-data endpoints stay open (SEO); watchlist/clip
  endpoints 401 without a token.
- Ingestion: `ingestTickers` = default `WATCHLIST` ∪ `store.AllWatchlistTickers()`
  (deduped, capped at maxIngestTickers).
- **Split storage** (owner's call): the collected/scraped corpus (securities,
  filings, quotes, news, social) is expensive to re-collect → keep it on a
  **durable** DB (`MARKET_DATABASE_URL`, e.g. Supabase). Per-user data (watchlist,
  clips) is cheap to rebuild → keep it **local** (`USER_DATABASE_URL`, the VPS
  Postgres; OK to lose). `store.Split{Market,User}` routes each method to the right
  backend and satisfies `store.Store`, so api/ingest are unaware. main.go builds
  the Split only when BOTH urls are set; else single `DATABASE_URL` (back-compat).
  Both DBs run the same idempotent schema (unused tables stay empty). Tested in
  `internal/store/split_test.go` (routing via two memory stores).
- Config: `SUPABASE_JWT_SECRET` (HS256, auth) + `MARKET_DATABASE_URL` +
  `USER_DATABASE_URL` (or single `DATABASE_URL`). docker-compose points
  `USER_DATABASE_URL` at the local pg and `MARKET_DATABASE_URL` at `.env`.

## Frontend — "Aurora" data-first app (`web/`)
- **Data-first, no marketing page** (explicit user direction). Layout (per owner):
  a compact **horizontal stock strip** over a two-column **News** + **Discussion**
  feed aggregated across those tickers (each item tagged with its ticker). One
  `Board` component, `variant` prop: **`/` = Markets** (`POPULAR_TICKERS`, public)
  and **`/watchlist` = the signed-in user's tickers** (separate pages so logged-in
  users switch via the TopNav Markets/Watchlist links). Backed by batched
  `GET /v1/news`, `/v1/social`, `/v1/bars` (`?tickers=…`, **deduped by id** — a
  post/article can be tagged to several tickers, capped). All list endpoints return
  `[]` not `null`, and feed setters coerce `?? []` (a null list once crashed the
  Saved-links tab via `null.length`). Synthesized from the user's design.
- **Design system** in `web/src/components/ui/` + `web/src/lib/ui.ts` (tokens):
  light-first Aurora (teal `#2DD4BF`/sky `#0EA5E9`) with a dark variant. Signature
  `SessionBadge` (pre=amber, regular=emerald+livedot, post=violet, overnight=blue,
  closed=slate — keyed to the API's `Quote.session`), `PriceTag` (flashes on tick),
  `TimelineItem` (news/disc/clip/filing), empty/error/skeleton, toasts, Inter font,
  CSS motion in `globals.css`.
- **Theme**: `.dark` class on `<html>`, read via `useSyncExternalStore` (single
  source of truth = the DOM class) + a no-flash inline script in `layout.tsx`.
  `useTheme`/`useDark` in `web/src/lib/theme.tsx`. Default light.
- **i18n** (zh/en) ✅ mirrors the theme pattern: chosen language lives on `<html lang>`
  (no-flash inline script + `useSyncExternalStore`), single source of truth in
  `web/src/lib/i18n.tsx` (`useLang`/`useT`; `tr=useT()` in components since `t`=tokens).
  TopNav has a 中/EN toggle. **All user-facing chrome is translated** — nav, home hub,
  Board (Markets/Watchlist), Hot, News/Discussion (FeedPage), Opportunities, Guru, WSB,
  Events, stock detail (StockView + PulseBar), Topics, error/empty states, auth
  (login/signup), Footer, Settings, feed timestamps. Data (prices, headlines, company
  names, source/platform labels) shows as-sourced. `{t}`/`{n}` placeholders +`.replace()`
  for interpolation. Tab/board keys stay English where they double as state. Left in
  English by design: the `/announcements` changelog (release-notes content).
  **Owner principle (2026-06): a single-language-only value defaults to ENGLISH.**
  Page `<title>`s (browser tab) were Chinese-led for SEO but showed Chinese on the
  English UI — fixed: `metadata.title` is now an absolute ENGLISH string (the crawl +
  default), and `components/LocalizedTitle.tsx` (client, reads `useLang`) swaps in the
  zh title for Chinese users. Applied to `/`, `/smart-money`, `/unusual`,
  `/opportunities`. Chinese SEO keywords stay in `description`/`keywords`/SSR body.
  Use the same pattern for any future single-value-but-bilingual UI.
- **Auth**: `web/src/lib/auth.tsx` (`AuthProvider`/`useAuth`) tracks the Supabase
  user + exposes `getToken()`; `web/src/lib/api.ts` private calls take that token
  as `Authorization: Bearer`. `web/src/proxy.ts` refreshes the session cookie
  (Next 16 renamed `middleware`→`proxy`; guarded no-op when Supabase env is unset).
  Email/password + optional **Google OAuth** (`signInWithOAuth` → `/auth/callback`
  route's `exchangeCodeForSession`); the Google button is gated behind
  `GOOGLE_OAUTH_ENABLED` (`NEXT_PUBLIC_GOOGLE_OAUTH=1`), hidden until configured.
- **Routes**: route groups — `(main)` = chrome (TopNav+Footer): `/`, `/stock/[ticker]`,
  `/settings`, `/announcements`; `(auth)` = centered: `/login`, `/signup`; `/designs/*`
  kept as references (self-contained). `/stock/[ticker]` is SSR with SEO metadata.
- **Responsive**: mobile-first; board/detail reflow to a single column. **TopNav**
  (rebuilt 2026-06): nav destinations come from one shared `NavItem[]` source
  (primary = Opportunities/Markets/Hot/News, `secondary` = Events/Community/+Notes-authed
  in a `More▾` dropdown). **Watchlist** is a top-level pill **when signed in** (also in
  the account menu). **< md** the desktop nav is replaced by a **hamburger → full
  mobile menu** (all destinations incl. Watchlist/Notes when authed + What's new) — the
  bar previously had NO nav links on mobile. Inline ticker search shows at **`lg`**
  (icon→dropdown below lg). Login+Signup stay visible at all widths (fits at 375px).
- **A11y**: global theme-aware keyboard focus ring in `globals.css` (`:focus-visible`
  + `--tw-focus`; element-type selectors outrank Tailwind `outline-none`, so it's
  keyboard-only). aria-current on active nav, aria-pressed + dynamic label on the
  theme toggle, aria-expanded/haspopup on the account menu + mobile search,
  aria-pressed on detail tabs; Escape closes the account menu + mobile search.
- **SEO/deploy**: `lib/config.SITE_URL` (`NEXT_PUBLIC_SITE_URL` → prod) drives
  `metadataBase` + OpenGraph in `layout.tsx`, `app/robots.ts`, and `app/sitemap.ts`
  (board + announcements + popular stock pages). `next.config.ts` sets baseline
  security headers. Frontend deploys on **Vercel** (Root Directory `web/`); see
  `DEPLOY.md` §5. CSP intentionally deferred (would need a nonce for the no-flash
  script + allowances for Supabase/API/SSE).
- **ChangeLine renders** the day's change (signed %/▲▼) on the board tile + detail
  header whenever `quote.prev_close` is present (real Alpaca data). **Sparkline
  renders** on the detail header (`GET /v1/stocks/{ticker}/bars`) and on every
  board tile (batched `GET /v1/bars?tickers=…` — parallel fan-out over the cache,
  capped at 30, one request per board). Alpaca daily closes via `ingest.BarCache`
  (cached 1h); `api.BarSource` iface, nil-safe → empty when Alpaca is off. Still no
  fake data: empty series → nothing rendered.
- Verify: `cd web && npm run lint && npm run build` (both green; 9 lint *warnings*
  are the experimental React-Compiler rules on intentional client-fetch/mount
  patterns, downgraded to warn in `eslint.config.mjs`).
- Env (`web/.env.local`, gitignored): `NEXT_PUBLIC_API_BASE`,
  `NEXT_PUBLIC_SUPABASE_URL`, `NEXT_PUBLIC_SUPABASE_ANON_KEY`.

## Tests / CI
- `make test` = `go test ./cmd/... ./internal/...` (scoped to skip `web/node_modules`).
- **CI** ✅ `.github/workflows/ci.yml` (push + PR to `main`): job **go** (build · vet ·
  gofmt-must-be-empty · `go test ./cmd/... ./internal/...`, `go-version-file: go.mod`)
  + job **web** (`npm ci` · lint · build, placeholder `NEXT_PUBLIC_*`). Actions pinned
  to **@v6** (Node24-ready). Watch a run: `gh run watch <id> --exit-status`. Green-verified.
- Covered: memory store, clip title extraction, alpaca session classifier, API
  httptest flows (health, watchlist CRUD, clip→social), and the **bars endpoints**
  (`internal/api/bars_test.go`: `/v1/bars` dedupe + cap + nil-source→empty via a
  fake `BarSource`, and the single `/v1/stocks/{t}/bars`). Each social source has a
  table-driven `_test.go` (httptest, incl. Reddit `-race`); network-dependent
  clients (edgar/finnhub/stocktwits/reddit/bluesky/tickertick/xueqiu) are also
  exercised live during dev runs.

## Environment notes (gotchas for future sessions)
- **Go proxy truncates large module zips** (e.g. `golang.org/x/text`) via
  goproxy.io/goproxy.cn in this network → use `GOPROXY=direct GOSUMDB=off` to
  fetch from git when `go get`/`go mod tidy` fails with "unexpected EOF".
- macOS dev box: **no `timeout`** command (BSD); use a background run + kill.
- `go test ./...` descends into `web/node_modules` (a stray `flatted` Go pkg);
  harmless, but list real packages (`./cmd/... ./internal/...`) — CI does this, and the
  CI go job has no `node_modules` checked out anyway.

## Pointers
- `ROADMAP.md` — phased plan + status (update each iteration).
- `DEPLOY.md` — free, domain-only deploy.
