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
      health, watchlist CRUD, 400/404, clip→social, bars dedupe/cap/nil-source).
      `make test` green

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
- ✅ Multi-source `SocialSource` interface — **5 post-based sources wired**
      (StockTwits, Tickertick, Reddit, Bluesky, Xueqiu), each `internal/<src>` with
      a `New()` + table-driven `_test.go`; disabled sources degrade to 0 posts:
  - ✅ **StockTwits** (keyless, always on) — live since Phase 3
  - ✅ **Tickertick** (keyless, always on) — free UGC/analysis links; OAuth-free.
        Live-verified (real Forbes/Fool AAPL stories flowing alongside StockTwits)
  - ✅ **Bluesky** `searchPosts` (AT Protocol) — session cached + 401-retry.
        **LIVE** (creds on the VPS; ~30 finance posts/ticker, e.g. AAPL feed =
        StockTwits + Bluesky + Tickertick merged)
  - 🚫 **Reddit** (owner's call, 2026-06): NOT pursued. Both keyless routes are
        datacenter-IP-blocked (verified from the VPS: `.json`→403, `.rss`→429), so
        only the official OAuth API works server-side — and that's commercially
        restricted/charged. Reddit's signal is already covered by **ApeWisdom**
        (mention buzz) + **Tickertick** (`T:ugc` Reddit links). The OAuth client
        (`internal/reddit`) stays in code, disabled by default; set
        REDDIT_CLIENT_ID/SECRET/USERNAME/PASSWORD to enable if ever wanted
  - ✅ **Xueqiu (雪球)** unofficial JSON (keyless, mints its own cookie). Best
        US-ticker fit of the China sources; datacenter IPs get soft-blocked
        (HTTP 200 empty → 0 posts, no error), so it mainly helps from residential/
        China egress
- ✅ **Numeric buzz/sentiment signals** — a new per-ticker `store.Signal` data
      shape (one row per (ticker, source), a rolled-up snapshot not a feed) +
      `ingest.SignalSource` (bulk: one call covers many tickers, run once/cycle)
      + `GET /v1/stocks/{ticker}/signals` + a frontend **PulseBar** (Reddit-buzz
      chip + news-sentiment chip on the detail page, hidden when empty):
  - ✅ **ApeWisdom** (`internal/apewisdom`, keyless) — Reddit/WSB mention
        momentum (mentions, rank, upvotes, 24h deltas). Scans up to 3 leaderboard
        pages, stops once all wanted tickers found. Live shape verified
  - ✅ **Alpha Vantage NEWS_SENTIMENT** (`internal/alphavantage`) — relevance-
        weighted per-ticker sentiment score + label + article count. Free tier is
        25/day, so the client self-budgets (daily cap + ≥90-min refresh + cache;
        rate-limit reply marks the day spent). Key verified live; off without one
- ✅ **Trending hot list** (`/hot`) — a market-wide leaderboard of the
      most-discussed US stocks. `store.HotStock` snapshot (replaced wholesale each
      cycle) + `ingest.HotSource` (ApeWisdom top-40, run once/cycle) +
      `GET /v1/hot` + a frontend `HotList` page (TopNav "Hot"). **Heat score** =
      mentions × (1 + clamp(24h mention growth, 0, 2)) — blends discussion VOLUME
      with MOMENTUM (loud AND getting louder; cooling names keep their raw volume,
      never penalised), shown transparently as mentions + Δ%. Verified live
      (QQQ/SPY top by volume×momentum; explosive low-volume risers boosted but
      capped). `rankHotList`/`heatScore` unit-tested
- 📋 **Opinion-source research (2026-06, 4 parallel agents)** — prioritized for
      future ingestion (engineering-first, redistribution-safe, $0-ish):
      **do-now:** fix Reddit OAuth (script app → `oauth.reddit.com` + proper UA),
      **Bluesky** `searchPosts` (free, open API), **ApeWisdom** (free Reddit/WSB
      mention-momentum, NOT sentiment), **Alpha Vantage NEWS_SENTIMENT** (free
      25/day, real per-ticker sentiment — batch+cache), **Tickertick** (free UGC/
      analysis links). **China:** 雪球 Xueqiu (best US-ticker fit, unofficial JSON,
      integrate first), 东方财富股吧 Eastmoney Guba (US boards `list,us<t>.html`).
      **later:** Substack RSS, YouTube comments (30-day cache cap), StockGeist,
      Benzinga (paid). **avoid:** X (~$5k/mo), Discord/TikTok/Threads (gated),
      Xiaohongshu/小红书 (keyword-only, monthly-rotating signature, steep legal risk —
      soft buzz signal at best), TradingView/SeekingAlpha/Yahoo (ToS/scrape-unsafe).
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
  - ✅ Split storage (`store.Split`): durable Market DB (collected corpus —
        securities/filings/quotes/news/social) + local User DB (watchlist/clips,
        OK to lose). Routes transparently; `MARKET_DATABASE_URL`+`USER_DATABASE_URL`
        (or single `DATABASE_URL`). compose wired; tested (`split_test.go`)
  - ⬜ Redeploy VPS backend (user): `git pull` + add `SUPABASE_JWT_SECRET`
        (+ optional `MARKET_DATABASE_URL`=Supabase for the durable corpus) +
        `docker compose up -d --build`
  - ✅ Mobile/responsive polish: TopNav fits one line at 375px (search collapses to
        an icon → dropdown row; theme + search are 36px tap targets; Log in/Sign up
        nowrap). Board + detail reflow cleanly. Verified at 375px in light + dark
  - ✅ A11y: theme-aware keyboard focus ring (global `:focus-visible` + `--tw-focus`,
        outranks `outline-none`, keyboard-only); aria-current on active nav,
        aria-pressed + dynamic label on theme toggle, aria-expanded/haspopup on the
        account menu + mobile search, aria-pressed on detail tabs; Escape closes the
        menu + mobile search
  - ✅ Google OAuth (Supabase) — "Continue with Google" on the auth form +
        `/auth/callback` route (exchangeCodeForSession). **Gated** behind
        `NEXT_PUBLIC_GOOGLE_OAUTH=1` (hidden by default); activate by enabling the
        Google provider in Supabase + setting the flag. Button render verified;
        setup documented in DEPLOY.md §5
- ⬜ HK (HKEXnews) + KR (DART) filings (needs DART key); later Futu/KIS realtime

---
_Working agreement: each `/loop` iteration picks the next unchecked item(s),
implements rigorously (Google style, OSS reuse, parallel subagents where safe),
verifies (build/vet/lint), updates this file + `CLAUDE.md`, and commits._
