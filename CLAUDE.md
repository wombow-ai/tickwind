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
- Domain `tickwind.com` (on Cloudflare). Frontend → **Cloudflare Pages**; API →
  `api.tickwind.com` via **Cloudflare Tunnel**; backend on an **Oracle Always-Free**
  ARM VM. **$0/month**, only the domain is exposed. See `DEPLOY.md`.

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
  (social). HK (HKEXnews/Futu) + KR (DART/KIS) come later.
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
- Phase 0 ✅ · Phase 1 ✅ · Phase 2 ✅ (prices REST + SSE live stream + frontend
  live price + Finnhub news; all auto-disable without keys). Alpaca prices
  LIVE-VERIFIED end-to-end with paper keys (local `.env`, gitignored). Finnhub
  news also LIVE-VERIFIED (real AAPL headlines). Phase 3: StockTwits social ✅
  (live-verified, no key) via `internal/stocktwits` → `Post` store →
  `/v1/stocks/{ticker}/social` + frontend `SocialFeed` (Discussion section).
  Social is now multi-source via `ingest.SocialSource` (Name + Posts); StockTwits
  live, Reddit client done but its public `.json` 403s from datacenter IPs (needs
  OAuth via REDDIT_CLIENT_ID/SECRET — graceful fallback). Clipper inbox ✅
  (`POST /v1/stocks/{ticker}/clip` → title fetch → `clip` Post; frontend paste box
  + Saved-links section). Phase 3 done. Phase 4 started: persisted editable
  watchlist ✅ (`/v1/watchlist` CRUD; scheduler + poller read it live, seeded from
  WATCHLIST; frontend add/remove board). Next in Phase 4: HK/KR FilingSource (needs
  DART key + HKEXnews scraping — deferred), optional LLM plugin, auth + polish.
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
- `internal/auth`: stdlib HS256 verify of Supabase JWTs (hardcoded HS256 to block
  alg confusion). `Middleware` attaches the user when a valid bearer token is
  present (does NOT reject anon — handlers gate via `requireUser`).
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
- **Data-first, no marketing page** (explicit user direction): `/` IS the board.
  Anonymous → popular US tickers (`POPULAR_TICKERS`) with live prices; signed-in →
  the user's watchlist (`/v1/watchlist`) with add/remove. Synthesized from the
  user's own design (`tickwind-app-claude.tsx`, kept as `.design-ref.tsx`).
- **Design system** in `web/src/components/ui/` + `web/src/lib/ui.ts` (tokens):
  light-first Aurora (teal `#2DD4BF`/sky `#0EA5E9`) with a dark variant. Signature
  `SessionBadge` (pre=amber, regular=emerald+livedot, post=violet, overnight=blue,
  closed=slate — keyed to the API's `Quote.session`), `PriceTag` (flashes on tick),
  `TimelineItem` (news/disc/clip/filing), empty/error/skeleton, toasts, Inter font,
  CSS motion in `globals.css`.
- **Theme**: `.dark` class on `<html>`, read via `useSyncExternalStore` (single
  source of truth = the DOM class) + a no-flash inline script in `layout.tsx`.
  `useTheme`/`useDark` in `web/src/lib/theme.tsx`. Default light.
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
- **Responsive**: mobile-first; TopNav collapses the ticker search to an icon →
  dropdown row < `sm`; one-line nav at 375px; board/detail reflow to a single column.
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

## Tests
- `make test` = `go test ./cmd/... ./internal/...` (scoped to skip `web/node_modules`).
- Covered: memory store, clip title extraction, alpaca session classifier, API
  httptest flows (health, watchlist CRUD, clip→social), and the **bars endpoints**
  (`internal/api/bars_test.go`: `/v1/bars` dedupe + cap + nil-source→empty via a
  fake `BarSource`, and the single `/v1/stocks/{t}/bars`). Network-dependent clients
  (edgar/finnhub/stocktwits/reddit) are exercised live during dev runs.

## Environment notes (gotchas for future sessions)
- **Go proxy truncates large module zips** (e.g. `golang.org/x/text`) via
  goproxy.io/goproxy.cn in this network → use `GOPROXY=direct GOSUMDB=off` to
  fetch from git when `go get`/`go mod tidy` fails with "unexpected EOF".
- macOS dev box: **no `timeout`** command (BSD); use a background run + kill.
- `go test ./...` descends into `web/node_modules` (a stray `flatted` Go pkg);
  harmless, but prefer listing real packages or exclude when adding CI.

## Pointers
- `ROADMAP.md` — phased plan + status (update each iteration).
- `DEPLOY.md` — free, domain-only deploy.
