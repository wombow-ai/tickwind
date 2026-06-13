# R2 — AI Deep-Research Report (design)

> Status: design / pre-implementation. Drives the next `/loop` tick.
> Author: lead-architect subagent (Understand → Design). Date: 2026-06-13.
> Priority: #2 (monetizable-later route). Engineering-first; LLM optional + off the critical path.

## 0. One-paragraph thesis

R2 is a **structured per-ticker Chinese equity research report** served at
`GET /v1/stocks/{ticker}/research`. It is the richer sibling of the existing
`GET /v1/stocks/{ticker}/summary` (`getSummary`, `internal/api/api.go:2360`) and of
`getMyDigest` (`api.go:2500`): a Go assembler computes a **typed fact-sheet** (every
number, sourced + labeled) from in-process caches, and the LLM writes **only
qualitative prose** per section, forbidden from emitting any number. When the LLM is
off, the same endpoint returns the fact-sheet alone (data-only report, no prose, no
500). The split — Go owns numbers, LLM owns words — is the same architecture already
proven in `briefing.buildMaterial` / `summaryInput` / `digestMaterial`, and is the
core anti-hallucination guarantee.

---

## 1. Report structure (6 sections)

The report is a fixed, ordered list of sections. Each section carries: pre-formatted
**facts** (the numbers, from the inventories), the LLM's **prose** (qualitative
judgment only), and a **citations** list (claim → source label + deep-link). Default
language is **zh**; `?lang=en` produces an English body (best-effort, falls back to zh
prose, exactly like `BriefingCache`).

Section order is `overview` last in assembly but rendered first (it summarizes the
others). Internally we assemble 估值/基本面/技术面/资金面/情绪面 first, then the LLM
writes 概览 over their fact-sheets.

### 1.1 估值 — Valuation (`key: "valuation"`)
**Injected facts** (single source per metric — see §2.4 de-dup rule):
- `MarketCap` (USD) — from `fundamentalsResp.MarketCap` (`api.go:1193`), = price × `Fundamentals.Shares`. nil → "—".
- `PE` — from `indicators` id `fundamental.pe-ttm` (Unit `x`); nil/insufficient on a loss → render "亏损/—". (Chosen over `fundamentalsResp.PE` so PE/PB/ROE all come from the one indicator set.)
- `PB` — `fundamental.pb` (Unit `x`).
- `DY` (dividend yield) — `fundamental.dy` (Unit `%`); insufficient for non-payers.
- `Price` (USD, delayed + labeled) — `store.Quote.Price` (`store.go:33`) with `Quote.Source`/`Quote.Session`.
- `Period` / `AsOf` — `Fundamentals.Period` ("FY2024") + `Fundamentals.AsOf` (freshness stamp).

**LLM judgment**: is the multiple high/low *relative to its own history and stated sector norms in the indicator `Interpretation` text* — never a target price, never "cheap/buy". Cite the data freshness (annual FY, not TTM — note the `pe-ttm` approximation per `fundamental.go`).

### 1.2 基本面 — Fundamentals (`key: "fundamentals"`)
**Injected facts** — all from `edgar.Fundamentals` (`fundamentals.go:18`) + indicator set:
- `Revenue`, `NetIncome`, `EPSDiluted` (USD; NetIncome can be negative) + `Period`.
- `RevenueGrowthYoY` — `fundamental.revenue-growth-yoy` (Unit `%`).
- `EarningsGrowthYoY` — `fundamental.earnings-growth-yoy` (Unit `%`).
- `GrossMargin` — `fundamental.gpm` (`%`); `NetMargin` — `fundamental.npm` (`%`).
- `ROE` — `fundamental.roe` (`%`).
- `FCF` — `fundamental.fcf` (**Unit `""` but DOLLARS — special-case formatting**); can be negative (burn).
- `DebtToAsset` — `fundamental.debt-to-asset` (`%`).

**LLM judgment**: growing/shrinking, profitable vs loss-making, margin trend qualitative read, cash-generative vs burning. Material-only; "0" extended fields mean "not reported" → omit, not "zero".

### 1.3 技术面 — Technical (`key: "technical"`)
**Injected facts** — from indicator set, only `Status==StatusOK`:
- `RSI` (`technical.rsi`, unitless 0-100), `MACD` line + `Extra{signal,hist}` (`technical.macd`),
- `SMA20` (`technical.sma-ma`, `price`), `EMA12` (`technical.ema`, `price`),
- `BOLL` `Value`=mid + `Extra{upper,mid,lower}` (`technical.boll`, `price`),
- `ATR` (`technical.atr`, `price`), `KDJ` %K + `Extra{k,d,j}` (`technical.stochastic-kdj`),
- `Volume` (`technical.vol`, share count).
- **Excluded**: `technical.vwap` (always insufficient — daily-only bars).
- Context: latest `Close` from `BarCache.DailyCandles` last bar + simple price-vs-SMA20/EMA12 position (computed in Go, not LLM).

**LLM judgment**: trend (above/below moving averages), overbought/oversold per the indicator `Interpretation` thresholds (RSI>70 / <30), MACD cross direction — strictly descriptive, no entry/exit calls.

### 1.4 资金面 — Smart-money / flows (`key: "flows"`)
**Injected facts** (each nil-safe; empty = "数据不足" or section omitted):
- Congress: `congress.TickerTrade` (`cache.go:24`) — count, `MemberName`, `Type` (purchase/sale), `AmountRange` **verbatim** (never synthesize a point $), latest `TxDate`. Deep-link `/congress/member/{Slug}`.
- 13F whales: `thirteenf.Holder` (`thirteenf.go:79`) — `Manager`/`FundName`, `Weight` (% of *fund* book), `Change` (new/add/trim/hold), `Period` (as-of quarter, **must show — stale ~45d**).
- Insider buys: `store.RecentInsiderBuys(ctx, since)` (`store.go:302`) filtered to this `.Ticker`, grouped → distinct buyers, total `Value`, avg `Price`, latest `FiledDate`. (Use this not the Opportunity board — board is small-cap-only $300M–$2.5B.)
- Options: `ingest.OptionsView` (`options.go:34`) — `PCVolume`, `PCOI`, `MaxPain` + `Expiry`, top `TopOI` contracts (`cboe.Contract`: Strike/Type/OI/Expiry). Label "delayed · Cboe ~15min". 404 (ok=false) for no-options names → omit.
- Daily short: `finrashvol.ShortVol.ShortPct` (`finrashvol.go:55`) + `Date`; trend from `History(sym)` ("rising/falling" qualitative). Derived % only (no bulk raw rows — FINRA display-only).
- Settlement short interest: `finra.ShortInterest` (`finra.go:38`) — `DaysToCover`, `ShortQty`, `SettlementDate`, `ChangePct`.

**LLM judgment**: do these signals point the same or opposite directions; who is accumulating vs reducing; describe — never "follow Pelosi", never realized-return claims.

### 1.5 情绪面 — Sentiment (`key: "sentiment"`)
**Injected facts**:
- **Market-wide** (NOT per-ticker — inject as context only): `sentiment.Result` (`sentiment.go:67`) `Score` 0-100 + `Label`/`LabelZh`; guard `Available>0` (don't present the neutral-50 fallback as real).
- Per-ticker buzz: `store.Signal` (`store.go:101`) buzz facet — `Mentions` vs `MentionsPrev`, `Rank`/`RankPrev` (source apewisdom, keyless).
- Per-ticker news sentiment: `store.Signal` sentiment facet — `Score` [-1,1], `Label`, `SampleSize` (alphavantage; may be absent without key).
- Hot-list presence: is this ticker on `store.HotList(ctx,"hot"/"wsb",n)` and at what `Rank`.
- News/social corpus: top-N `store.News.Headline` (+`HeadlineZH` fallback) + `store.Post.Body` (UGC — quote/attribute, never restate as fact).

**LLM judgment**: rising/falling attention, bullish/bearish lean *as reported by the sources* with explicit attribution ("据社区讨论/per community"), market mood as backdrop. This section reuses the existing `Summarize` guardrail prose verbatim.

### 1.6 概览/结论 — Overview / Takeaway (`key: "overview"`)
**Injected facts**: none new — it references the five section fact-sheets. Optionally a 1-line `headline` (company `Name` + `nextEarningsLabel(es, lang)` from `api.go:2615`).
**LLM judgment**: 3-5 sentence synthesis weaving the five dimensions into a balanced picture (strengths/risks both sides), ending with the mandatory disclaimer. No advice, no target, no recommendation.

---

## 2. Anti-hallucination data-injection contract (the crux)

### 2.1 The fact-sheet is the SOLE source of numbers

Define a flat, typed bundle in the new package `internal/research`. The LLM never sees
raw structs — it sees a **pre-formatted material string** built from this fact-sheet,
and is instructed: *"use ONLY the numbers in the material; never compute or invent a
number; write qualitative prose."* This is the same contract as
`enrich.systemPrompt` (`enrich.go:91`) and `briefPrompt` (`enrich.go:167`), extended.

```go
// internal/research/factsheet.go

// Fact is one labeled, source-attributed datum. Value carries the already-
// formatted string the report shows (e.g. "41.2x", "亏损", "$4.5T", "—"); Raw is
// the underlying number when present (for the frontend / future PDF). Source +
// SourceURL are the citation. Status mirrors indicators.Status so the frontend
// can render "数据不足" with the Reason instead of a blank.
type Fact struct {
    Key       string   `json:"key"`        // stable id, e.g. "pe", "roe", "rsi"
    LabelZH   string   `json:"label_zh"`   // "市盈率(P/E)"
    LabelEN   string   `json:"label_en"`   // "P/E (TTM)"
    Value     string   `json:"value"`      // formatted display string
    Raw       *float64 `json:"raw,omitempty"`
    Unit      string   `json:"unit,omitempty"`   // "%" | "x" | "price" | "USD" | ""
    Status    string   `json:"status"`           // "ok" | "insufficient" | "unsupported"
    Reason    string   `json:"reason,omitempty"` // verbatim from indicators when not ok
    Source    string   `json:"source"`           // citation label, e.g. "SEC XBRL FY2024"
    SourceURL string   `json:"source_url,omitempty"`
    AsOf      string   `json:"as_of,omitempty"`  // freshness stamp
}

// SectionFacts is one report section's pre-LLM data: its facts + the citations
// they collapse to. Title carries both languages; the prose is filled by the
// composer (empty when the LLM is off).
type SectionFacts struct {
    Key      string     `json:"key"`        // "valuation" | "fundamentals" | ...
    TitleZH  string     `json:"title_zh"`
    TitleEN  string     `json:"title_en"`
    Facts    []Fact     `json:"facts"`      // only Status==ok facts carry a Value
    Citations []Citation `json:"citations"`
}

// Citation maps a section's claim space to a source. The frontend turns Anchor
// into a deep-link (/stock sub-section) or uses URL directly (filing / member page).
type Citation struct {
    Label  string `json:"label"`            // "SEC EDGAR · companyfacts"
    Anchor string `json:"anchor,omitempty"` // in-page section id, e.g. "#fundamentals"
    URL    string `json:"url,omitempty"`    // external (SEC filing, member page)
}

// FactSheet is the entire numeric backbone for one ticker, assembled with NO LLM.
// It is the data-only report when the LLM is disabled, and the material source
// when it is enabled. Pure + unit-testable.
type FactSheet struct {
    Ticker     string         `json:"ticker"`
    Name       string         `json:"name,omitempty"`
    AsOf       string         `json:"as_of"`            // newest underlying date across sources
    PriceLabel string         `json:"price_label"`      // "$190.12 · alpaca · delayed · regular"
    Sections   []SectionFacts `json:"sections"`         // valuation/fundamentals/technical/flows/sentiment
    Disclaimer string         `json:"disclaimer"`       // "AI 生成 · 数字来自公开数据 · 非投资建议"
}
```

### 2.2 The assembler (pure, NO LLM)

```go
// Sources is the narrow slice of in-process caches the assembler needs — each is
// an interface (nil-safe), so the assembler is unit-testable with fakes, mirroring
// briefing.BriefEnricher's narrow-interface pattern (briefing.go:24).
type Sources struct {
    Indicators  IndicatorCalc   // StockIndicators(ctx, ticker) indicators.StockIndicatorsResult
    Fundamentals FundProvider   // Fundamentals(ctx, ticker) (edgar.Fundamentals, error)
    Quote        QuoteProvider  // GetQuote + LatestQuote fallback (matches getFundamentals)
    Options      OptionsProvider
    Congress     CongressProvider
    ThirteenF    WhalesProvider
    ShortVol     ShortVolProvider
    ShortInt     ShortIntProvider
    Sentiment    SentimentProvider
    News, Social, Signals, Insider StoreReader
    Earnings     EarningsProvider
}

func Assemble(ctx context.Context, ticker string, src Sources) FactSheet
```

`Assemble` calls `(*indicators.Computer).StockIndicators(ctx, ticker)` **once**
(`compute.go:417` — it does all I/O internally and never errors), then walks the
returned `[]StockIndicator`, and for each fact:
- **Status gate (mandatory)**: read `Value` only when `Status == indicators.StatusOK`
  (`compute.go:29`). `insufficient` → emit a `Fact` with `Status:"insufficient"`,
  `Value:"数据不足"`, `Reason` verbatim. `unsupported` (the 7 crypto ids) → skip entirely.
- **Unit handling**: format by `StockIndicator.Unit` — `%`→"42.0%", `x`→"41.2x",
  `price`→"$xxx", `""`→raw, **except `fundamental.fcf` which is `""` but DOLLARS** →
  `$1.2B` compact. (Same gotchas the K-line/Fundamentals cards already respect.)

### 2.3 Composer (LLM, degrades to Noop)

```go
// internal/research/compose.go

// Composer turns a FactSheet into per-section prose via a narrow Enricher slice.
type ResearchEnricher interface {        // narrow, testable (briefing.BriefEnricher style)
    Enabled() bool
    Brief(ctx context.Context, material, lang string) (string, error)
}

// Compose returns the FactSheet with per-section prose filled in. When enr is
// disabled or a call errors, prose stays "" and the data-only FactSheet is returned
// unchanged — NEVER an error to the caller (off the critical path).
func Compose(ctx context.Context, fs FactSheet, enr ResearchEnricher, lang string) FactSheet
```

The composer builds **one material string per ticker** (all five sections' facts,
formatted like `briefing.buildMaterial`, `briefing.go:166`) and makes **one
`Brief`-style call** that returns prose keyed by section. To keep it to one call
(under the ~85s budget — §7), the system prompt asks for a JSON object
`{"valuation":"…","fundamentals":"…",…,"overview":"…"}` using
`response_format:{type:"json_object"}` (the `TranslateTitles` idiom, `enrich.go:256`),
parsed back by key; any missing key → that section's prose stays "". This needs a new
`enrich` method — see §3.1.

### 2.4 Rules baked into assembly + prompt

- **De-dup**: PE/PB come ONLY from the indicator set (`fundamental.pe-ttm`/`pb`),
  `MarketCap` ONLY from `fundamentalsResp` math — never emit both so two slightly
  different numbers can't appear (gotcha: PE/PB exist in both places).
- **Missing data**: insufficient → `Fact` with `Value:"数据不足"`+`Reason`; a whole
  section with zero `ok` facts is **omitted** from `Sections` (frontend renders nothing).
  Never fabricate, never 0-as-value.
- **Negatives are meaningful**: NetIncome / FCF / ROE / earnings-growth can be < 0 —
  format the sign; PE for a loss-maker is intentionally "亏损/—", not 0.
- **Prompt guardrails** (extend `enrich.systemPrompt`): material-only; never invent or
  recompute a number; no buy/sell/target/valuation-call; attribute source type;
  neutral tone; "数据不足" when a section is thin.
- **Citation map**: each `Fact.Source`/`SourceURL` and each `SectionFacts.Citations`
  is set in Go from the known provenance (SEC EDGAR, Cboe, FINRA, House Clerk,
  ApeWisdom, etc.). The LLM is told the sources but writes no URLs.

### 2.5 LLM-disabled path (engineering-first, hard requirement)

`Compose` with a `Noop` enricher (LLM_API_KEY empty → `enrich.New` returns `Noop`,
`enrich.go:49`) returns the FactSheet with all `prose==""`. The endpoint returns
**200 with the full data-only report** — never 503, never 500. This mirrors
`getMyDigest` (rows always serve, Summary best-effort) rather than `getSummary` (503).
`/healthz` already exposes `"llm"` (`api.go:440`) so the frontend knows prose may be absent.

---

## 3. Backend plan

### 3.1 New package `internal/research`
- `factsheet.go` — the types in §2.1 + `Assemble` (pure, the unit-test target).
- `compose.go` — `Compose` + the material builder + JSON-keyed prose parse.
- `format.go` — unit-aware formatters (`%`/`x`/`price`/FCF-dollars/compact-USD),
  reused from the same logic the Fundamentals card uses.
- `_test.go` — table-driven: fakes for every `Sources` interface; assert status-gating
  (insufficient never yields a Value), unit formatting (FCF dollars vs `%`), section
  omission, and that `Compose` with a disabled enricher == data-only FactSheet.

### 3.2 New `enrich` method (one new capability, same idiom)
Add to `enrich.Enricher` (`enrich.go:23`) + both impls:
```go
// ComposeReport writes per-section research prose from a pre-built material string,
// returning a section-key→prose map. Same guardrails (material-only, no numbers,
// no advice). Noop returns ErrDisabled.
ComposeReport(ctx context.Context, material, lang string) (map[string]string, error)
```
Implemented exactly like `Brief` (`enrich.go:189`) but with `response_format:
{"type":"json_object"}` and a section-keyed system prompt (zh default + en variant).
`Noop.ComposeReport` returns `nil, ErrDisabled`. *(Alternative if we want zero
interface churn: reuse `Brief` with a richer material string and parse fenced output —
but a dedicated JSON method is more robust, matching `TranslateTitles`.)*

### 3.3 API endpoint — setter pattern (do NOT touch `api.New`'s positional signature)
- Add `researchCalc ResearchSource` field on `Server` + `SetResearch(src ResearchSource)`
  setter (mirrors `SetIndicatorCompute`, `api.go:1930`). `ResearchSource` is satisfied
  by a tiny `*research.Service` that holds the `Sources` + the enricher.
- New route: `mux.HandleFunc("GET /v1/stocks/{ticker}/research", s.getResearch)`
  (registered alongside `/indicators`/`/summary`, `api.go:359-361`).
- `getResearch` (new handler):
  1. `ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))`, `lang` from `?lang=en`.
  2. Cache check: `researchCache map[string]researchEntry` keyed `ticker|day|lang`
     (ET day via `summaryDay()`, `api.go:2348`), guarded by `researchMu`, single-flight
     via `researchInflight` — **copied verbatim from `getSummary`** (`api.go:2373-2419`).
  3. Daily generation cap `researchDailyCap` (start 80/day across all tickers; smaller
     than `summaryDailyCap=150` since R2 is a bigger call) — over cap → still serve the
     **data-only** FactSheet (assemble is cheap, no LLM), so the cap only gates *prose*,
     not the report. Failed prose gen refunds the counter (like `getSummary`, `api.go:2434`).
  4. Assemble → (if enabled & under cap) Compose → cache → respond.
  5. `AsOf` empty (no data at all for a real-but-empty ticker) → still 200 with empty
     sections + `nextEarningsLabel`-style headline. 404 only if the ticker is invalid.

### 3.4 Response JSON shape
```json
{
  "ticker": "AAPL",
  "name": "Apple Inc.",
  "as_of": "2026-06-12",
  "price_label": "$190.12 · alpaca · delayed · regular",
  "generated_at": "2026-06-13T11:02:00Z",
  "model": "deepseek-chat",                       // s.enrich model, "" when disabled
  "llm": true,                                    // prose present
  "disclaimer": "AI 生成 · 数字来自公开数据 · 非投资建议",
  "sections": [
    {
      "key": "valuation",
      "title_zh": "估值", "title_en": "Valuation",
      "facts": [
        {"key":"market_cap","label_zh":"市值","value":"$2.9T","raw":2.9e12,"unit":"USD","status":"ok","source":"SEC XBRL × delayed quote","source_url":"https://www.sec.gov/cgi-bin/browse-edgar?CIK=AAPL"},
        {"key":"pe","label_zh":"市盈率(P/E)","value":"31.2x","raw":31.2,"unit":"x","status":"ok","source":"SEC XBRL FY2024","as_of":"2024-09-28"}
      ],
      "prose": "估值处于其历史区间偏高位…(qualitative, no numbers invented)",
      "citations": [{"label":"SEC EDGAR · companyfacts","anchor":"#fundamentals"}]
    }
    // fundamentals / technical / flows / sentiment / overview …
  ]
}
```
`model` is read from the configured `LLM_MODEL` (surface for transparency; matches the
"AI 生成" labeling). When `llm:false`, every `prose:""` and the frontend renders the
data-only view.

### 3.5 Wiring in `cmd/server/main.go`
Construct `research.NewService(research.Sources{...}, enricher)` after the indicator
computer is built (`main.go:471-477`) — reuse the same `bars`/`fundCache`/`store`/
`congressCache`/`thirteenFCache`/`shortVolumeCache`/`shortCache`/`sentimentCache`/
`optionsCache`/`store` handles already in scope — then `apiServer.SetResearch(svc)`
near the other setters (`main.go:447-450`). Gate nothing on `enricher.Enabled()`: the
service must serve the data-only report regardless (off the critical path).

---

## 4. Frontend plan

### 4.1 Where it renders — a new StockView section/tab
`StockView.tsx` already stacks AI + data cards above its tab strip
(`AISummaryCard` at line 538, `FundamentalsCard` 536, `IndicatorsPanel` 549,
`OptionsCard` 552) and has a tab list (`TABS_ANON`/`TABS_AUTH`, lines 81-82).

**Decision**: add a **"深度研报 / Research"** entry. Because R2 is long-form, render it
as a **dedicated tab** (added to both `TABS_ANON` and `TABS_AUTH`, public — good for
SEO) rather than another always-open card, so the heavy LLM fetch only fires when the
user opens it. New component `ResearchReport.tsx`:
- Fetches `GET /v1/stocks/{ticker}/research?lang=…` via a new `getResearch` in
  `web/src/lib/api.ts` (mirror `getStockIndicators`, `api.ts:1577`).
- Renders each section: a Chinese `title_zh` header, a compact **facts grid** (label +
  value chip, "数据不足" muted chip for `status!="ok"` with a tooltip = `reason`), the
  `prose` via the existing `<Markdown>` component, and a `citations` footer row.
- Reuses the violet "AI" badge + `Sparkles` icon + loading skeleton from `AISummaryCard`.

### 4.2 Single-language exception (justify)
Per CLAUDE.md the owner principle is *"a single-language-only value defaults to
ENGLISH"* (line 406) and `LocalizedTitle.tsx` swaps zh titles for zh users. R2 is the
**explicit exception**: the report *body is Chinese-by-design* (the product is
Chinese-first, the LLM prompt is zh-default, and `getSummary`/`BriefingCache` already
ship zh-first prose). So: **English-default chrome** (tab label "Research", section
labels carry `title_en`, fact labels carry `label_en`) wrapping a **zh report body**
when the UI is zh; when the UI is en we pass `?lang=en` and the backend returns en
prose (falling back to zh text if the en gen is empty — same as `getBriefing`,
`api.go:2133`). This is consistent with how data (headlines, company names) already
"shows as-sourced" while chrome is translated.

### 4.3 Citations → deep-links
- `Citation.Anchor` (e.g. `#fundamentals`) scrolls to the matching existing card on the
  same page (`FundamentalsCard`/`IndicatorsPanel`/`OptionsCard`) — add `id=` anchors to
  those components.
- `Citation.URL` (SEC filing, `/congress/member/{slug}`, fund page) opens the source —
  reuses the existing deep-link targets (`CongressChip`, `WhalesChip`, `ShortChip`).

### 4.4 States
- **Loading**: labeled animated skeleton (copy `AISummaryCard`'s loader — an LLM call
  takes seconds).
- **Empty** (real ticker, no data yet): show the section scaffolding with "数据不足"
  chips; the report never 500s.
- **LLM disabled** (`llm:false`): render the **data-only** report (facts grids + a small
  "AI 摘要暂未启用 / AI summary unavailable" note) — no broken card.
- **Over daily cap**: data-only with prose absent (silent — same UX as disabled).

### 4.5 Labeling (mandatory)
Every report shows **"AI 生成 · 数字来自公开数据 · 非投资建议"** (+ en variant) at the
top and the `disclaimer` field at the bottom — the second-layer disclaimer that
backs the prompt guardrails. Delayed-quote/Cboe/FINRA freshness labels travel inside
each `Fact.Source`/`AsOf` and must be shown (redistribution posture, §7).

---

## 5. Monetization-platform (DEFERRED — plumbing only)

Per CLAUDE.md (line 89): **monetization is deferred — build NO paywall/pricing/payment
now.** R2 only lays the *plumbing*:
- The per-(ticker, ET-day, lang) cache + `researchDailyCap` generation counter already
  exist for cost control; they are the same mechanism a future free-tier limit would use.
- Future gate (LATER, not now): free = N reports/day + current-day only; paid = unlimited
  + history (persist FactSheets to the store instead of in-memory) + PDF export. None of
  this is built in P0. No per-user counter, no Stripe, no tier field. Keep R2 free + the
  quotes delayed-labeled so it stays redistribution-safe (§7).

---

## 6. Phasing

### P0 — vertical slice (one `/loop` tick)
The thinnest end-to-end report that proves the architecture:
1. `internal/research` package: `FactSheet`/`Fact`/`SectionFacts`/`Citation` types +
   `Assemble` (pure) over the indicator set + fundamentals + quote (the three richest,
   already-wired sources) → the three sections **估值 / 基本面 / 技术面** only.
2. `enrich.ComposeReport` method (+ `Noop` impl + zh/en section-keyed prompt) and
   `research.Compose` (degrades to data-only on disabled/error).
3. API: `SetResearch` setter + `GET /v1/stocks/{ticker}/research` handler with the
   `getSummary`-cloned cache/single-flight/daily-cap, data-only when LLM off.
4. Wiring in `main.go` (reuse in-scope caches) + `getResearch` returns the 3-section report.
5. Frontend: `ResearchReport.tsx` + a "Research" tab in `StockView` + `getResearch` in
   `api.ts`; facts grid + Markdown prose + disclaimer + loading/empty/disabled states.
6. Tests: `internal/research/*_test.go` (status-gating, unit formatting incl. FCF,
   section omission, disabled==data-only); `go build ./... && go vet ./... && gofmt -l .`
   clean + `cd web && npm run build`.

### Follow-ups (later ticks)
- F1: add **资金面** (congress + 13F + insider + options + short) and **情绪面**
  (sentiment + signals + news/social) sections to `Assemble` + the composer prompt.
- F2: add the **概览/结论** synthesis section (composed last over all five).
- F3: citation deep-link anchors on the existing cards; en-prose polish.
- F4 (monetization-adjacent, deferred): persist FactSheets to the store for history;
  PDF export; per-user free-tier counter — only when the owner greenlights paid.

---

## 7. Risks / open questions

**Risks (mitigated in design):**
- **Redistribution**: R2 injects delayed quotes (Alpaca/Yahoo = RED), Cboe (~15m), FINRA
  (display-only). Mitigation: report uses *derived/qualitative* values, every number
  carries its `Source`+freshness label, raw FINRA rows are never bulk-exposed (only the
  derived `ShortPct`), and the report is free. This is a future-paid-tier gate, not a
  now-blocker, but the labeling must ship in P0.
- **LLM cost/rate**: R2 is a bigger call than `getSummary`. Mitigation: per-(ticker,day,
  lang) cache + single-flight + `researchDailyCap` (~80/day) + one call per report
  (JSON-keyed sections) sized under ~85s; failed gen refunds the counter; in-memory
  cache resets on redeploy (first visitor re-pays — acceptable, inherited from existing
  caches).
- **No retry/failover** (enrich has none): a provider outage degrades R2 to data-only —
  acceptable per engineering-first.
- **Congress under-coverage**: `ByTicker` returns nil both for "none traded" and "PTR
  parsing disabled (no pdftotext on box)" — never assert "no member traded this"; phrase
  as "no disclosed trades found".

**Open questions for the owner:**
1. `researchDailyCap` value — 80/day a sane starting backstop, or tune for the live
   DeepSeek/OpenRouter budget?
2. One JSON call returning all sections vs one call per section — design assumes ONE
   (cheaper, fits the 85s budget); confirm DeepSeek reliably returns valid keyed JSON at
   this size, else fall back to per-section `Brief` calls (more calls, more cost).
3. Research tab **public** (good for SEO, like the data cards) vs login-gated — design
   assumes public + free; confirm given monetization is deferred.
4. Persist generated reports to the store (durable history, survives redeploy, enables a
   future paid "history" feature) vs in-memory only (P0 default, matches existing caches)?
