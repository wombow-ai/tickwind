# Indicator history (time-series charts) — plan & status

Owner idea (2026-06-22): indicators today show only a single latest value per stock (the
`/indicators` panel). TradingView / Looknode-style sites chart each indicator over time
(x = date, y = indicator value). Build that for Tickwind, validate the data, and let the
platform AI reference it.

## Feasibility — CONFIRMED

The backend already computes indicators via **full series internally**, then takes the latest
value. `internal/indicators/technical.go` has `emaSeries`, `smaSeries`, `rsiSeries`,
`wmaSeries`, `smmaSeries`, `cmoSeries`, `obvSeries`, `adlSeries`, `rocSeries`, etc. Daily
candles (~1300 bars / 5y) flow via `OHLCVSource.DailyCandles` (same source the backtester
uses). `Point{Date, Value}` already exists. So a history chart is just exposing the series.

**Anti-hallucination:** every value is deterministic Go math over public candles — the LLM
never invents a number; the series' latest point equals the single-point `/indicators` value
(asserted by tests), so there is ONE source of truth.

## Increments

- **Inc 1 — core (DONE, `internal/indicators/history.go` + `_test.go`):** `IndicatorHistory(candles, id, period) (HistorySeries, bool)`.
  Supported ids: `technical.sma-ma`, `technical.ema`, `technical.rsi` (single line) +
  `technical.macd` (line / signal / histogram) + `technical.boll` (middle / upper / lower)
  via aligned `Lines`. Warmup bars omitted. Tests assert each series' latest point (+ extra
  lines) equals the point computeFn. `HistoryableID` / `HistoryableIDs` helpers.
- **Inc 2 — endpoint (DONE, `commit 648a424`, deployed):** `GET /v1/stocks/{ticker}/indicator-history?id=<catalog id>&period=<n>`
  → `{ticker, history:{indicator, period, unit, points:[{date,value}], lines:{...}}}`. 404 nil-bars / no-history, 400 bad id, 422 too-short. **FREE for now** (mirrors the free single-point indicators); gating under `INDICATORS_PAYWALL_ENABLED` is an open owner decision.
- **Inc 3 — frontend chart (NEXT):** a line-chart component (study the existing `KLineChart` /
  charting deps — likely a lightweight SVG/`recharts`-style; reuse the app's chart lib). An
  indicator picker (the historyable ids) + period control; overlay price-unit indicators
  (SMA/EMA/BOLL) on the price axis, oscillators (RSI 0-100, MACD) in a sub-pane. Add to the
  stock page's Indicators panel ("chart history" toggle) and/or the indicator detail page.
  EN-first. Validate the rendered series against the live endpoint (a few tickers).
- **Inc 4 — AI integration:** add a chat tool / widget `indicator_history` so the platform AI
  can surface "show me RSI over the last year for AAPL" → the Go-computed series as a widget
  (numbers stay Go-owned; the model only describes the shape). Extend the deterministic
  signals/deep-research to reference trends ("RSI has been falling for 3 weeks") grounded on
  the series, never fabricated.
- **Inc 5 (optional):** more indicators (ATR, KDJ, OBV, ADX, ROC — many already have `*Series`),
  multi-indicator overlay, downloadable PNG (reuse the export pipeline), monetization decision.

## Open owner decisions
- Free vs Pro (the point values are free; competitors charge for history — could be a Pro hook).
- Which indicators to surface first in the UI (default: SMA/EMA/RSI/MACD/BOLL — the most-charted).
- Whether to overlay on the existing KLine price chart or a dedicated indicator pane.
