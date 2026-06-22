# Competitive roadmap — post indicator-history (2026-06-22)

Landscape scan (TradingView, Finviz, Seeking Alpha, Seasonality.ai, TrendSpider, tickeron,
Chinese 同花顺/雪球/Looknode). Filter every idea through Tickwind's invariants: **anti-hallucination
(Go owns every number, LLM only writes prose, no advice/targets), free/redistribution-safe data
sources only, solo-dev scope, EN-first.** Ranked by value × feasibility × on-brand fit.

## Prioritized buildable backlog

1. **★ Seasonality analysis (BUILD NEXT).** Competitors charge for it (Seasonality.ai, Optuma,
   TrendSpider). Pure deterministic Go over the daily candles we already have (`DailyCandles`,
   same source as backtest + indicator-history): average / median forward return per calendar
   month (and per weekday / month-of-year), win-rate, sample size, over N years. A disclosed
   HISTORICAL statistic — never a prediction or advice (same framing as the backtest widget).
   Reuses everything built this week: candle infra, the `Point`/series pattern, the
   lightweight-charts component, and the chat `surface_widget` plumbing. **Increments:** (1) Go
   core `Seasonality(candles)` + test; (2) endpoint `GET /v1/stocks/{t}/seasonality`; (3) a
   monthly-bars/heatmap frontend card on the stock page; (4) chat `seasonality` widget.
   High value, low risk, maximal reuse → the clear next feature.

2. **Earnings reaction history.** We have an earnings calendar + candles. Compute how the stock
   historically moved on/after past earnings dates (avg move earnings-day & next-day, up/down
   frequency, sample size). Deterministic, anti-hallucination-safe, complements the calendar.

3. **Relative strength vs SPY / sector.** A deterministic line of (stock return − benchmark
   return) over time — "is it leading or lagging the market?" Trivial reuse of candles + the
   beta alignment code already in `compute.go`. Small, high-utility.

4. **Multi-factor scorecard (no-advice-safe variant).** Seeking Alpha's Quant Rating is the
   draw, but a buy/sell "rating" violates our no-advice rule. The safe version: show each
   factor (value / growth / quality / momentum) as a PERCENTILE vs the universe — a disclosed
   percentile, not a recommendation. Reuses the computed indicators/fundamentals + the screener
   universe cache. Medium effort; watch the advice line carefully.

5. **NL → screener bridge.** Let the AI chat drive the existing signals screener ("bullish
   semis under $50") by adding a `run_screener` tool over the closed screener params. The
   trend (TrendSpider's GPT assistant, tickeron) is real. Higher complexity (tool + param
   mapping); do after the deterministic wins above.

6. **Side-by-side compare page** `/compare?a=…&b=…`. The chat compares ad-hoc; a structured,
   crawlable compare page (price + the computed indicators/fundamentals, all Go-owned) is good
   pSEO + utility. Medium value.

## Deliberately NOT doing (constraint conflicts)
- Real-time L2 / order-flow, broker trade-from-chart (TradingView): out of scope + data cost.
- Alt-data (web traffic, hiring) (AltIndex): not free/redistribution-safe.
- Any "AI price target / fair value / rating" (Seeking Alpha Quant, many AI tools): violates
  the no-advice rule — this is a deliberate differentiator, not a gap.

Decision: **build Seasonality next** (started this tick: the Go core). Then earnings-reaction
+ relative-strength as quick deterministic follow-ons.

## Status (updated 2026-06-23)
- ✅ **#1 Seasonality** — SHIPPED end-to-end (endpoint + stock-page card + chat widget).
- ✅ **#3 Relative strength vs SPY** — SHIPPED end-to-end (endpoint + card + chat widget).
- ✅ **#2 Earnings-reaction history** — SHIPPED end-to-end (endpoint + card + chat widget).
- ✅ **#4 Multi-factor percentile scorecard** — SHIPPED end-to-end (core → background population
  cache → `GET /v1/stocks/{t}/scorecard` → stock-page card → chat widget). FREE.
- ✅ **#6 Side-by-side compare page** — SHIPPED (`/compare` pSEO + `CompareTable` client-self-heal).
- ⏭️ **#5 NL→screener bridge** — NOT built (intersects the Pro-gated screener = monetization; surface
  before building). The deterministic-analytics suite (round 1) is otherwise complete.

---

## Round-2 backlog (re-scan 2026-06-23 — Workflow: competitor-gaps / codebase-extensions / AI-data-trends → synthesis)

Same invariants (anti-hallucination · free/redistribution-safe data · solo-dev · EN-first). Prioritized
by value × feasibility × on-brand fit, favoring REUSE of the existing engine over new ingestion. Full
synthesis (incl. rejected ideas + why) lives in the scan output; the top items:

1. **★ Market-wide factor screen + leaderboard (BUILDING THIS TICK).** Rank the EXISTING
   `ScorecardCache.Population` (value/growth/quality/momentum) market-wide via `GET /v1/screen/factors`
   + a `/screen/factors/[factor]` pSEO family ("Top Quality Stocks", "Cheapest by Value", …). Zero new
   ingestion — the dormant population cache is already built per-stock; this is the per-stock→market-wide
   inversion the signals screener validated. Reuses the SAME `ComputeScorecard` path so the leaderboard
   percentile == the stock-page percentile. FREE + crawlable (market-wide view of the free scorecard). M.
2. **Earnings-reaction badge on the earnings calendar.** Join each upcoming-earnings row with the existing
   per-stock `EarningsReaction` stat ("historically moves ±6.2%, up 58%"). Pure join, no new compute. M.
3. **Scorecard-percentile + earnings-ahead alert kinds.** New deterministic alert kinds powered by the
   already-cached data (`factor_quality_top` / `earnings_soon`), reusing the whole existing alert
   evaluator/store/UI. Lowest effort (S); alerts are the stickiest retention surface.
4. **Dividend surface (SEC-derived yield / payout / coverage) + `/dividends` hub.** Tickwind's biggest
   competitor gap (zero dividend feature). Reuses `edgar` `DividendsPaid` + live price. Strictly
   descriptive coverage stats, NO "safety grade". CAVEAT: store holds only a latest-FY ANNUAL flow → scope
   copy to "annual dividend trend", not per-payment history (that needs a new source). M.
5. **Relative-strength leaderboard + screen vs SPY.** Per-stock RS → market-wide rank via a new paced RS
   scan (clone `SignalScanCache`) feeding `GET /v1/screen/relative-strength`. M (needs a new background scan).
6. **Watchlist correlation & beta matrix.** Pairwise correlation + beta-vs-SPY over the user's watchlist
   (closed-form Go stats over candles already cached). Portfolio-risk lens; descriptive, no allocation advice. M.
7. **Seasonality pSEO page + "this month historically" market ranking.** Crawlable per-stock seasonality
   page (zero new compute, pure pSEO surfacing) + a `/screen/seasonal[/month]` market ranking. M.

REJECTED (invariant conflicts, from the synthesis): total-return/ex-div calendar (needs per-payment
history we don't store), revenue-by-segment XBRL parsing (brittle solo-dev L), net-new ingestion
(N-PORT/USASpending/USPTO/EDGAR-FTS — multi-week builds, at most ONE later as a differentiation bet),
provenance/claim-validator panel (already enforced server-side; trust-hardening, not a headliner).
