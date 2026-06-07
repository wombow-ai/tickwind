# Tickwind — Future Features Research (2026-06)

Parallel subagent research dispatched during the polish loop. Agents: **A** competitor
gaps, **B** free data sources, **C** AI/LLM design, **D** polish audit. (D socket-failed at
launch — polish is being done inline instead.) This doc is the synthesis input for ROADMAP.

---

## C — AI/LLM feature design (ranked by value / (cost + legal risk))

**Shared infra to build once (every later feature depends on these 3):**
- **Content-hash cache** — key = `sha256(model + prompt_version + sorted(source_doc_ids))`,
  stored in Postgres/pgvector. Immutable filings ⇒ summarize **once ever**, $0 thereafter.
- **Daily budget guard** — token/USD counter (mirror `internal/alphavantage` self-budget:
  daily cap + cooldown + "day spent" flag); on exhaustion serve cached / 503, never overspend.
- **Source-citation schema + validator** — every output carries `sources:[{type,id,url,quote}]`;
  reject any claim lacking a source; structured/validated JSON before persist.
- Model tiering: nano/Flash-Lite tier (~$0.10/$0.40 /1M) for extraction/digests; Haiku-class
  ($1/$5) only for red-flags/RAG; **never frontier on these paths**. Batch API (−50%) for all
  non-interactive jobs. Prompt-cache the frozen system+disclaimer prefix.

**Guardrails (shared):** extractive-first (quote/cite, don't synthesize); **NO price targets /
buy-sell** (prompt + post-hoc reject); store provenance (model, prompt-ver, source-ids, ts) for
auditability. **Legal:** disclaimers alone don't shield (FINRA) — protection is *structural*:
descriptive "summary of public sources," never prescriptive/personalized; per-user briefs stay
neutral-info ("X filed an 8-K", never "you should…"); UGC rollups stay §230-neutral (attribute,
don't adopt). Standard disclaimer string defined.

**Ranked features:**
1. **8-K/10-K/10-Q plain-English summary + filing diff** — 🏆 SHIP FIRST. Inputs = existing
   `internal/sec`+`edgar` filing text (already fetched, stored 730d). Immutable ⇒ near-zero
   recurring cost, smallest hallucination surface, lowest legal risk; compounds the SEC/Form-4
   edge that already powers the Opportunity board. Diff = current vs prior same-type filing.
2. **Per-stock news digest** — 5 cited bullets atop `NewsTimeline`; existing Finnhub news;
   batch + content-hash cache (Finnhub = attribution-required, the cite design satisfies it).
3. **Daily "what changed" watchlist brief** — retention/DAU driver; stitches cached #1/#2 +
   `Signal` pulse + `InsiderBuy`; near-zero marginal cost; cache per-watchlist-membership (not
   per-user). Framing-sensitive: strictly neutral info about followed tickers, never advice.
4. **Filing red-flag scanner** — muted "things to note" chips (going-concern, auditor change,
   material weakness, dilution/shelf, related-party), each with a verbatim quote+location;
   Haiku-class; strictest extractive validation; descriptive ("detected disclosures"), not "risky".
5. **Semantic search / RAG over a stock's filings+news** — pgvector already in the stack; embed
   chunks once (cheap, cached); answers Haiku-class, rate-limited (mirror comments 10/10min),
   refuse advice-shaped questions.
6. **Earnings-call summary** — ⛔ BLOCKED: no transcript source ingested yet (needs a feed +
   redistribution-license clearance). #1-class workload once available.
7. **Sentiment rollup over social** — low marginal value (numeric buzz/sentiment chips already
   shipped; "精不在多"), highest UGC/§230 framing care → ranked last; keep thematic, never directional.

**Build order:** #1 → #2 → #3 → #4 → #5, building the 3 shared subsystems alongside #1.
Pricing/legal confirmed via the claude-api skill + web research (Haiku 4.5 $1/$5, Sonnet 4.6
$3/$15, Batch −50%, prompt caching, citations API; FINRA/SEC: grounding + recordkeeping expected).

---

## A — Competitor gaps & new feature opportunities

Ranked across 12 ideas / 3 tiers (vs TradingView, Webull, Seeking Alpha, Koyfin, Fintel,
Unusual Whales, Quiver, Stocktwits, Finchat, Simply Wall St, Yahoo, Public). Features reusing
already-ingested data or public-domain gov data rank higher.

**Tier 1 — high value, owned or rock-solid free data:**
1. **Congressional/politician trading board ("Capitol Flow")** — House Clerk XML + Senate EFD
   (STOCK Act PTRs), free public-domain gov records; new `internal/congress`. **M.** Top
   virality driver (Unusual Whales/Quiver signature); slots into the evidence-first board
   pattern; paywall candidate. Low risk. Start House-only (Senate EFD = harder/PDF).
2. **Fundamentals/Financials tab (XBRL)** — SEC EDGAR companyfacts/frames API (free, keyless);
   **extends existing `internal/sec`.** **M** (normalize us-gaap tags → canonical schema +
   ratios; reuse edgartools patterns). Closes the biggest functional gap (no statements today).
   **GREEN.**
3. **Price & event alerts (push/email)** — Tickwind's OWN data; new `store.Alert` + evaluator
   goroutine (mirrors pruner/ingestor). **M.** The #1 retention mechanic across every
   competitor; natural premium tier. Low risk (web-push free).
4. **Earnings/dividend/economic calendar** — Finnhub calendar (token already wired) + Nasdaq
   keyless fallback; overlay onto `NotesCalendar`/watchlist. **S–M.** High-freq return trigger.
   YELLOW (Finnhub personal-use; same plan as news).

**Tier 2 — strong differentiators, modest new pipelines:**
5. **Short interest & off-exchange short volume** — FINRA free files (bi-monthly SI + daily
   short vol); new `internal/finra`; float from dei shares already pulled. **M.** "Squeeze
   radar"; pairs with WSB/Hot. Low risk (label cadence honestly).
6. **Institutional / 13F ownership tracker** — SEC 13F via EDGAR (extends `internal/sec`).
   **M–L** (13F-HR XML + CUSIP→ticker mapping is the fiddly part; start with marquee filers).
   Deepens smart-money theme. GREEN (quarterly/45-day-lag — label staleness).
7. **Analyst ratings & price-target consensus** — Finnhub recommendation/price-target/
   upgrade-downgrade (token wired). **S.** High-demand data Tickwind lacks. YELLOW (most
   license-sensitive Tier-2 — value-added vendor product).
8. **Signals-aware screener** — own data; screen on buzz/insider/short facets pure-fundamental
   screeners don't combine. **M.** Edge = the social+insider+signal layer. Low risk (ship few
   correct filters first, "精不在多").

**Tier 3 — pipeline-heavy / lower-fit / higher-risk:**
9. **Earnings-call transcripts (+LLM TL;DR)** — ⛔ RED on data (S&P Capital IQ / Finnhub-paid;
   no clean free feed; don't scrape). Defer or pay; cheap safe angle = parse the 8-K earnings
   press release (free via EDGAR).
10. **Community upgrade** (trending tickers from on-site activity, threads, bull/bear, likes) —
    own data; extends Comments (§230/DMCA groundwork done). **M.** Network-effect retention but
    cold-start risk → sequence after data features draw traffic.
11. **Paper-trading / virtual portfolio P&L** — own quotes + per-user `store.Position`; reuses
    User-DB split + auth. **M.** Strong engagement; honors "never touch a funded brokerage"
    (pure simulation). Low risk.
12. **Gov alt-data (contracts/lobbying/patents)** — USASpending + Senate/House LDA + USPTO
    PatentsView (free public); 3 small clients + entity→ticker resolution. **L.** Thin per-ticker
    signal; sequence later. Low risk.

**Cross-cutting:** the **"Follow the Money" suite** (#1 Congress, #2 XBRL, #5 short interest,
#6 13F, #12 gov alt-data) runs on free public-domain gov data (SEC/FINRA/Congress/USASpending) —
safest footing for a paid product, several extend the owned EDGAR client → Tickwind's most
defensible lane. **License flags:** #9 RED; #4/#7 YELLOW (Finnhub; #7 most sensitive). **Owned-data
engagement (no new license):** #3 Alerts, #8 Screener, #10 Community, #11 Paper-trading. **Suggested
sequence:** #3 Alerts + #2 Financials → #1 Congress + #5 Short interest → #6 13F + #8 Screener →
#4/#7 (when Finnhub tier sorted) → #10/#11/#12 → #9 (only with budget).

---

## B — Free / redistribution-safe data sources → features

Legal anchor: US federal-gov works are public domain (17 U.S.C. §105) → the whole
SEC/Treasury/FDA/USAspending/ClinicalTrials/BLS/BEA/Census cluster is **GREEN at source**.
Risk classified for a future PAID product.

**Ranked shortlist (redistribution-clean first):**
1. **SEC 13F institutional holdings** — GREEN, keyless (quarterly Data Sets TSV + EDGAR XML);
   reuses `internal/sec`. ~45-day lag → institutional-ownership panel + "whale moves" board.
2. **SEC Company Facts / XBRL frames** — GREEN, keyless JSON (data.sec.gov); dei-parsing pattern
   already in place. Near-real-time → fundamentals tab + P/E,P/S valuation.
3. **US Treasury yield curve** — GREEN, keyless XML; tiny build → macro rail (2y/10y/30y +
   10y–2y spread recession chip).
4. **SEC EFTS full-text search** — GREEN, keyless (efts.sec.gov) → primary-source filing keyword
   alerts / themes.
5. **Wikimedia Pageviews** — GREEN/CC0; the redistribution-safe **Google-Trends replacement** →
   retail-attention line / Hot-list input.
6. **FINRA short interest + short volume** — **YELLOW → RED for bulk redistribution** (FINRA owns
   the compilation IP); fine as a displayed number/signal with "Source: FINRA" → short-interest %
   / days-to-cover + squeeze watch. Confirm a FINRA data agreement before redistributing files.
7. **GDELT 2.0** — GREEN + mandatory citation; 15-min news volume/tone, multi-language (good for
   TW/HK) → global buzz/tone line.
8. **ClinicalTrials.gov + openFDA** — GREEN, keyless → biotech catalyst / recall tracker.
9. **Wikidata + OpenFIGI** — GREEN (CC0/MIT) → symbology/entity glue (company↔ticker↔CUSIP↔ISIN);
   enables the 13F / contract / pageview mappings.
10. **BLS / BEA / FRED** — GREEN data (prefer BLS/BEA origin over FRED for copyright-clean
    redistribution; FRED needs a free key + attribution) → macro prints.
11. **Earnings calendar** — YELLOW via Nasdaq front-end JSON (no redistribution grant); for a paid
    product source the *dates* cleanly (SEC 8-K / IR).
12. **ETF holdings** — GREEN via SEC N-PORT (monthly, ~60-day lag); issuer daily files YELLOW →
    "which ETFs hold this stock".
13. **USAspending** — GREEN; niche federal-contract signal.

**RED (avoid for redistribution):** Google Trends (ToS-violating scraping → use Wikimedia
Pageviews), CoinGecko free tier (commercial use forbidden; even paid = no syndication).
**Net:** the biggest clean wins extend the SEC/EDGAR backbone (13F, XBRL, EFTS, N-PORT) + two tiny
keyless adds (Treasury curve, Wikimedia pageviews).

---

## Synthesis & recommended build sequence (A + B + C)

**Strong convergence:** the **SEC/EDGAR backbone is Tickwind's defensible, redistribution-safe
lane** — three independent agents land there (XBRL fundamentals, 13F, EFTS, filing-AI all reuse
the owned `internal/sec` client on public-domain data). **Alerts** are the single highest
retention-per-effort play, on owned data.

**Recommended order (free/GREEN data + highest value — owner to greenlight which):**
1. **Price & event alerts** (A#3) — own data; #1 retention mechanic; premium-tier seed. **M.**
2. **Fundamentals / Financials tab** (A#2 / B#2, XBRL, GREEN) — closes the biggest functional gap;
   extends `internal/sec`. **M.**
3. **AI filing summary + diff** (C#1) — immutable → cacheable → ~$0 recurring; lowest legal risk;
   builds the shared AI infra (content-hash cache · budget guard · citation schema). **M.**
   Needs an `LLM_API_KEY` to activate.
4. **Congress trading board** (A#1, gov public-domain) — viral; reuses the evidence-card pattern.
   **M** (House-first; Senate EFD harder).
5. **13F institutional holdings** (A#6 / B#1, GREEN) — "smart-money" suite alongside the insider
   board. **M–L** (CUSIP→ticker mapping is the work; OpenFIGI/Wikidata help).
6. **FINRA short interest** (A#5 / B#6) — squeeze radar; ship as a displayed signal, gate bulk
   redistribution behind a FINRA agreement. **M.**

Then: signals-aware **screener** (A#8), **earnings calendar** (A#4 / B#11, clean sourcing),
**Treasury macro rail** (B#3, tiny), **Wikimedia attention** (B#5), **community upgrade** (A#10),
**paper-trading** (A#11). Defer: **earnings-call transcripts** (C#6 / A#9 — RED data, needs a paid
feed); **gov alt-data** (A#12, thin signal).

**Monetization:** the "Follow the Money" suite (Congress / 13F / short-interest / financials) is
all GREEN public data → safe behind a future paywall. The standing **RED blocker is unchanged**:
live **quote** redistribution (Alpaca/Yahoo) still needs a licensed vendor before charging.

**Note:** the polish-audit agent (D) socket-failed; polishing is being done inline (KLineChart
view-preservation shipped this tick).
