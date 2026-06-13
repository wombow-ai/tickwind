# Build Prompt — Stock & Crypto Analytics Platform (paste into Claude Code)

> Copy everything below the line into Claude Code, after placing `SPEC.md`, `indicators.json`
> (and optionally `indicators.csv` / `indicators.yaml` and the reference doc
> `股票与区块链金融_指标库全集.md`) into the repo, e.g. under `./data/`.
> Edit the **PROJECT CONFIG** block first — that's the only part you need to touch.

---

You are the lead engineer building an indicator-driven **stock + crypto analytics platform**.
A curated, audited **indicator dataset is the single source of truth** for what the product
measures. Your job is to turn that dataset into a working product: ingest data, compute every
indicator, serve it via an API, and render it in a dashboard — built in priority order.

## PROJECT CONFIG (edit these, then proceed)

```yaml
product_scope:   full-stack MVP            # options: full-stack MVP | backend+API only | compute library only | metadata-driven UI only
asset_classes:   [crypto, stock]           # build whichever you need; dataset field `applies_to` tells you which indicators apply
start_phase:     P0                          # build P0 first, then P1, then P2
backend:         Python + FastAPI            # your choice
compute:         pandas + TA-Lib / pandas-ta # use these for `technical` indicators where `talib_or_lib` is set
database:        PostgreSQL + TimescaleDB    # time-series store for prices + computed indicator series
frontend:        React + TypeScript + TradingView Lightweight Charts
data_providers:                             # wire per the SPEC "data_source → provider" table; keys via .env; support a MOCK mode
  crypto_ohlcv:  Binance (or CCXT)
  stock_ohlcv:   Polygon / Alpha Vantage
  onchain:       Glassnode API
  derivatives:   Coinglass
  sentiment:     Alternative.me (Fear&Greed), CBOE (VIX)
deploy_target:   Docker compose (local) → cloud later
language:        code & comments in English; UI may show Chinese names via `name_zh`
```

## Source of truth — read these first

1. **`data/SPEC.md`** — the contract. Read it fully before writing code. It defines the record
   schema, enums, conventions, the suggested build order, and the data_source→provider map.
2. **`data/indicators.json`** — the canonical dataset, **414 indicators**. Every record has:
   `id` (stable key), `domain`, `subcategory`, `priority` (P0/P1/P2), `applies_to`
   (stock/crypto/both), `name_en`, `name_zh` (Chinese display name — the only non-English field),
   `abbr`, `definition`, `formula`, `inputs` (OHLCV for technical), `default_params`,
   `talib_or_lib` (TA-Lib/lib hint), `output_type` (overlay/oscillator/volume/value/series/…),
   `data_source`, `interpretation` (how to read it / thresholds).
   `indicators.csv` and `indicators.yaml` are the same data in other formats — pick whichever
   fits your tooling.
3. **`data/股票与区块链金融_指标库全集.md`** (optional) — long-form bilingual reference with
   fuller derivations; use it for rich tooltips/help text. JSON is canonical for code.

## Hard rules (guardrails)

- **The dataset drives the code.** Generate the indicator registry, DB schema, and UI metadata
  *from* `indicators.json` — do not hand-maintain a parallel list. Re-running against an updated
  dataset should update the product.
- **Never invent or alter formulas.** Implement exactly what `formula` (and the reference doc)
  says. If a formula is ambiguous or you can't implement it faithfully, mark that indicator
  `unsupported` with a reason — do not guess.
- For `domain == technical`, compute via **TA-Lib / pandas-ta** when `talib_or_lib` is set;
  honor `default_params`; use `inputs` (OHLCV) for required series.
- For `fundamental` / `onchain` / `sentiment`, implement from `formula` + `data_source`; many
  are direct provider pulls or simple arithmetic over fetched series.
- Use `applies_to` to decide whether an indicator is offered for a given asset.
- Use `name_zh` **only** for display; never as a key. Keys are `id`.
- `definition` may be empty for some formula-first ratios — fall back to `formula` +
  `interpretation` for UI copy. Do not fabricate definitions.
- Never hardcode market data or fake values. Provide a **MOCK/fixture mode** so the app runs
  without live API keys, and real adapters behind interfaces.
- Keep secrets in `.env`; never commit keys.

## Build plan (do in order; confirm architecture before Phase 1)

**Phase 0 — Scaffold & data layer**
- Read `SPEC.md`. Print a short summary of the dataset (counts by domain/priority) to confirm you loaded it.
- Propose the architecture & folder structure for the chosen CONFIG, then proceed.
- Create DB schema + migrations: `indicators` (metadata from the dataset), `assets`,
  `price_bars` (OHLCV time-series), `indicator_values` (computed series, keyed by indicator `id`
  + asset + timestamp).
- Write a **seeder** that loads `indicators.json` into the `indicators` table.
- Generate typed models/interfaces (e.g., TS types + Python pydantic) from the schema.

**Phase 1 — P0 (37 indicators): end-to-end vertical slice**
- Implement provider adapters (behind interfaces, with MOCK mode) for the data sources the P0
  set needs (group by `data_source` so each integration unlocks many indicators).
- Build an **indicator registry**: `id → compute(params, series) -> value/series`. Implement all
  P0 indicators. Technical via TA-Lib/pandas-ta; others from `formula`.
- Expose an API: list indicators (filter by domain/priority/applies_to/asset), and get an
  indicator's latest value + historical series for an asset.
- Build a minimal dashboard: searchable indicator catalog (show `name_en`/`name_zh`, abbr,
  subcategory, priority, interpretation), an asset selector, and charts that respect
  `output_type` (overlays on price; oscillators in a sub-pane; values as cards/series).
- Tests: unit-test each P0 formula against known fixtures (e.g., RSI/MACD vs TA-Lib reference
  values; ratios vs hand-computed cases). Add a coverage test asserting **every P0 `id` resolves
  to an implementation** (or is explicitly `unsupported` with a reason).

**Phase 2 — P1 (61)** then **Phase 3 — P2 (316)**: extend adapters (options/IV, COT,
entity-adjusted on-chain, breadth, macro), registry, API, and UI the same way.

## Acceptance criteria

- Seeding `indicators.json` populates the catalog; the catalog count matches the file.
- A registry-coverage test passes for the current phase (every in-scope `id` implemented or
  explicitly unsupported with reason).
- Formula unit tests pass for the implemented indicators.
- The app runs end-to-end in MOCK mode with no external keys, and with real adapters when keys
  are present.
- No indicator metadata is hardcoded outside the dataset-derived layer.

## Working method

Read `SPEC.md` and `indicators.json` first and summarize them back to me. Propose the
architecture and Phase-1 scope, list any indicators you expect to mark `unsupported` and why,
then build incrementally with tests. Keep a short `IMPLEMENTATION_NOTES.md` mapping `id →
implementation status`. Ask me only if a decision isn't covered by the CONFIG or SPEC.

**Start now with Phase 0.**
