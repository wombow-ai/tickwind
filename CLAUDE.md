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
- Frontend: **Next.js** (App Router, TypeScript, Tailwind), **static export** → Pages.
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
  LIVE-VERIFIED end-to-end with paper keys (local `.env`, gitignored); Finnhub
  still needs a token. Next: Phase 3 (social — Reddit/StockTwits — + Clipper inbox).
- Frontend live price: `web/src/lib/useQuotes.ts` (one shared EventSource for all
  cards) + `PriceTag`/`SessionBadge`; shows "—" gracefully when `/quote` 404s.
- News: `internal/finnhub` → `News` store → `GET /v1/stocks/{ticker}/news`,
  ingested on the scheduler (needs `FINNHUB_TOKEN`); frontend `NewsTimeline`.
- API `?limit=` parsing is shared via `queryLimit` in `internal/api`.
- Prices: Alpaca REST `trades/latest` (feed-aware, all-session ET classifier) →
  `Quote` in store → `GET /v1/stocks/{ticker}/quote`. Poller auto-disables when
  `ALPACA_API_KEY/SECRET` are unset, so the app always runs. Needs a real Alpaca
  paper key to live-verify.
- Live push: `GET /v1/stream` = Server-Sent Events via `internal/stream.Hub`
  (chose SSE over WebSocket — one-way, stdlib-only). Poller publishes each quote;
  handler sends an initial `: connected` so headers flush immediately.
- Frontend lives in `web/` (Next 16, src-dir layout): `src/app` (pages),
  `src/components`, `src/lib`. Static export to `web/out`. Detail page is
  `/stock?ticker=XYZ` (query param, no dynamic route — keeps export simple).
- Backend packages: `internal/{config,store,store/memory,store/postgres,edgar,alpaca,ingest,api}`.

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
