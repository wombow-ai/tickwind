# Build spec — 机会榜 (Opportunity board) · 大V rail · 热点话题条 (Hot Topics)

Synthesized from 4 research passes (2026-06). All endpoints verified live from a
datacenter context with a descriptive `User-Agent`. Everything here is FREE,
redistribution-safe, and datacenter-IP-friendly (no X API, no Yahoo, no Reddit
direct, no Finnhub fundamentals, no StockTwits redistribution).

Guiding UX principle (applies to every board): **present signals as observed,
sourced facts — never as conclusions/recommendations.** Lead with the evidence
and its source link, not an upside %. Disclaimers go ON the card (point of
dissemination), not only in ToS.

---

## 1. 机会榜 — Opportunity board (SEC insider-buy backbone)

**v1 definition:** US common stocks where, in the last 30 days, ≥1 insider made an
**open-market purchase (Form 4 transaction code `P`)**, AND market cap ∈
**[$300M, $2.5B]**. Ranked by insider conviction: (1) # distinct buyers → (2) total
$ value → (3) ApeWisdom buzz tiebreaker (buzz never promotes a no-insider-buy name).

**Why defensible:** every row = "a real insider spent real money buying their own
small-cap on the open market last month, per their SEC filing." Factual, sourced,
public-record. Each row links to the source Form 4.

**Inverted pipeline (key simplification):** Form 4 buys GENERATE the candidate set
(tens–hundreds of CIKs/day) → we only price <300 tickers/day, never the whole market.

### Data sources (all SEC public-domain + Alpaca we already have)
- **Shares outstanding (market-wide):** SEC XBRL frames, ONE row per CIK:
  `https://data.sec.gov/api/xbrl/frames/dei/EntityCommonStockSharesOutstanding/shares/CY{YYYY}Q{q}I.json`
  (use `dei` not `us-gaap` — dei is cover-page total, no per-class dedup). Pull the
  2 most recent completed quarters, take latest `val` per CIK. JSON: `data[].{cik,val,end,entityName}`.
  Per-CIK fallback: `https://data.sec.gov/api/xbrl/companyconcept/CIK{10-digit}/dei/EntityCommonStockSharesOutstanding.json`
- **CIK↔ticker map:** `https://www.sec.gov/files/company_tickers.json` (refresh weekly).
- **Form 4 daily sweep:** `https://www.sec.gov/Archives/edgar/daily-index/{YYYY}/QTR{n}/form.{YYYYMMDD}.idx`
  (discover via `.../QTR{n}/index.json`). Fixed-width: `Form Type | Company | CIK | Date Filed | File Name`.
  Filter form-type == `4` (exclude `4/A` v1). **Re-scan yesterday+today** (forms filed after 10pm ET disseminate next day).
- **Form 4 XML:** resolve accession → folder `https://www.sec.gov/Archives/edgar/data/{CIK}/{accNoDashless}/`
  → read folder index, pick the `*4*.xml` (do NOT hardcode `doc4.xml`). Leaves are `<value>`-wrapped, HTML-entity-escaped.
  - `issuerTradingSymbol` → ticker
  - `reportingOwner/.../rptOwnerName`, `isDirector`, `isOfficer`, `officerTitle`
  - loop `nonDerivativeTransaction`: `transactionCoding/transactionCode`,
    `transactionAmounts/transactionShares/value`, `transactionPricePerShare/value`,
    `transactionAcquiredDisposedCode/value`
  - **Open-market BUY = transactionCode == `P` only.** Exclude A(award) M(option exercise)
    S(sale) G(gift) F(tax) C(conversion) X. $ value = shares × price (drop price==0).
- **Prices/market-cap:** Alpaca multi-snapshot `https://data.alpaca.markets/v2/stocks/snapshots?symbols=A,B,...`
  (≤100 symbols/req, 200 req/min; use `dailyBar.c` or `prevDailyBar.c`). Empty price → drop row.
- **Buzz tiebreaker:** ApeWisdom (already ingested).

### Gates (anti-pump, must pass all)
real Alpaca price; mktcap ≥ $300M; code `P` with price>0 and $value ≥ $25k;
issuer has recent XBRL facts (real operating company, not SPAC/shell).

### Store tables
- `security_shares(cik PK, ticker, entity_name, shares_outstanding, as_of_date, fetched_at)`
- `insider_filing(accession_no PK, cik, ticker, filed_date, owner_name, is_director, is_officer, officer_title, raw_xml_url)`
- `insider_transaction(id PK, accession_no FK, ticker, txn_date, txn_code, shares, price, dollar_value, acquired_disposed)`
- `opportunity_board(ticker PK, cik, company_name, price, shares_outstanding, market_cap, market_cap_band, buyers_30d, total_buy_value_30d, buy_txn_count_30d, last_buy_date, buzz_rank, rank_score, computed_at, explainer_text, top_buyers JSON)`

### Cadence (Go scheduler, single shared SEC token-bucket ≤10 req/s, UA w/ contact email)
- `syncSharesFrames` weekly · `ingestForm4Daily` daily (yesterday+today, idempotent on accession) ·
  `recomputeBoard` daily (CIKs with P-buy in 30d → shares → Alpaca batch → market cap → gates → rank).
- Backfill once: walk daily-index back 30 days.

### "Why it's here" explainer (denormalized for UI)
`explainer_text` = "3 insiders bought $1.2M of TICKER in the last 30 days"; `top_buyers` JSON
= [{name,title,date,shares,price,value}]. Drawer links to `raw_xml_url` (the trust anchor).

### Defer to v2: analyst targets (no free redistributable source), 13F clustering, congress trades, revenue growth, sector filters.

---

## 2. 大V rail — "Guru-watch" (newsletter cadence, NOT live tweets)

Achievable at hours-to-days latency via public RSS + Bluesky AT-Proto + SEC. NOT live X.

### Serenity (@aleabitoreddit) — VERIFIED
- Feed: `https://aleabitoreddit.substack.com/feed` (200, no auth, cashtag-dense free posts).
- Caveats: shallow feed (often 1 item) → poll 1–6h + PERSIST (can't backfill from RSS).
  Custom-domain gotcha: a Substack on a custom domain serves an empty stub at `*.substack.com/feed`;
  fetch `customdomain.com/feed` instead. (Serenity has no custom domain → ok.)

### Ticker extraction (Go)
1. fetch (UA "TickwindBot/1.0 (contact@tickwind.com)"), parse `encoding/xml`, dedup on `<guid>`.
2. strip HTML from `content:encoded` → text.
3. cashtag regex `\$([A-Z]{1,5})(?:\.[A-Z])?\b` (high-confidence).
4. name→ticker pass: scan against a company-name→ticker dict built from SEC `company_tickers.json` + hand aliases.
5. validate candidates against the master US-symbol universe; stoplist common all-caps ($I $A $CEO $USD $AI).
6. score: title hit > first-paragraph > body frequency → mark lead vs also-mentioned.
7. store {influencer, post_url, pubDate, lead_ticker, mentioned_tickers[], teaser}.

### Curated KOL feeds (verified live, full text on free posts)
RSS: `aleabitoreddit.substack.com/feed` (Serenity, AI/semis micro-cap) ·
`www.capitalemployed.com/feed` (aggregator of best pitches) ·
`thevalueroad.substack.com/feed` (micro/small-cap deep value) ·
`microcapnewsletter.substack.com/feed` (Planet MicroCap) ·
`emergingvalue.substack.com/feed` · `triplesinvesting.substack.com/feed` (special situations) ·
`www.stockmarketnerd.com/feed` (custom domain!) · `adventuresincapitalism.com/feed/` (Kuppy — expired SSL, handle gracefully).
Bluesky (AT-Proto getAuthorFeed, cashtag facets): `firstadopter.bsky.social` (Tae Kim, semis) ·
`downtownjoshbrown.bsky.social` · `carlquintanilla.bsky.social` · `unusualwhales.bsky.social` (macro/large-cap skew).

### Presentation/legal
Attribute every card (name·platform·date); link-out is primary CTA; show only derived data +
≤200-char teaser (never republish paywalled bodies); badges: `✔ verified figure` /
`self-reported·unaudited` / `Analyst·PT range` / `Sponsored` (only if paid, disclose).
Keep the rail SEPARATE from the opportunity score. Disclaimer: "仅供参考，不构成投资建议 / For
information only, not advice; views are authors' own, may be self-reported and unaudited."
Position as "长线大V观点追踪 / Guru-watch", NOT real-time signals/copy-trading.

### Supplementary (datacenter-safe, redistributable): SEC 13F superinvestors + Form 4 (public domain).
### Do NOT use StockTwits for redistribution (ToS forbids; their API is closed). [Flag: we currently ingest StockTwits social — separate pre-existing review.]

---

## 3. 热点话题条 — Hot Topics strip (two-layer, stdlib Go)

Clickable chip row (`AI capex · Fed · Earnings · Semiconductors …`) with counts → filtered view.
Computed from already-ingested data (Finnhub news + cached AV articles + social) — NO new AV calls.

### Two layers
1. **Curated keyword→theme dictionary (~25-30 themes)** matched (word-boundary, case-insensitive)
   over headline+summary. THE source of the good chips. Seed:
   AI capex {ai capex, data center, datacenter, gpu, hyperscaler, accelerator, inference} ·
   Semiconductors {chip, semiconductor, foundry, wafer, tsmc, hbm, euv} ·
   Fed {fed, fomc, powell, rate cut, rate hike, interest rate, basis points} ·
   Inflation {inflation, cpi, pce, ppi} · Earnings {earnings, eps, guidance, beat estimates} ·
   Tariffs {tariff, trade war, export control, sanction} · Crypto {bitcoin, btc, ethereum, crypto, stablecoin} ·
   Layoffs {layoff, job cuts, restructuring} · M&A {acquisition, merger, takeover, buyout} ·
   Oil & Energy {oil, opec, crude, wti, brent, natural gas, lng} · EV {electric vehicle, battery, charging} ·
   Jobs report {nonfarm, payrolls, jobless claims, unemployment} · IPO {ipo, goes public, public offering}.
2. **AV `topics` (15 fixed buckets)** as structured supplement + related-ticker/sentiment source
   (we already store AV articles). API values (lowercase): blockchain, earnings, ipo,
   mergers_and_acquisitions, financial_markets, economy_fiscal, economy_monetary, economy_macro,
   energy_transportation, finance, life_sciences, manufacturing, real_estate, retail_wholesale, technology.
   Generic buckets (financial_markets, economy_macro, finance, earnings) = demote.

### Ranking (per topic, 24h window vs prior 24h)
`recency(t)=Σ exp(-ageH/τ)` (τ≈10h) · `momentum=(now+k)/(prior+k)` (k≈3) ·
`z = (now-mean14d)/stdev14d` ("unusual for THIS topic") · `genre_penalty` ×0.35 for generic ·
`hotness = recency * momentum^0.6 * genre_penalty * (0.5+z)`.
Floors: min count ≥3; momentum gate ≥1.15 to be "hot"; dedupe by family (keep Fed over economy_monetary);
keep ≥1 evergreen anchor so it's never empty. Show 8 chips (6 mobile, 10 wide).

### Data model + serving
`HotTopic{Key, Label, Count, Momentum, Sentiment, RelatedTickers[], Source("keyword"|"av_topic"), Match}` ·
`HotTopicsSnapshot{GeneratedAt, Window, Topics[]}`. Recompute every 5 min from in-process data;
serve via `GET /v1/topics` (atomic snapshot). Keep 14-day per-topic counts for the z baseline.
Chip click → `/news?theme=<key>` (keyword) or `/news?av_topic=<match>` → filter stored articles
(+ optional RelatedTickers intersect). Labels: only show curated/AV clean labels, never raw n-grams.

### Pitfalls: generic-topic domination (genre penalty + z + momentum gate); single-article noise (min 3);
gaming (counts from news primarily, social as bounded multiplier, dedupe near-identical headlines);
stale strip (evergreen anchor + fallback to top-by-volume). CN labels later via `label_zh` + CN dict.

---

## 4. UX / cards / compliance (shared)

- **Opportunity row:** ticker+name+sparkline · "Small cap · $480M" pill · "Why it's here" sentence (HERO,
  sourced) · evidence chips ("3 insiders bought $1.2M · informative") · implied range deemphasized in
  SLATE not green · muted buzz indicator · CTA "View signals"/"See the evidence" (never "Buy"/"Trade").
  Composite "Signal" score with drill-down to evidence (TipRanks model), labeled Outperform/Neutral not Buy/Sell.
- **AVOID:** giant green upside %, "Buy"/"Strong Buy"/🚀, single analyst PT as fact, urgency/countdowns,
  ranking by raw upside %, counting uninformative insider txns.
- **Microcopy:** global "Tickwind is a research/information tool, not a broker or adviser. Nothing here is
  investment advice." Board header "Opportunities are surfaced by data signals (insider activity…), not
  recommendations." Per-card ⓘ "Why am I seeing this?". Topics "Trending reflects what's being discussed,
  not an indication these are good investments." Ban urgency/scarcity copy. Disclaimers legible, on-surface.
