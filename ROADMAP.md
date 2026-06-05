# Tickwind Roadmap

Status: ✅ done · 🟡 in progress · ⬜ todo

## Phase 0 — Backbone ✅
- ✅ Go service skeleton (config, `store.Store` iface, ingest scheduler, HTTP API)
- ✅ SEC EDGAR client + filings ingestion (live, verified end-to-end)
- ✅ In-memory store
- ✅ Deploy wiring: docker-compose + cloudflared + `DEPLOY.md` (Oracle + CF Tunnel + Pages)

## Phase 1 — Persistence + Frontend  ✅ code · 🟡 VM verify
- ✅ Postgres store (pgx/pgxpool) implementing `store.Store`; idempotent schema
      migrations (plain tables; Timescale/pgvector extensions deferred until needed)
- ✅ Next.js frontend (Next 16, App Router, TS, Tailwind): watchlist + per-stock
      filings timeline; static export → `web/out` (Cloudflare Pages); build green
- ✅ Wired Postgres into the server (`STORE_BACKEND=postgres`, fatal on init error)
- ⬜ Verify Postgres end-to-end on the VM (blocked on provisioning the Oracle VM)
- ✅ Tests: table-driven unit tests — memory store (CRUD/order/dedupe/copy), clip
      (og:title/title/entities/scheme), alpaca (session classifier), API (httptest:
      health, watchlist CRUD, 400/404, clip→social). `make test` green

## Phase 2 — Prices + News  ✅
- ✅ Alpaca REST client: latest trade incl. extended-hours/overnight (feed-aware;
      ET session classifier pre/regular/post/overnight); price poller (auto-disabled
      without keys)
- ✅ `Quote` type + store (memory + postgres) + `GET /v1/stocks/{ticker}/quote`
- ✅ Live-price stream `GET /v1/stream` (Server-Sent Events via in-process Hub;
      SSE chosen over WebSocket — one-way push, stdlib-only; poller broadcasts)
- ✅ Frontend: live price on watchlist + per-stock page (single shared EventSource;
      session badge; graceful "—" when no quote)
- ✅ Finnhub company news: client + `News` store (memory + postgres) +
      `GET /v1/stocks/{ticker}/news` + scheduler ingest; frontend NewsTimeline
      (per-stock News + Filings sections; auto-disabled without a token)
- ✅ Live-verified Alpaca prices end-to-end (AAPL/NVDA, regular session, live SSE push)
- ✅ Live-verified Finnhub news end-to-end (AAPL: 243 articles via /news)

## Phase 3 — News + Social  ✅
- ✅ Per-stock unified timeline (News + Discussion + Saved links + Filings)
- ✅ StockTwits social ingestion + `GET /v1/stocks/{ticker}/social` + Discussion
      feed (live-verified, no key required)
- ✅ Multi-source `SocialSource` interface (StockTwits + Reddit plug in uniformly)
- 🟡 Reddit ingestion: client done, but public `.json` returns 403 from datacenter
      IPs → needs OAuth (REDDIT_CLIENT_ID/SECRET) to be reliable (handled gracefully)
- ✅ Clipper inbox: `POST /v1/stocks/{ticker}/clip` fetches the page title and
      saves it as a `clip` post; frontend paste box + "Saved links" section
      (video/Whisper transcription deferred to Phase 4)

## Phase 4 — Multi-market + polish  🟡
- ✅ Persisted, editable watchlist: store CRUD + `GET/POST/DELETE /v1/watchlist`;
      scheduler + price poller read it live each cycle (seeded from `WATCHLIST`);
      frontend add/remove board on the home page
- ⬜ HK (HKEXnews) + KR (DART) filings — HKEXnews needs stock-id scraping; DART
      needs a free API key. Deferred (hard to verify from here / needs key); the
      watchlist already accepts any ticker, so this is purely a new FilingSource
- ⬜ Optional LLM enrichment plugin (translate / summarize / relevance) — flagged
- ⬜ Single-user auth + UI polish; later Futu/KIS realtime (isolated, data-only)

---
_Working agreement: each `/loop` iteration picks the next unchecked item(s),
implements rigorously (Google style, OSS reuse, parallel subagents where safe),
verifies (build/vet/lint), updates this file + `CLAUDE.md`, and commits._
