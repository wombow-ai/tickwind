# Free, redistribution-safe data — expansion research (verified 2026-06)

Bar: **free AND commercially redistributable.** A technically-open endpoint (no
key, returns JSON) is NOT enough — several exchanges serve open data but their
ToS forbid redistribution. Reference-clean sources we already use: US (SEC/Alpaca/
Finnhub), Taiwan (TWSE/TPEx), Korea (KRX + OpenDART).

## Part A — more free markets

**The one clean next win: 🇧🇷 Brazil / B3.**
- EOD: keyless ZIP files — daily `https://bvmf.bmfbovespa.com.br/InstDados/SerHist/COTAHIST_D{DDMMYYYY}.ZIP` (~441 KB, HTTP 200 verified) + annual `COTAHIST_A{YYYY}.ZIP`. Fixed-width, latin-1. OHLC for all equities/options since 1986.
- License: B3 Market Data Consumption Policy (Sep 2025) — "End-of-day and historical data from D-1 … may be distributed free of charge by distributors or redistributors without prior authorization." **CLEAN** (like TWSE).
- Symbols: same COTAHIST files (name + ticker + ISIN). Filings/fundamentals: **CVM** open data (`dados.cvm.gov.br`) — Brazil's EDGAR/DART analogue, free gov data.
- Suffix: `.SA` (e.g. `PETR4.SA`); native code `PETR4`. One new `internal/b3` client + TWSE-style adapter slot → unlocks a top-15 global market on $0.

**Everything else is paid or ToS-gray:**
- **Japan / J-Quants**: free key, but data is **12-week delayed** + ToS bars distributing it "in a viewable form" → disqualified for a public feed.
- **SGX, NSE (India), BSE (India)**: open keyless endpoints BUT ToS **explicitly prohibit** copying/redistribution (BSE cites criminal/civil penalties; NSE geo-blocks overseas IPs 2025). Do NOT ship — legal trap.
- **TMX (Canada), ASX, LSE, Euronext (redistribution), BMV, SET, IDX, Tadawul**: paid/licensed market-data agreements. No free redistribution tier.

Suffix conventions for the symbol model: BR `.SA`, JP `.T`, SG `.SI`, IN `.NS`/`.BO`, CA `.TO`/`.V`, AU `.AX`, UK `.L`, Euronext `.PA/.AS/.BR/.LS/.OL`, MX `.MX`.

## Part B — free "app-info" enrichment (any market)

**Top 3 (all verified free + commercially redistributable):**
1. **SEC XBRL `companyfacts` / `frames`** (`data.sec.gov`, public domain, UA + ~10 req/s). `companyfacts/CIK{10}.json` = every historical financial line item per US company; `frames/us-gaap/{concept}/USD/{period}` = one metric across all filers (instant peer/screen tables). We already have the EDGAR plumbing → biggest enrichment-for-effort: price-only → **fundamentals + valuation for the whole US market**.
2. **World Bank + OECD macro** (both **CC-BY 4.0, commercial OK** with attribution). World Bank `api.worldbank.org/v2` (~1,400 country indicators); OECD SDMX `sdmx.oecd.org/public/rest` (CPI, leading indicators). Global macro layer beside our FRED events.
3. **Frankfurter FX** (`api.frankfurter.app`, ECB rates, no key, no limits, open-source). Normalize TWD/KRW/BRL/JPY → USD/EUR for cross-market + portfolio views. Trivial; daily granularity fits an EOD watcher.

**Avoid for the commercial product:** CoinGecko Demo (non-commercial tier) + FINRA short-interest (non-commercial-only). CoinCap v3 now needs a key.

## Recommended next builds (when greenlit)
1. **SEC fundamentals** (`internal/secfin` over data.sec.gov XBRL) — US-wide, public domain, reuses EDGAR patterns. Highest value/effort.
2. **🇧🇷 Brazil/B3** market — the only clean new exchange left; `internal/b3` + adapter slot (mirrors TWSE) + CVM filings.
3. **Frankfurter FX** — cross-market price normalization for TW/KR(/BR).
