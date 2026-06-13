# Indicator Library — Build Spec (for Claude Code)

This package is the machine-readable source of truth for the indicators of a stock + crypto
analytics platform. It is designed to be consumed by an LLM coding agent (Claude Code) to
generate database schemas, calculation modules, API endpoints, and UI metadata.

## 1. Why this format

For LLM-driven development, **structured JSON with stable IDs beats prose Markdown**:

- **Deterministic to parse** — every indicator is a record with the same fields, so you can
  loop over it to generate models, migrations, seed data, and calculation stubs.
- **Stable `id`s** — reference indicators from code, configs, and tests without string drift.
- **Priority is a field, not a separate file** — the old "full" and "priority/simple"
  documents are unified here. The simple view is just `filter(priority == "P0")`.
- **English-first structure** — keys, enums, IDs, categories, params, and English names are
  all in English so generated code/identifiers are clean.

**Primary file:** `indicators.json` (one array of records).
**Also provided (same data, pick what your stack likes):** `indicators.csv` (flat, for spreadsheets / SQL `COPY`), `indicators.yaml` (human-diffable config style).
**Human reference (detailed, audited, bilingual):** `股票与区块链金融_指标库全集.md` — the long-form prose with full derivations and sources. Treat JSON as canonical for code; use the MD when you need fuller explanation.

## 2. Record schema (JSON Schema, draft-07)

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "array",
  "items": {
    "type": "object",
    "required": ["id","domain","subcategory","priority","applies_to","name_en","name_zh"],
    "properties": {
      "id":            { "type": "string", "description": "Stable slug, unique. Pattern: <domain>.<slug>. e.g. technical.macd" },
      "domain":        { "enum": ["technical","fundamental","onchain","sentiment"] },
      "domain_name":   { "type": "string", "description": "Human label of the domain" },
      "subcategory":   { "type": "string", "description": "Sub-group within the domain (zh label)" },
      "priority":      { "enum": ["P0","P1","P2"], "description": "Build/importance tier. P0=MVP core, P1=advanced, P2=pro/long-tail" },
      "applies_to":    { "enum": ["stock","crypto","both"] },
      "name_en":       { "type": "string" },
      "name_zh":       { "type": "string", "description": "Chinese display name — the ONLY non-English field, kept for the UI" },
      "abbr":          { "type": "string" },
      "definition":    { "type": "string", "description": "One-line meaning (English; may be empty for formula-first ratios)" },
      "formula":       { "type": "string", "description": "Formula / calculation logic (English; math + symbols preserved)" },
      "inputs":        { "type": ["array","null"], "items": {"type":"string"}, "description": "OHLCV inputs for technical indicators: open/high/low/close/volume" },
      "default_params":{ "type": ["object","null"], "description": "Suggested default parameters, e.g. {\"period\":14}" },
      "talib_or_lib":  { "type": ["string","null"], "description": "TA-Lib function name or library hint where one exists" },
      "output_type":   { "type": ["string","null"], "description": "Render/served shape hint: overlay | oscillator | volume | value/series | ratio/value | pattern" },
      "data_source":   { "type": "string", "description": "Raw data needed (English). Codes: IS/BS/CF=income statement/balance sheet/cash-flow statement, MKT=market data, C/O/H/L/V=OHLCV" },
      "interpretation":{ "type": "string", "description": "How to read it / typical thresholds & signals (English)" }
    }
  }
}
```

### Enums

- **domain**: `technical` (price action), `fundamental` (financials), `onchain` (blockchain), `sentiment` (sentiment & derivatives).
- **priority**: `P0` (37 — MVP, build first), `P1` (61 — differentiation), `P2` (316 — pro / long-tail).
- **applies_to**: `stock`, `crypto`, `both`. (Default by domain: technical & fundamental → `stock`; onchain → `crypto`; sentiment → `both`. A few cross-apply; override as needed.)
- **output_type**: `overlay` (drawn on price), `oscillator` (sub-pane, bounded), `volume`, `pattern`, `ratio/value`, `value/series`.

## 3. Language

**Every field is English except `name_zh`** (the Chinese display name, kept for a Chinese UI).
`definition`, `formula`, `interpretation`, `subcategory`, and `data_source` were all translated
to English from the audited source; numbers, thresholds, tickers, abbreviations, library names,
and formula math structure were preserved unchanged during translation. If you don't need the
Chinese names, you can drop `name_zh` entirely.

### Formula term reference (zh → en) — optional

The dataset is already fully English; this table is kept only as a glossary for cross-checking
against the original Chinese reference document.

| zh | en | zh | en |
|---|---|---|---|
| 市值 | market cap | 已实现市值 | realized cap |
| 已实现价格 | realized price | 流通供应 | circulating supply |
| 营收 / 营业收入 | revenue | 营业成本 | COGS |
| 毛利 | gross profit | 营业利润 | operating income |
| 净利润 / 归母净利润 | net income (attrib. to parent) | 利润总额 | pre-tax income |
| 所得税 | income tax | 利息费用 | interest expense |
| 总资产 | total assets | 总负债 | total liabilities |
| 流动资产 / 流动负债 | current assets / liabilities | 存货 | inventory |
| 应收账款 / 应付账款 | accounts receivable / payable | 所有者权益 / 净资产 | equity |
| 货币资金 | cash & equivalents | 有息负债 | interest-bearing debt |
| 经营现金流 | operating cash flow (OCF) | 资本开支 | capex |
| 自由现金流 | free cash flow (FCF) | 股价 | share price |
| 总股本 | shares outstanding | 成交量 | volume |
| 收盘 / 开盘 / 最高 / 最低 | close / open / high / low | 均线 | moving average |
| 标准差 | std. deviation | 平均 / 中位 | mean / median |
| 期货价 / 现货价 | futures / spot price | 持仓量 | open interest |
| 流入 / 流出 / 净流量 | inflow / outflow / netflow | 转账量 | transfer volume |
| 算力 / 难度 | hash rate / difficulty | 矿工 | miner |
| 质押 | staking | 稳定币 | stablecoin |
| 市占率 | dominance | 涨跌家数 | advances / declines |
| 见顶 / 见底 | top / bottom | 超买 / 超卖 | overbought / oversold |
| 看涨 / 看跌 | bullish / bearish | 背离 | divergence |

## 4. Conventions

- **IDs** are `<domain>.<slug-of-abbr-or-name>`; duplicates get a numeric suffix. Treat as opaque + stable.
- **default_params** are populated for common technical indicators; `null` means "implementer's choice / not parameterized." Expose them as configurable.
- **talib_or_lib** maps to TA-Lib where a direct function exists (compute technical indicators with TA-Lib / pandas-ta rather than hand-rolling). `null` → compute from `formula`.
- **inputs** is OHLCV only and only for `domain=technical`. For other domains, read `data_source`.

## 5. Suggested build order

Build in priority waves; within a wave, group by `data_source` so each data integration unlocks many indicators at once.

1. **Phase 1 — P0 (37):** the MVP. Wire up: OHLCV feed (covers most technical P0 via TA-Lib),
   one financials provider (fundamental P0), one on-chain provider e.g. Glassnode (onchain P0),
   and a derivatives/sentiment source e.g. Coinglass + a Fear&Greed + VIX feed (sentiment P0).
2. **Phase 2 — P1 (61):** differentiation. Mostly reuses the same four data sources plus
   options data (IV / PCR) and COT.
3. **Phase 3 — P2 (316):** pro tier / paywall. Entity-adjusted on-chain, options Greeks & GEX,
   market-breadth, macro correlations, niche technicals.

### Data-source → provider hints

| data_source signal | suggested provider(s) |
|---|---|
| `C/O/H/L/V` (OHLCV) | exchange API, TradingView, Polygon, Alpha Vantage; compute via **TA-Lib / pandas-ta** |
| `IS/BS/CF/MKT` (financials) | Tushare / Wind / SEC EDGAR / Financial Modeling Prep |
| `链上` (on-chain) | Glassnode, CryptoQuant, Coin Metrics, Dune, Santiment |
| derivatives / options / 情绪 | Coinglass, Deribit, CME, CBOE, Alternative.me (F&G), CFTC (COT) |
| DeFi / TVL / staking | DefiLlama, Token Terminal, Dune, Staking Rewards |

## 6. Example record

```json
{
  "id": "technical.macd",
  "domain": "technical",
  "domain_name": "Stock Technical",
  "subcategory": "Trend",
  "priority": "P0",
  "applies_to": "stock",
  "name_en": "MACD",
  "name_zh": "平滑异同移动平均线",
  "abbr": "",
  "definition": "Difference between two EMAs, measuring trend direction, strength and momentum shifts.",
  "formula": "DIF=EMA(C,12)−EMA(C,26); DEA=EMA(DIF,9); Histogram=(DIF−DEA)×2 (×2 is the Chinese TongDaXin/THS convention; StockCharts/TradingView do not multiply by 2)",
  "inputs": ["close"],
  "default_params": { "fast": 12, "slow": 26, "signal": 9 },
  "talib_or_lib": "MACD",
  "output_type": "oscillator",
  "data_source": "C",
  "interpretation": "DIF crossing above DEA = golden cross (buy), crossing below = death cross (sell); new price high while DIF does not = top divergence (bearish)."
}
```

## 7. Counts

414 indicators total — technical 85, fundamental 99, onchain 132, sentiment 98.
Priority: P0 37, P1 61, P2 316. Generated programmatically from the audited reference MD (the `_gen*.py` scripts in this folder), so the dataset is fully regenerable.
