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
- ⬜ Tests: unit tests for edgar client + store impls (quality debt to pay down)

## Phase 2 — Prices (headline feature)
- ⬜ Alpaca client: US all-session incl. **overnight** (REST snapshot + WS stream)
- ⬜ `Quote` type + store; WebSocket push to frontend; live price on per-stock page
- ⬜ Finnhub fallback + company news

## Phase 3 — News + Social
- ⬜ Per-stock unified news/announcement timeline
- ⬜ Reddit + StockTwits ingestion; social tab
- ⬜ Clipper inbox (paste 小红书/抖音/X links); optional Whisper transcription

## Phase 4 — Multi-market + polish
- ⬜ HK (HKEXnews) + KR (DART) filings; later Futu/KIS realtime (isolated, data-only)
- ⬜ Optional LLM enrichment plugin (translate / summarize / relevance) — feature-flagged
- ⬜ Single-user auth, persisted watchlist, UI polish

---
_Working agreement: each `/loop` iteration picks the next unchecked item(s),
implements rigorously (Google style, OSS reuse, parallel subagents where safe),
verifies (build/vet/lint), updates this file + `CLAUDE.md`, and commits._
