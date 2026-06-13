# Per-stock indicator compute layer — P0 implementation notes

This package's compute layer (`technical.go`, `fundamental.go`, `compute.go`)
evaluates every **P0 stock-applicable** catalog id (priority `P0`, `applies_to ∈
{stock, both}`) for a single ticker. There are **28** such ids: 9 technical, 10
fundamental, 9 sentiment (2 market-context + 7 crypto-only). Each is reported as a
`StockIndicator` with `status` ∈ `ok | insufficient | unsupported`.

The `TestRegistryCoversAllP0` test fails the build if any P0 stock-applicable id
is left silently unhandled (not registered, not market-context, not crypto).

## Status map

### Technical (9) — pure math mirrors `web/src/lib/indicators.ts` (8 computed; VWAP insufficient-by-design under daily-only bars)

| id | status | unit | extra lines | notes |
|----|--------|------|-------------|-------|
| `technical.sma-ma` | implemented | price | — | SMA, default period 20 (catalog `periods` list is a multi-line render hint; the headline scalar uses the documented 20-day window). |
| `technical.ema` | implemented | price | — | SMA-seeded EMA (StockCharts standard); headline period **12** — the primary/fast EMA from the catalog's `{periods:[12,26]}` hint. |
| `technical.rsi` | implemented | "" | — | Wilder smoothing, period 14. Needs ≥ period+1 closes. |
| `technical.macd` | implemented | "" | `signal`, `hist` | DIF = EMA(12)−EMA(26); DEA = EMA(9) over the defined MACD points; hist = DIF−DEA. **International convention (no ×2)** for chart parity (the catalog notes ×2 is the THS/TongDaXin variant; the K-line panes are StockCharts-style). |
| `technical.boll` | implemented | price (mid) | `upper`, `mid`, `lower` | Population σ (÷period), period 20, 2σ. Headline = middle band (SMA20). |
| `technical.atr` | implemented | price | — | TR = max(H−L, \|H−Cₚᵣₑᵥ\|, \|L−Cₚᵣₑᵥ\|), Wilder smoothing, period 14. |
| `technical.stochastic-kdj` | implemented | "" (K) | `k`, `d`, `j` | International Stochastic: RSV over n=9, %K = SMA(RSV,3), %D = SMA(%K,3), J = 3K−2D. Flat window → RSV 50. |
| `technical.vwap` | **insufficient (by design)** | price | — | VWAP "resets daily" (catalog) and is an intraday S/R level. Phase 1 has only daily bars, so a faithful intraday VWAP can't be computed — reported `insufficient` (reason: "intraday VWAP needs intraday bars") rather than ship a multi-day mean mislabeled as VWAP. The pure `vwap()` helper stays for when intraday bars land. |
| `technical.vol` | implemented | "" | — | Latest bar volume. |

All technical indicators report `insufficient` when there are not enough bars for
the period (warmup), or no bars at all (e.g. a name with no Alpaca history).

### Fundamental (10) — implemented; faithful to the dataset formulas

Unit convention (per the shared contract): margins / ROE / growth / yield /
debt-ratio are **percent** values (`0.42 → 42.0`, unit `%`); PE/PB are plain
multiples (unit `x`); FCF is a **USD dollar amount** (unit `""`).

| id | status | unit | formula used | insufficient when |
|----|--------|------|--------------|-------------------|
| `fundamental.pe-ttm` | implemented | x | `price / EPSDiluted` | EPS ≤ 0 (loss/zero) or no price. **Note:** the catalog formula is "market cap / trailing-4Q net income"; `Fundamentals` exposes latest-FY diluted EPS, so per the shared contract this is an **annual** P/E (price/EPS ≡ mktcap/(EPS·shares)), not strict TTM. |
| `fundamental.pb` | implemented | x | `price·shares / equity` | price/shares/equity ≤ 0. |
| `fundamental.roe` | implemented | % | `NetIncome / equity · 100` | equity ≤ 0. **Note:** catalog formula uses *average* equity; only latest period-end equity is available, so this is a point-in-time ROE (per the shared contract). |
| `fundamental.npm` | implemented | % | `NetIncome / Revenue · 100` | revenue ≤ 0. |
| `fundamental.gpm` | implemented | % | `GrossProfit / Revenue · 100` | revenue ≤ 0 or GrossProfit absent (0). |
| `fundamental.revenue-growth-yoy` | implemented | % | `(Rev − RevPrior)/RevPrior · 100` | no prior-year revenue. |
| `fundamental.earnings-growth-yoy` | implemented | % | `(NI − NIPrior)/\|NIPrior\| · 100` | prior net income is 0. (abs base per the catalog formula → swing out of a loss reads positive.) |
| `fundamental.fcf` | implemented | "" (USD) | `OperatingCashFlow − CapEx` | operating cash flow absent (0). |
| `fundamental.dy` | implemented | % | `DividendsPaid / (price·shares) · 100` | non-payer (no dividends), or no price/shares. Market-cap dividend yield. |
| `fundamental.debt-to-asset` | implemented | % | `TotalLiabilities / TotalAssets · 100` | no total assets. |

All fundamental indicators report `insufficient` with reason
`"no SEC fundamentals available"` when XBRL data is absent (e.g. a non-US name, or
a ticker with bars but no companyfacts).

### Sentiment (9)

| id | status | reason |
|----|--------|--------|
| `sentiment.cboe-volatility-index` | **market-context** | `ok` from `MarketContextProvider.VIX()`; else `insufficient` ("no market VIX available"). |
| `sentiment.cnn-fear-greed` | **market-context** | `ok` from `MarketContextProvider.FearGreed()`; else `insufficient` ("no market Fear & Greed available"). |
| `sentiment.crypto-fear-greed` | **unsupported** | crypto-market data source; not applicable to US equities |
| `sentiment.fr` (funding rate) | **unsupported** | crypto-market data source; not applicable to US equities |
| `sentiment.liquidations` | **unsupported** | crypto-market data source; not applicable to US equities |
| `sentiment.lsr` (long/short ratio) | **unsupported** | crypto-market data source; not applicable to US equities |
| `sentiment.oi` (open interest) | **unsupported** | crypto-market data source; not applicable to US equities |
| `sentiment.spot-btc-etf-net-flows` | **unsupported** | crypto-market data source; not applicable to US equities |
| `sentiment.spot-eth-etf-net-flows` | **unsupported** | crypto-market data source; not applicable to US equities |

The market-context indicators expose the raw reading (VIX index value; Fear &
Greed score) in `value`. The same readings are also surfaced at the top level of
the API result (`VIX`, `FearGreed`) for the response's `market_context` block.

## Dataset guardrail

No formula is invented or altered. Where the available data forces an
approximation of a catalog formula (annual vs TTM P/E; latest vs average equity in
ROE; rolling vs session VWAP), the deviation is documented above and in the code
doc comments — the value computed is real, never fabricated. A missing or invalid
input always yields `insufficient` with a concrete reason and a `nil` value (the
`TestComputeNeverFabricates` test enforces this).

## Wiring (for the API handler / next phase)

The api layer should construct a `*Computer` and call:

```go
func (c *Computer) StockIndicators(ctx context.Context, ticker string) StockIndicatorsResult
```

returning:

```go
type StockIndicatorsResult struct {
    Ticker     string            // the requested ticker
    AsOf       string            // newest underlying data date (YYYY-MM-DD), may be ""
    VIX        *float64          // nil when unavailable
    FearGreed  *FearGreed        // nil when unavailable; {Score int; Label string}
    Indicators []StockIndicator  // sorted ok → insufficient → unsupported
}
```

Build it with `NewComputer(catalog, ohlcv, fund, price, market)` — any source may
be `nil`, in which case the dependent indicators degrade to `insufficient`
(graceful: a name with bars but no XBRL still returns the technicals). The four
source interfaces (`OHLCVSource`, `FundamentalsProvider`, `PriceProvider`,
`MarketContextProvider`) are defined in `compute.go`; `ingest.BarCache` already
satisfies `OHLCVSource` (it has `DailyCandles`), and `edgar`/cache wrappers
satisfy the others with thin adapters.
