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
- ✅ Optional LLM enrichment plugin: `internal/enrich` (OpenAI-compatible, stdlib;
      Noop when disabled) + `GET /v1/stocks/{ticker}/summary` (503 when off). Off
      without `LLM_API_KEY`. (Frontend "Summarize" button = future polish.)
- ✅ Multi-tenant + Supabase auth (商用 pivot): Supabase JWT (HS256, stdlib
      verify, no dep); per-user watchlist + private clips; public market-data
      endpoints stay open (SEO); ingest = default ∪ all users' watchlists (capped);
      Supabase Postgres (session pooler). Verified e2e against real Supabase.
- 🟡 Frontend rebuild — **"Aurora" data-first app** (Next 16 SSR + Supabase Auth):
  - ✅ Design system ported from the product spec: light-first Aurora palette
        (teal/sky) + dark variant via `.dark` + `useSyncExternalStore` (no-flash);
        signature `SessionBadge`, `PriceTag` (live tick-flash), timeline feed,
        empty/error/skeleton states, toasts, Inter — all in `web/src/components/ui`
  - ✅ **Data-first entry** (no marketing page): `/` IS the board — popular US
        stocks with live prices for anyone; the user's watchlist when signed in
  - ✅ `/stock/[ticker]`: live header + News / Discussion / Filings (+ Saved links
        when signed in) from the real API; add-to-watchlist; clip box
  - ✅ Supabase email/password `/login` + `/signup`; account menu; `/settings`;
        `/announcements`; JWT attached to private API calls; session-refresh `proxy`
  - ✅ Route-group layout split (app chrome vs auth vs `/designs`); build + lint green
  - ✅ Deploy prep: `DEPLOY.md` "Frontend on Vercel" section (root=web/, the 3
        NEXT_PUBLIC_* envs, Cloudflare DNS records, Supabase redirect URLs);
        canonical metadata + OpenGraph (`metadataBase`/`SITE_URL`); `robots.txt` +
        `sitemap.xml` (board + popular stock pages); baseline security headers.
        SSR build Vercel-ready (14 routes, green)
  - ⬜ Deploy on Vercel; re-point `tickwind.com` DNS; set env (user action)
  - ✅ Backend `prev_close` via Alpaca **snapshot** endpoint (honest prior close) →
        `ChangeLine` (signed %/▲▼) now renders on the board + detail header.
        Verified e2e locally (AAPL 307.23 / prev 311.21 = −1.28%; light + dark)
  - ✅ Bars endpoint `GET /v1/stocks/{ticker}/bars` (Alpaca daily bars, 30 closes,
        server-cached 1h) → **`Sparkline` renders** on the detail header (real trend,
        green up / rose down). Verified e2e (AAPL up, NVDA down; light + dark)
  - ✅ Board-tile sparklines via a batched `GET /v1/bars?tickers=…` (parallel
        fan-out over `BarCache`, capped at 30) — one request per board, each
        `StockCard` shows a compact trend (hidden when empty). Verified light + dark
  - ✅ Default `WATCHLIST` bumped to `POPULAR_TICKERS` (config + `.env.example`) so
        every public tile is live after redeploy
  - ⬜ Redeploy VPS backend multi-tenant + Supabase (`DATABASE_URL` pooler +
        `SUPABASE_JWT_SECRET`, `WATCHLIST` set)
  - ✅ Mobile/responsive polish: TopNav fits one line at 375px (search collapses to
        an icon → dropdown row; theme + search are 36px tap targets; Log in/Sign up
        nowrap). Board + detail reflow cleanly. Verified at 375px in light + dark
  - ✅ A11y: theme-aware keyboard focus ring (global `:focus-visible` + `--tw-focus`,
        outranks `outline-none`, keyboard-only); aria-current on active nav,
        aria-pressed + dynamic label on theme toggle, aria-expanded/haspopup on the
        account menu + mobile search, aria-pressed on detail tabs; Escape closes the
        menu + mobile search
  - ⬜ Optional Google OAuth
- ⬜ HK (HKEXnews) + KR (DART) filings (needs DART key); later Futu/KIS realtime

---
_Working agreement: each `/loop` iteration picks the next unchecked item(s),
implements rigorously (Google style, OSS reuse, parallel subagents where safe),
verifies (build/vet/lint), updates this file + `CLAUDE.md`, and commits._
