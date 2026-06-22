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

## Status (updated 2026-06-22)
- ✅ **#1 Seasonality** — SHIPPED end-to-end (endpoint + stock-page card + chat widget).
- ✅ **#3 Relative strength vs SPY** — SHIPPED end-to-end (endpoint + card + chat widget).
- ✅ **#2 Earnings-reaction history** — BACKEND shipped + live (`GET /v1/stocks/{t}/earnings-reaction`;
  dates from SEC 8-K item 2.02; collapsed to ~1/quarter; min-sample floor; timing-robust ~2-session
  window). Frontend card + chat widget = the next follow-on tick.
- ⏭️ Remaining: #2 frontend, then #4 multi-factor percentile scorecard, #5 NL→screener bridge,
  #6 side-by-side compare page. Re-scan competitors fresh once these are done.
