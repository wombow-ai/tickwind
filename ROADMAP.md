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
> + docker logs 看 "connected + subscribed"。WS 出错会优雅退回 poller（无害）。**#2b 动态按浏览订阅 = 可选增强，暂缓**（30 上限够覆盖热门/自选）。
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
> `OwnershipRef.Filer`（sec 加 FetchFiler，ingestor 每条调用，限流+capped）；这是 owner 想看的核心("贝莱德加仓了谁")。#3d：前端 `/institutional`
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
