# Selectable per-stock indicators — design doc

Date: 2026-06-14 · Author: architect (R1 follow-up) · Status: design, not yet implemented

## Goal & hard rule

The owner wants per-stock indicators to be **selectable / customizable** over a
**larger computable set** than today's 28 P0 ids.

Two phases:

- **Phase B (backend):** expand the per-stock *computable* set — only indicators we can
  compute **faithfully** from data we actually have (technical from daily OHLCV via
  `BarCache.DailyCandles`; fundamental from SEC XBRL via `edgar.Fundamentals`).
- **Phase A (frontend + tiny prefs surface):** a selection/customize UI. Backend computes
  the **full available set** per stock (cached per ticker/day, as today); the frontend
  shows the user's **selected subset** with a picker to add/remove/reorder.

**HARD RULE — no fabrication.** Every value is a deterministic latest scalar over data we
hold. Anything needing options chains, intraday ticks, analyst forecasts, a market-index
return series, headcount, or long valuation history is **not** addable and stays
`unsupported`/`insufficient`. The honest computable ceiling is **~135–140 of 282**, not 282.

---

## The honest count (set expectations up front)

| Bucket | Today | After Phase B |
|---|---|---|
| Technical computed (P0+P1+P2 from daily OHLCV) | 8 (9 registered, VWAP insufficient) | **~58** (8 + ~50 new) |
| Fundamental computed (XBRL) | 10 | **~68** (10 + ~58 new) |
| Market-context (VIX, Fear&Greed) | 2 | 2 |
| **Faithfully computable total** | **19 producing values** | **~128** producing values; **~135–140** *menu* ids |
| Sentiment (98 ids) | unsupported | **still unsupported** (no options/onchain/breadth/macro feeds) |
| Tech patterns / intraday VWAP / IV | unsupported | **still unsupported** (~12 ids) |
| Fund forecasts / headcount / index-beta / long-history | insufficient | **still not addable** (~35 ids) |

`addable_count` after expansion ≈ **138** faithfully-computable indicator ids (the menu the
picker offers as `ok`/`insufficient`, not `unsupported`). Producing a real *value* for a
given stock depends on its data (a name with no XBRL still gets the technicals).

---

# 1) PHASE B — backend computable-set expansion

## 1.0 How `StockIndicators` should iterate the expanded set

Today the engine iterates **only P0** records and dispatches each to a registry closure /
market-context / crypto-unsupported (`compute.go:417-440`, `evaluate` at `compute.go:485-515`).
The dispatch in `evaluate` already handles *any* catalog record — the only two gaps are
**which records are iterated** and **which have closures**. Change both, together:

1. **Iterate the registered set, not a priority slice.** Replace `p0StockIDs()`
   (`compute.go:389-391`) with an iteration over **exactly the catalog records whose id is in
   `c.registry`** — i.e. the implemented indicators — *plus* the market-context + crypto-only
   ids so those keep their existing dispatch. Concretely:

   ```go
   // computedIDs returns the catalog records this Computer evaluates: every id with a
   // registered closure, plus the market-context and crypto-only ids (whose dispatch lives
   // in evaluate). Iterating the registry — not a priority filter — means adding a closure
   // is the ONLY step needed to surface a new indicator, and unimplemented ids are simply
   // absent (never shown as a broken "not computed" row).
   func (c *Computer) computedIDs() []Indicator {
       out := make([]Indicator, 0, len(c.registry)+len(cryptoOnlyIDs)+2)
       for _, rec := range c.catalog.All() { // catalog order preserved
           _, registered := c.registry[rec.ID]
           _, crypto := cryptoOnlyIDs[rec.ID]
           isCtx := rec.ID == idVIX || rec.ID == idFearGreed
           if registered || crypto || isCtx {
               out = append(out, rec)
           }
       }
       return out
   }
   ```

   `StockIndicators` calls `records := c.computedIDs()` instead of `c.p0StockIDs()`. **Net
   effect:** unimplemented P1/P2 ids are **absent** from the response (not a misleading
   "not computed" insufficient row). The picker (Phase A) only ever offers ids the backend
   actually returns, so the menu and the engine can never drift.

2. **Add the closures** to `technicalRegistry()` / `fundamentalRegistry()` (the new ids in
   §1.1 / §1.2). Each registered id then participates automatically.

3. **Update the coverage test.** `compute_test.go:492-504` (`TestRegistryCoversAllP0`)
   asserts every P0 id is handled — keep it (P0 is a subset of the registry, still passes).
   Add a sibling test asserting **every registered id is a real catalog id** (no typos) and
   **every closure returns `ok` or `insufficient` on synthetic-but-valid input** (never a
   panic, never a fabricated value when an input is missing). See §3.

No new module deps. All math reuses the existing pure helpers in `technical.go` /
`fundamental.go` plus a handful of small new pure helpers (listed inline).

---

## 1.1 (a) Technical indicators — from daily OHLCV, **no new data**

All inputs are already in `computeInput` (`highs/lows/closes/volumes`, `compute.go:61-69`),
**except** `Open` — one indicator (RVI) needs it, so thread `Candle.Open` into a new
`opens []float64` field in `gather()` (`compute.go:445-479`; `store.Candle.Open` already
exists at `store.go:61-68`). All others need only existing fields.

**New tiny pure helpers to add to `technical.go`** (each dependency-free, unit-tested):
`wma`, `smma` (extract the Wilder recursion already inside `atrWilder`/`rsiWilder`),
`rollingStd` (population σ of a window — already computed inside `bollinger`, expose it),
`stdLogReturns`, `linregForecast` (small OLS over a window), `percentRank`, `rsiSeries`
(RSI at each index, for StochRSI/ConnorsRSI), `cmoSeries`, `adlSeries`, plus `dmiADX`,
`obv`, etc. as needed. None pull a dependency.

### Moving averages & bands (closes only unless noted)

| id | formula / approach | inputs |
|---|---|---|
| `technical.wma` | WMA = (n·Cₙ+…+1·C₁)/(n(n+1)/2) | closes |
| `technical.dema-tema` | DEMA = 2·EMA − EMA(EMA); TEMA = 3·EMA1 − 3·EMA2 + EMA3 (compose `emaSeries`) | closes |
| `technical.zlema` | lag = (n−1)/2; ZLEMA = EMA(C + (C − C[t−lag]), n) | closes |
| `technical.hma` | HMA = WMA(2·WMA(C,n/2) − WMA(C,n), √n) | closes (needs `wma`) |
| `technical.smma-rma-wilder` | SMMA = (prev·(n−1) + C)/n (extract `smma`) | closes |
| `technical.kama` | ER = |net|/Σ|step|; SC = [ER·(2/3 − 2/31) + 2/31]²; KAMA recursion | closes |
| `technical.alma` | Gaussian-weighted window sum (9, 0.85, 6) | closes |
| `technical.vidya` | VIDYA = C·k|CMO| + prev·(1 − k|CMO|) | closes (needs `cmoSeries`) |
| `technical.env` | Upper/Lower = MA·(1 ± p%) | closes |
| `technical.gmma` | two EMA groups (3,5,8,10,12,15 / 30,35,40,45,50,60) → `Extra` lines | closes |
| `technical.bbw` | BBW = (Upper − Lower)/Mid from `bollinger()` | closes |
| `technical.b` | %B = (C − Lower)/(Upper − Lower) from `bollinger()` | closes |
| `technical.sd` | rolling population σ(20) — expose from `bollinger` internals | closes |

### Channels / trend (H,L,C)

| id | approach | inputs |
|---|---|---|
| `technical.dc` | Donchian: U = n-high, L = n-low, Mid = (U+L)/2 | H,L |
| `technical.kc` | Keltner: Mid = EMA(C,20), bands = Mid ± 2·ATR(10) (reuse `atrWilder`) | H,L,C |
| `technical.st` | Supertrend: HL2 ± m·ATR with flip recursion | H,L,C |
| `technical.sar` | Parabolic SAR recursion (AF 0.02..0.20) → latest scalar + trend side | H,L |
| `technical.dmi-adx` | +DM/−DM/TR → +DI/−DI/ADX, Wilder smoothing | H,L,C |
| `technical.vi` | Vortex: +VI = Σ|H−Lprev|/ΣTR, −VI = Σ|L−Hprev|/ΣTR, n=14 | H,L,C |
| `technical.aroon` | Aroon Up/Down = (n − bars-since-extreme)/n·100; osc = Up − Down | H,L |
| `technical.pp` | Pivot Points from **prior** bar: PP = (H+L+C)/3, R1/S1… | H,L,C |
| `technical.gaps` | gap up/down = today L > yest H (or H < yest L); boolean→scalar | H,L |
| `technical.fractals-bill-williams` | bar whose H/L exceeds the 2 bars each side | H,L |
| `technical.alligator` | three SMMA(HL2) lines (13,8,5), displaced; emit current values | H,L (needs `smma`) |

### Momentum / oscillators

| id | approach | inputs |
|---|---|---|
| `technical.cci` | (HLC3 − SMA(HLC3,20)) / (0.015·mean-abs-dev) | H,L,C |
| `technical.williams-r` | %R = (n-high − C)/(n-high − n-low)·−100, n=14 | H,L,C |
| `technical.mtm` | Momentum = C − C[t−n] | closes |
| `technical.roc` | ROC = (C − C[t−n])/C[t−n]·100 | closes |
| `technical.cmo` | CMO = 100·(SU − SD)/(SU + SD) over close deltas | closes |
| `technical.rmi` | RSI-variant over C − C[t−m] deltas, Wilder smoothing | closes |
| `technical.tsi` | 100·EMA(EMA(ΔC,25),13)/EMA(EMA(|ΔC|,25),13) | closes |
| `technical.trix` | triple-EMA ROC, n=15 | closes |
| `technical.ppo` | (EMA12 − EMA26)/EMA26·100, signal = EMA(PPO,9) | closes |
| `technical.dpo` | DPO = C[t−(n/2+1)] − MA(C,n) | closes |
| `technical.kst` | weighted sum of 4 smoothed ROCs + signal MA | closes |
| `technical.stochrsi` | (RSI − minRSI)/(maxRSI − minRSI) over an RSI series | closes (needs `rsiSeries`) |
| `technical.uo` | Ultimate Osc = 100·(4·Avg7 + 2·Avg14 + Avg28)/7 | H,L,C |
| `technical.smi` | double-EMA of (C − midpoint)/range | H,L,C |
| `technical.ao` | SMA(HL2,5) − SMA(HL2,34) | H,L |
| `technical.bull-bear-power` | Bull = H − EMA(C,13), Bear = L − EMA(C,13) | H,L,C |
| `technical.fisher-transform` | ½·ln((1+x)/(1−x)) on normalized price position | H,L |
| `technical.coppock-curve` | WMA(ROC14 + ROC11, 10) — daily variant, labeled | closes |
| `technical.tii` | 100·Σpos-dev/(Σpos + Σneg-dev) vs MA | closes |
| `technical.cfo` | (C − linreg-forecast)/C·100 (needs `linregForecast`) | closes |
| `technical.crsi` | [RSI(C,3) + RSI(streak,2) + percentRank(return,100)]/3 | closes |
| `technical.dynamic-mi` | adaptive-period RSI: n = 14/(recentσ/longσ) then RSI | closes |

### Volatility

| id | approach | inputs |
|---|---|---|
| `technical.hv` | Std(ln(C/Cprev), n)·√252 (needs `stdLogReturns`) | closes |
| `technical.chv` | Chaikin Vol: (EMA(H−L,10) − its value n ago)/that·100 | H,L |
| `technical.chop` | 100·log10(ΣTR/(n-high − n-low))/log10(n), n=14 | H,L,C |
| `technical.mi` | Mass Index = Σ25(EMA(H−L,9)/EMA(EMA(H−L,9),9)) | H,L |
| `technical.rwi` | Random Walk Index from H,L vs ATR·√n | H,L,C |
| `technical.rvi` | weighted(C−O)/weighted(H−L) + signal — **needs Open** | O,H,L,C |

### Volume

| id | approach | inputs |
|---|---|---|
| `technical.obv` | up-day +V, down-day −V cumulative | C,V |
| `technical.adl` | MFM = ((C−L) − (H−C))/(H−L), ADL += MFM·V | H,L,C,V |
| `technical.cmf` | Σ(MFM·V)/ΣV, n=20 | H,L,C,V |
| `technical.cho` | Chaikin Osc = EMA(ADL,3) − EMA(ADL,10) | H,L,C,V |
| `technical.mfi` | Money Flow Index, n=14 | H,L,C,V |
| `technical.fi` | Force Index = EMA((C − Cprev)·V, 13) | C,V |
| `technical.pvt` | PVT = prev + V·(C − Cprev)/Cprev | C,V |
| `technical.pvi-nvi` | PVI/NVI on up/down-volume days by close return | C,V |
| `technical.kvo` | Klinger: EMA(VF,34) − EMA(VF,55) + signal | H,L,C,V |
| `technical.eom-emv` | EMV = (HL2 − HL2prev)/[(V/scale)/(H−L)], 14-MA | H,L,V |
| `technical.vo-pvo` | (EMA(V,12) − EMA(V,26))/EMA(V,26)·100 | V |
| `technical.vroc` | (V − V[t−n])/V[t−n]·100 | V |

**Technical total added ≈ 50** → ~58 producing values.

### Technical — stays unsupported (NOT faithfully addable, daily OHLCV only)

`technical.vwap` (intraday only — keep `setInsufficient`, `compute.go:180-189`),
`technical.vix` (SPX option IV, not a stock indicator), and all subjective
pivot/geometry pattern recognition (no deterministic latest scalar):
`fib-retracement`, `fib-extension`, `trend-lines-channels`, `reversal-chart-patterns`,
`continuation-chart-patterns`, `candlestick-reversal-patterns`, `three-candle-patterns`,
`andrews-pitchfork`, `zig-zag`, `vpvr` (volume profile — needs price binning; borderline,
**mark unsupported** for now). These are simply **absent** from the registry → absent from
the response (per §1.0), so the picker never offers them.

---

## 1.2 (b) Fundamental indicators — from XBRL, grouped by the NEW `edgar.Fundamentals` fields

The extraction is `extractFundamentals` (`internal/edgar/fundamentals.go:93-201`); adding a
us-gaap concept is a few-line `pick()` + `latestInstant`/`latestAnnual`/`annualForFY`
change (helpers at `fundamentals.go:213-275`). **Extend the XBRL extraction ONCE**, grouped
so each new field unlocks a family. All ratios are pure `(value, ok)` over
`edgar.Fundamentals` + price (the existing `fundamental.go` contract).

### Group 0 — NO new fields (compute today from existing struct)

These need only fields already on `Fundamentals` (`Shares`, `Revenue`, `NetIncome`,
`EPSDiluted`, `Equity`, `GrossProfit`, `TotalAssets`, `TotalLiabilities`,
`OperatingCashFlow`, `CapEx`, `DividendsPaid`, `RevenuePrior`, `NetIncomePrior`) + price.
**Ship this group first** (Phase B increment 1) — zero extraction risk.

| id | formula | notes |
|---|---|---|
| `fundamental.market-cap` | price × Shares | — |
| `fundamental.eps-diluted` | already extracted as `EPSDiluted` | expose as id |
| `fundamental.bvps` | Equity / Shares | — |
| `fundamental.sps` | Revenue / Shares | — |
| `fundamental.ps` | price·Shares / Revenue | — |
| `fundamental.d-e` | TotalLiabilities / Equity | — |
| `fundamental.equity-multiplier` | TotalAssets / Equity | — |
| `fundamental.roa` | NetIncome / TotalAssets | point-in-time (avg variant needs prior Assets — see Group 3) |
| `fundamental.gp-a` | GrossProfit / TotalAssets | — |
| `fundamental.total-asset-turnover` | Revenue / TotalAssets | — |
| `fundamental.ocf-ni` | OperatingCashFlow / NetIncome | — |
| `fundamental.ocf-cfo` | OperatingCashFlow (stated) | expose as id |
| `fundamental.fcf-conversion` | fcf() / NetIncome | — |
| `fundamental.capex` | CapEx | expose as id |
| `fundamental.capex-sales` | CapEx / Revenue | — |
| `fundamental.fcf-yield` | fcf() / (price·Shares) | — |
| `fundamental.pcf` | price·Shares / OperatingCashFlow | — |
| `fundamental.cfps` | OperatingCashFlow / Shares | — |
| `fundamental.fcfps` | fcf() / Shares | — |
| `fundamental.payout-ratio` | DividendsPaid / NetIncome | — |
| `fundamental.retention-ratio` | 1 − payout | — |
| `fundamental.dps` | DividendsPaid / Shares | — |
| `fundamental.sgr` | roe() × retention | — |
| `fundamental.accruals` | (NetIncome − OperatingCashFlow) / TotalAssets | — |
| `fundamental.tobin-s-q` | (price·Shares + TotalLiabilities) / TotalAssets | — |
| `fundamental.pe-lyr` | price·Shares / NetIncomePrior | `NetIncomePrior` already extracted |

**Group 0 ≈ 26 ids, zero new XBRL.**

### Group 1 — income-statement concepts (highest leverage; ~10 ratios)

**New fields:** `OperatingIncomeLoss`, `InterestExpense`, `IncomeTaxExpenseBenefit`,
`DepreciationDepletionAndAmortization`, `IncomeLossFromContinuingOperationsBeforeIncomeTaxes`
(latest-FY flows, via `latestAnnual`). EBIT can be derived as `NetIncome + InterestExpense
+ IncomeTaxExpenseBenefit` (the catalog's own `ebit-margin` formula) when
`OperatingIncomeLoss` is absent.

| id | formula | needs |
|---|---|---|
| `fundamental.opm` | OperatingIncome / Revenue | OperatingIncomeLoss |
| `fundamental.ebit-margin` | EBIT / Revenue | InterestExpense, IncomeTaxExpenseBenefit (EBIT = NI + int + tax) |
| `fundamental.pre-tax-margin` | pre-tax income / Revenue | IncomeLossBeforeIncomeTaxes (or NI + tax) |
| `fundamental.ebitda-margin` | (OpIncome + D&A) / Revenue | OperatingIncomeLoss, D&A |
| `fundamental.icr-tie` | EBIT / InterestExpense | InterestExpense (+ EBIT inputs) |
| `fundamental.roce` | EBIT / (TotalAssets − current liabilities) | OperatingIncomeLoss + LiabilitiesCurrent (Group 2) |
| `fundamental.roic` | NOPAT / invested capital; NOPAT = EBIT·(1 − taxrate) | OperatingIncomeLoss, tax, debt, cash (Group 2/4) |
| `fundamental.op-growth` | (OpIncome − priorOp)/|priorOp| | OperatingIncomeLoss + prior-FY |

### Group 2 — current-balance-sheet concepts (~12 ratios)

**New fields:** `AssetsCurrent`, `LiabilitiesCurrent`, `InventoryNet`,
`CashAndCashEquivalentsAtCarryingValue`, `AccountsReceivableNetCurrent`,
`AccountsPayableCurrent`, `PropertyPlantAndEquipmentNet`, and a `CostOfRevenue` **field**
(currently COGS is only derived inline for `GrossProfit` — promote it to a struct field).

| id | formula | needs |
|---|---|---|
| `fundamental.current-ratio` | AssetsCurrent / LiabilitiesCurrent | AC, LC |
| `fundamental.quick-ratio` | (AssetsCurrent − Inventory) / LiabilitiesCurrent | AC, LC, InventoryNet |
| `fundamental.cash-ratio` | Cash / LiabilitiesCurrent | Cash, LC |
| `fundamental.inventory-turnover` | COGS / InventoryNet (point-in-time) | COGS, InventoryNet |
| `fundamental.dio` | 365 / inventory-turnover | COGS, InventoryNet |
| `fundamental.receivables-turnover` | Revenue / AR | AR |
| `fundamental.dso` | 365 / receivables-turnover | AR |
| `fundamental.payables-turnover` | COGS / AP | AP, COGS |
| `fundamental.dpo` | 365 / payables-turnover | AP, COGS |
| `fundamental.ccc` | DIO + DSO − DPO | InventoryNet, AR, AP, COGS |
| `fundamental.fixed-asset-turnover` | Revenue / net PP&E | PP&E |
| `fundamental.current-asset-turnover` | Revenue / AssetsCurrent | AC |
| `fundamental.wc-turnover` | Revenue / (AssetsCurrent − LiabilitiesCurrent) | AC, LC |
| `fundamental.ocf-ratio` | OperatingCashFlow / LiabilitiesCurrent | LC |

### Group 3 — prior-year balance/income values (growth + averages; ~6 ratios)

**New fields:** prior-FY values for `Equity`, `Assets`, `GrossProfit`, `EPSDiluted` (add
`EquityPrior`, `TotalAssetsPrior`, `GrossProfitPrior`, `EPSDilutedPrior` via `annualForFY(...,
fy−1)` / a prior `latestInstant` window — the helpers already support pulling a prior FY).

| id | formula | needs |
|---|---|---|
| `fundamental.eps-growth` | (EPS − priorEPS)/|priorEPS| | EPSDilutedPrior |
| `fundamental.equity-growth` | (Equity − priorEquity)/priorEquity | EquityPrior |
| `fundamental.asset-growth` | (Assets − priorAssets)/priorAssets | TotalAssetsPrior |
| `fundamental.gp-growth` | (GP − priorGP)/priorGP | GrossProfitPrior |
| `fundamental.eps-basic` | EarningsPerShareBasic (own field) or NI / wtd-basic-shares | EPSBasic and/or WeightedAvgBasicShares |

Note: the **average-denominator** variants of ROA / turnover ratios (e.g. NI / avg assets)
become faithful once prior-FY balances exist. Until then the point-in-time variant in
Group 0/2 is the honest fallback (matches how `roe()` already uses latest equity).

### Group 4 — debt / EV / capital-structure concepts (~10 ratios)

**New fields:** `LongTermDebtNoncurrent` (or `LiabilitiesNoncurrent`), `DebtCurrent`,
`Goodwill`, `IntangibleAssetsNetExcludingGoodwill`,
`PaymentsForRepurchaseOfCommonStock`, `ResearchAndDevelopmentExpense`. EV =
market cap + interest-bearing debt − cash (minority/preferred default 0).

| id | formula | needs |
|---|---|---|
| `fundamental.lt-debt-ratio` | LT liabilities / (LT liabilities + Equity) | LongTermDebt |
| `fundamental.net-gearing` | (debt − cash) / Equity | debt, Cash |
| `fundamental.cash-st-debt` | Cash / short-term debt | Cash, DebtCurrent |
| `fundamental.dtnw` | TotalLiabilities / (Equity − intangibles − goodwill) | Goodwill, Intangibles |
| `fundamental.goodwill-equity` | Goodwill / Equity | Goodwill |
| `fundamental.tbv` | tangible PB = mktcap / (Equity − goodwill − intangibles) | Goodwill, Intangibles |
| `fundamental.ev` | mktcap + debt − cash | debt, Cash |
| `fundamental.ev-sales` | EV / Revenue | EV inputs |
| `fundamental.ev-fcf` | EV / fcf() | EV inputs |
| `fundamental.ev-ebitda` | EV / (OpIncome + D&A) | EV inputs, OpIncome, D&A |
| `fundamental.r-d-intensity` | R&D / Revenue | R&D |
| `fundamental.buyback-yield` | buyback / mktcap | PaymentsForRepurchaseOfCommonStock |

### Group 5 — composite scores (data-heavy; lower confidence — ship LAST, behind tests)

| id | formula | needs |
|---|---|---|
| `fundamental.altman-z-score` | 1.2X1 + 1.4X2 + 3.3X3 + 0.6X4 + 1.0X5 | AssetsCurrent, LiabilitiesCurrent, RetainedEarnings, OpIncome (EBIT), + Group 0 |
| `fundamental.piotroski-f-score` | 9-point: ROA, OCF, ΔROA, OCF>NI, ΔLT-debt, Δcurrent-ratio, share issuance, Δgross-margin, Δasset-turnover | all component fields + prior-FY (Group 3) |
| `fundamental.beneish-m-score` | 8-variable manipulation score | AR, GP, Assets/AC/PP&E, Revenue, D&A, SG&A, debt — **current AND prior** |

`RetainedEarningsAccumulatedDeficit` is the only extra field Altman-Z needs beyond Groups
0–4. **Beneish-M is borderline** (needs a full current+prior IS/BS/CF set; some catalog
formula terms are A-share flavored) — **mark it unsupported for now** unless every input is
clean; revisit after Groups 0–4 land.

**Fundamental total added ≈ 58** (26 Group-0 + ~32 across Groups 1–5) → ~68 producing
values. New `edgar.Fundamentals` fields added across Groups 1–4: **~22 concepts**, all via
the existing `pick()` mechanism, extracted once.

### Fundamental — stays NOT addable (no faithful source)

`forward-p-e`, `peg` (analyst forecast EPS/growth — not in XBRL), `rev-per-employee`
(headcount not in companyfacts), `beta`, `tsr` (need a price-return series vs a market
index we don't ingest — *computable later from bars*, not from XBRL), `pe-pb-percentile`
(needs long valuation history), `dividend-streak`/`dgr` (multi-year dividend walk),
`adjusted-eps`/`core-earnings` (non-recurring-items judgement), plus the A-share-specific
per-share items (`crps`, `reps`) that don't map to US-GAAP. These stay **absent** from the
registry → never offered by the picker.

---

# 2) PHASE A — selection / customize UX

## 2.0 Decision (locked)

- **Backend computes the FULL available set per stock**, cached per ticker/day exactly as
  today (`GET /v1/stocks/{ticker}/indicators` is unchanged in shape — it just returns more
  ids after Phase B). No per-user compute, no server-side filtering of indicators.
- **Frontend renders the user's SELECTED subset** with a picker to add/remove/reorder.
  Selection is purely a **view filter + ordering** over the already-computed payload.
- **Default (no saved selection) = today's behavior.** The default selection is the
  **current P0 set** (the 19 producing ids grouped by domain), so nothing regresses.

This keeps Phase A shippable on **Vercel independently** of the Phase B backend deploy: the
picker works against whatever ids the endpoint returns today, and silently gains the new
ones the moment the backend deploys.

## 2.1 `IndicatorsPanel` changes (`web/src/components/IndicatorsPanel.tsx`)

1. **Selection state** — a `Set<string>` of selected indicator ids + an ordered
   `string[]` (ids in display order). Source of truth resolved on mount:
   - signed-in: GET `/v1/me/prefs` → `indicators` blob (see §2.3); else
   - anonymous: `localStorage` (see §2.2); else
   - neither → **default** = the ids whose `priority === 'P0'` in the returned payload,
     in `(technical, fundamental, sentiment)` domain order (today's grouping). Because the
     default is derived from the *payload itself*, it stays correct even as the catalog
     grows.

2. **Render the selected subset.** The existing `groups` memo
   (`IndicatorsPanel.tsx:88-106`) is filtered to `selected.has(ind.id)` and ordered by the
   saved order (fallback: current domain/status order). `unsupported` stay hidden as today.
   When a stock can't compute a selected id, it shows the existing `—` (`insufficient`) row —
   no regression.

3. **A "Customize" control** — a small button in the panel header (next to "learn more",
   `IndicatorsPanel.tsx:135-139`) opening an `IndicatorPicker` (new component,
   `web/src/components/IndicatorPicker.tsx`):
   - lists **all available** indicators from the payload, **grouped by domain**, with a
     **search box** (filter by name_en / name_zh / abbr — mirror the catalog `matchesText`),
     a domain filter chip row, and an **add/remove checkbox** per row;
   - shows a small status hint per row (`ok` value preview / `—` insufficient / hides
     `unsupported`), so the user can prefer ids that actually compute for this stock;
   - a **reorder** affordance on the selected list (drag handles or up/down — keep minimal;
     up/down buttons are the smallest faithful option, no new dep);
   - "Reset to default" → clears the saved selection (back to the P0 default).
   - i18n keys under `ind2.picker.*` (title, search, add, remove, reset, count).

4. **Persistence write** — on every change, persist via §2.2 (anon) or §2.3 (signed-in)
   debounced (~500ms). The selection is a tiny `{ids: string[]}` (order = the array).

## 2.2 Anonymous persistence — `localStorage`

- **Key:** `tickwind.indicators.v1` (versioned so a future shape change is a clean reset).
- **Shape:** `{ "ids": string[] }` — the selected indicator ids **in display order**.
  Absence of the key ⇒ default (P0). No per-ticker key: the selection is a **global user
  preference** (same indicators across all stocks), which matches how a watchlist of metrics
  works and keeps it tiny. (If per-ticker is ever wanted, bump to `.v2` keyed by ticker.)
- Read on mount, write debounced on change. Robust to malformed JSON (try/catch → default).

## 2.3 Signed-in persistence — a minimal `/v1/me/prefs` surface

A small, **generic** per-user JSON-prefs blob (not indicator-specific — future UI prefs reuse
it). Routed to the **User** store via `Split` (cheap-to-rebuild, same class as watchlist/
notes/alerts). Mirrors the existing per-user CRUD exactly.

### Store method (`internal/store/store.go`, in the `Store` interface)

```go
// Prefs is a per-user JSON preferences blob (small UI state: selected indicators,
// future view prefs). Opaque to the store — the API owns the shape. Routed to the
// User store via Split. GetPrefs returns ok=false when the user has none (the
// caller then falls back to defaults, so nothing regresses).
GetPrefs(ctx context.Context, userID string) (json.RawMessage, bool, error)
PutPrefs(ctx context.Context, userID string, blob json.RawMessage) error
```

- A new `store.Prefs` type is **not** needed — the blob is opaque `json.RawMessage`. Cap the
  size in the handler (e.g. ≤8 KB) so it can't be abused as arbitrary storage.

### Memory impl (`internal/store/memory/...`)

A `map[string]json.RawMessage` guarded by the existing mutex; `GetPrefs` returns the stored
copy + `ok`, `PutPrefs` overwrites. (~15 lines, mirrors the watchlist map.)

### Postgres impl (`internal/store/postgres/...`)

```sql
CREATE TABLE IF NOT EXISTS user_prefs (
  user_id text PRIMARY KEY,
  prefs   jsonb NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);
```
`PutPrefs` = `INSERT ... ON CONFLICT (user_id) DO UPDATE SET prefs=$2, updated_at=now()`;
`GetPrefs` = `SELECT prefs FROM user_prefs WHERE user_id=$1` → `ok=false` on no rows. The
table is in the **User** schema (same DB as watchlist); idempotent `CREATE TABLE IF NOT
EXISTS` per the project convention.

### Split routing (`internal/store/split.go`)

```go
func (s Split) GetPrefs(ctx context.Context, userID string) (json.RawMessage, bool, error) {
    return s.User.GetPrefs(ctx, userID)
}
func (s Split) PutPrefs(ctx context.Context, userID string, blob json.RawMessage) error {
    return s.User.PutPrefs(ctx, userID, blob)
}
```

### API endpoints (`internal/api/api.go`)

```go
mux.HandleFunc("GET /v1/me/prefs", s.getMyPrefs) // requireUser → {} when none
mux.HandleFunc("PUT /v1/me/prefs", s.putMyPrefs) // requireUser, body ≤8KB
```

- `getMyPrefs`: `u, ok := s.requireUser(...)`; `GetPrefs(u.ID)`; on `ok=false` return
  `200 {}` (empty object — client falls back to default). 401 without a token.
- `putMyPrefs`: `requireUser`; `io.LimitReader(r.Body, 8<<10)`; validate it's a JSON object;
  `PutPrefs(u.ID, blob)`; `204`.
- **Shape stored:** `{"indicators": {"ids": [...]}}` — namespaced so future prefs (e.g.
  default chart panes) slot in under sibling keys without a migration. The indicators client
  reads/writes only the `indicators` sub-key; `PUT` replaces the whole blob, so the client
  must read-merge-write (GET, set `indicators`, PUT) to avoid clobbering a future sibling
  key — or, simpler now, the backend `putMyPrefs` does a **shallow merge** of the posted
  top-level keys into the stored blob (recommended: keeps the client trivial). Document
  whichever is chosen; **shallow-merge in the handler** is the smaller client surface.

### Frontend client (`web/src/lib/api.ts`)

`getMyPrefs(token)` → `GET /v1/me/prefs` (auth); `putMyPrefs(token, blob)` → `PUT`. The
`IndicatorsPanel` calls these via the existing `useAuth().getToken()` pattern
(`web/src/lib/auth.tsx`). When signed-in, prefs win over `localStorage`; on first login the
client may **migrate** the local selection up to the server once (optional, nice-to-have).

---

# 3) IMPLEMENTATION PLAN — ordered, shippable increments

### Increment 0 — Phase A frontend only (ships on Vercel, **no backend deploy**)
1. `IndicatorsPanel` selection state + view-filter + ordering, default = P0-from-payload.
2. `IndicatorPicker` component (search, domain groups, add/remove, up/down reorder, reset).
3. Anonymous `localStorage` persistence (`tickwind.indicators.v1`, §2.2).
4. i18n keys `ind2.picker.*`. Verify `cd web && npm run lint && npm run build`.
   → **Ships immediately.** Works over today's 19-id payload; auto-grows after Phase B.

### Increment 1 — Phase B backend, fundamentals **Group 0** (no new XBRL) + iteration switch
1. Switch `StockIndicators` to `computedIDs()` (§1.0); update `p0StockIDs` callers.
2. Add the **26 Group-0 fundamental closures** (§1.2) — pure over existing struct fields.
3. Add the **~50 technical closures** (§1.1) + the small new pure helpers + thread `Open`.
4. Extend the coverage tests (§ below). Verify `go build ./... && go vet ./... && gofmt -l .`
   (empty) + `go test ./internal/indicators/...`. → **First backend deploy** (the menu jumps
   from 19 → ~84 producing ids with zero new XBRL extraction).

### Increment 2 — Phase B backend, fundamental Groups 1–4 (new XBRL fields, batched)
1. Extend `extractFundamentals` with the new concepts (Groups 1→2→3→4), each behind a
   `pick()` + the existing instant/annual helpers; add fields to `Fundamentals` struct.
2. Add the dependent ratio closures as each field group lands.
3. Add `edgar` extraction tests (synthetic companyfacts → expected fields). → deploy.

### Increment 3 — composite scores (Group 5) + prefs persistence
1. `fundamental.altman-z-score`, `piotroski-f-score` (Beneish-M deferred/unsupported).
2. `GetPrefs`/`PutPrefs` store method (memory + postgres + Split) + `/v1/me/prefs`
   endpoints (§2.3) + `user_prefs` table + `api` httptest.
3. Frontend: signed-in prefs read/write, prefer server over local, optional one-time
   migrate-up. → frontend ships on Vercel; backend deploy for the endpoint + scores.

## Tests to add

- **`internal/indicators/technical_test.go` / `fundamental_test.go`** — table-driven per new
  pure helper + closure: a known-input → known-output case (cross-checked against the catalog
  formula / a reference value), plus a too-short / zero-denominator case asserting `ok=false`
  (and therefore `insufficient`, never a value). Mirror the existing style.
- **`internal/indicators/compute_test.go`** — (a) keep `TestRegistryCoversAllP0`; (b) **new**
  `TestRegistryIDsAreRealCatalogIDs` (every registry key ∈ catalog, catches typos); (c)
  **new** `TestComputedClosuresNeverPanicOrFabricate`: run every registered closure over a
  synthetic full `computeInput` and over an *empty* one, assert no panic and that any
  non-`ok` result carries no `Value` (extends the existing `TestComputeNeverFabricates`,
  `compute_test.go:~516`).
- **`internal/edgar/fundamentals_test.go`** — synthetic companyfacts JSON → assert each new
  field extracts (and is 0/absent when the concept is missing, never invented).
- **`internal/store/...`** — `GetPrefs`/`PutPrefs` round-trip in memory + (if the pg harness
  runs) postgres; `split_test.go` asserts prefs route to `User`.
- **`internal/api/...`** — httptest: `PUT /v1/me/prefs` then `GET` round-trips per-user; 401
  without a token; oversized body rejected; `GET` with no prefs → `200 {}`.

## Verification & no-fabrication guarantees

- **Guarantee 1 — absence over fabrication.** Unimplemented ids are *absent* from the
  response (not "not computed" rows), enforced by `computedIDs()` iterating the registry.
  The picker only offers ids the backend returns → menu/engine can't drift.
- **Guarantee 2 — insufficient over invention.** Every closure returns `ok` only when the
  inputs are present and the denominator valid; otherwise `setInsufficient` (no `Value`),
  asserted by `TestComputedClosuresNeverPanicOrFabricate` +
  `TestComputeNeverFabricates`.
- **Guarantee 3 — faithful formulas.** Technical math reuses the chart-parity helpers
  (SMA-seeded EMA, Wilder RSI/ATR, population-σ Bollinger — `technical.go:5-19`); fundamental
  ratios follow the catalog formulas (point-in-time approximations documented inline, as
  `roe()`/`peTTM()` already do).
- **Guarantee 4 — no new deps.** All math is stdlib + the existing pure helpers; XBRL uses
  the existing `pick`/`latestInstant`/`latestAnnual`/`annualForFY`.
- Run gates: backend `go build ./... && go vet ./... && gofmt -l .` (empty) + `go test
  ./cmd/... ./internal/...`; frontend `cd web && npm run lint && npm run build`. Live-verify a
  real ticker (e.g. AAPL has full XBRL; MSTR for the loss/no-EPS edge) post-deploy.

---

# 4) RISKS / OPEN QUESTIONS

1. **The count ceiling is ~135–140, NOT 282 — be explicit with the owner.** The headline
   "282 stock-applicable" is misleading as a compute target: **98 are sentiment** (options /
   onchain / breadth / macro feeds we don't ingest at all), ~12 are technical patterns /
   intraday VWAP / IV, and ~35 fundamentals need forecasts / headcount / index-beta /
   long-history. The faithful selectable menu after full Phase B is **~138 ids**. Selecting
   over "everything" is impossible without fabrication; the picker should show only the
   computable menu and never list the unsupported ~145.

2. **Borderline ids — mark unsupported now, revisit later:**
   - `technical.vpvr` (volume profile) — needs price-bin construction; not a clean latest
     scalar. **Unsupported** for v1.
   - `fundamental.beneish-m-score` — needs a full current+prior IS/BS/CF set and some catalog
     terms are A-share flavored. **Unsupported** until every input is clean.
   - `fundamental.beta` / `tsr` — faithfully computable **later from bars** (price returns vs
     an index series we'd have to ingest), but **not** from XBRL. Out of Phase B scope; flag
     as a possible Phase C ("technicals that need an index series").
   - Coppock / KST / Mass Index are canonically **monthly** — daily variants are faithful if
     labeled as such; label them in the catalog/interpretation, don't silently relabel.

3. **XBRL extraction complexity (Increment 2 is the real work).** ~22 new us-gaap concepts.
   Risks: (a) tag heterogeneity across filers/eras — use priority lists in `pick()` like the
   existing code; (b) **average-denominator** ratios need prior-FY balances — until those
   land, ship the point-in-time variant and document it (as `roe()` does); (c) sign
   conventions (CapEx/dividends/buybacks stored positive via `abs` — apply the same guard);
   (d) some firms omit a concept → the ratio is correctly `insufficient`, not 0. Batch by
   field group so each adds a small, testable extraction slice.

4. **Per-user prefs storage choice.** Recommended: a **generic opaque `json.RawMessage`
   blob** at `/v1/me/prefs` routed to the **User** store (cheap, losable — same class as
   watchlist/notes), with **shallow-merge on PUT** so the client stays trivial and future
   prefs slot in without migration. Alternative considered & rejected: an indicators-specific
   table/endpoint (less reusable) or stuffing it into an existing endpoint (none fits).
   Open question: **size cap** (8 KB proposed) and whether to **migrate the anon localStorage
   selection up on first login** (nice-to-have, not required for correctness).

5. **Default must not regress.** The default selection is derived from the *payload's* P0 ids
   (not a hardcoded list), so a signed-out user with no saved prefs sees exactly today's
   panel even after the catalog grows. Verify this explicitly in Increment 0 before shipping.

6. **Per-ticker vs global selection.** Proposed: selection is a **global** user preference
   (same metrics across stocks), matching the smallest useful shape. If the owner wants
   per-ticker selections, bump the localStorage key to `.v2` keyed by ticker and store a
   `map[ticker]ids` in the prefs blob — a clean, non-breaking extension. Decision deferred to
   the owner ("你拍板"); global is the recommended default.
