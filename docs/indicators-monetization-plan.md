<!-- Authored 2026-06-20 by the autonomous loop (indicators-monetization workstream, task #12),
grounded in the existing internal/indicators infra + the paid-indicator-platform research.
STATUS: design + autonomous build behind a flag (test mode). Pricing / free-vs-Pro split / go-live = owner-gated. -->

# Tickwind Indicators — monetization plan

The **next paid point** after the Deep Research paywall ([[tickwind-monetization-plan]]). Tickwind
already has a deep indicators stack; this plan turns it into a Pro tier WITHOUT breaking the two
load-bearing constraints: **the anti-hallucination contract** (every number/signal is Go-computed,
never LLM-invented) and **free/redistribution-safe data only** ([[tickwind-paid-ai-free-sources]]).
Reuses the Phase-1 Stripe `tierOf` spine — the indicators paywall is "just another gate."

## 1. What exists today (all free)

- `internal/indicators/`: a **Catalog** (`indicators.json`, **414 entries** — a browsable reference
  library with formulas/metadata/facets) + a `Computer` that computes the **28 P0 stock-applicable**
  indicators per ticker, **all deterministic Go math**: 9 technical (SMA, EMA, RSI, MACD, BOLL, ATR,
  KDJ, VWAP [insufficient on daily bars], VOL), 10 fundamental (PE, PB, ROE, NPM, GPM, rev-growth,
  earnings-growth, FCF, DY, debt-to-asset), 9 sentiment.
- `StockIndicator` = `{id, status: ok|insufficient|unsupported, value: *float64, extra: map}` (extra
  carries MACD signal/hist, BOLL upper/mid/lower, KDJ k/d/j).
- API: `GET /v1/indicators` (catalog: Filter/Facets/Len) + `GET /v1/stocks/{ticker}/indicators`
  (per-stock computed). Frontend: `/indicators` library page + `[id]` detail + `IndicatorsPanel`
  (per-stock) + `IndicatorPicker`/`IndicatorLibrary`.

## 2. The angle — a deterministic SIGNALS layer

The competitor hook (LuxAlgo $40-120/mo, TrendSpider $54-82/mo, looknode) is **actionable signals**:
"RSI oversold", "MACD bullish cross", "golden cross". Because Tickwind's indicators are **Go-computed**,
we can emit these signals as **deterministic rules over the computed values** — the same product
promise as the paid platforms, but **never LLM-invented** (the anti-hallucination contract holds: a
signal is a rule, traceable to a Go-computed indicator + its threshold). This is the killer Pro hook,
safe, and cheap (no LLM cost, reuses the existing compute).

Two signal classes:
- **Posture signals** (current state, latest values only — buildable NOW from the existing compute):
  RSI < 30 oversold / > 70 overbought; price vs BOLL bands (below lower / above upper); KDJ overbought
  (>80) / oversold (<20); MACD DIF vs DEA (value vs extra.signal) above/below; histogram sign; price
  vs SMA/EMA (above/below the trend line); ATR-relative volatility note; volume vs its average.
- **Event signals** (a transition — needs the PREVIOUS bar too): MACD bullish/bearish crossover
  (hist sign flip), **golden/death cross** (SMA50 × SMA200), KDJ K×D cross, price reclaiming/losing a
  moving average, BOLL squeeze→breakout. Needs (a) a couple more computed indicators (SMA50, SMA200 —
  not in the P0 28) and (b) the indicator's last-2 values (recompute the series, take the tail). A
  small extension of the Computer; still 100% deterministic.

Each signal: `{id, label, direction: bullish|bearish|neutral, strength, basis}` where `basis` cites
the Go-computed indicator + value + threshold (e.g. "RSI 27.4 < 30"). NO advice / price targets /
ratings (same scope-boundary as Deep Research) — a signal describes a disclosed technical condition,
not a recommendation.

## 3. Free vs Pro (proposed — owner to confirm §7)

| | Free | Pro |
|---|---|---|
| Indicator catalog/library (`/indicators`) | ✓ browse all 414 | ✓ |
| Per-stock computed indicators (`IndicatorsPanel`) | ✓ the 28 P0 | ✓ + expanded set (P1) |
| **Signals** (the new layer) | a teaser (e.g. 1-2 signals, or count only) | **✓ full signal list** |
| **Screener** by indicator/signal conditions | — | ✓ |
| **Alerts** on an indicator/signal condition | (basic price alerts already exist) | ✓ indicator/signal alerts |
| Backtesting a signal rule | — | ✓ (later) |

Signals are the primary Pro draw; everything keys off the existing `tierOf`. Same Stripe Pro tier as
Deep Research (one subscription unlocks both) is the simplest, highest-conversion option — confirm §7.

## 4. Build order (phased, lowest-risk-first, each behind a flag in test mode)

- **C1 — Posture signals (backend + API): ✅ DONE (local, flag-off, not deployed; commits 79ab370 + 7ce2ee7).**
  `internal/indicators/signals.go` — pure `Signals(StockIndicatorsResult) []Signal` (RSI <30/>70,
  KDJ %K >80/<20, MACD DIF vs DEA + hist sign; each `Signal{id,label,direction,basis}` cites the
  Go value + threshold). `GET /v1/stocks/{ticker}/indicator-signals` (NOTE: `/signals` was already
  taken by the news/social **buzz-sentiment** signals — a different concept — so this uses the
  `indicator-signals` path), gated by `tierOf` + `INDICATORS_PAYWALL_ENABLED` (default OFF → full
  list for everyone, current-behavior-safe; ON → non-Pro gets the first `freeSignalTeaserLimit`=2 +
  `paywall_locked` + `total_signals`). Unit-tested (signals rules + handler + teaser cap). Zero new
  data, zero LLM, zero user impact (flag off). Result carries no current price yet → price-vs-MA /
  BOLL-band / cross signals deferred to **C3** (extend the Computer with price + last-2 values).
- **C2 — Signals UI: ✅ DONE (local, not deployed; commit 840b623).** `web/src/components/SignalsCard.tsx`
  (mounted in StockView under the indicators block): per-direction rows (bullish green / bearish rose /
  neutral slate) with the Go-computed `basis` shown, a trust line, and — when `paywall_locked` — an
  honest "unlock N more with Pro" CTA → /pro (reuses the Phase-2 banner look). `api.ts`
  `getIndicatorSignals(ticker, token)` + `IndicatorSignal` type; `dict.ts` `signals.*` (EN+zh). tsc +
  next build green; preview renders + card hides gracefully on 404. Populated render pending backend deploy.
- **C3 — Price & event signals: ✅ DONE (local, not deployed; flag off).** C3.1 price-vs-SMA/EMA
  (bullish/bearish) + Bollinger band breach (neutral) via `StockIndicatorsResult.Price`. C3.2a MACD
  cross (hist sign flip, via `Extra.prev_hist` = macd over `closes[:n-1]`). C3.2b golden/death cross
  (SMA50×SMA200, via `StockIndicatorsResult.Closes`, id `technical.ma-cross`, ≥201 closes). C3.3 salience
  ordering (`salienceOf`: events > extremes > always-on trend) so the full list + the free teaser lead
  with the meaningful signals. **Final signal catalog:** RSI overbought/oversold · KDJ overbought/oversold
  · MACD above/below signal + bullish/bearish cross · price above/below SMA · price above/below EMA ·
  price above/below Bollinger band (neutral) · golden/death cross. All deterministic Go rules, each with a
  traceable `basis`, no advice/targets/ratings. Remaining (owner-scope): price-reclaim event, then C4–C6.
- **C4 — Screener by signal conditions (Pro): IN PROGRESS.** Screen the universe for stocks whose
  signals match (e.g. "golden crosses", "RSI oversold"). **Research:** the existing `GET /v1/screen`
  filters the in-memory universe quote snapshot (price/change/session) — fast, no compute. The universe
  is **~200 ingested tickers** and ALL data is cached (BarCache candles, fundCache fundamentals,
  store/bars price), so computing signals universe-wide is in-memory math — but the right architecture
  is a **background signals cache** (like `sentimentCache`/`oppCache`: a scheduled job computes
  ticker→signals every N min) that the endpoint reads instantly, vs recomputing 200× per request (and
  risking a cold-ticker network fetch). **C4.1 ✅ DONE (commit, ahead 20):** `internal/indicators/screen.go`
  — pure `ScreenSignals(map[ticker][]Signal, SignalScreen{Direction,SignalID}) []SignalMatch` (the
  deterministic core + shared query/result types), unit-tested. **C4.2 ✅ DONE (commit, ahead 22):**
  `internal/ingest/signalscan.go` `SignalScanCache` (mirrors OptionsCache: `Run` recomputes
  ticker→signals over the universe every 15 min OFF the request path, atomic swap, keeps previous on
  empty) + `GET /v1/screen/signals?direction=&signal=&limit=` reading it, Pro-gated (`tierOf` +
  `INDICATORS_PAYWALL_ENABLED`; screener is Pro-only per §3 → flag-on + non-Pro = empty + paywall_locked
  HARD lock, not a teaser; flag off = all). Wired in main.go. Tests: cache scan/screen/keep-previous +
  handler nil-404/flag-off/flag-on-hard-lock. **C4.3 ✅ DONE (commit, ahead 24):**
  `/screen/signals` page (static segment, overrides `/screen/[preset]`) — `SignalsScreen.tsx` with
  direction + signal-type filter dropdowns, a results list (ticker + matching signals as
  direction-coloured chips with their basis), a whole-page Pro lock on `paywall_locked`, loading/empty
  states; `api.ts getScreenSignals` (404→empty graceful) + `dict.ts sigscreen.*` (EN+zh) + a cross-link
  from the main `/screen` page. tsc + next build green; preview renders + degrades gracefully. **C4
  SCREENER COMPLETE (C4.1 core + C4.2 backend + C4.3 UI), flag off, not deployed.** Optional follow-up:
  pSEO preset landing pages for the signals screener (like `/screen/[preset]`).
- **C5 — Signal alerts (reuse the existing alerts feature): C5.1 backend ✅ DONE (commit, ahead 26).**
  The app already has a full alert system (store CRUD + `GET/POST/DELETE/PATCH /v1/alerts` + the
  `AlertEvaluator` in `internal/ingest/alerts.go` + web AlertsCenter/Bell/Panel). C5.1 added 6
  self-describing **signal-condition** kinds (Threshold ignored): `golden_cross`, `death_cross`,
  `rsi_oversold`, `rsi_overbought`, `signal_bullish`, `signal_bearish`. The evaluator checks them against
  a ticker's cached signals (`SignalScanCache.SignalsFor` via the new `AlertSignalSource`); a kind fires
  when the ticker has ≥1 matching signal — deterministic, never fabricated. `validAlertKinds` extended +
  threshold exempted for signal kinds; evaluator moved into the bars block so it can read the signals
  cache. Unit-tested. **NEXT C5.2:** the alert-creation UI (add the signal kinds to the AlertsPanel form,
  hide the threshold input for them) — the existing AlertsCenter already LISTS any alert kind. Delivery is
  in-app (the existing alerts UI / bell); external channels (email/Telegram) remain owner-gated.
- **C6 — Backtesting** (heaviest; defer).

C1 is the highest-value, lowest-risk, anti-hallucination-safe first increment.

## 5. Constraints kept central

- **Anti-hallucination:** signals are deterministic Go rules over Go-computed indicators — never an LLM.
  `basis` makes every signal traceable. No advice/targets/ratings.
- **Free/redistribution-safe sources only:** signals derive from the EXISTING computed indicators
  (Alpaca IEX bars, SEC fundamentals) — no new gray source.
- **tierOf reuse + flag-gated:** the indicators paywall reuses Phase-1 entitlements; everything ships
  dark behind `INDICATORS_PAYWALL_ENABLED` (default off) until owner go-live.

## 6. Pointers

`internal/indicators/compute.go` (StockIndicator/Result + the per-id compute), `technical.go`/
`technical_more.go` (the math), `indicators.json` (catalog), `web/src/components/IndicatorsPanel.tsx`
(per-stock render). Stripe spine: [[tickwind-monetization-plan]] / `tierOf`.

## 7. Open decisions for the owner (recorded; loop does not block on them)

1. **Free-vs-Pro split** — esp. the signals teaser depth (count-only? 1-2 signals? posture-free /
   event-Pro?). Default proposed: free = a small teaser, Pro = full signals.
2. **Pricing** — fold indicators into the SAME Pro tier as Deep Research (recommended, simplest), or a
   separate indicators price?
3. **Build order / scope** — confirm signals-first; how far to take screener/alerts/backtest.
4. **Go-live** — same boundary as the Deep Research paywall: nothing user-facing flips on without owner go.

The loop builds C1 (posture signals backend, flag-off) autonomously and records anything owner-gated here.
