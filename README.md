# Tickwind

A personal, web-based command center for global stocks (US / HK / KR): real-time
prices across **all sessions** (pre-market, intraday, after-hours, overnight),
company **announcements & filings**, and **social-media chatter** — unified on one
screen. Engineering-first aggregator; LLM enrichment is an optional, pluggable layer.

## Status

v0.1 — backbone. Runs with **zero infrastructure** (in-memory store, stdlib only):
one real source wired (SEC EDGAR filings, no API key needed).

## Quickstart (no Docker / DB needed)

```bash
make run          # or: go run ./cmd/server
# then:
curl localhost:8080/healthz
curl localhost:8080/v1/stocks/AAPL
curl localhost:8080/v1/stocks/AAPL/filings | jq
```

Config via env (see `.env.example`): `WATCHLIST`, `EDGAR_USER_AGENT`, `INGEST_EVERY`, `STORE_BACKEND`.

## Architecture

```
cmd/server            entrypoint: wires config + store + ingest + api
internal/config       env config
internal/store        Store interface + domain types (Security, Filing, ...)
internal/store/memory in-memory Store (v1 — zero infra)
internal/edgar        SEC EDGAR client (ticker→CIK, recent filings)
internal/ingest       scheduler: pulls sources → store on an interval
internal/api          HTTP/JSON surface (stdlib net/http)
web/                  Next.js frontend (added next)
```

Storage is behind an interface: `memory` now, Postgres (TimescaleDB + pgvector)
on the server via `docker-compose.yml`.

## Data sources (all free; redistribution-safe / personal use)

| Domain        | Source                                  | Status |
|---------------|-----------------------------------------|--------|
| US filings    | SEC EDGAR (`data.sec.gov`)              | ✅ wired |
| US prices     | Alpaca (incl. overnight) / Finnhub      | next   |
| HK filings    | HKEXnews                                | later  |
| KR filings    | DART (OpenDART)                         | later  |
| Social        | Reddit, StockTwits, Xueqiu (best-effort)| later  |

## Deploy (server)

```bash
cp .env.example .env      # fill in
docker compose up -d --build
```

## Roadmap

1. ✅ Backbone + EDGAR filings (in-memory)
2. Postgres store (TimescaleDB + pgvector) + docker-compose on server
3. US prices (Alpaca all-session incl. overnight) + WebSocket push
4. News + per-stock unified timeline
5. Social (Reddit/StockTwits) + Clipper inbox
6. Next.js web UI (watchlist board + per-stock page)
7. HK/KR markets; optional LLM enrichment plugin
