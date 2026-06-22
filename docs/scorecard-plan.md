# Multi-factor percentile scorecard (roadmap #4) — plan

A per-stock "factor scorecard": where a stock ranks, **as a percentile vs Tickwind's tracked
universe**, on four classic factors — **Value · Growth · Quality · Momentum**. The draw is Seeking
Alpha's Quant Rating, but a buy/sell/hold **rating violates our no-advice rule**, so we ship the
*descriptive* version only: percentile ranks of public, Go-computed metrics, with no composite
"score to act on" and no recommendation.

## Anti-advice design (the crux — owner flagged it)
- **No single composite "score".** Four factors shown SEPARATELY as percentiles. A blended 0–100
  "quant score" reads as a rating → forbidden. (We may show the 4 side by side; we never sum them
  into one act-on-this number.)
- **Percentiles are descriptive facts, not verdicts.** "Profitability: 85th pct" = its ROE/ROIC is
  higher than 85% of the tracked universe — a fact. We never say cheap/expensive=buy/sell, never
  "undervalued", never a direction.
- **Disclaimer on the card:** "Percentile rank vs Tickwind's tracked universe (~N names) — a
  descriptive statistic from public data, not a rating or recommendation."
- Reuses the existing anti-hallucination contract: every metric is Go-computed; no LLM.

## Factors → sub-metrics (all already computed per-stock as indicators)
- **Value** — `pe-ttm`, `pb`, `ps` (LOWER raw = higher value-percentile → invert when ranking).
- **Growth** — `revenue-growth-yoy`, `earnings-growth-yoy` (higher = higher pct).
- **Quality** — `roe`, `roic`, `ebit-margin`, `piotroski-f-score` (higher = higher pct).
- **Momentum** — `tsr` (1y total return) and/or relative-strength vs SPY (higher = higher pct).

Each factor's percentile = the mean of its available sub-metric percentiles (value sub-metrics
inverted). A factor with no available sub-metric → omitted (insufficient-not-wrong), never 0/50.

## Percentile method
`percentile(v, pop)` = 100 × (count of pop ≤ v) / len(pop), over the population's non-NaN values for
that sub-metric. Deterministic. A tiny population (< minScorecardPopulation) → omit the factor (a
percentile vs 5 names is noise).

## Population = Tickwind's tracked universe (bounded, already warm)
NOT all ~7k tickers (most lack fundamentals + the SEC fetch cost is prohibitive). The population is
the bounded set the platform already scans/caches (`ingestTickers` ∪ POPULAR, ~200 names whose
fundamentals are in `FundamentalsCache`). Disclosed as "tracked universe (~N)". A background
`ScorecardCache` (increment 2) recomputes the population's FactorMetrics every ~30–60 min (like
`SignalScanCache` / `OptionsCache`), so the endpoint never computes the distribution on the request
path.

## Increments
1. **(this tick) Deterministic core + plan** — `internal/indicators/scorecard.go`: `FactorMetrics`,
   `ExtractFactorMetrics(StockIndicatorsResult)`, `percentile()`, `ComputeScorecard(target, population)`
   → `Scorecard{Value,Growth,Quality,Momentum *FactorScore}`. Pure, unit-tested. NOT wired/deployed.
2. **Universe factor cache** — `internal/ingest/scorecardscan.go` `ScorecardCache`: periodically
   compute `ExtractFactorMetrics` over the bounded universe (reusing the per-ticker indicators +
   FundamentalsCache), retain the population; expose `Scorecard(ticker)`.
3. **Endpoint** — `GET /v1/stocks/{ticker}/scorecard` → the 4 factor percentiles + the underlying raw
   metrics + the population size + as-of. Free (a descriptive stat). 404/422 when insufficient.
4. **Frontend card** — a stock-page `ScorecardCard` (4 factor bars/percentiles, raw metrics on hover,
   the no-rating disclaimer). Client-fetch (per deploy-gotcha #7 — never SSR-fetch the API).
5. **Chat widget** (optional) — `surface_widget(scorecard)`.

Risk watch: keep the four factors descriptive + separate; never blend into one number; disclaimer
always present. Adversarially review the advice-line before deploying increment 3.
