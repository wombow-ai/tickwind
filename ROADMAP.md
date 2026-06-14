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
- ✅ **Surging board** (`/hot?board=surging`) — same `store.HotStock` family, a
      second `Board`; surge = mention-share shrinkage × clamped 24h growth with a
      min-mention floor; `/hot` is tabbed (Hot / Surging).
- ✅ **热点话题条 (Hot Topics)** — `internal/topics` curated keyword dictionary over
      ingested news (recency×momentum, generic demotion); `GET /v1/topics` + a
      `?topic=` news filter; frontend `TopicsStrip` on the home hub.
- ✅ **机会榜 (Opportunity board)** (`/opportunities`) — small-cap US stocks with SEC
      Form-4 insider open-market buying. `internal/sec` (Form-4 index/fetch/parse,
      code P only + dei shares) → `store.InsiderBuy` → `internal/opportunity` (pure
      ranker: market-cap $300M–$2.5B gate, rank by #buyers then $value) +
      `OpportunityIngestor`; market cap = dei shares × `alpaca.Snapshots`.
      `GET /v1/opportunities` + evidence-first `OpportunityBoard`. **LIVE** on the VPS.
- ✅ **大V / Guru-watch rail** — `internal/substack` (public-RSS KOL feeds incl.
      **Serenity**; cashtag extraction) → `internal/guru` (rank/dedupe/cap) +
      `GuruIngestor`; `GET /v1/gurus` + `GuruRail` under the Opportunity board + a
      home-hub module. X live tweets avoided ($5k/mo) — newsletters as the proxy.
      **LIVE**.
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
- ✅ TW market live (TWSE + TPEx EOD, keyless). HK **prices** live via Yahoo delayed
  quotes (owner-authorized "gray" source — Tencent `0700`, Zhipu `2513`, MiniMax `0100`).
- ⬜ HK **filings** via HKEXnews — **deferred (blocked)**: titleSearchServlet returns
  JSON but filters only by an internal `stockId` (NOT the stock code); `prefix.do`
  (code→stockId) returns empty from here (likely datacenter-IP-gated, like Xueqiu/TPEx),
  and the global feed is too sparse to filter by `STOCK_CODE`. Revisit from the VPS IP
  or with a static stockId map for the 3 codes.
- ⬜ KR (KRX prices + OpenDART filings): code-ready + inert; **DEFERRED** — owner's
  KRX-site access is blocked; they'll supply the free KRX key later (then one env var
  to go live).
- ⬜ Later: Futu/KIS realtime; add the foreign seed tickers (TW/HK) to symbol search.

### Shipped 2026-06 (user-feature batch)
- ✅ **Private notes** (个股 + 日历) — `/v1/notes`, Notes tab + `/notes`. (v1.1: calendar grid.)
- ✅ **Comments** (个股 + 综合评论区) — public `/v1/comments` + §230 safeguards (rate-limit,
  report, soft-delete, admin takedown via `ADMIN_USER_IDS`, IP capture); Comments tab + `/community`.
- ✅ **K-line + indicators** — `/v1/stocks/{t}/candles` + `lib/indicators.ts` + lightweight-charts
  (MA/MACD/RSI/Volume).
- ✅ **Fix**: on-view single-flight collection (`$MU` all-empty bug); ~90s frontend poll.
- ✅ **Commercialization risk audit** — `docs/feature-research-2026-06.md` (Alpaca/Yahoo quote
  redistribution = RED; fix before charging).
- ⬜ Owner actions before wide launch: set `ADMIN_USER_IDS` (UUID **or login email**);
  register a DMCA agent ($6/3yr, `dmca.copyright.gov/osp/`) + add on-site DMCA notice page.

### Shipped 2026-06 (ops / UX polish)
- ✅ **Mobile nav** (hamburger menu — bar had no nav links < md) + **Watchlist** top-level
  pill (authed) + **Notes calendar** redesign (compact cells, 2-col on lg, Events overlay).
- ✅ **Admin allowlist matches by UUID *or* email** (`Server.isAdmin`).
- ✅ **CI** — `.github/workflows/ci.yml` (Go build/vet/gofmt/test + web lint/build), actions
  @v6, green-verified. Surfaced + fixed a SearchBox combobox a11y gap.
- ✅ **K-line preserves the user's view** across dark/Bollinger toggles (was resetting to the
  last ~130 sessions on every rebuild).
- ✅ **i18n session badges** — Pre-market/Regular/After-hours/Overnight/Closed now translate
  (zh 盘前/盘中/盘后/夜盘/休市) on every price tag; + the account-menu 'Signed in' fallback.
- ✅ **HomeHub loading skeletons** — the 5 module previews showed their empty state during the
  initial fetch (landing page flashed "No data"); now per-module skeletons until each settles.
- ✅ **a11y: More-menu Escape** — the More dropdown owned its own state so the global Escape
  handler missed it (Esc did nothing); now closes + restores focus to its trigger.

### Future features — researched 2026-06 (see `docs/future-features-2026-06.md`)
> **Owner directive (2026-06): MONETIZATION DEFERRED — build everything EXCEPT paid/monetization
> work** (no pricing/payments/quote-licensing/paywalls/subscriptions). Strategy round-2's
> monetization plan (`docs/strategy-research-2026-06.md`) is parked until the owner says go;
> the rest of that doc (growth/SEO, positioning, engineering, legal) is in scope. Also:
> **web-push deferred**; the dev loop ran at a **1-min cadence** (owner, 2026-06-08) with parallel
> planning subagents. **The 9-idea batch is 100% SHIPPED** (2026-06-09): #24-#31 all live (incl. #29
> holdings front+back; #26 ETF search — SIVEF-class pink sheets remain unindexed by design). A ~1h
> VPS SSH outage (1GB-RAM OOM + fail2ban) that blocked deploys is **RESOLVED** (swap added, deploy IP
> whitelisted, GitHub-pull deploy method — see CLAUDE.md).
>
> **▶ v2 plan IN PROGRESS (owner-confirmed 2026-06-09), 1-min `/loop`, this order:** ✅#0 remove gray
> sources Reddit+Xueqiu (deployed, verified gone). → ✅#1 K-line **full timeframes LIVE**
> (1D/5D/D/W/M/Q/Y): intraday endpoint + 5y daily history + client aggregation + 1D/5D buttons
> (`bcf95da`) [task 32 done]. → ✅#2 cache all US stocks [33]: (a) universe price cache via
> UniverseIngestor + `GET /v1/universe` **LIVE `f9efe70` — ~6.5k US stocks pre-cached, verified**; (b) bulk market
> cap → **decided: fold into screener #5** (per-stock cap already served by `edgar.Fundamentals`; no
> consumer yet for bulk-cap plumbing); (c) banner reworded ✅ `51f3e7c`. → ⬜#3 earnings calendar
> [34] → ⬜#4 Congress board (Senate-first) [35] → ⬜#5 screener (needs #2) [36] → ⬜#6 notes/comments
> enhance [37] → ⬜#7 Brazil B3 (brapi, key in VPS .env) [38] → ⬜#8 FINRA squeeze radar [23].
> Yahoo HK kept (gray but controllable while free; revisit at monetization). brapi key provided.
> **✅#3 earnings — FULLY LIVE 2026-06-09:** (a) `finnhub.EarningsCalendar`+`store.Earning` `ec45870`; (b) store CRUD
> + EarningsIngestor `21c47bd`; (c) API `GET /v1/earnings?from=&to=` + `GET /v1/stocks/{t}/earnings` (`EarningsSource`
> in api.New, 5 call sites) + `api.ts` client `27dc91f`; (d) StockView `EarningsChip` ("下次财报", hide-on-empty, i18n)
> `32914da`. **Backend deployed on the 5th SSH attempt — `/v1/earnings` verified `{count:332,…}` (real EPS est/act),
> `/v1/stocks/{t}/earnings` valid, healthz 200, universe 6683.** DEPLOY LESSON: the flaky SSH eventually gets through
> — one single spaced attempt per tick (NO spinning) drains the backlog; 4 drops then success, no fail2ban trip.
> ✅#4 Congress trading board (35) — **COMPLETE 2026-06-09:** data source = official House Clerk FD (disclosures-clerk.house.gov,
> public-domain, keyless; Stock-Watcher S3 dumps now 403/acquired).** (a) `internal/congress` client+parser+test `9e34450`
> (downloads annual `{year}FD.ZIP`, unzips in-memory, parses XML index, keeps FilingType "P" = Periodic Transaction Reports,
> builds official PTR PDF link `/public_disc/ptr-pdfs/{yr}/{docid}.pdf`); (b) **cache + `CongressIngestor` (8h, keyless,
> unconditional) + nil-safe `CongressSource` in api.New (5 call sites) + `GET /v1/congress?limit=` ✅ `2f6ec00` — DEPLOYED
> + LIVE-VERIFIED (clean first SSH attempt, ~30s): real PTRs (Shreve IN-06, Allen GA-12, 2026 dates, working PDF links),
> count 60, healthz 200.** (c) `/congress` board page (member·state-district·filed date·"official PDF" link, sourced-facts
> framing + disclaimer) + `CongressBoard` + nav (secondary/More▾) + `api.ts getCongress` + zh/en i18n ✅ `f3b22bf` —
> **LIVE-VERIFIED on Vercel (`/congress` 200, title rendered, ~20s).** (Ticker-level detail = PTR PDF parsing, deferred; v1 links to the official PDF.)
> ◐#5 Stock screener (36): (c) `/screen` frontend page (filter controls + results table) + `Screener` + nav + `api.ts getScreen`
> + zh/en i18n ✅ `19325ed` — **LIVE** (`/screen` 200, verified). **Vercel had stalled (too-frequent pushes exhausted the Hobby
> deploy quota — owner-confirmed); owner manually redeployed main 2026-06-10 → frontend back. FIX ADDED: `web/vercel.json`
> `ignoreCommand: git diff --quiet HEAD^ HEAD .` so only `web/` changes trigger a Vercel build (backend/docs pushes no longer
> consume the quota; fails safe to "build" if CWD/HEAD^ ambiguous). Plus: fewer pushes (one batched commit/tick).**
> (a) **`GET /v1/screen` over the universe cache (~6.6k) — price/%-change/session filters,
> sortable, capped — reusing the wired `universe` field via `Snapshot()` (no api.New change); pure `screenQuotes` unit-tested**
> ✅ `b509589` + DEPLOYED. LIVE-VERIFY caught delayed-IEX prev_close split artifacts (bogus +4010% gainers) → **data-hygiene
> guard: change outside [-95%,+300%] marked unknown** (still in price screens, excluded from change rank) ✅ `76a1e9b` — RE-VERIFIED
> (top gainers now CHAI +300/AZI +191/RGNT +151, sane). Next: (b) market-cap filter (needs SEC `Shares()` whole-market cache,
> 3 req/day → ticker→shares; cap=price×shares) [separate tick]; (c) frontend `/screen` page (filter controls + results table).
> ✅#6 notes/comments (37) — **COMPLETE 2026-06-10:** notes inline-edit `d97db72`; (a) **Markdown** — `Markdown.tsx` wraps
> react-markdown (10.1.0; NO raw HTML→XSS-safe; images stripped; links→_blank/noopener; `.tw-md` CSS) rendering note + comment
> bodies; (b) **comment EDIT** — `store.UpdateComment` (author-only, `edited_at`) across iface/memory/postgres(+col,idempotent
> ALTER)/split + `PATCH /v1/comments/{id}` + CommentsPanel inline-edit UI (Pencil → textarea → save, "edited" badge);
> (c) **comment LIKE** — `store.LikeComment` toggle (per-user dedup via `comment_likes` table; ListComments returns count) +
> `POST /v1/comments/{id}/like` + Heart button (optimistic, count) + memory tests. "Markdown supported" compose hint; i18n zh/en.
> Owner paused #7 (Brazil) + #8 (FINRA) — NOT starting those.**
> **▶ v3 owner ideas (2026-06-10): ①盘前/盘后价格分行卡片 ②价格更实时 ③机构信号。决定：①+② 做；②直接上 Alpaca IEX
> WebSocket 真实时；③不并入 Hot/Surging（被动三巨头≠信念信号、13F季度滞后会污染社交榜）——改为日后单独做 13D举牌/13F主动加仓榜；
> #7/#8 仍暂停。 ◐①价格卡(39): (a) 后端 `Quote.RegularClose`（=Alpaca dailyBar.c，盘前缺失则回退 prevClose；LatestQuote+
> SnapshotQuotes+postgres quotes 加 regular_close 列幂等 ALTER+UpsertQuote/GetQuote；poller 走 LatestQuote 自动带上）+ (b) 前端
> StockView 头部两行（主行=正常盘价+当日涨跌 vs 昨收；盘前/盘后/夜盘副行=延伸价+涨跌 vs 正常盘收盘；非美股/旧报价 regular_close
> 缺失则优雅回退原样）✅ `9bf3b31` LIVE 验证。 ◐②价格实时(40, WebSocket): #2a `internal/alpacaws`——Alpaca 免费 IEX
> WS（`wss://stream.data.alpaca.markets/v2/iex`，dep `github.com/coder/websocket` v1.8.14，零依赖纯 Go）：auth→subscribe trades→
> 读循环解析 trade（修了一个 JSON 大小写坑：head 只含 "T" 时 "t" 时间戳会污染 Type→改用同时含 T/t 字段的行结构）→ merge 到
> seeded quote（prev/regular_close 来自 REST snapshot 种子，盘中 regular_close 跟随实时价）→ 推 SSE hub + 限流 UpsertQuote；
> 30s ping 保活 + 指数退避重连；订阅集=watchlist∪POPULAR 的**美股**（剔除 .HK/.TW/.KS）上限 30，其余仍靠 REST poller。
> config `ALPACA_WS_URL`/`ALPACA_WS_ENABLED`(默认开)；main 有 key 时与 poller 并存启动；trade 解析 + 30 上限单测。
> ✅ `349953c` **已部署**（VPS 成功拉到 coder/websocket + healthz 200 + universe 6685）。**实时效果待开盘验证**：当前为休市/盘前
> 极薄（quote `at` 仍是 6/9 收盘前的最后成交，无实时成交可推流）；WS 连通日志核对 SSH 掉线未成——开盘后看热门票 `at` 是否秒级刷新
> + docker logs 看 "connected + subscribed"。WS 出错会优雅退回 poller（无害）。**✅#2b 查看即实时订阅（owner 2026-06-10 要求）**：
> streamer 重构为 writer-goroutine 独占 WS 写（auth/订阅/ping/动态增删），`Subscribe(ticker)` 把"正在看的票"加入 LRU（base 上限 20 +
> viewed 上限 10 ≤ MaxSymbols 30，淘汰最久未看；新票订阅前先 reseed prev/regular_close）；nil-safe `LiveSubscriber` 接口接进 api.New
> （6 处调用点）+ `POST /v1/stocks/{ticker}/subscribe`；前端 `subscribeLive` + StockView 打开详情页即调用。lruAdd/Subscribe 单测 + go/web 全绿。
> 效果：打开任意股票（含非自选如 RDW）即进实时流。✅ `e1b0d5e` **已部署+LIVE 验证**：`POST /v1/stocks/RDW/subscribe`→`{ok:true}`，
> healthz 200，universe 回血 6650，institutional 2，无回归。**且本环境 Alpaca 数据已追上 6/10 实时**（RDW/RKLB/AAPL 现在 session=`pre`、
> `at` 在 ~1 分钟内）→ **①价格卡盘前分行 + ②实时 现在都能真实演示**：RDW 日内 -15.19%(=Google)+盘前价分行。v3 ①②②b③ 全部 LIVE。
> **✅小票报价陈旧修复（owner 2026-06-10 选 A+B）`869b174`：** 根因=免费 Alpaca 是 IEX 单一所(~1-2% 成交量)，小票几小时甚至几周无 IEX 成交
> （实测 HOTH 上一笔 IEX print 是 5/27）。A=合并行情兜底：`finnhub.Quote`(/quote, parseQuote 单测) + BarCache `ConsolidatedQuoter`
> （IEX 报价 >5min 旧或无数据→overlay 合并行情价/时间/来源，保留 IEX 基准，overlayConsolidated 单测）+ api getQuote 对 store 旧报价
> 也走按需刷新取较新者；main 复用 newsClient（typed-nil guard）。B=诚实文案：徽标改"最后成交 X前 · src"(i18n quote.lastTrade) +
> useQuotes 新鲜度不回退守卫。**LIVE 验证三分支**：HOTH alpaca·5/27→finnhub·6/9(+13天)；YOUL 合并源不更新→保留 alpaca；AAPL 活跃
> 不触发。**预览实测页面渲染 "Last trade 1d ago · finnhub"**。注：Finnhub 黄源、免费展示 OK（付费转售红线不变）。
> **🚚 VPS 升级迁移（owner +$100/年预算，Claude 拍板）2026-06-10：** 1GB→**4GB RackNerd `104.168.38.21`**（$59.99/yr，根治
> `go build` OOM→杀 sshd→fail2ban 锁门）。迁移：装 key+Docker(29.5.3/Compose v5.1.4)→拉最新仓库→`.env` 逐字节复制→`pg_dump --clean`
> 7 张用户表 dump→restore（**watchlist=3, notes=2** 零丢失）→新 cloudflared 作第 2 连接器加入隧道→停老箱 cloudflared+api。**4G 上 Go 镜像
> 构建一次过、零 OOM**（坐实升级价值）。公网验证全绿：healthz 200 / universe 5577 / earnings 319 / AAPL 289.19·alpaca(实时) /
> HOTH finnhub 兜底 / 前端 200。**域名 DNS 零改动**（Tunnel 出站）。老箱 `104.168.46.15`=停机冷备（postgres 留数据），回滚 =
> `docker compose start cloudflared api`。SSH 经验：两箱都"连接保持>1-2s 即掉"→全程后台+轮询、文件传输用 `cat|ssh` 不用 scp。
> ⏭️ 预算余款待 owner 采购：住宅代理(~$10 解锁港股公告+雪球) + LLM 充值(~$10-15 激活中文 AI 摘要)。
> **✅v3.1（owner 2026-06-10 四连）①K线与卡片价格一致**：KLineChart 接收卡片同款实时 quote，`stitchTail` 缝合到末根蜡烛
> （盘中图任何时段缝分钟柱；日线只缝 regular 时段防污染日K收盘=Google/富途行为；W/M/Q/Y 只延展末桶），逐笔走 `series.update()`
> 不重建图表；指标盘下次重建时刷新。**②首页指数改真实点位**：实测 Finnhub 免费版拒绝指数(CFD 需订阅)、Stooq 404；
> **Yahoo v8 chart 从 VPS 可用**(^GSPC 7312.99 实时)→ 复用既有 internal/yahoo 客户端 + `ingest.IndicesCache`(60s 刷、失败保留旧值、
> 单测) + `GET /v1/indices` + IndicesStrip 改造（真实点位+名称，ETF 代理自动降级，tooltip 标 yahoo 源）。**③Vercel/Supabase 锁定+
> 暂停调研**：Supabase 免费版 7 天无活动会暂停，但"活动"含直连 Postgres 的真实查询——咱们后端每隔几分钟写入市场库=永动 keepalive，
> **基本免疫**（唯一暴露面=后端宕机≥7天；可选 $0 保险=GH Actions 每日 ping）。锁定风险：Vercel 低（纯 Next.js 无私有服务，可随时
> 自托管/换 CF Pages）；Supabase 中低（市场库纯 pg_dump 可迁；Auth 用户含密码哈希可经直连导出）。结论：现阶段 $0 方案即可，
> Pro($300/yr) 不必。**④评论 cashtag** → 排期 #39（owner：不紧急，等用户量）。
> 📋 **#39 评论 at 股票（cashtag）**：个股评论自动带 $TICKER；评论体内 $XXX 解析为链接并 fan-out 到对应个股评论区；
> 公共区可 at 多股。等用户量上来再做（owner 2026-06-10）。✅ **已于 2026-06-11 由 owner "不等用户量直接开发" 落地**（见上）。
> **✅盘前/盘后再排查（owner 2026-06-11 "还是不行"）`bf00270`：** 经反复核对,**流动票盘前数据+逻辑本就正确**（Futu/Google 风格:
> 主区=昨收+昨日涨跌,小字第二行=盘前价 vs 昨收;owner 确认"现状即可")。一度误判 prev_close 错位想改,发现会破坏正确显示已撤销。
> 真正修掉两个 bug:①StockView 盘中主数字用实时价、涨跌却用 regular_close → 不一致(RDW 大数字 16.19 显示 +6.9% 实为 +8.9%)
> → 统一为 `regularPrice`(盘中=实时价,其余=昨收),数字与百分比同源;盘前/盘后渲染不变。②`overlayConsolidated` 小票走 finnhub
> 兜底时 regClose(IEX)与 prev_close 混源 → 假日内涨跌(HOTH 显示 +92.94%)→ 扩展时段把 prev_close 锚定到 regClose(日内涨跌归零、
> 扩展 delta 显示真实变化)。单测更新,公网验证 HOTH prev_close 1.36=regClose、假涨幅消除。
> **🚀 v4 启动（owner "直接开干"）：** ①AI 中文化包(待 owner 给 OpenRouter/智谱 key——OpenRouter 兼容现有 enrich 插件,设
> LLM_BASE_URL/KEY/MODEL 即可,零改码)。②速赢三连:**✅财报日历页 /earnings**（后端早 LIVE、前端补页:按日分组+BMO/AMC+EPS 预期/实际
> beat 绿 miss 红+点击进个股,公开页,Vercel 部署)→ 下:提醒中心(铃铛+全局页+重武装)→ 热榜补涨跌幅。③搜索中文化(别名+CJK)。
> ④期权面板(Cboe 免费延迟链)。注:调研称"站点对 Google 隐形"经核实**仅首页**(价格客户端拉取),个股页已 SSR 出 title+名,SEO 没那么糟。
> **✅AI 中文化包·功能①「新闻标题中译」LIVE(owner 2026-06-11 给 key)`a23e94e`:** OpenRouter(DeepSeek v3,$5 额度)主力 + 智谱免费备用,
> key 仅入 VPS `.env`(未覆盖)。enrich.TranslateTitles + `news.headline_zh` 列(翻一次永久缓存、重抓不丢)+ TranslateIngestor(每 3min 扫 20 条最新未译)
> + 前端 zh 界面显示中文标题 +「AI 译」角标(悬停原文)。**调试三连(都修了,各带单测)**:①模型把 JSON 裹 ```代码块 → 三级容错解析;
> ②批量偶尔少返一条 → 改**序号锚定协议** {items:[{i,zh}]},缺的留下轮、绝不串位;③40 条/批超 30s 客户端超时 → 批 20+90s+3min 扫。
> **公网实测**:NVDA 14/40、GOOGL 6、MSFT 5…共 36+ 条中文标题,质量专业(上调评级/跑输大盘/业绩超预期/再融资)。新闻在 Supabase 市场库(非本地 pg)。
> 成本:~$0.00002/条,稳态扫到 0 条即跳过不调 LLM。**AI 包下一步**:个股 AI 速览(每日缓存)→ 每日中文晨报 → NL 选股。
> **✅v4 速赢①热榜价格 LIVE（`b6d87cd`+`4d6ee18`）**：getHot join universe 快照补 price+guarded change_pct(复用 screener 守卫 → 抽出
> guardedChangePct)；universe 快照缺的票回退 store.GetQuote。前端 HotRow 加价格列(sm+)。注:非 ingest 集且不在快照窗口的票(SPY/QQQ 等)
> 暂无价——universe 缓存特性,记为跟进。**✅v4 速赢②提醒中心 LIVE（`<this>`）**：后端 getAlerts 本就全量(不按 ticker)→ 只加重新激活:
> store.ReactivateAlert(active=true+triggered_at 清零,owner 校验,5 层 + 单测)+ api `PATCH /v1/alerts/{id}`。前端:`/alerts` 全局页
> (AlertsCenter,按 触发/监控中 分组,触发的带"重新激活"+删除,股票可点)+ TopNav `AlertsBell`(登录态轮询 60s 数已触发,红点角标,
> 任意页可见)+ secondary nav 加"提醒"+ i18n zh/en。**不碰 web-push（DEFERRED）**。下:③ 搜索中文化 → ④ 个股 AI 速览 → ⑤ 晨报 → ⑥ 期权。
> **✅v4③搜索中文化 LIVE（`e3b2e81`，公网验证：英伟达→NVDA/苹果→AAPL/台积电→TSM/英文无回归）**：aliases.go ~100 票中文别名 +
> Symbol.Aliases + Build 合并(ASCII 别名进 token 索引) + Search CJK 路径(精确 rank0/子串 rank2) + hasCJK + 单测。
> **✅v4④个股 AI 速览 LIVE（`b583ec8`，验证：首次 3s 生成、复调 0.7s 缓存命中同 generated_at，中文带"据新闻/据社区讨论"来源标注）**：
> getSummary 按(ticker,ET日)缓存+inflight 去重+失败退额度+150/日全局上限;enrich 中文防幻觉 prompt;AISummaryCard(紫 Sparkles+
> AI 角标+免责)+i18n。**✅v4⑤中文晨报（本 commit）**：enrich.Brief(晨报编辑 prompt,材料 only,无建议)+`ingest.BriefingCache`
> (每日 ET≥07:00 生成一次,30min 检查,材料全自有零请求:指数+涨跌 Top5(防伪影/仙股)+今日财报+国会/13D 前 3,缺节跳过,失败下轮重试)+
> `GET /v1/briefing`(404=未生成;api.New 第 20 参,5 调用点同步)+ /briefing 页(BriefingView:Markdown 正文+AI 角标+免责+日期)+
> nav"晨报"+i18n。token:1 次/日≈忽略。下:⑥ 期权面板(Cboe)→ 收尾小项。
> **✅v4⑥期权面板 LIVE（后端 `48248a0` + 前端本 commit；公网验证 AAPL：P/C 0.63量/0.71持仓、最大痛点 $295(6/12到期)、OI Top10
> 91k…，二次缓存命中；预览 AAPL 渲染卡片、0700.HK 正确隐藏）**：internal/cboe(Cboe 延迟 CDN 无鉴权,OCC 解码+P/C+MaxPain(最近到期)+
> OITop,全单测)+ ingest.OptionsCache(15min TTL+inflight 去重+负缓存)+ GET /v1/stocks/{t}/options(api.New 第 21 参,5 调用点同步)+
> OptionsCard(沽购比双指标变色+最大痛点+OI 龙虎榜表 C 绿/P 红+「延迟15分·Cboe」角标,404 隐藏)+ i18n。免费展示不转售。
>
> ## 🏁 v4 主线 6 项全部交付（2026-06-11/12,本会话 owner 解锁 #39 后连续 /loop 自主开发）
> ① 热榜价格 ② 提醒中心(铃铛+/alerts+重激活) ③ 搜索中文化(英伟达→NVDA) ④ 个股 AI 速览(日缓存) ⑤ 中文晨报(/briefing) ⑥ 期权面板。
> 加 owner 临时插入:#39 评论 cashtag、巴西 B3、AI 新闻标题中译、盘前价 bug 复核。全部线上验证。AI 用 OpenRouter(DeepSeek)+智谱备用。
> **建议下一步**(待 owner):SEO/SSR(首页对 Google 隐形)· 住宅代理解锁港股公告+雪球 · 期权异动榜 · 13F 大佬持仓 · 站外推送提醒。
> 收尾小项(可选):13D/G 榜 CIK→ticker 可点、评论"我已赞"回传、指数条加 ^HSI、i18n 英文硬编码扫尾。
>
> ## v5 计划（owner 2026-06-12："先做 1/3/4,2 放后面"）
> **① SEO/SSR**(进行中)→ **③ 期权异动榜 / 13F 大佬持仓 / 站外推送**(注:web-push 仍 DEFERRED,push 走邮件/TG 或再缓)→ **④ 收尾小项**。
> **⏸ ② 住宅代理(~$10/年,解锁港股公告+雪球)→ 延后**,待 owner 采购代理凭据再做(代码框架早已写好,卡在 IP)。
> **✅v5①(a) SEO 首发(本 commit,纯前端)**:发现 SEO 基础其实已成熟(个股页 SSR+generateMetadata+JSON-LD+ISR、robots.ts、layout OG、sitemap 有个股页)。
> 本增量补缺口:sitemap 补 /smart-money /screen /earnings /briefing(原先漏,Google 发现不了);旗舰看板页中文关键词 metadata——
> smart-money→「国会山股神·佩洛西持仓·13D举牌」、opportunities→「美股内部人买入·高管增持」(瞄准研究指出的中文搜索空档,零中文工具竞争)。
> SEO 下一步(下 tick 评估):首页是客户端壳(有 layout metadata 但服务端内容薄)→ 可加 SSR 内容块;或 pSEO 中文关键词落地页。
> **✅v5①(b) 首页 SSR 增量(本 commit,纯前端)**:page.tsx(服务端)加 JSON-LD(WebSite+SearchAction+Organization,中文 alternateName"潮汐美股")+
> 关键词 metadata(美股/国会山股神/内部人买入/财报/期权/轧空)+ 服务端渲染介绍段 + 8 看板内链目录(给爬虫真内容+内链,实时模块仍客户端)。
> 预览验证:JSON-LD 2 schema、介绍段、8 内链、hub 不破、零报错。**SEO(①)到此收官**(基础已足;pSEO 落地页留 backlog)。转 ③ 期权异动榜。
> **✅v5③(a) 期权异动榜(本 commit,后端 Go + 前端 web)**:复用 internal/cboe。OptionsCache 加后台 Run(ctx) goroutine——每 30min 对 40 支
> 重仓期权美股(科技巨头/meme/主要 ETF)逐票拉 Cboe 延迟链(票间 1s 限速,后台不阻塞请求),汇总所有有成交合约,按**单合约成交量降序**取 top 30
> (附量比 vol/OI)。GET /v1/options/unusual 暴露(给现有 OptionsSource 接口加 Unusual() 方法,免 api.New 签名 churn 5 处)。前端 /unusual 页 +
> UnusualOptions 表格(ticker链/看涨看跌徽标/行权/到期/成交量/未平仓/量比/IV,"延迟15分·Cboe")+ nav secondary「期权异动」+ zh/en i18n +
> 中文关键词 metadata(期权异动/量比/期权龙虎榜)。免费展示已标注延迟、不转售。部署后首次扫描 ~1-2min 出数。
> **③ 下一步**:13F 大佬持仓(SEC 13F datasets + OpenFIGI CUSIP→ticker,名人基金白名单季度 diff→smart-money 加 tab)→ 站外推送(web-push DEFERRED→邮件/TG 或问 owner)。
> **✅[插入修复] 盘后价 bug(owner 报 RDW 17.09 冻结)**:免费源盘后冻结(Finnhub /quote + IEX 稀疏)→ 兜底改 Yahoo includePrePost 分时,source=yahoo,实时(详见 CLAUDE.md「Extended-hours freshness fallback」)。
> **✅[owner 反馈两项,本 commit 纯前端]**:(1) **首页底部介绍英文页显示中文**——该介绍段是 SSR(不能用客户端 useT),改为 zh+en 双语都渲染、按 `<html lang>` 用 CSS `[data-i18n]` 只显示当前语言(globals.css;爬虫两语都收录、读者只见当前语言;production CSS 已含规则,预览验证 EN/ZH 切换正常)。审计其余组件无硬编码中文泄漏(仅 TopNav 语言切换按钮是故意双语)。(2) **AI 总结(盘前晨报)从独立页并入首页**——新 BriefingCard 挂 HomeHub(行情条下方,无晨报时自隐),删 /briefing 页+BriefingView+nav 项+sitemap 项+nav.briefing i18n;sitemap 顺带补 /unusual。原则:场景类似可合并,不必每功能独立 nav+页。
> **④ i18n/页面收尾备忘**:首页晨报正文仍是 AI 中文(数据按源展示,Chinese-first;若要双语晨报=改 LLM prompt 生成两语,留 backlog)。首页 metadata title 仍英文默认(可中文关键词化)。
> **✅v5③(b) 13F 大佬持仓 后端(本 commit,纯 Go)**:数据路径全验证后实现——`internal/sec/thirteenf.go`(submissions API 取最近 2 个 13F-HR → 从 filing index.json 找信息表 XML[非 primary_doc.xml] → 解析 infoTable,按 CUSIP 聚合多 lot,value=整数美元/PRN 不计股数)+ `internal/openfigi`(CUSIP→ticker,keyless 批量≤10、25/min、进程内永久缓存)+ `internal/thirteenf`(8 家名人基金白名单[Berkshire/Scion-Burry/Pershing-Ackman/Himalaya-李录/Duquesne/ThirdPoint/Baupost/Bridgewater,CIK 均已验证]→ 最新季 top15 + 环比 new/add/trim/hold + pct + Cache.Run 每 12h)。API `GET /v1/13f`(ThirteenFSource 接口 + setter 免不掉,走 api.New 新增参数+同步 5 处)。单元测试:infotable 聚合/PRN 跳过、compute 排序+环比标签、openfigi 缓存。**前端 /smart-money 13F tab 下 tick 做**。13F 滞后~45 天须前端标注「截至 Qx」。首次扫描约 30-40s(SEC 限速 + OpenFIGI 2.5s/批 warmup)。
> **✅v5③(b) 13F 前端(本 commit,纯前端→Vercel)**:`/smart-money` 加第三 tab「大佬持仓」(SmartMoneyTab 加 '13f',?tab=13f 入口)。新 `ThirteenFBoard`:每家基金卡片(经理+firm+「截至 2026 Q1」+组合总值)+ 持仓表(ticker链/issuer/市值 fmtCompactUSD/占比%/环比徽标 new蓝·add绿·trim红·hold灰 + chg_pct)。unmapped CUSIP(如 Chubb 外国 CUSIP)显示 issuer 不可点。api.ts getThirteenF+类型,i18n zh/en(13f.*)。预览实测 EN+ZH 双语:8 家基金、巴菲特 AAPL 22%/GOOGL 加仓+204%/CVX 减仓-35% 真实数据、徽标配色、滞后免责声明、零报错。**③ 期权异动+13F 全部交付**;转 ③(c) 站外推送(web-push DEFERRED → 邮件/TG 或问 owner)。
> **⏭️③(c) 站外推送 → backlog(owner 2026-06-12 选择"先跳过,做 ④")**:剩余渠道(邮件/Telegram)都需 owner 账号/凭据,待其决定再做。
> **✅v5④ 盘后价兜底接入 poller(本 commit,纯 Go)**:把 BarCache 已用的 Yahoo includePrePost 兜底逻辑(overlayConsolidated,IEX 过期/缺失时覆盖盘前盘后真实价)也接到 PricePoller(price.go US 路径,新 SetConsolidatedFallback setter,复用同一 quoteFB)。原先只有按需路径(冷门股如 RDW)有此兜底,现在**热门∪自选轮询集里的冷门股盘后/盘前也实时**,不再冻结在收盘价。新鲜 Alpaca 报价不触发(仅 Price==0 或 >5min 过期才兜底)→ 主流股无行为变化。复用已测 overlayConsolidated,build/vet/test/gofmt 绿。
> **✅v5④ 首页 title 中文化 + 评论"我已赞"回传(本 commit,前端+后端)**:(1)首页 page.tsx metadata 加 title:{absolute:'潮汐 Tickwind · 美股实时行情/国会山股神/期权异动/13F大佬持仓/财报'}(绕过 layout %s 模板,瞄准中文关键词;预览验证 document.title 已变)。(2)评论已赞状态从服务端回传:store.Comment 加 Liked、ListComments 加 viewerID 参数(memory 查 cmtLikes 集合、postgres 加 EXISTS 子查询 $1=viewer、split 透传、getComments 取可选 auth.UserFrom 传 viewer),前端 CommentsPanel 用 c.liked 作初始态(刷新后已赞仍亮)+ api.ts Comment 加 liked。anon viewer="" → liked 永 false。Go+web 全绿。**剩余 ④**:指数条 ^HSI、13D/G 可点、双语 AI 晨报。
> **🐞HOTFIX(本 commit):上一条引入的回归**——postgres `ListComments` 把匿名 viewerID="" 绑到 uuid 列 `comment_likes.user_id` → `/v1/comments` 对匿名用户报 `22P02 invalid input syntax for type uuid:""`(社区板/未登录全挂)。修:viewerID="" → 绑 NULL(非"")+ `$1::uuid` 显式转换,NULL 永不匹配→liked=false。(memory 路径无此问题,uuid 是 postgres 专有;无 pg 集成测试→靠公网 curl 验证。)
> **✅v5(owner UI批#1+#5,本 commit 前端)**:#1 移动端 Footer 从竖排纯文本链接改为 2 列 chip 方块网格(sm:hidden 切换,桌面保持文本列;呼应首页目录卡片观感)。#5 个股详情页头部:股票编码(AAPL)上移为 h1 粗体 + MarketBadge/SessionBadge,公司全称下移为灰色小字(与首页 StockCard 一致;占位名==ticker 时不显示)。**owner UI 批剩余**:#2 详情页排版重组(options 移到 K线下方+模块左右组合)、#3 AI 解读按语言+加载动画、#4 盘前后正股涨跌显示前一交易日(非零)。
> **✅v5(owner UI #3,本 commit 前端+后端)**:AI 解读双语 + 加载动画。(a)`enrich.Summarize` 加 lang 参数 + 英文 systemPromptEN(同防幻觉/免责护栏),按 lang 选 prompt;getSummary 读 `?lang=`(默认 zh,Chinese-first)、缓存键加 lang(ticker|day|lang)、日清理 suffix→`Contains("|day|")`修正。(b)前端 AISummaryCard 传 useLang().lang(切语言重新拉)+ api.ts getSummary(ticker,lang,signal)。(c)加载态从裸 skeleton 改为带标题+Loader2 转圈+「正在解读最新动态…/Reading the latest…」+ 3 条 shimmer(LLM 调用要几秒,不再突兀空白)。i18n ai.loading。Go+web 全绿;部署后 curl `?lang=en` 验证英文要点。**owner UI 批剩余**:#2 排版重组、#4 盘前后正股涨跌。
> **✅v5(owner UI #4,本 commit 纯前端)**:盘前/盘后正股涨跌显示前一交易日(非零)+ 股票卡盘前盘后小数据。诊断:扩展时段 overlayConsolidated 把 quote.prev_close 锚到 regClose(防冷门股幻象涨幅)→正股日涨跌=regClose-prev_close=0。修法(不动后端锚定、零回归):前端正股日涨跌改用**可靠日线 bars 的前收**(closes[-2]),仅扩展时段启用(常规时段仍用 quote.prev_close=今日内移动)。StockView 头 + StockCard 都加 priorClose=isExt&&closes>=2?closes[-2]:prev_close。StockCard 重构:主图=正股价(regClose)+正股涨跌(vs priorClose),下方加小行「盘前/盘后 {extPrice} {Δ vs regClose}」(owner 要的盘前盘后小数据;pre|post|overnight 同一 isExt 路径→盘后同样显示)。**盘前实测验证**(source=yahoo 即锚定 bug 场景):AAPL 详情头 $295.48 +1.37%(正股,非零!)/ Pre-market $296.22 +0.25%;首页 AAPL/NVDA/TSLA/MSFT 卡均主图正股涨跌+小盘前行,红绿独立正确。web build/lint 绿。**owner UI 批剩余**:#2 排版重组。
> **✅v5(owner UI #2,本 commit 纯前端)= owner 5 项 UI 批全清**:详情页 StockView 模块重排。原序:PulseBar/Earnings/Short→AISummary→Options→Fundamentals→**KLineChart**(在最底,紧邻 tabs)。新序:PulseBar/Earnings/Short→**KLineChart【Price&indicators】**(上移为锚)→**OptionsCard**(紧跟 K线,owner 要)→**grid lg:grid-cols-2 [FundamentalsCard | AISummaryCard]**(窄模块左右合并,减少滑到 News/Discussion 的空档)→ login gate → tabs。grid 只用 gap-x-6(卡片自带 mb-6 管纵向),lg 双列/mobile 单列。预览验证:桌面 h2 序 Price&indicators→Options→Fundamentals→AI Digest、Fundamentals/AI grid=436px×2;mobile=343px 单列。web build/lint 绿。**🎉 owner 本批 5 项 UI(#1 footer/#2 排版/#3 AI双语/#4 盘前后涨跌/#5 ticker换位)全部交付**。转 ④ 收尾:双语晨报(briefing 按 lang)、指数条^HSI、13D/G 可点。
> **🔧#2 二次调整(owner 2026-06-12 截图反馈)**:左右双列因 AI 篇幅不固定→Fundamentals 旁留大片空白(且窄列里数值被截断 $26…/$12…)。改为**各自整行**且**移到 K线上方**。新序:chips→**FundamentalsCard**(整行)→**AISummaryCard**(整行)→**KLineChart**→**OptionsCard**→tabs。撤掉 grid-cols-2。预览验证:h2 序 Fundamentals→AI Digest→Price&indicators→Options、无 2 列 grid、Fundamentals 整行数值完整显示($26.91B/$120.07B)。纯前端,web 绿。
> **✅v5④ 双语晨报(本 commit 前端+后端)**:晨报(BriefingCard 首页)按用户语言出。enrich.Brief 加 lang 参数 + 英文 briefPromptEN(同护栏,材料同源[中文小节标记+数字],英文 prompt 重组为 Indices/Movers/Earnings/Smart money)。**BriefingCache 是每天生成一次缓存**,故改为**同一物料同时生成 zh+en 两版**(textEN 字段;en 失败非致命→回退 zh),Get(lang) 按 lang 返回。BriefingSource.Get 加 lang,getBriefing 读 ?lang=,前端 BriefingCard 传 useLang().lang(切语言重拉)。每天 2 次 LLM 调用(便宜)。Go+web 全绿;部署后 curl `?lang=en` 验证英文晨报(需 ingestor 生成周期~几分钟出 en)。**剩余 ④**:指数条^HSI、13D/G 可点。
> **✅v5④ 指数条 ^HSI 恒生(本 commit 前端+后端)**:indices.go indexSymbols 加 {"^HSI","Hang Seng"}(Yahoo 源,实测出数 24718/HKD/+1.93%;港股时段,美指收盘时仍在交易)。IndicesStrip 原 hardcode grid-cols-3→改为按实际格数动态(真实指数4格=grid-cols-4 / ETF 兜底3格=grid-cols-3,保持单行+左边框分隔正确)。strip 显示点位+%(无货币符号),HSI 同款展示无需特殊处理。Go+web 全绿;部署后 curl /v1/indices 出 ^HSI(4指数)+ 预览首页第四格(桌面 grid-cols-4 / 移动 4 格,长标题截断但 %/点位清晰)。
> ## 🏁 v5 会话收官（2026-06-12,连续 /loop 自主开发 + owner 多轮反馈）
> **本会话 v5 全部交付**（按时间）:① SEO(首页 SSR+JSON-LD+中文关键词 metadata、sitemap 补全)· ③(a) 期权异动榜(internal/cboe 全市场扫描 + /unusual 页)· ③(b) 13F 大佬持仓(internal/sec/thirteenf+openfigi+thirteenf,8 家名人基金,/smart-money 第三 tab)· **盘后价 bug 修复**(owner 报 RDW 冻结→Yahoo includePrePost 兜底,BarCache+poller 双路径)· 首页中英双语 SSR([data-i18n] CSS 切换)· AI 晨报并入首页(删 /briefing 独立页)· **AI 解读&晨报双语**(Summarize/Brief 加 lang+英文 prompt)· 评论已赞回传(ListComments+viewerID)· 首页 title 中文关键词化· **owner 5 项 UI 批**(#1 移动端 Footer 方块/#2 详情页排版重组/#3 AI解读双语+加载动画/#4 盘前后正股涨跌显示前一交易日+卡片小行/#5 详情页 ticker 上全称下)· ④收尾(双语晨报、指数条 ^HSI)。**HOTFIX**:匿名评论 uuid 22P02 回归(viewerID="" 绑 uuid 列)。
> **Backlog(待 owner 或后续)**:③(c) 站外推送(owner 选跳过,渠道[邮件/TG]待定)· ② 住宅代理(~$10/年,待 owner 采购凭据)· **13D/G CIK→ticker 可点**(InstitutionalFiling 无 ticker 字段,符号缓存丢弃 CIK→需给 Symbol+CIK 字段/解析/TickerByCIK 索引+api 接口+前端,属大改,owner 嘱backlog)· 双语晨报正文随 caches 自愈(首次 provisional 缺指数段)· StockCard/StockView 的 priorClose/扩展时段计算可抽公共 helper· 移动端 4-格指数条长标题截断(可 2×2 或缩字号)。
> **loop 已停**(④ 收官,不再 ScheduleWakeup)。所有改动单 commit/tick、Go/web 全绿、公网 curl/预览验证、ROADMAP 同步更新。
> **🧹 老箱清空（owner 2026-06-10 要求腾给其他项目）**：先复核新箱用户数据完好(watchlist=3/notes=2)→ `104.168.46.15` 容器/卷/镜像
> 全删、/root/tickwind(含 .env)删除、shell 历史清除。Docker 引擎+部署公钥保留可复用。**老箱不再是回滚备机**；恢复路径=新箱
> `/root/tw_users_only.sql` + Supabase 市场库 + 迁移 runbook。
> **✅页面合并（owner 2026-06-10）**：`/institutional` + `/congress` → **`/smart-money`（聪明钱）双 tab**（13D/G 机构举牌 | 国会交易），
> 旧路由 permanentRedirect 保收藏/外链，导航二级菜单少一项。预览实测：重定向带 tab 预选、切 tab 内容+URL 同步、零控制台错误。
> 其余页面评估过不合并：机会榜=旗舰保持独立；/discussion(聚合社交)≠/community(真人评论)；财报已融合在事件时间线。
> **▶️ 解除暂停（owner "可以开干了"）**：#23 FINRA 轧空雷达 — **匿名 API 已验证可用**（consolidatedShortInterest 含
> daysToCover/short qty/ADV/change%；⚠️默认返回最老期，需按 settlementDate 过滤最新结算期）。#38 巴西 B3 — **brapi key 已验证**
> （PETR4 实时 41.83 BRL + marketCap）。循环按 #23 → #38 顺序开工。#39 cashtag 仍按 owner 指示等用户量。
> **✅#23 FINRA 轧空雷达 LIVE（`86f4f37` 后端 + `546e116` 前端）**：API 契约实测=settlementDate 是分区键（sortFields 被拒须
> EQUAL）→ `finra.LatestSettlementCandidates`（15日/月末工作日回调+节假日±2天余量，单测）探测最新已发布期 →
> `ingest.ShortCache` 每日全分区分页（5000/页+500ms 礼貌间隔，~1.6万股入内存，失败保旧表，日期无关化单测）→
> `GET /v1/stocks/{t}/short`（404=无行）→ 前端 ShortChip（回补天数/空头仓位 M/B/环比变色/「轧空风险」徽标 DTC≥5 或环比≥+20%/
> 截至日·FINRA，404 整体隐藏）。**公网验证抓到最新期 2026-05-29**：GME DTC 11.99+徽标 ✓ / AAPL 3.38 无徽标 ✓ /
> SPY 有数据（FINRA 覆盖 ETF，意外之喜）/ 0700.HK 正确隐藏 ✓。零控制台错误。
> **✅#38 巴西 B3 市场 LIVE（`7052015`）**：`internal/brapi` 客户端（token-gated，parseQuote 单测）+ `BRAdapter`（照 HK 延迟报价
> 模板：canonical `.SA` 后缀路由、调用时 strip 成裸码喂 brapi、BRT 时钟 session、Source `brapi`）+ `market.Of` 加 `.SA`→BR（单测）+
> config `BRAPI_API_KEY` + main 注册（key 在则启用 + brazilSeed 6 支注入 ingest，缺 key 则 warn 跳过）+ `symbols.ForeignSeeds` 加
> 12 支 B3 蓝筹（Country=BR/Exchange=B3）。**公网实测**：search "PETR"→PETR3/PETR4.SA 置顶、"vale"→VALE3.SA、PETR4.SA 报价
> `41.71 regular brapi`（实时）。多市场框架现含 US/TW/HK/KR/BR。注：brapi 黄源、免费展示 OK（付费转售红线不变）。
> **🏁 开发循环阶段性收官**：roadmap 仅余 #39 评论 cashtag（owner 指示等用户量再做）。v3 计划 + #23 + #38 全部交付。
> **✅#39 评论 cashtag（owner 2026-06-11 解除等待"直接开始开发"）**：`internal/cashtag`（$TAG 正则：1-6 位字母数字+可选
> 场所后缀；纯数字无后缀=价格剔除；上限 8 个；10 用例单测）→ Comment.Mentions + `comment_mentions` 表（幂等 schema）→
> SaveComment/UpdateComment 事务化写 mentions（编辑全量替换）→ **ListComments 并集**（个股列表 = 发在本股 ∪ 提及本股，
> postgres OR 子查询 + memory 切片匹配，fan-out 单测）→ 前端：Markdown `linkifyCashtags`（跳过代码块；$tag→内链 /stock/，
> Node 5 用例全过）+ 个股评论框**默认预填 $TICKER**（只剩前缀禁发）+ mdHint 文案。Go/web 全绿。
> **🔭 调研第三轮（owner："想法扩散开，可以有费用"）**：5 subagent 并行 → `docs/research/2026-06-11-*.md`（竞品缺口 12 条/
> 老功能迭代审计 Top8/新数据源实测 Top10/社区+增长各 6 条/AI 功能含 token 成本核算）。跨报告共识：AI 中文化、站外触达、
> 期权数据（Cboe 免费链实测可用）、13F、**SSR/SEO（站点对 Google 隐形=最高优先工程项）**。待 owner 定 v4 优先级。
> ◐③ 机构/13D举牌榜(41)：**数据源核查** —— SEC 直连从本沙箱 IP 被 403（curl+WebFetch 都不行），但 VPS 上现有 `internal/sec`
> 客户端（带 UA/gzip/限流）能成功取每日索引（机会榜 Form-4 count:14 为证）；efts.sec.gov 从 VPS 可达(200)但需调参。**结论：复用
> 已验证的 sec 客户端走每日索引路径。** #3a `internal/sec/ownership.go`：`DailyBeneficialOwnership(date)`(复用 `c.get`) +
> `parseOwnershipIndex`(提取 "SC 13D/13D-A/13G/13G-A" 行；13D=活跃举牌 Activist=true，13G=被动；Company=标的issuer) + 单测 ✅ 本 tick
> （go 全绿；未接服务=dead code，无需部署）。**下一步 #3b**：cache + ingestor（仿 congress，扫近 2-3 天去重）→ 部署验证 SEC 实时取数返回真实 13D/13G
> → API `/v1/institutional` → 前端榜单页（13D/13G 标签区分；申报人(BlackRock 等)从 filing header 解析，可作 #3c 增强）。
> 注：被动三巨头 13G 信号弱，UI 诚实区分。 #3b：`internal/institutional` Cache + `ingest/institutional.go`(InstitutionalIngestor，
> 扫近 4 天去重，每 8h) + nil-safe `InstitutionalSource` 接口(api.New 5 处调用点同步) + `GET /v1/institutional`(`?type=13d|13g`,`?limit=`) +
> main 无条件起 ingestor（sec.New，公开数据）+ config `INSTITUTIONAL_SWEEP_EVERY` ✅ `46a7a34` **已部署+LIVE 验证**：`/v1/institutional`
> 返回真实 13D/13G（例：`SC 13D/A · GENCO SHIPPING & TRADING LTD · 20260608 · activist:true`，healthz 200）——**确认 SEC 实时取数在 VPS
> 端到端工作**。注：本合成 2026 环境 13D/13G 数据稀疏(count=1)，真实生产会有几十条；索引日期是 `YYYYMMDD` 格式(前端需格式化)。
> **#3c 下一步：申报人(institution)解析** —— filing `.txt` SGML header 的 "FILED BY:" → "COMPANY CONFORMED NAME:" 抠出机构名，丰富
> `OwnershipRef.Filer`（sec 加 FetchFiler，ingestor 每条调用，限流+capped）；这是 owner 想看的核心("贝莱德加仓了谁")。**#3c ✅ 本 tick**：
> `OwnershipRef.Filer` + `sec.FetchFiler`(读 filing `.txt` 头前 64KB via 新 `getLimited`) + `parseFiler`(取 "FILED BY:" 后首个
> "COMPANY CONFORMED NAME:"，单测 GENCO/CENTERBRIDGE) + ingestor 对最新 60 条填充 Filer（OwnershipFetcher 接口加 FetchFiler）。go 全绿，
> ✅ LIVE 验证：`/v1/institutional` filer 已填充（真实例：**DIANA SHIPPING INC. → GENCO 的 SC 13D/A 主动举牌**）。**#3d ✅ 本 tick**：
> 前端 `/institutional` 榜单页（`InstitutionalBoard`：申报人→标的+13D活跃/13G被动标签+申报日期(YYYYMMDD格式化)+SEC文件夹链接；全部/13D/13G 过滤切换；
> 诚实标注 13D 主动 vs 13G 被动；空/错/骨架态）+ `api.ts getInstitutional` + 导航(secondary)`机构举牌`/Institutions + zh/en i18n inst.*。web build+lint 绿。
> **→ v3 三想法全部交付：①价格卡盘前盘后(LIVE) ②实时 WebSocket(部署，实时待开盘) ③机构/举牌榜(LIVE)。** 旧 #3d：前端 `/institutional`
> 榜单页（申报人+标的+13D活跃/13G被动标签+日期+SEC链接，非投资建议）+ 导航。#7/#8 暂停；#2a WS 实时待开盘验证（本环境市场锚定 6/9，演示不了）。**
> **▶ RESUMED 2026-06-09 — owner restored SSH; the #2a+#3a backlog deployed + verified (universe
> ~6.5k stocks; #3a is dead code until #3b wires it). KEY DEPLOY FIX: background the ENTIRE deploy
> script via `nohup` so the SSH command returns sub-second (the flaky link drops connections held open
> >~a few seconds — e.g. during the remote curl/tar — but a sub-second launch survives). Verify via
> public curl. See CLAUDE.md. Loop continues at #3(b) earnings store+ingestor.**

3 parallel research agents (competitor gaps · free data sources · AI/LLM). **Convergence: the
SEC/EDGAR backbone is the defensible, redistribution-safe lane.** Owner picks which to build:
- **Top sequence (free/GREEN data):** ① Price/event **Alerts** (own data, #1 retention) · ②
  **Fundamentals/Financials tab** (XBRL, GREEN) · ③ **AI filing summary+diff** (cacheable, low
  risk; needs `LLM_API_KEY`) · ④ **Congress trading board** (gov public-domain, viral) · ⑤ **13F
  institutional holdings** · ⑥ **FINRA short interest** (display-only; bulk redistribution gated).
- Then: screener · earnings calendar · Treasury macro rail · Wikimedia attention · community
  upgrade · paper-trading.  **RED:** earnings-call transcripts (paid feed), Google Trends,
  CoinGecko free tier.  Standing RED unchanged: live quote redistribution (Alpaca/Yahoo).

**✅ Shipped this session (2026-06):**
- **Financials tab** (free SEC XBRL): `edgar.Fundamentals` (latest-FY revenue/net-income/EPS +
  shares/equity, weighted-avg fallback) + `GET /v1/stocks/{t}/fundamentals` (market cap / P/E / P/B
  from live price) + `FundamentalsCard` on StockView (市值/市盈率/营收/净利润). Live-verified AAPL/MSTR.
  TTM is a later enhancement (v1 = latest fiscal year).

- **SEO**: full-universe sitemap (popular ∪ live boards, ISR) + per-stock JSON-LD (Corporation +
  BreadcrumbList + financials Dataset) + canonical + company-name titles. Live. ⚠️ hreflang /
  bilingual SEO deferred (needs URL-level i18n — design / owner).
- ✅ **CI security**: govulncheck (blocking — confirmed no reachable vulns) + gosec (informational)
  + Dependabot (gomod / github-actions / npm, weekly). All 3 CI jobs green.
- **Alerts v1**: `store.Alert` + `/v1/alerts` CRUD + StockView "Alerts" tab (price-above/below,
  daily-move %, new-filing) + evaluator goroutine (every 2m → triggered) + in-app "triggered"
  badge. All store backends + tests; live. ⑤ web-push DEFERRED (owner; iOS needs a PWA; email alt
  needs SMTP creds).

**🏗 Owner feature batch (2026-06-08) — 9 ideas from real usage, built at 1-min `/loop` cadence;
scoped by 5 parallel planning agents (full plans in session). Priority = bugs/quick-wins first:**

1. ✅ **Watchlist remove** (#25) — remove was already wired backend→api.ts→board; the gaps were UX:
   the detail page was add-only and the board's X was hover-only (invisible on touch). Fixed:
   detail-page Add button is now a toggle (the "On watchlist" pill reveals a rose "Remove" on hover)
   + the board card's X is always visible. Frontend-only, live.
2. ✅ **Homepage indices strip** (#24) — `IndicesStrip` above the Markets strip, ETF proxies
   **SPY/DIA/QQQ** via the existing `useQuotes`/Alpaca path (free IEX serves ETFs, not `^GSPC`;
   Yahoo stays HK-only). Honest design: **% change is the headline** (tracks the index), ETF
   ticker+price on an attributed sub-line (so "SPY 745" isn't misread as the S&P level); QQQ =
   "Nasdaq 100". Live-verified quotes (SPY/DIA/QQQ all return price+prev_close). i18n `home.indices`.
   Prices are on-demand via `getQuote`→snapshot; optional later: add the 3 to `ingestTickers` for SSE.
3. ✅ **Search: index ETFs + OTC** (#26) — LIVE (verified: DRAM→Roundhill Memory ETF/Cboe BZX,
   TQQQ→ProShares/Nasdaq now autocomplete). New `internal/symbols/nasdaq.go` `FetchNasdaqTrader`
   (keyless Nasdaq Trader files) merged SEC-first in `ingest/symbols.go`. Deploy needed a
   **detached `nohup` build** (SSH was dropping mid-build) — now recorded in CLAUDE.md. SIVEF-class
   pink sheets remain unindexed (no free source) → reachable via #27's "go anyway" fallback.
   DRAM lives in **Nasdaq Trader `otherlisted.txt`** (keyless, pipe-delimited, ETF col; skip the
   `File Creation` trailer + Test-Issue rows) → new `internal/symbols/nasdaq.go` `FetchNasdaqTrader`,
   merge **SEC-first** in `ingest/symbols.go:~59` (~+5.7k symbols). SIVEF-class pink sheets are in NO
   free keyless file → reachable via #27's "go anyway" fallback (don't pursue paid OTC data).
4. ✅ **Search results page** (#27) — LIVE (frontend, Vercel). new `(main)/search/page.tsx`; gave `SearchBox` an `onSubmit` →
   `/search?q=` (replace the blind `choose(q)` Enter fallback); wire BOTH TopNav instances; render
   0/1/many states + a "Go to /stock/{Q} anyway →" escape hatch.
5. ✅ **Holdings/portfolio** (#29) — **FULLY LIVE** (2026-06-09). `store.Holding` upsert-by-(user,ticker),
   Split→User, `holdings` table, `/v1/holdings` CRUD (verified live: 401 = requireUser) + StockView
   "Holdings" tab + `/portfolio` page & nav. Value/P&L derived from live quotes. Backend deploy was
   blocked for ~1h by a **VPS SSH outage** (1GB-RAM OOM killed sessions → transfers dropped; fail2ban
   then banned the IPs) — resolved by adding swap + whitelisting the deploy IP + the **GitHub-pull
   deploy** method (box pulls source from the public repo via a short SSH command). See CLAUDE.md.
6. ✅ **Hot-topic → topic page** (#28) — LIVE (frontend, Option A). New `/topic/[key]` page reuses
   `/v1/topics` `related_tickers` for a stocks strip + batched topic-filtered news; `TopicsStrip`
   href flipped off `/news?topic=`. Optional later (Option B): a `GET /v1/topics/{key}` endpoint for
   cold/deep-link topics + SEO (needs backend deploy).
7. ✅ **Event-title i18n (zh)** (#30) — LIVE (frontend). events carry a stable `Subtype` enum
   (fomc/cpi/nfp/ppi/gdp/jobs/eci/election). New `web/src/lib/eventTitle.ts` subtype→{en,zh} map,
   wired at the `EventsTimeline.tsx` render site (fallback to the English title). No backend change.
8. ✅ **Events restyle** (#31) — LIVE: shipped safe refinements (rail gradient fade, brighter
   low-importance node, category hue macro=sky/world=violet with amber reserved for importance).
   Deeper redesign (horizon grouping, timeline skeleton) handed to owner as a paste-ready **design
   prompt** (presented in chat 2026-06-08) for a pro designer.

**⏸ Paused (resume after the batch): FINRA short-interest "squeeze radar"** — per-stock short
pressure, a free "follow the money" signal that's ticker-keyed (no CUSIP/entity mapping). Attribute
"Source: FINRA"; display-only (no bulk redistribution). **Fallback (SEC 13F) NOT needed — reachable.**

✅ **Step ① data-access verified (2026-06-08), both sources keyless + reachable from local AND VPS:**
- **Daily short volume** — `GET https://cdn.finra.org/equity/regsho/daily/CNMSshvol{YYYYMMDD}.txt`
  (the consolidated NMS file). Pipe-delimited, header
  `Date|Symbol|ShortVolume|ShortExemptVolume|TotalVolume|Market`. Signal = **% short of daily
  volume** = ShortVolume/TotalVolume (e.g. 20260605 AAPL ≈48.5%, MSTR ≈40.3%, GME ≈61.3%, NVDA ≈34%).
  Whole-universe file (~8k symbols, a few MB) → fetch once/day, keep an in-memory `map[symbol]`,
  serve per-ticker instantly. Try today's date, fall back to prior trading days until 200.
- **Bi-monthly consolidated short interest** — `POST
  https://api.finra.org/data/group/otcMarket/name/consolidatedShortInterest`, `Accept:
  application/json`, body `{"limit":N,"compareFilters":[{"compareType":"EQUAL","fieldName":"symbolCode","fieldValue":"<T>"}]}`.
  Returns the famous fields: `daysToCoverQuantity`, `currentShortPositionQuantity`,
  `previousShortPositionQuantity`, `changePercent`, `averageDailyVolumeQuantity`, `settlementDate`,
  `accountingYearMonthNumber`. **Keyless** (no OAuth). Caveat: `sortFields` needs the partition key
  `settlementDate` as an EQUAL filter → just fetch the symbol's rows and sort client-side by
  `accountingYearMonthNumber` desc to get the latest. (Monthly bulk dir is 403 — not needed.)

Build plan (next ticks): ⬜ ② `internal/finra` client (pure parser for the pipe file + SI JSON +
unit tests) → ⬜ ③ ingest wiring (`ShortVolumeCache` daily whole-file map; per-symbol SI fetch with
TTL) → ⬜ ④ `GET /v1/stocks/{t}/short` (short_volume_pct, days_to_cover, SI change; display-only) →
⬜ ⑤ "Short pressure" card on the stock page near Fundamentals/PulseBar + i18n + "Source: FINRA".

### Backlog (owner-approved, in `/loop` order)
- ✅ ① CI.  ✅ ② Opportunity seen-set persistence (was already built+live — `seen_form4`,
  verified `loaded ... count=3362` on restart; corrected stale note).  ✅ ③ Bollinger
  Bands (toggle).  ⬜ ④ K-line >3yr lazy history (`?before=`).  ⬜ ⑤ Notes/comments
  enhancements (Markdown/edit/like).  ⬜ ⑥ Watchlist grouping/sorting.  ⬜ ⑦ Brazil B3
  market.  ⬜ ⑧ Error monitoring/metrics.

---
_Working agreement: each `/loop` iteration picks the next unchecked item(s),
implements rigorously (Google style, OSS reuse, parallel subagents where safe),
verifies (build/vet/lint), updates this file + `CLAUDE.md`, and commits._
> **🔧收官后微调(owner 2026-06-12)**:(1) **晨报折叠靠前**(owner 选)——BriefingCard 默认 collapse 到 max-h-40 + 底部渐隐 + 「展开全文/Show more」toggle(useRef 量 scrollHeight>172 才显 toggle;summary-first 高曝光但不占高度)。(2) **Smart Money 标签换位**——institutional 内容少→把「大佬持仓 13F」提到第一并设为默认 landing(tabs 序 13f|congress|institutional;page.tsx 默认 '13f');机构举牌移到最后。(3) **英文导航不换行**——「What's new」加 whitespace-nowrap(不再折行)+ 搜索框 lg 由 w-56 收窄到 w-44(腾~48px,中文 lg 仍带内联搜索不受损)。预览验证:折叠态渐隐+toggle 展开/收起、tab 序 Whale 13F 首+默认、英文导航 1080px 单行不溢出(bar 1070/1070)。纯前端,web 绿。

## 🛠️ v6 — ops · UX · growth (owner 2026-06-12, 90s /loop)
> Order: **① 运维护栏(监控+备份) → ② 体验补全(13D/G可点·移动端指数条·自选排序) → ③ 拉用户 SEO进阶(双语 hreflang + pSEO 落地页)**。每 tick 一可验证增量,90s 节奏。
> **✅v6①a healthz 就绪探针(本 commit,纯 Go)**:/healthz 原来恒返回 {ok}(DB 挂了也 200)。改为真正的 readiness——store 加 `Ping(ctx)`(memory→nil / postgres→pool.Ping / split→Market+User 都 ping),health handler 2s 超时 ping DB,**DB 不通返回 503 + {status:degraded,db:down}**(外部 uptime 监控才能抓到故障),并附子系统状态(db/llm/prices/options/13f)。Go 全绿;部署后 curl /healthz 验证新结构。
> **◐v6①b DB 备份(VPS,部署待验证)**:`/tmp/tw-backup-setup.sh`→ 装 `/root/backup.sh`(`docker exec tickwind-postgres-1 pg_dump -U tickwind tickwind | gzip` → `/root/backups/user-TS.sql.gz`,轮转保留最新 7 份)+ 04:30 daily cron + 即跑一次。SETUP_LAUNCHED 成功但 SSH 在验证步掉线(链路节流)→ 下个 tick 单次 spaced SSH 验 `ls /root/backups` + `crontab -l | grep backup.sh`。market 库=Supabase 托管已备份,只备本地 user 库(watchlist/notes/holdings/clips/alerts)。
> **✅v6②a 移动端指数条 2×2(本 commit,纯前端)**:IndicesStrip 4 格(含 ^HSI 恒生)在移动端长标题"Hang Seng"被截成"Hang Se…"。原 hardcode 单行 + 每格 `border-l` 左分隔(靠 `first` prop)。改为 **gap-px 发丝分隔技法**:容器 `grid gap-px` + 分隔色背板(slate-200/800),每格自绘 `t.card` 背景→背板只透过 1px 缝隙=任意布局(2×2 或 1×N)都正确分隔,无需 per-cell 边框/序号逻辑(删 `first` prop)。4 格 colsClass `grid-cols-2 sm:grid-cols-4`(移动 2×2、桌面 1×4),3-ETF 兜底维持 `grid-cols-3` 单行。预览验证:mobile 375px=2×2(170px×2,"Hang Seng" scrollW==clientW **不截断**)、desktop 1180px=1×4(274px×4 单行)、gap=1px 发丝缝。web build/lint 全绿。**剩余②**:(b) 自选股排序、(c) 13D/G 公司名→/stock 链接(改 Go)。
> **✅v6②b 股票条排序(本 commit,纯前端)**:Board(自选)+ HomeHub(首页 Markets 条)加排序段控——**Default/默认**(自然序:自选=添加序、首页=热门序)· **Change/涨跌幅**(当日涨跌降序,涨幅在前、无报价排最后)· **A–Z/字母**(按代码)。抽出共享 `components/SortControl.tsx`(`SortKey` + `changePct` + `sortSecurities` + `SortPills`),`changePct` **镜像 StockCard 的常规时段涨跌**(regular_close/priorClose + 扩展时段 `closes[-2]` 护栏),故 Change 排序与卡片显示的 % 完全一致。Board 用 `useMemo`(quotes/barsMap 为 deps→报价跳动时实时重排);HomeHub 同款工具条置于 Markets 条上方(`cards.length>=2` 才显示;自选默认标签=添加序、首页=默认)。i18n 加 `board.sort{Added,Default,Change,Alpha}` 四键(中/英)。预览验证(首页 `/`,公开,无需登录):Default=AAPL/NVDA/TSLA…(热门序)、A–Z=AAPL/AMD/AMZN/AVGO…(严格字母)、Change=AMD +5.66 / GOOGL +0.87 / TSLA +0.66 … AMZN −2.17(严格降序,与卡片 % 吻合);段控 teal 高亮、ArrowUpDown 图标。web build/lint 全绿。**剩余②**:(c) 13D/G 公司名→/stock 链接(改 Go,大改:Symbol+CIK 字段 / sec.go 解析 / ByCIK 索引 / api 暴露 / 前端链接)。
> **✅v6③a 双语 hreflang 基础(本 commit,纯前端)**:单 URL 客户端切换做不了有效 hreflang→改 **URL 级 i18n**。(1) `langNoFlashScript` 支持 `?lang=zh|en`(URL 参数**优先**且回写 localStorage→分享链接/爬虫命中即出对应语言;无参回退已存偏好)。(2) `config.langAlternates(path)` 助手→关键页 `metadata.alternates` 出 `canonical` + `hreflang en(?lang=en) / zh-CN(?lang=zh) / x-default`(已接 **首页 / smart-money / opportunities / unusual**;smart-money、unusual 原 canonical 升级、opportunities、首页新增)。(3) `sitemap.ts` 每条 entry 加 `xhtml:link` en/zh-CN 双语(`xmlns:xhtml` 已声明)。验证:`curl /smart-money` head 出 4 条 link(canonical+en+zh-CN+x-default);`/sitemap.xml` 每 url 出双语 xhtml:link;预览 `/?lang=zh`→`<html lang=zh>` + 存储 zh + h1"今日市场" + 导航中文;`/?lang=en`(存储为 zh 时)→强制 en(URL 参数胜出并回写)。web build/lint 全绿。**剩余③**:(b) pSEO 中文关键词主题落地页(最后一项,清完即停 loop 总结)。
> **✅v6②c 13D/G 公司名可点(本 commit,Go+前端)+ ✅①b 备份真正装好(VPS 运维)**:
> **②c**:举牌榜(13D/G)公司名→`/stock/{ticker}` 链接。`internal/symbols`:`Symbol` 加 `CIK`、`Index` 加 `byCIK` map + `ByCIK(cik)` 方法(nil/0 安全)、`sec.go` 解析 company_tickers 的 `cik` 列(`cellInt` 解 JSON float64;列缺失用 -1 守卫不误指 col0)、`Cache` 透传 `ByCIK`;`internal/sec` `OwnershipRef` 加 `Ticker` 字段;api `SymbolSearcher` 接口加 `ByCIK`(唯一实现 `*symbols.Cache`,无测试 fake→不破测试),`getInstitutional` 把每条 filing 的 CIK→ticker 解析后填 `Ticker`(**拷贝到新 slice,不污染共享缓存快照**);前端 `InstitutionalBoard` 公司名有 ticker 时渲染 teal `<Link>`(hover 下划线)、无则原灰 span;`api.ts` `InstitutionalFiling` 加 `ticker?`。加 `TestByCIK`(命中/未命中/CIK=0/nil)。Go build/vet/gofmt/test + web build/lint 全绿。**部署后** curl `/v1/institutional` 看 `filings[].ticker` 出值验证。
> **①b**:上 tick slim SSH 查出备份**根本没装**(B=0 C=0;早先 SETUP_LAUNCHED 是假阳性——`cat|ssh` stdin 传输被 RackNerd >1-2s 持连掉线,脚本没落地)。**根因+修复**:(1) 持连必掉→改 **base64 嵌命令**法(脚本编码进 ssh 命令体、非 stdin 流→秒回成功),装上 `/root/backup.sh`(**自动探测 postgres 容器名 + 用容器 env 取库凭据**,健壮);实测产出真备份 `user-20260612-183149.sql.gz`(B=1)。(2) cron 没装(C=0)是 setup 的 `set -e`+`grep -v`(空输入返 1)在子壳提前中止、`crontab -` 收空输入→清空;本 tick 单条快命令 `(crontab -l|grep -v…||true; echo…)|crontab -` 补装 + 拉起 cron 服务,验得 **CRON_SET=1 CRON=up**。→ **①b 完成**:每日 04:30 dump 本地 user 库 + 保留最新 7 份 + 服务在跑。**教训**:RackNerd SSH 单条秒回可靠,**持连/stdin 流必掉**→传文件用 base64 嵌命令、长操作 nohup 后台。**剩余②**:无(②全清)。
> **✅v6②c 部署验证**:`/v1/institutional` 公网出 `GNK | GENCO SHIPPING & TRADING LTD | SC 13D/A`——CIK→ticker 生产环境解析成功,公司名前端已可点进 /stock/GNK。
> **✅v6③b pSEO 中文关键词落地页(本 commit,纯前端)= ③全清**:新增 `/guide/[slug]` 主题落地页(SSG)+ `/guide` 汇总页,5 篇覆盖核心关键词簇:国会山股神(→/smart-money)、美股内部人买入(→/opportunities)、美股期权异动(→/unusual)、13F 大佬持仓(→/smart-money)、美股轧空雷达(→/hot)。`lib/guides.ts` 数据(slug/标题/keywords/双语正文/CTA/FAQ/相关链接);每页 SSR 双语([data-i18n] CSS 切换)+ 英文默认 tab 标题(LocalizedTitle)+ 中文 desc/keywords + `langAlternates` hreflang + **FAQPage + BreadcrumbList JSON-LD** + CTA 进实时看板 + 交叉链接 + 公开数据免责声明。sitemap 加 /guide + 5 篇;首页目录加"新手指南"内链。验证:curl 出 canonical+en/zh-CN/x-default hreflang + 正文 + FAQPage;预览 `/guide/congress-stock-tracker?lang=zh` 出完整中文页。web build/lint 全绿(/guide SSG 5 slug 预渲染)。
>
> **🎉 v6 三大块全部完成,loop 停**:**①运维**(healthz 就绪探针 + DB 每日备份)· **②体验**(移动端指数条 2×2 + 股票条排序 Default/Change/A–Z + 13D/G 公司名可点)· **③拉用户**(双语 hreflang URL 级 i18n + 5 篇 pSEO 中文落地页)。①c 错误可观测(可选)未做,留作下次。

## 🚀 v7 — 增长功能(owner 2026-06-13,并发多 subagent,战略见 docs/research/2026-06-13-growth-strategy.md)
> 解锁:TG token + dataimpulse 住宅代理已入 VPS .env(TELEGRAM_BOT_TOKEN/RESIDENTIAL_PROXY_URL,**不进 git**);Google OAuth 已 Supabase 配置 + 前端 flag 默认开。纪律:新功能优先并入已有页面不新建页;单语种默认英文、同功能内不混中英、用 i18n key;秘钥只进 .env;绿区数据(SEC/FINRA/Cboe/国会/预测市场)做付费铺路、行情不转售。
> **✅v7 第0波 图卡引擎(乘法器,纯前端)**:`/api/og/[kind]` next/og 渲染 1200×630 品牌卡;攻克 satori CJK(Google Fonts 动态子集 + 强制 TrueType,失败回退不 500)。动态 OG 接入 layout(默认)+首页+smart-money+opportunities+unusual+每个 guide 页(页面特定中文 eyebrow/title)。预览实测中文完美渲染。
> **✅v7 Google OAuth 上线**:provider 已 Supabase 配置,前端 flag 默认开(NEXT_PUBLIC_GOOGLE_OAUTH!=='0'),/login 出 "Continue with Google" 按钮。
> **✅v7 wave0 三新数据信号(并行 2 agent:后端接线 + 前端并入,全绿,已部署验证)**:
>   - **finrashvol** FINRA 日度做空量:client(ErrNoData 回退)+ Cache(latest map + 滚动历史 + Top 榜);ingest goroutine(收盘后拉、跳周末回退);GET /v1/short-volume(Top 榜,minTotalVolume=1M 过滤,FINRA display-only 不暴露全量);getShort 加 daily 字段(今日做空% + 历史)。**修了一个 bug**:2026 FINRA 文件做空量是小数(380098.039916),原 int 解析丢了 ~全部行(只剩 2 只)→改 ParseFloat+round(commit 63d4a97),公网验证 /v1/short-volume 出 FTGC 98.2%/FTMU 97.7%… 数千行正常。前端:ShortChip 加"今日做空%"+迷你曲线、/hot 加 "Short volume" tab(并入不新建页)。
>   - **sentiment** 潮汐恐贪指数:纯计算(VIX/PCR/宽度/动量/Heat/Short% → 0-100 + label)+ atomic Cache 历史;ingest 每日 Compute(已接 VIX=yahoo ^VIX / PCR=cboe SPY 链 / Short%=finrashvol 均值,宽度/新高新低/Heat 留 TODO,缺成分自动等权);GET /v1/sentiment。前端:首页 HomeHub IndicesStrip 下 SentimentChip 仪表(并入不新建页)。
>   - **ratecut** 降息概率:Kalshi + Polymarket keyless client → 统一 Market/Outcome + 聚合 Cache;ingest 20min;GET /v1/ratecut。前端:/events 加 RateCutOdds section(并入不新建页)。
> **接线手法**:api.New 改返回 *Server(实现 ServeHTTP)+ setter 注入(SetShortVolume/SetSentiment/SetRateCut),**避免给 New 加位置参数 + 改测试调用点**。验证:/v1/short-volume(FTGC 98.2%…)、/v1/sentiment(score 44 Fear,VIX/PCR/Short 三成分)、/v1/ratecut(Kalshi Fed 决议盘口)公网全出数;预览 首页恐贪 chip + /hot 做空 tab 渲染正确。
> **v7 follow-up / 待办**:① **sentiment Short% 成分校准**:日度做空量占总量常态~45%(并非空头仓位),直接喂 [10,50]→fear 映射会让指数长期偏 Fear——应改用相对自身基线的偏离、或从指数移除该成分(精度问题,owner 重精度)。② sentiment 补 宽度/新高新低/Heat 成分。③ ratecut 阈值盘口("Above X%")展示可更直观(转成"降息N次概率")。
> **🔄 第1波尖刀 佩洛西集群(进行中)**:PTR PDF 解析 research+prototype subagent 后台跑(硬骨头:数字版 PTR 抽 ticker/方向/金额,pdftotext vs Go 库选型)→ 议员页 pSEO + 个股 chip + 提醒 + 13F 扩白名单 + 回测。后续波次:TG 推送 / IPO(走代理)/ 中概专区 / A股词典 / 个性化晨报 / AI 深度报告。
> **✅v7 第1波 佩洛西集群·后端管线(本 commit + 部署验证)**:PTR PDF 解析(internal/congress/ptr,`pdftotext -layout` 经 os/exec,纯 stdlib 无新依赖)接进 congress ingestor。**Dockerfile 运行级 distroless/static→debian:12-slim + ca-certificates/tzdata/poppler-utils**(部署命令补 `cp Dockerfile`)。congress.Cache 加 byTicker/byMember 索引(MemberTx/TickerTrade/Slugify);Client.FetchPDF;ingestor 增量解析(seen-set 不重抓、节流 400ms/PDF·每轮≤60、DocID len≥8 数字版筛选、扫描版 ErrScanned 跳过、pdftotext 缺失优雅降级只存 filings)。API:GET /v1/stocks/{t}/congress + /v1/congress/member/{slug}(setter,不动 New)。**部署硬验证**:换基镜像后 HTTPS 摄取全好(/healthz ok、short-volume/indices count=3/4、13F Berkshire、sentiment 出数);**PTR 解析生产出真数据**——Mike Kelly→BMY/CMCSA、Chip Roy→AESI×3、反查 CMCSA→Kelly+Cisneros、买卖方向正确。覆盖率~85-90%(扫描版~13% 兜底 PDF 链接)。
> **🔄 佩洛西集群·前端(agent 进行中)**:议员详情页(SSR /congress/member/[slug],pSEO"佩洛西持仓",hreflang+OG卡+sitemap)+ 个股"议员近期买卖"chip(并入)+ smart-money 议员名内链。**后续**:13F 扩白名单(段永平/高瓴/李录)+ 个股"哪些大佬持有"反查 + 跟单回测 + 议员交易提醒。
> **✅v7 佩洛西集群·前端(本 commit,前端)= 尖刀上线**:议员详情页 `/congress/member/[slug]`(SSR+ISR,pSEO"佩洛西持仓",英文默认标题+LocalizedTitle zh+hreflang+OG卡+BreadcrumbList+45天滞后免责;交易表:日期/买绿卖红/资产+ticker链接/金额区间;未知 slug→404)+ 个股页 CongressChip(并入,"议员近期买卖"→议员页)+ smart-money 国会名内链 + sitemap +68 议员页(204 双语条目)。**坑修**:生产 API 是 snake_case(非契约写的 PascalCase),agent 实测后改对。预览实测 mike-kelly 出 BMY/CMCSA、AESI chip 出 Chip Roy×3。lint 0 error/build 绿。
> **✅v7 13F 反查 + 基金页(本 commit + 部署验证)= smart-money 集群完成**:thirteenf.Cache 加 byTicker/bySlug 索引(原子重建);ThirteenFSource 扩 Holders/Fund(不动 New);GET /v1/stocks/{t}/whales(生产验证 AAPL→Berkshire 22%+Himalaya)+ /v1/13f/{slug}。前端:个股 WhalesChip(并入)+ /fund/[slug] pSEO 页(实测 /fund/berkshire 出 Buffett 29 持仓、权重、QoQ Trim/Add%、ticker 链接)+ ThirteenFBoard 内链 + sitemap。基金 slug 用白名单现成 Slug 字段(无 slugify 脆弱性)。换基镜像后 HTTPS 摄取仍正常。**→ 佩洛西议员 + 13F 大佬 双向(个股 chip + pSEO 详情页)全通**。
> **🔄 佩洛西集群收尾·跟单回测(agent 进行中)**:议员披露交易模拟跟单收益 vs SPY(用已有日线历史),议员页"跟单模拟"段 + 分享卡。
> **✅v7 跟单回测(本 commit + 部署验证)= 佩洛西集群尖刀完成**:internal/congressbt 纯函数回测(注入 CloseFn+now,无网络/时钟,8 例表驱动)——每笔 purchase 披露日收盘等权买入、同票 sale 平仓锁定、否则持有至今 mark-to-market,无价历史 ticker 跳过计 coverage,基准 SPY buy-and-hold;窗口最早买入→今。API GET /v1/congress/member/{slug}/backtest(永不 500,数据不足→insufficient+coverage,per-slug 日缓存)。前端 FollowTradeSim(议员页:大字 +X% vs SPY +Y% 跑赢绿/跑输红 + 双线净值 SVG + 始终可见方法 + 琥珀醒目"模拟复盘非真实收益非投资建议" + 覆盖率;无"跟单买入"按钮)。sanity:chip-roy +26.71% vs SPY +10.29%、gottheimer +9.83% vs +3.87%、纯债券议员正确 insufficient。
> **✅v7 internal/telegram 客户端(本 commit,scaffold)**:SendMessage/SendPhoto(后者发 OG 图卡 URL)、HTML/无预览 Option、EscapeHTML、429 RateLimitError 退避、空 token no-op;11 测试全绿。TG token(新)+ TELEGRAM_CHANNEL=@tickwind 已在 VPS .env(bot 管理员 can_post 验证)。**下一步**:接 broadcaster(main+config+ingest)每日把晨报+图卡播 @tickwind。
> **✅v7 价格小数位修复(已部署 Vercel)**:fmtPrice 分价位精度(priceDecimals:<$1→4 位、<$10→3 位、≥$10→2 位)——RZLV 盘后 2.7287 由"2.73"→"2.729" 追平富途。owner 报 RZLV vs 富途对不上:盘后价=纯小数位(已修);涨幅 6.34 vs 5.93=Alpaca 日线 vs 富途半分钱数据源差(原始值算 %,非 bug,换持牌源才齐)。
> **✅v7 TG 播报上线(本 commit + 部署验证)= owner 解锁全部落地**:internal/ingest/telegram_broadcast.go BriefingBroadcaster——每 ET 日把中文 AI 晨报 + OG 图卡播到 **@tickwind**(date 去重、启动查+30min tick、SendPhoto 卡+HTML caption 失败 fallback SendMessage、429 退避、token 空 no-op);config 加 TelegramBotToken/TelegramChannel/PublicSiteURL;main 在 LLM/briefing 块内接线。owner 建好频道 @tickwind + bot 管理员(can_post 验证)+ 换新 token,均已进 VPS .env(不进 git)。**验证**:频道 intro 帖 msg_id=2 落地(token+频道+发帖端到端通);部署后 healthz + 全端点(backtest/whales/short-volume/sentiment)200 无回归;当前 06-13 晨报未生成,broadcaster 正确等待,生成后自动播。
> **✅v7 IPO 日历(本 commit + 部署验证)= 住宅代理首个功能落地**:Nasdaq IPO API 数据中心 IP 被挡→经 dataimpulse 住宅代理 + 完整浏览器头(UA/Accept/Origin/Referer)实测出数。config 加 ResidentialProxyURL + ProxyHTTPClient(透明降级);internal/nasdaq client;internal/ingest/ipo.go(4h,atomic cache,失败保留上次);GET /v1/ipo(setter,nil-safe)。前端 /ipo 页(Upcoming/Recently priced/Filed 分组,ticker→/stock,Nasdaq 来源+延迟免责)+ TopNav More 入口 + sitemap + i18n。**部署验证**:VPS→代理→Nasdaq 通,/v1/ipo priced=20/upcoming=1/filed=18(FRBT $18/EROC $600M…),/ipo 页渲染正确。代理凭据只 env 不进 git。
>
> **🎉 v7 阶段性里程碑(本会话)**:三个 owner 解锁全部落地见效 [Google OAuth · TG 播报@tickwind · 住宅代理→IPO] + 图卡引擎 + wave0(做空日更/恐贪/降息) + **smart-money 全集群**(佩洛西议员页/交易表/个股 congress chip + 13F 基金页/whale 反查 + 跟单回测) + 价格小数位修复。**剩余波次**:中概退市专区 · A股词典 pSEO · 个性化晨报/持仓体检 · AI 深度报告 · 议员交易提醒 · 13F 扩白名单 · OG 分享卡 kind。**遗留修**:backtest obscure 票价格覆盖 · sentiment Short% 校准 · ratecut 阈值盘口展示 · LocalizedTitle ?lang=zh tab 标题。
> **✅v7 IA 重构(owner 要求,本 commit,纯前端)= 页面合并/排版收口**:24 页→收口,导航 ~17 项→~9 项。① **/calendar** 共享标签外壳 + 三子路径 /calendar/earnings|macro|ipo(各保 SSR metadata+langAlternates,合并旧 earnings/events/ipo)。② **/me 个人中心**(客户端标签 ?tab=watchlist|holdings|notes|alerts,登录门,合并旧 watchlist/portfolio/notes/alerts;PortfolioView 抽出、notes 日历/alerts 增删交互完整搬)。③ **/discussion** 加标签[Discussion·Community](并 community)。TopNav:secondary 去 IPO/Earnings/Events/Community→加 Calendar+Discussion;去 watchlist pill+authed Notes/Portfolio/Alerts→authed My pill(/me);AccountMenu/AlertsBell/Footer/home 目录同步。next.config 8 条 permanent 308 重定向(旧 URL→新,curl 验证)。sitemap 更新(/calendar/* 带 hreflang,去 community/旧日历)。i18n nav.calendar/discussion/my + cal/me/disc.* en+zh。删 8 个旧页目录。预览验证:/calendar 三标签 + 财报分日、/me 登录门、/discussion 两标签;lint 0 error/build 绿/旧八页消失。

## 💎 v8 — 两大重点收费路线(owner 2026-06-13)
> owner 定为重点、后续可收费。数据集 `data/indicators/`(414 指标,SPEC.md + indicators.json/csv/yaml + 原始 build prompt;owner 提供)。注:prompt 假设 Python/FastAPI,**我们用 Go + Next.js 既有栈适配**。
### R1 · 超丰富指标库引擎(Glassnode/LookNode 式,dataset-driven)
- **数据集=单一真相源**:414 条(domain: onchain132/fundamental99/sentiment98/technical85;priority P0=37/P1=61/P2=316;applies_to stock184/crypto132/both98 → **stock-applicable=282**)。每条含 id/domain/subcategory/priority/applies_to/name_en/name_zh/abbr/definition/formula/inputs/default_params/talib_or_lib/output_type/data_source/interpretation。**代码从 json 生成 catalog/registry/UI,不手维护平行清单;绝不臆造/改公式,不能忠实实现就标 unsupported+原因**。
- **Go 栈适配**:后端 `internal/indicators`——从 json seed catalog + registry(id→compute:technical 用 BarCache OHLCV、fundamental 用 SEC XBRL、sentiment/macro 用现有源)。API:GET /v1/indicators(catalog,按 domain/priority/applies_to/asset 过滤)+ GET /v1/stocks/{t}/indicators(某票指标最新值+历史序列)。前端:可搜索指标目录页 + 按 output_type 渲染(overlay 叠K线/oscillator 子盘/value 卡)+ 个股"指标"区。复用现有 web/lib/indicators.ts(技术)、XBRL fundamentals、sentiment 成分。
- **范围**:stock-applicable(282)优先;crypto/onchain(132,需 crypto OHLCV+Glassnode 新基建)=后续可选扩展(数据集已支持)。
- **分阶段**:P0(28 stock,多已算:RSI/MACD/BOLL/ATR/EMA/SMA/KDJ/VWAP/Vol + PE-TTM/PB/ROE/毛利/净利/营收增长/FCF/股息率/资产负债率 + VIX)→ P1 → P2;每阶段 registry-coverage 测试。
- **收费铺路**:免费=P0 核心+目录浏览;付费=P1/P2 全量+历史深度+多指标叠加+导出(对齐 Koyfin/Glassnode 历史深度分层)。绿区数据(SEC/FINRA/Cboe/价格延迟展示)。
### R2 · AI 深度分析报告/研报(可收费,依赖 R1)
- AI 速览升级为长篇中文研报:综合 指标库(R1)+ SEC 财报/filings + smart-money(国会/13F/内部人/做空)+ 期权情绪 → 结构化研报,每论断挂数据源链接(可溯源,对齐 Fiscal.ai);数字从结构化数据注入、LLM 只写定性(防幻觉)。
- **收费**:每日限量免费 / 付费全量+历史+PDF(对齐 Seeking Alpha/Motley Fool/Zacks 最高客单品类)。绿区数据 + 自产 LLM 内容,不踩行情转售红线。
> **打法**:Phase 0 dataset 入库(已 cp data/indicators/)+ scaffold(catalog seed/API/目录页)→ Phase 1 P0 vertical(28 stock 指标接进 dataset-driven 框架,多数已算)→ P1/P2 扩。R2 待 R1 有指标输出后做。其余路线(港股/雪球、议员提醒、13F扩白名单等)降到重点路线之下。
>
> **✅v8 R1 Phase 0 指标库目录(本 commit + 部署验证)= dataset-driven catalog 上线**:`internal/indicators`——`go:embed indicators.json`(414 全集→按 `applies_to∈{stock,both}` 过滤到 **282 美股可用**:184 stock + 98 both,排除 crypto-only 132);`Catalog.All/Filter(domain/priority/subcategory/q)/Facets`;**公式原样透传不杜撰**(守则:不发明/改公式,实现不了标 unsupported+原因)。facets:domain[fundamental 99 / sentiment 98 / technical 85]、priority[P0 28 / P1 44 / P2 210]。API:`GET /v1/indicators`(filters + facets,IndicatorSource setter 不动 New,nil-safe);main `indicators.Load()` 接线。前端:`/indicators` SSR/ISR 目录页(IndicatorLibrary——搜索 + domain/priority facet 过滤 + domain→subcategory 分组 + P0 高亮 + 公式 monospace;API 慢/挂降级空目录不 500)+ TopNav 入口 + sitemap + i18n(英文默认标题 `Stock Indicator Library`、zh `美股指标大全` + OG 卡)。**无 paywall**(monetization deferred)。go build/vet/gofmt/test 绿;web lint 0/build 绿(/indicators 预渲染)。Phase 1(P0 per-stock compute,registry id→compute,GET /v1/stocks/{t}/indicators)待接。
>
> **✅v8 R1 Phase 1 个股指标计算引擎(本 commit + 部署验证,多 subagent 并发工作流)= P0 per-stock compute 上线**:28 个 P0 stock-applicable id 全量接进 dataset-driven registry——**19 计算 + 2 大盘context + 7 crypto 标 unsupported**(诚实:不为 equities 杜撰 crypto 数据)。`internal/indicators/{technical,fundamental,compute}.go` + tests:**纯函数数学**镜像 `web/lib/indicators.ts`(SMA-seeded EMA、Wilder RSI、population-σ Bollinger、StockCharts MACD、Wilder ATR、国际 KDJ、VWAP)+ 10 基本面比率(PE/PB/ROE/净利率/毛利率/营收·盈利 YoY/FCF/股息率/资产负债率);registry `id→compute`,Computer 注入窄接口(OHLCV/Fundamentals/Price/MarketContext 源),`Compute` 每 ticker 取数一次评估全部 P0,**registry-coverage 测试**确保无 P0 id 静默漏实现。**edgar XBRL 扩展**(`internal/edgar/fundamentals.go` +8 字段:GrossProfit/TotalAssets/TotalLiabilities/OperatingCashFlow/CapEx/DividendsPaid + 上年 Revenue/NetIncome,年度 duration 过滤 + 同概念上年配对 + CapEx/分红 abs)。API:`GET /v1/stocks/{t}/indicators`(IndicatorComputeSource setter 不动 New,graceful 200 + 仅全空才 404)。前端:个股页 `IndicatorsPanel`(并入 StockView K线区,按 domain 分组、ok 值 + 解读 + 链回 /indicators 目录、market-context 条 VIX/恐贪、unsupported 隐藏、404 优雅隐藏)+ i18n。**精不在多守则落地**:VWAP 仅日线无盘中→标 insufficient 不发误导值;EMA headline 用 catalog default_params 主周期 12;PE 标注年度非严格 TTM;ROE 用期末非均权益(均文档化)。对抗式公式复核(独立 agent)逐一核对 28 id 对齐 dataset formula + TA-Lib 标准,无杜撰无静默错值。go build/vet/gofmt/test 全绿;web lint 0/build 绿。**收费铺路**:免费=P0 全量;P1/P2 全量 + 历史深度 + 多指标叠加 + 导出留作付费(后续)。
>
> **✅v8 R2 P0 AI 深度研报 vertical(本 commit + 部署验证,多 subagent 并发工作流)= 反幻觉研报上线**:设计文档 `docs/research/2026-06-13-r2-ai-research-design.md`(理解→设计工作流产出)。**核心反幻觉契约(对抗式复核判定 UNBREAKABLE)**:`internal/research` 包——`Assemble(ctx,ticker,Sources)` 纯函数(无 LLM,不出错)产出 typed `FactSheet`(Fact{Value/Raw/Unit/Status/Reason/Source/SourceURL/AsOf}、SectionFacts{Prose}、Citation),**每个数字都在 Go 从结构化源算出**(R1 indicators + edgar fundamentals + quote),LLM 只填 `SectionFacts.Prose`、喂给它的 material 只有格式化字符串不含 Raw、乱填数字键被忽略(`TestComposeNeverMutatesNumbers` byte-identical 断言证明)。`enrich.ComposeReport(material,lang)→map[string]string`(json_object、分节键 zh/en prompt 禁数字/建议、Noop→ErrDisabled);`Compose` 禁用/出错→返 data-only 不报错。API `GET /v1/stocks/{t}/research`(SetResearch setter 不动 New,clone getSummary 缓存+single-flight,researchDailyCap=80 **只限 prose 不限数据**、退款逻辑、LLM 关→200 data-only 不 503)。前端 `ResearchReport.tsx` 公开 Research tab(估值/基本面/技术面 3 节、facts 网格 + 数据不足 muted chip + reason tooltip、`<Markdown>` prose、citations、"AI 生成·数字来自公开数据·非投资建议"、禁用态 data-only)。**修对抗式复核 must-fix**:无 XBRL 的 ETF/ADR/外国票(有价无基本面)P/E insufficient 原误显"亏损"→改按 reason 区分(含 EPS/loss 才"亏损"否则"数据不足"),加回归测试。go build/vet/gofmt/test 全绿(含反幻觉测试套件);web lint 0/build 绿。**收费铺路 DEFERRED**:免费=每日限量(cap 计数器已建,无 paywall/收银台)。**后续**:F1 资金面(国会/13F/内部人/期权/做空)+ 情绪面节;F2 概览结论节;F3 引用深链锚点;F4(付费侧)FactSheet 持久化历史 + PDF。
>
> **✅v8 R2 F1 研报差异化两节(本 commit + 部署验证,多 subagent 工作流)= 资金面 + 情绪面上线,研报达 5 节**:`internal/research` 加 `flows.go`(资金面)+ `sentiment.go`(情绪面)+ Sources 加 6 个 nil-safe provider 接口(Congress/ThirteenF/Options/ShortVol/ShortInt/Market + StoreReader)。**资金面**:国会议员买卖(distinct 人数 + 最新议员/方向/金额区间**逐字不合成点值** + 日期;空→不发"无人交易"假断言,说"未发现披露交易"/省略节;链 /congress/member)、13F 大佬(top holders 权重%/增减标 + **Period 必显~45d滞后**、链 /fund)、内部人买入(90d 窗口过滤本票→distinct buyers/总额/均价/最新日)、期权(沽购比量/持仓 as-computed + 最大痛点 + top OI,"Cboe·延迟15分",无期权→省略)、做空(FINRA 衍生 ShortPct + 趋势 + 结算 days-to-cover/变动,**只衍生不批量原始行**)。**情绪面**:大盘恐贪(**仅 Available>0 才注入**,中性50兜底不当真) + 个股 buzz(mentions vs prev/rank) + 新闻情绪(signed score/label/样本) + 热度榜在位 + 新闻/社区头条作**归因 Context**("据新闻/据社区讨论",非数字 Fact、不杜撰情绪数)。compose material 加新节 + 归因标注;enrich prompt 加 flows/sentiment 节 + 归因/无数字守则。前端**零改**(ResearchReport 数据驱动渲染任意节,新 citation 类型自动成链)。**性能修**(对抗式复核 medium):options 原在请求路径 live Cboe 拉取(~1-2MB 阻塞 assemble)→加 `OptionsCache.Cached` cache-only 读 + main cachedOptionsProvider 适配器(冷票省略期权不阻塞)。**对抗式反幻觉复核判定 SAFE**(新节数字全 Go 注入、LLM 只 prose、UGC 仅归因 context、stray keys 忽略;`TestComposeNeverMutatesNumbersFlowsSentiment` 证明)。go build/vet/gofmt/test 全绿;前端无改 build 绿。
>
> **✅v8 R2 F2 概览/结论节(本 commit + 部署验证,inline)= 研报达 6 节(总览置顶)**:`compose.go` 在 LLM 产出 prose 后,若有 `overview` 键则 prepend 一个**纯 prose 无 facts**的 overview 节(Key/TitleZH 概览/TitleEN Overview),渲染在最前;**data-only(LLM 关)无 overview**(总览是纯综合,无数据时无意义)。`enrich.go` composePrompt+EN 加 overview 指令:综合全部板块 3-5 句中文/英文均衡叙述(优势+风险),结尾"以上为基于公开数据的客观梳理,非投资建议",**同样禁编造数字/目标价/买卖建议**。前端零改(ResearchReport `facts.length>0` 才渲网格、prose 单独渲,纯 prose 节优雅显示;overview 在 sections[0] 自动置顶)。`TestComposeOverviewPrepended`(enabled→overview 置顶 prose-only、technical 仍得 prose;disabled→无 overview)。go build/vet/gofmt/test 全绿。**R2 研报 6 节完整**:概览/估值/基本面/技术面/资金面/情绪面。后续:F3 引用深链锚点。
>
> **✅v8 R2 F3 引用深链锚点(本 commit,纯前端 Vercel)= R2 研报路线收口**:研报每节"数据来源"citation 的 Anchor(后端 Go 已设 #fundamentals/#indicators/#options/#congress/#whales/#short/#signals 等)现可点击滚动到个股页对应卡——StockView 给 PulseBar/ShortChip/CongressChip/WhalesChip/FundamentalsCard/IndicatorsPanel/OptionsCard 各包 `<div id=... className="scroll-mt-20">`(scroll-mt 避开 sticky TopNav 遮挡);未匹配 anchor(#insiders/#hot/#sentiment 无专属卡)优雅 no-op。**预览实测**:点 #fundamentals citation→scrollY 0→3210、目标卡 top=80(scroll-mt-20 偏移正确)。web lint 0/build 绿。**R2 研报完整**:6 节(概览/估值/基本面/技术面/资金面/情绪面)+ 反幻觉(数字全 Go 注入 LLM 只 prose)+ LLM 关 data-only + 引用深链 + 全免责。遗留:后端 overview facts:[] 修(fc712a5)SSH 连掉未部署,前端 null-guard 已覆盖用户,随下次后端部署带出。
>
> **✅v8 质量修:恐贪指数 Short% 成分校准(本 commit,internal/sentiment 纯函数)**:`scoreShortPct` 原把 FINRA 日度做空**量**占比按 [10,50]→[80,10] 映射,但日度做空量结构性常态~45-50%(含做市商对冲,非方向性空头仓位),致正常 ~48% 被打成 ~13(极度恐惧),长期把指数拽向 Fear(生产实测:VIX74+PutCall46+Short13→44 Fear,而无该偏差应是 60 Greed)。改为以结构基线 ~48% 居中:[40,56]→[65,35](≈48%→50 中性),作"相对常态的偏离"而非"做空即恐惧";温和带宽(做空量是弱噪信号)。校准后生产输入应得 ~56 Greed(更贴合低 VIX/中性 put-call 的真实盘面)。测试同步更新(基线点 48→50、保留 clamp);build/vet/gofmt/test 全绿。**TODO**:更稳健做法=相对自身近期均值的偏离(需留存做空量历史)。

## 🌱 v8 增长(owner 2026-06-14:两大路线已成→拉新增长)
> owner 选定方向=拉新增长(SEO/pSEO 深化 + 传播,把已建强功能推给用户、为收费铺路)。
> **✅ 每指标 pSEO 落地页 /indicators/[id](本 commit,纯前端 Vercel,多 subagent 工作流)= 282 可索引双语词条页**:把 R1 指标数据集变成 Investopedia/Glassnode 式词条 SEO——SSG+ISR(generateStaticParams 全 282,API 挂则 []-降级 + ISR 按需),slug=id 的 `.`→`-`(indicatorSlug;indicatorBySlug 用 slugify-比较反查,对已含连字符的 id 如 fundamental.pe-ttm 鲁棒、282 条零碰撞验证)。页面只渲染数据集真字段(definition→formula→interpretation 兜底,151 条空 definition 无杜撰)+ 公式逐字 monospace + default_params + 解读 + domain/P0 徽章 + 面包屑 + 同子类相关指标内链(≤8)+ 返回/看个股 CTA。SEO:英文默认标题(LocalizedTitle 切 zh)+ 中文关键词 description/keywords(N是什么/计算公式/解读)+ langAlternates hreflang(canonical 用记录自身 slug)+ ogImageMeta 卡 + JSON-LD(DefinedTerm + DefinedTermSet + BreadcrumbList)。**内链乘数**:目录卡 + 个股 IndicatorsPanel 行 + 相关卡全部链到 /indicators/{slug};sitemap 加全 282 条带 hreflang。对抗式 SEO 复核判定 **solid**(无杜撰/canonical-hreflang slug 正确/零碰撞/内链全通/优雅降级),仅 3 项 low 非阻塞(已修 canonical 用 resolved slug)。lint 0/build 绿(282 页预渲染验证 technical-rsi/fundamental-pe-ttm 等)。
> **✅ 传播激活:分享卡按钮接入新表面(本 commit,纯前端)**:ShareCardButton(已存在,OG 卡引擎)接到两个新高价值表面——① 每指标 pSEO 页 /indicators/[id] 头部(eyebrow=domain、title=name_zh+abbr、subtitle=定义片段);② R2 研报 ResearchReport 头部(eyebrow=深度研报、title=公司名、subtitle=overview 片段或 price_label,data-only 降级)。让刚上线的指标词条页 + AI 研报可一键存图分享(激活社媒传播,配合 pSEO 搜索流量双轮)。卡片中文优先(OG CJK 引擎),不含杜撰数字。lint 0/build 绿。

## 🔄 v8 转化/留存(owner 2026-06-14:增长漏斗下一环)
> owner 选定=转化/留存(把 pSEO/分享带进来的流量转成活跃用户;守 data-first 无营销页)。
> **✅ pSEO→产品 激活漏斗(本 commit,纯前端)**:每指标词条页 /indicators/[id] 原"在个股页查看"CTA 只回链目录(弱),改为**具体激活漏斗**——热门美股 chips(POPULAR_TICKERS 前5)深链到 `/stock/{ticker}#indicators`(F3 锚点,落到该股个股页的实时指标面板看到本指标的计算值)+ "全部美股→"链 /screen。把 282 个搜索流量入口转成"看实时数据→探索产品"的激活路径(data-first:导向数据非营销)。配合个股页 IndicatorsPanel 已反向链回词条页=双向闭环。lint 0/build 绿。
> **✅ 注册价值传达(本 commit,纯前端)= 转化漏斗收口**:发现个股页对匿名用户**已有**"+加自选→/login"入口(转化入口在),但 /signup 页是 7 行裸表单(无价值传达,易弃单)。给 AuthForm signup 模式在副标题下加**真实免费功能清单**(✓自选股追踪 ✓每日 AI 中文晨报 ✓价格&财报提醒 ✓私人投资笔记;data-first 诚实功能非营销),副标题改"创建免费账户,即可解锁:"。降低注册弃单率(用户已到 /signup=高意向,告诉其解锁什么→提高完成率),不在数据页加任何打扰式 nudge。仅 signup 显示(login 不显)。i18n auth.perk* en+zh。lint 0/build 绿。**转化漏斗闭环**:pSEO 搜索流量→指标词条页→激活 chips 看个股实时数据→个股页"+自选"→/signup 价值传达→注册留存。

## 🎨 v8 UX 修(owner 2026-06-14 截图反馈)
> **✅①首页榜单行高对齐**:Boards 行(Hot/Opportunity/Guru-watch)默认 items-stretch 致 Hot/Opportunity 被 Guru 内容拉高、下方留空。改 `items-start`——各卡自然高度顶对齐(实测 Hot/Opp 225px、Guru 286px,不再被撑到 286 留白)。预览 3 列实测干净。
> **✅②分享卡语言+画质**:(a)语言:OG 卡引擎硬编码中文 chrome(badge"中文美股数据台"+footer tag)致英文 UI 下卡片仍中文。加 `lang` 参数贯穿 og.ts→/api/og 路由(badge/tag 随 lang:en"Data-first US stocks"/"Congress·13F·Options flow·Insider buys")→ShareCardButton 自动注入当前 UI lang;各 caller 卡文案改语言感知(SentimentChip[首页/被点名]、StockView'数据延迟'、FollowTradeSim 客户端跟 useLang;fund/indicator 服务端默认英文)。(b)画质:卡片 1200×630 设计经 transform scale(2) 渲染到 2400×1260(高清,小红书/高 DPI 转发更锐),ogImageMeta 同步 2400×1260。**预览实测渲染**:英文卡全英文 chrome+锐利、中文卡 CJK 正确,2x 无裁切。lint 0/build 绿。

## 🎛️ v8 个股可选指标(owner 2026-06-14:R1 落地为个股可用工具)
> owner 选 Phase A+B。诚实上限~138 可算(非282;sentiment 98=期权/链上/宽度/宏观源未接,技术 patterns/盘中 VWAP/IV、基本面 forecast/headcount/index-beta/长历史 不可算)。设计 docs/research/2026-06-14-selectable-indicators-design.md。
> **✅ Phase A(前端 picker)+ Phase B 增量1(后端扩算)本 commit**:**Phase B**:`internal/indicators` 加 `technical_more.go`(64 技术指标 closures,纯 OHLCV:WMA/DEMA/TEMA/HMA/KAMA/ALMA/VIDYA/KC/Donchian/Supertrend/SAR/DMI-ADX/Vortex/Aroon/CCI/Williams%R/TSI/TRIX/PPO/StochRSI/UO/Fisher/OBV/ADL/CMF/MFI/Klinger… + ~30 纯 helper)+ `fundamental_more.go`(26 Group-0 比率,无新 XBRL:市值/EPS/BVPS/SPS/PS/D-E/ROA/资产周转/OCF比/FCF转化/派息率/Tobin-Q/PE-LYR…)。`compute.go` 合并 4 子注册表(共 **109 closures**,disjoint 测试守)+ `computedIDs()` 改为**按 registry 迭代**(未实现 id 直接 absent 不出"未计算"行)。**对抗式复核判定 faithful 无 must-fix**:每个 Value 仅 setOK 真值、缺输入/零分母/不足→setInsufficient 无值,`TestComputedClosuresNeverPanicOrFabricate` 跑遍每个 closure 空输入断言不杜撰。**Phase A**:`IndicatorsPanel` 加选择模型(默认=payload 的 P0 集=今日面板无回归,随后端自动长大)+ `IndicatorPicker`(搜索/domain 筛/勾选增删/上下排序/重置/a11y)+ `indicatorSelection.ts`(localStorage `tickwind.indicators.v1`,server-prefs 可后接)+ i18n。前端实测 picker 工作。go build/vet/gofmt/test 全绿;web lint 0/build 绿。**后续增量**:Group1-4 新 XBRL 比率(→~128)、Group5 Altman-Z/Piotroski、登录云端 /v1/me/prefs 同步。

## 🛠️ v8 owner 反馈批(2026-06-14 · 4 项)
> **①审查指标正确性(2 个对抗式 subagent,owner 要求)= 双双判定 FAITHFUL 零 bug**:技术指标 audit 独立复算 11 个到 10 位小数吻合(RSI 63.49/CCI 92.16/ADX 32.08/MFI 64.85/ATR 2.96…),Wilder 平滑/权重/符号全对、无杜撰;基本面 audit 手算全部 36 比率(PE 10x/PS 8x/ROA 13.33%/SGR 35%…)、单位正确、无杜撰、近似项均文档化。结论:109 指标计算正确。
> **②恐贪折线图(本 commit)**:后端给恐贪日度分值加**持久化**(store.FearGreedPoint + SaveFearGreed/FearGreedHistory,memory+postgres[fear_greed 表]+split→Market;ingestor 每日写;启动从 store backfill 进 sentiment.Cache.Seed)——历史跨重新部署累积不再清零。前端 SentimentChip 展开区加 `FearGreedTrend` SVG 折线(0-100,按最新档着色绿/红/蓝 + 50 中性虚线 + 日期/高低区间;≥2 天才显示,1 天优雅隐藏)。随天数累积成形。
> **③搜索空结果幻影按钮(本 commit)**:搜不到股票时"Go to {q}"按钮会让非股票词(如 hreflang)进入幻影详情页。改为仅当 query **像 ticker**(`^[A-Za-z0-9]{1,6}([.-][A-Za-z0-9]{1,4})?$`)才显示"直接打开"——hreflang→只显"无匹配"无按钮;qxqz(像 ticker)→保留按钮(未收录 OTC 仍可达)。预览双向验证。
> **④调研 TradingAgents(subagent,Apache-2.0 可商用复用)→ R2 增强思路**:可复用其角色分解(分析师映射我们已有数据节)+ **牛熊辩论两轮生成 → 综合**(提升研报定性平衡,~3 次 DeepSeek,完全符合我们数字-从结构化-源、LLM-只定性的反幻觉约束)+ **自我批判 pass**(删无据论断)+ 双层模型路由。**不采纳**其交易/执行/评级层(违反"非投资建议/不执行")。详见调研报告。
