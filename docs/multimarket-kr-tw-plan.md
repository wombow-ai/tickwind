# Multi-market Phase 1 — Korea (KRX) + Taiwan (TWSE/TPEx)

Design (live-verified 2026-06) for adding KR + TW stocks on a $0 budget via
official, redistribution-safe open APIs. **Build Taiwan first** (no key, verified
working from a datacenter IP); Korea second (needs a free key, ~1-day approval).

## What the current code assumes is "US" (must become market-aware)
1. `internal/ingest/ingest.go` `ingestFilings` → `edgar.RecentFilings` for *every*
   ticker (EDGAR is US-only; also hardcodes `Security.Market="US"`).
2. `internal/ingest/price.go` `LatestQuote` → Alpaca-only (404s on non-US).
3. `internal/ingest/ingest.go` `ingestNews` → Finnhub-only (sparse KR/TW coverage).
4. `cmd/server/main.go` wires concrete US singletons; one Scheduler/PricePoller
   iterates a flat ticker list with no market dispatch.

## What already works unchanged (verified)
- The store keys by `strings.ToUpper(TrimSpace(ticker))`, so `005930.KS` / `2330.TW`
  flow through store + Split + all `/v1/stocks/{ticker}/*` endpoints with **no change**.
- `store.Security.Market` and `symbols.Symbol.Country` fields already exist;
  `symbols.Build` dedupes by ticker → appending KR/TW symbols is additive.
- Frontend already maps `.KS`/`.KQ`→KR + ₩ (`StockView.guessMarket`, `ui.marketCurrency`,
  `MarketBadge`). **Only TW (`.TW`/`.TWO`→TW, NT$) is missing** (≈2 lines + a currency case).

## Live API specs (real captured shapes)

### Taiwan — no key, datacenter-OK, Taiwan OGDL (commercial + redistribution OK). EOD.
- **TWSE daily quotes (whole market, 1 call):** `GET https://openapi.twse.com.tw/v1/exchangeReport/STOCK_DAY_ALL`
  Record: `{"Date":"1150605","Code":"2330","Name":"台積電","ClosingPrice":"2365.00","Change":"-20.0000","OpeningPrice":..,"HighestPrice":..,"LowestPrice":..,"TradeVolume":..}`
  - `Date` is **ROC**: `1150605` → 2026-06-05 (`year = 1911 + roc`).
  - All numbers are **strings**; some rows `"--"`/empty (halted) → skip. `prev_close = ClosingPrice − Change`.
- **TPEx (OTC) daily quotes:** `GET https://www.tpex.org.tw/openapi/v1/tpex_mainboard_daily_close_quotes`
  Record: `{"Date":"1150605","SecuritiesCompanyCode":"006201","CompanyName":..,"Close":"48.12","Change":"-1.72 ","Open":..,"High":..,"Low":..}` (different field names; trailing spaces → TrimSpace).
- **Symbol lists:** TWSE `…/v1/opendata/t187ap03_L` (1090 cos, Chinese keys 公司代號/公司名稱), TPEx `…/openapi/v1/mopsfin_t187ap03_O` (889 cos, English keys SecuritiesCompanyCode/CompanyName/CompanyAbbreviation). **Cheaper:** the daily-quote feeds already carry Code+Name → build the directory as a side-effect of the price fetch.
- **Filings:** no clean keyless per-symbol MOPS JSON endpoint → **TW Filings tab stays empty for v1** (UI already handles). Structured datasets (monthly revenue t187ap05, etc.) can be rendered as pseudo-filings later.
- Suffix: TWSE `.TW`, TPEx `.TWO`. Source tags `twse`/`tpex`. `Quote.Session="closed"`.

### Korea — free `AUTH_KEY` header (openapi.krx.co.kr, ~10k/day), datacenter-OK (401 without key). EOD.
- **Daily quotes (whole market, 1 call):** `GET https://data-dbg.krx.co.kr/svc/apis/sto/stk_bydd_trd?basDd=YYYYMMDD` (KOSPI), `ksq_bydd_trd` (KOSDAQ), `knx_bydd_trd` (KONEX). Header `AUTH_KEY: <key>`. Wrapper `{"OutBlock_1":[…]}`.
  Fields (spec-confirmed): `ISU_CD` (6-digit code), `ISU_NM` (Korean name), `MKT_NM`, `TDD_CLSPRC` (close), `CMPPREVDD_PRC` (Δ), `TDD_OPNPRC/HGPRC/LWPRC`, `ACC_TRDVOL`, `MKTCAP`, `LIST_SHRS`. Strings. `prev_close = TDD_CLSPRC − CMPPREVDD_PRC`. ⚠️ field casing NOT live-captured (no key) — `curl` once on key approval and diff.
- **Symbol lists:** `…/sto/stk_isu_base_info` / `ksq_isu_base_info` (or build from the daily feed's ISU_CD+ISU_NM).
- **Filings — OpenDART** (`opendart.fss.or.kr`, free key ~20k/day): `GET /api/list.json?crtfc_key=KEY&corp_code=00126380&bgn_de=YYYYMMDD&sort_mth=desc` → `{"status":"000","list":[{"corp_code","corp_name","stock_code","report_nm","rcept_no","rcept_dt",…}]}`. `status:"013"`=no data (empty, not error). Ticker→corp_code via `GET /api/corpCode.xml?crtfc_key=KEY` (a **ZIP** of CORPCODE.xml with corp_code/corp_name/stock_code; fetch once/day). Viewer URL: `https://dart.fss.or.kr/dsaf001/main.do?rcpNo=<rcept_no>`.
- Suffix: KOSPI `.KS`, KOSDAQ `.KQ` — **derive the suffix from MKT_NM/corp_cls, never guess from the code.** Source tag `krx`.

## Market-aware design (US path stays untouched)
New `internal/market`: `market.Of(ticker) Market` (suffix classifier; bare → US) + `market.Base(ticker)` (strip suffix). New `internal/ingest/adapter.go`:
```go
type MarketAdapter interface {
    Market() market.Market
    Quote(ctx, ticker) (store.Quote, bool, error)   // bool=false → no quote
    Filings(ctx, ticker) (store.Security, []store.Filing, error)
    News(ctx, ticker) ([]store.News, error)         // may be nil
}
```
Scheduler/PricePoller get `adapters map[market.Market]MarketAdapter`. US has **no** adapter (`adapters[US]==nil`) → existing EDGAR/Alpaca/Finnhub branch runs byte-for-byte. KR/TW use a daily EOD table cached in the adapter (one HTTP call prices the whole market; the 10s US poll loop stays cheap).

## BASE.SUFFIX convention
| Market | Board | Suffix | Example |
|---|---|---|---|
| KR | KOSPI | `.KS` | `005930.KS` Samsung |
| KR | KOSDAQ | `.KQ` | `247540.KQ` |
| TW | TWSE | `.TW` | `2330.TW` TSMC |
| TW | TPEx/OTC | `.TWO` | `006201.TWO` |

## Ordered build plan (each = a self-contained commit; US untouched until step 8, then only additive)
1. `internal/market` (Of/Base/consts + table test).
2. `internal/twse` — `EODQuotes`/`Companies` for STOCK_DAY_ALL; `rocDate`+`parseTWNum` helpers; test w/ real TSMC JSON fixture.
3. `internal/tpex` — same for tpex_mainboard_daily_close_quotes (diff field names).
4. **TW adapter + wiring** — `adapter.go` + `twAdapter` (Quote dispatch .TW/.TWO; Filings nil; News nil); add `adapters` to Scheduler+PricePoller (3-line guards); wire main (keyless → always on); FE `guessMarket`+NT$. **First end-to-end slice.**
5. `internal/krx` — AUTH_KEY header client, `New(key)` self-disables when empty.
6. `internal/dart` — corpCode.xml (archive/zip + encoding/xml) + list.json; `New(key)`.
7. **KR adapter + config + wiring** — `krAdapter`; add `KRX_API_KEY`+`OPENDART_API_KEY` to config; wire when keyed.
8. **Symbol union** — extend `SymbolIngestor` to merge TW (+KR if keyed) into `symbols.Build`; add twse/tpex/kospi/kosdaq to `exchRank`.
9. **Docs/seed** — example WATCHLIST (keep default US), CLAUDE/ROADMAP, "EOD" UI tag when `session=="closed" && source∈{twse,tpex,krx}`.

## Pitfalls to encode
- **ROC date** (TW): `1911 + roc`; unit-test (wrong = off by 1911y).
- **String numerics + `"--"`/empty/trailing-space** (KR+TW): a `parseNum` that trims + returns `(0,false)`; skip rows with close ≤ 0.
- **KR board suffix** from MKT_NM/corp_cls — directory is source of truth, don't guess.
- **HK leading zeros (future):** never `TrimLeft(code,"0")` — codes are fixed-width strings. (`edgar.go` TrimLeft of CIK is US-only, keep it US-only.)
- **No per-symbol MOPS JSON** — don't sink time; TW Filings empty for v1.

## TW-first minimal slice (commits 1–4)
`WATCHLIST=2330.TW,2317.TW,2454.TW` → PricePoller groups TW → `twse.EODQuotes` (1 call) → upsert `Quote{Ticker:"2330.TW",Price:2365,PrevClose:2385,Session:"closed",Source:"twse"}` → `/v1/stocks/2330.TW/quote` → FE renders NT$2,365.00 + TW badge + ChangeLine + "EOD" tag. No US code touched, no key needed.

## Riskiest unknowns (validate first)
1. KRX key approval + exact live `OutBlock_1` field casing (curl once on approval).
2. OpenDART zip handling + per-corp rate (200 tickers/day ≪ 20k cap, fine).
3. Confirm a new TW IPO appears in the daily feed (low risk; feed is comprehensive).
