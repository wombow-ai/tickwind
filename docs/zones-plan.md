# Tickwind 专区 (Investment-Theme Zones) — Plan

> Produced by an autonomous research workflow (2026-06-20, owner away). 4 framework researchers (WebSearch) + a synthesis architect. **Owner makes the final split/merge call; the strong recommendation is HYBRID.**

## Decision: **HYBRID**

The four research frameworks settle it. Three of them (Jensen's 5-layer cake, TSMC's 上中下游, the 13-layer 卡脖子 stack) are THE SAME OBJECT seen at three resolutions: a vertical, chokepoint-ranked AI build-out stack. The fourth (the "10x" theme map) is broader — and critically, its single largest, deepest sub-section IS that same AI value-layer cake, sitting as a sibling next to Space / Quantum / Medical themes. So the data itself is hybrid-shaped: one editorial engine ("curated zone = layers/sub-themes → ticker list + rationale, with chokepoint flags"), instantiated once as the AI flagship (richest, deepest, the marquee narrative) and N more times as lighter 10x theme siblings (Space, Quantum, GLP-1/obesity, Gene-editing, AI-software, etc.).

SPLIT is wrong: it duplicates rendering, JSON-LD, i18n, sitemap, anti-hallucination plumbing, and the curation schema twice for two things that are structurally identical — a curated editorial tree of (group → ticker + rationale) rendered with live Go numbers. The owner's bar is "精不在多 / reuse OSS patterns"; two engines violates it.

MERGE-into-one-flat-feature is also wrong: the AI zone needs DEPTH the 10x themes don't (5+ layers, chokepoint sub-clusters, a "who owns the bottleneck" editorial spine), and the speculative 10x buckets (quantum/eVTOL/gene-editing) need a prominent SPECULATIVE disclaimer the AI flagship's chokepoint layers don't. One flat list can't honor both. They must be siblings of differing weight, not equals.

HYBRID gives exactly that: ONE zone engine + schema + `/zone/[key]` pSEO route (cloned from topic/screen), where each zone declares its own depth (AI = many layers w/ chokepoint flags; a 10x theme = a few sub-themes), its own `speculative` flag, and its own ticker rationales — all curated structure, all numbers live from Go. AI is the flagship zone; the 10x themes are lighter siblings reusing the same renderer. This is the prior hypothesis, but it's the right answer because the framework evidence is literally a fractal of one structure.

## Engine design

# Zone Engine — design

## 0. One-line thesis
A **zone** is a curated editorial tree — `zone → layers/sub-themes → [ticker + rationale + flags]` — rendered as a pSEO page that pulls **every number live from Go**. AI is the flagship zone (deep, chokepoint-flagged); the "10x" themes (Space, Quantum, GLP-1, Gene-editing, AI-software) are lighter siblings reusing the identical engine. This is a direct deepening of the proven `presets.ts` + `topic/[key]` shape — one level of nesting added (layers), zero new rendering primitives.

## 1. Data model (the curation schema)
Mirror `ScreenPreset` exactly, nested one level. Bilingual fields inline (EN-first), every field hand-authored, **no numbers ever**.

```ts
/** One curated stock inside a layer. NO numbers — only identity + editorial rationale. */
interface ZoneTicker {
  ticker: string;                 // canonical (dot form, e.g. BRK.B) — must resolve in Go universe
  rationaleEn: string;            // one line, editorial — why it sits in this layer
  rationaleZh: string;
  chokepoint?: boolean;           // owns-the-bottleneck (monopoly/near-monopoly node)
  speculative?: boolean;          // pre-profit / binary / extreme-multiple — drives the disclaimer
}

/** One layer / sub-theme inside a zone. */
interface ZoneLayer {
  key: string;                    // kebab, stable, used as the in-page anchor
  titleEn: string; titleZh: string;
  blurbEn: string; blurbZh: string; // 1–2 sentence editorial framing of the layer
  isChokepoint?: boolean;         // the layer itself is a bottleneck (e.g. EUV, CoWoS, HBM)
  tickers: ZoneTicker[];
}

/** A zone = the top-level curated config. */
interface Zone {
  key: string;                    // /zone/{key} slug — stable, kebab
  kind: 'ai-flagship' | 'tenx-theme';
  titleEn: string; titleZh: string;
  descEn: string;  descZh: string; // meta description + intro
  speculative?: boolean;          // whole-zone disclaimer (quantum, eVTOL, gene-editing)
  layers: ZoneLayer[];
}
```

Key properties:
- **Strictly additive over `ScreenPreset`** — same bilingual-inline convention, same "config is the editorial spine, Go owns numbers" split the codebase already proves.
- `chokepoint`/`speculative` are the only "editorial judgment" flags; they drive badges + disclaimer copy, never a number.
- A zone's *depth is self-declared*: AI carries 5–7 layers; a 10x theme carries 2–4 sub-themes. Same type, different fill — that is the whole point of HYBRID.

## 2. Where the curation lives — RECOMMENDATION: a web config (`web/src/lib/zones.ts`), NOT Go
Recommend **`web/src/lib/zones.ts`** (clone of `presets.ts`), explicitly **NOT** `internal/zones`.

Reasons, grounded in the existing split:
1. **It is pure editorial structure with bilingual copy** — identical in nature to `presets.ts` (also web-side) and unlike `internal/topics` (which *derives* from live data). Curation that a human hand-tunes belongs next to the other hand-tuned web catalog.
2. **Zero new Go endpoint, zero deploy coupling.** The page already calls existing Go endpoints per ticker (`/quote`, `/fundamentals`). Editing a rationale or adding a ticker is a **frontend-only change → Vercel auto-deploy**, no VPS/SSH deploy, no backend restart. Given the documented SSH flakiness + "deploy backend BEFORE frontend" gotchas, keeping curation web-side removes the whole backend-deploy hazard from routine zone edits.
3. **Build-time `generateStaticParams` needs the catalog synchronously** — exactly how `presets.ts` feeds the screen route. A Go file would force a fetch in `generateStaticParams` (the topic route does this and must defensively try/catch); a static TS catalog is simpler and never breaks the build.
4. **Anti-hallucination is unaffected** by location: the config has no numbers regardless of where it lives. The number-safety guarantee comes from the *render path* (§4), not the config's host.

(If a zone ever needs server-side ranking — e.g. "sort layer by live market cap" — add a thin Go helper later; not needed for v1. The ticker→number join already exists via the quote/fundamentals endpoints the screen+stock pages use.)

## 3. Rendering / pSEO reuse — `/zone/[key]` (+ `/zone` hub)
Clone the `topic/[key]` + `screen/[preset]` shape verbatim; the only new thing is the extra nesting loop (layers → tickers).

- **Route:** `web/src/app/[locale]/(main)/zone/[key]/page.tsx` + a `/zone` hub (like `/screen`).
  - Pick `/zone` over `/themes`: shorter, matches the owner's "专区" framing, and leaves `/themes` free if a lighter taxonomy is ever wanted.
- **`generateStaticParams`**: `LOCALES × ZONES` (synchronous, from `zones.ts`) — e.g. 6 zones × 2 = 12 prerendered, same pattern as screen presets. New/rare zones stay ISR via `dynamicParams`.
- **`generateMetadata`** per-locale: `title:{absolute}`, `description`, `alternates: langAlternates(path, loc)`, `ogImageMeta({lang: loc, eyebrow: zh?'专区':'Zone', title, subtitle})` — byte-identical to the screen route.
- **Single-locale render** chosen from the `[locale]` segment (no dual-render). h1 = zone title; per-layer `<section id={layer.key} className="scroll-mt-20">` with the layer blurb + a grid of ticker cards, each `<LocalLink href={'/stock/'+ticker}>` (internal-linking + crawl discovery, exactly like topic's related-tickers grid).
- **Live numbers per ticker (the only data fetch):** reuse the **batched** `GET /v1/bars?tickers=…` + the shared `useQuotes` EventSource the board already uses — collect every ticker across all layers, dedupe, one batched request, hydrate price + day-change client-side. Fundamentals (market cap / P/E) optional per-ticker via the existing `/v1/stocks/{t}/fundamentals` on hover/expand. **No new Go endpoint.** Server HTML stays crawlable (ticker, name, rationale, layer structure); numbers paint client-side just like every other Tickwind list — zero crawl impact, and a down API degrades to "—", never a 500 (the established pattern).
- **JSON-LD**: `CollectionPage` → nested `ItemList` per layer (or one flat `ItemList` of all tickers) + `BreadcrumbList` (Tickwind → Zones → zone), **all `item`/`url` locale-prefixed** (the FIXED pattern — never bare-path). Same as topic/screen.
- **ISR** `revalidate = 1800` (zones change slowly; the numbers are client-live anyway so the HTML cache window is generous).
- **noindex-when-thin**: if a zone resolves to <3 valid tickers (e.g. catalog typo) → `robots:{index:false, follow:true}`, fail-open (only a definitive empty zone deindexes), mirroring the topic + `/stock` guards.
- **Sitemap**: add `ZONES × LOCALES` locs to `app/sitemap.ts` (a handful of high-authority pages); add a `/zone` hub entry + a TopNav "More ▾" / Footer entry point.
- **Speculative/chokepoint badges**: `isChokepoint`/`chokepoint` → a small "瓶颈 / Chokepoint" pill; `speculative` (zone or ticker) → a prominent "投机性 / Speculative — pre-profit, binary" banner at the top of the zone and a per-ticker tag. Pure copy off the curated flags.

## 4. Anti-hallucination guarantee (the hard contract)
Same contract the whole platform runs on ("Go owns every number"):
1. **The config contains ZERO numbers** — only ticker symbols, editorial rationale strings, and boolean flags. There is no price/cap/share/multiple field to drift or fabricate. (This is enforceable by a tiny unit test: assert no `ZoneTicker`/`ZoneLayer` field matches a number-bearing regex except the ticker itself.)
2. **Every displayed number comes from the live Go endpoints** (`/v1/bars`, `/v1/stocks/{t}/quote`, `/v1/stocks/{t}/fundamentals`) — the identical sources the screen/stock pages already use, which are themselves anti-hallucination-audited.
3. **Ticker integrity at build:** the research JSON flagged many `uncertain_ticker` foreign listings (SK Hynix `000660.KS`, Samsung, Schneider, Lasertec `LSRCY` OTC, Zeiss/TRUMPF private, Innolight). Rule: **only US-listed / clean-ADR symbols go in the config**; foreign/private chokepoint owners appear as **name-only editorial context inside the layer blurb** (e.g. "HBM leader SK Hynix is Korea-listed — the US-investable proxy is MU"), never as a `ZoneTicker` with an invented symbol. A CI check resolves every `ticker` against `GET /v1/symbols` (the 16k universe) and fails the build on an unknown symbol — closing the "mapped a chokepoint to a non-existent ticker" risk the research explicitly warned about.
4. **No LLM anywhere in the zone path** — it is curated structure + Go numbers only, like `/screen` and `/topic`. Disclaimer footer reused verbatim ("Delayed data · For reference only · Not investment advice"), plus the speculative banner where flagged.

## 5. EN-first i18n
- Canonical `/en`, secondary `/zh` (per the English-first directive). Develop + verify the EN path first.
- Bilingual fields inline in the config (`titleEn/titleZh`, `rationaleEn/rationaleZh`, …) — same as `presets.ts`; **single-value-only fields default to English** (the owner principle).
- `langAlternates` emits per-locale canonical + `{en, zh-CN, x-default}`; single-locale render so `/en/zone/ai` and `/zh/zone/ai` are genuinely distinct crawlable HTML (Stage-2 pSEO pattern). OG card per-locale `lang`.
- Ticker symbols, company names, source labels render as-sourced (not translated), matching every other data surface.

## 6. Build increments (each: verify → commit → deploy → live-verify; frontend-only unless noted)
1. **Schema + AI flagship config**: `web/src/lib/zones.ts` with the `Zone`/`ZoneLayer`/`ZoneTicker` types + the AI zone (5 layers from Jensen's cake, chokepoint flags on Chips/EUV/CoWoS/HBM). Unit test: no-numbers-in-config + (CI) symbols-resolve check.
2. **`/zone/[key]` page + `/zone` hub**: clone `screen/[preset]` + `screen/page.tsx`; render layers→tickers grid, batched `useQuotes`/`/v1/bars` hydration, JSON-LD, ISR, noindex-thin. EN-first verify, then zh.
3. **Sitemap + nav entry**: add zones×locales to `sitemap.ts`; TopNav "More ▾" + Footer + a cross-link hub on `/zone`.
4. **10x theme siblings**: add Space, Quantum (speculative), GLP-1/obesity, Gene-editing (speculative), AI-software zones — config-only additions, the engine already renders them. Speculative banner verified on quantum/gene-editing.
5. **(Optional, later)** per-ticker fundamentals expand (market cap / P/E from the existing endpoint) + a "chokepoint-only" filter chip; thin Go ranking helper only if server-side sort-by-cap is ever wanted.

## 7. Why this is the minimal, owner-aligned build
- **One engine, one schema, one route** — reuses `generateStaticParams`/`generateMetadata`/`langAlternates`/single-locale-render/JSON-LD/ISR/noindex/sitemap wholesale. Honors "精不在多" + "reuse proven patterns."
- **Curation web-side** → routine zone edits are Vercel-only, sidestepping the documented backend-deploy/SSH hazards.
- **Anti-hallucination is structural** (config has no numbers; Go owns all) — no new audit surface.
- **AI flagship + 10x siblings** drop out of one config by varying depth + the speculative flag — the HYBRID recommendation falls out of the data, not imposed on it.

## Top-level zones

### `ai` — AI 专区 — The AI Stack (flagship)  _(ai-flagship)_

The flagship zone. Jensen's 5-layer cake as the editorial spine: Energy → Chips → Infrastructure → Models → Applications, with chokepoint flags on the Chips foundry/packaging + HBM + EUV sub-clusters. Layers: (1) Energy/Power & Cooling [VRT ETN GEV PWR CEG VST TLN; Schneider name-only], (2) Chips — CHOKEPOINT [NVDA AMD AVGO MRVL TSM ASML MU ARM AMAT LRCX KLAC; SK Hynix/Samsung name-only], (3) Infrastructure/AI-factory [SMCI DELL HPE ANET ALAB CRDO COHR LITE CIEN VRT], (4) Models [GOOGL META MSFT; OpenAI/Anthropic name-only private], (5) Applications [MSFT CRM NOW PLTR SNOW TSLA DDOG]. Sub-chokepoint badges: EUV (ASML), CoWoS/advanced-packaging (TSM, AMKR, ASX), HBM (MU). Deepest zone; the marquee narrative.

### `ai-software` — AI 应用与软件 — Applied AI & Software  _(tenx-theme)_

Lighter sibling carved from the AI cake's top layer for a standalone application-layer narrative (value-capture). PLTR CRWD SNOW NOW MSFT CRM DDOG. Higher-multiple, valuation-risk note (not chokepoint, not speculative-binary). Overlaps the flagship's Applications layer by design — different editorial angle (enterprise AI-budget share).

### `space` — 航天 专区 — Launch & Space  _(tenx-theme)_

Sub-themes: Launch & infra [RKLB LUNR; FLY VOYG KRMN speculative recent-IPOs], Satellites & D2D [ASTS PL GSAT], Defense-space primes [LMT RTX NOC LHX KTOS LDOS], eVTOL [JOBY ACHR — speculative, certification-gated]. Mixed-conviction; per-ticker speculative flags on new-IPO + pre-revenue names; primes are the quality anchor.

### `quantum` — 量子计算 专区 — Quantum Computing  _(tenx-theme)_

WHOLE-ZONE speculative banner (lottery-ticket risk/reward, extreme P/S). Pure-plays [IONQ RGTI QBTS QUBT — all speculative] + big-tech/enabling exposure [IBM GOOGL NVDA — de-risked option value]. Lead copy with the speculation disclaimer.

### `glp1-obesity` — GLP-1 / 减肥药 专区 — GLP-1 & Obesity  _(tenx-theme)_

Most fundamentally-grounded 10x theme (real large revenue today). Anchors [LLY, NVO ADR — profitable duopoly] + challenger [VKTX — speculative, binary on trials]. Single layer or split anchors/challengers; not a chokepoint zone.

### `gene-editing` — 基因编辑 专区 — Genomics & Gene Editing  _(tenx-theme)_

WHOLE-ZONE speculative banner (clinical-stage, binary trial-readouts). Editors [CRSP NTLA BEAM EDIT — speculative] + de-risked anchor [VRTX — profitable, commercial CRISPR partner]. Pair with AI-drug-discovery [RXRX SDGR TEM ABSI RLAY] as a sibling or a second layer.

## Open questions for the owner

- 路由用 /zone 还是 /themes?(我建议 /zone,更短、贴合「专区」叫法;/themes 留作以后更轻的分类)
- AI应用软件专区会和 AI 旗舰的 Applications 层重叠(PLTR/SNOW/NOW 等)——接受这种「同股票、不同编辑角度」的重叠,还是 10倍股侧不单独做软件专区、只保留旗舰内的那一层?
- 首批上线哪几个专区?我建议先 AI 旗舰 + 1-2 个兄弟专区(如航天 + GLP-1)验证引擎,量子/基因这种整区投机的后置;还是六个一次性全上?
- 投机性专区(量子、基因编辑、eVTOL)除了醒目声明,要不要在 sitemap 里 noindex,避免被当成荐股页?(我倾向正常 index + 强声明,但这是合规取向,你定)
- 专区入口放哪?TopNav「More ▾」+ Footer + /zone 中心页(复用 /screen 的交叉链接),还是也在首页 HomeHub 给一个模块位?
- 以后要不要把专区和现有付费墙挂钩(例如卡脖子瓶颈深度版 Pro-gated),还是专区永久免费做 SEO 流量入口?(我默认免费引流,符合现状)


---
## Appendix — framework research (layer/theme → tickers, the curation source)

### Framework: five-layer

_Five layers, bottom to top. (1) ENERGY — real-time intelligence requires real-time electricity; power generation, grid/electrical gear, and cooling are the hard physical floor. (2) CHIPS — processors that turn energy into computation: GPUs, custom AI ASICs, the foundry that fabs them, the litho tools, and HBM memory. (3) INFRASTRUCTURE ("the AI factory") — wiring tens of thousands of chips into one machine: rack/server integration, in-rack connectivity silicon, optical interconnect, and switch fabrics. (4) MODELS — foundation models across language, biology, physics, robotics. (5) APPLICATIONS_

**Layer 1 — Energy (power & cooling)** — The foundation. Huang: 'Intelligence generated in real time requires power generated in real time.' Spans electricity generation/supply, electrical distribution gear (transformers, switchgear, UPS/PDU), and thermal management (liquid cooling) for 50-130+ kW racks. Power availabil
- `VRT` Vertiv Holdings — Cleanest public pure-play on AI data center power management + thermal/liquid cooling; backlog doubled past ~$15B.
- `ETN` Eaton — Broadest electrical portfolio (power distribution, UPS/PDU); added liquid cooling via Boyd Thermal; data center orders up ~240% YoY.
- `GEV` GE Vernova — Supplies the electrons — gas turbines, grid electrification gear; booked record data center electrification orders.
- `PWR` Quanta Services — Builds and connects the power infrastructure (grid, substations, electrical construction) feeding new AI campuses.
- `CEG` Constellation Energy — Largest US merchant/nuclear generator; signs power-purchase deals directly with hyperscalers for 24/7 carbon-free baseload.
- `VST` Vistra — Independent power producer (gas + nuclear) positioned to sell firm capacity to data center load growth.
- `TLN` Talen Energy — Nuclear-heavy IPP; pioneered co-located data-center-at-the-reactor power deals.
- `ETApLF or local listing` Schneider Electric ⚠️named-only(no clean US ticker) — Major power/cooling systems vendor but FRENCH-listed (Euronext Paris, SU.PA); only thin US OTC ADRs exist — verify a tradable symbol in Go before listing.

**Layer 2 — Chips (compute silicon) — PICK-AND-SHOVEL CHOKEPOINT** **[CHOKEPOINT]** — Processors that transform energy into computation at massive scale: merchant GPUs, custom AI ASICs (XPUs), the CPU/IP, and the upstream that nobody can route around — the foundry (TSMC), EUV lithography (ASML), and HBM memory. This sub-cluster is THE pick-and-shovel bottleneck: T
- `NVDA` NVIDIA — Dominant merchant AI GPU + CUDA + NVLink; the reference platform for the entire stack.
- `AMD` Advanced Micro Devices — Instinct MI-series GPU + EPYC CPU; the credible second source to NVIDIA in training/inference accelerators.
- `AVGO` Broadcom — Co-designs custom AI ASICs (XPUs) for Google/Meta/OpenAI/Anthropic; ~70% of the custom-accelerator design market; also networking silicon.
- `MRVL` Marvell Technology — Other custom-ASIC house (Amazon Trainium, Microsoft Maia) + data-center networking/optical silicon; with AVGO ~95% of custom-XPU co-design.
- `TSM` Taiwan Semiconductor (ADR) — THE chokepoint: sole leading-edge foundry; ~90% of CoWoS advanced packaging that gates every NVDA/AMD accelerator. US ADR verified.
- `ASML` ASML Holding (ADR) — Monopoly on EUV lithography — no advanced AI logic chip exists without ASML's machines. US-listed (Nasdaq).
- `MU` Micron Technology — Only US-listed HBM supplier; HBM is a sold-out 3-player oligopoly that gates GPU memory bandwidth. The investable US HBM proxy.
- `ARM` Arm Holdings (ADR) — CPU instruction-set IP underpinning NVIDIA Grace and most data-center/edge CPUs; royalty toll on the compute layer. US ADR (Nasdaq).
- `AMAT` Applied Materials — Largest wafer-fab equipment maker (deposition/etch); arms the foundries expanding leading-edge + packaging capacity.
- `LRCX` Lam Research — Etch/deposition leader critical to HBM stacking and advanced-node + packaging buildout.
- `SK Hynix — no clean US ticker` SK Hynix ⚠️named-only(no clean US ticker) — HBM market leader (~62%) but KOREA-listed (KRX 000660); no liquid US common ticker. Do NOT invent one — handle as non-US or omit.
- `Samsung — no clean US ticker` Samsung Electronics ⚠️named-only(no clean US ticker) — Third HBM supplier + foundry, but KOREA-listed (KRX 005930); only foreign/GDR lines. Do NOT invent a US symbol.

**Layer 3 — Infrastructure (the AI factory: systems, networking, interconnect, storage)** — Huang's literal layer-3: 'land, power delivery, cooling, construction, networking and the systems that orchestrate tens of thousands of processors into one machine.' Investably this splits into (a) rack/server integration, (b) in-rack connectivity silicon, (c) optical interconnec
- `SMCI` Super Micro Computer — Leading GPU server / liquid-cooled rack integrator; assembles NVIDIA/AMD silicon into deployable AI factory racks.
- `DELL` Dell Technologies — Tier-1 AI server + storage integrator shipping full GPU rack systems to hyperscalers and enterprises.
- `HPE` Hewlett Packard Enterprise — AI servers + Cray supercomputing systems and the orchestration to run them at scale.
- `ANET` Arista Networks — Ethernet switching leader for AI back-end fabrics; the merchant-Ethernet alternative to InfiniBand in GPU clusters.
- `ALAB` Astera Labs — In-rack connectivity silicon pure-play — PCIe/CXL retimers and fabric controllers that wire GPUs to CPUs/memory inside the node.
- `CRDO` Credo Technology — Active Electrical Cables (AECs) and SerDes connecting GPUs within/between racks; rack-scale connectivity pure-play.
- `COHR` Coherent — Optical transceivers + silicon photonics connecting servers across the data center; NVIDIA committed $2B. Bottleneck-adjacent (the copper wall).
- `LITE` Lumentum Holdings — Optical components / lasers (EMLs) for 1.6T transceivers; NVIDIA committed $2B; named-supplier in next-gen interconnect.
- `CIEN` Ciena — Coherent optical / DWDM systems for data-center interconnect (campus-to-campus); record backlog, added to S&P 500.
- `VRT` Vertiv Holdings — Also straddles here — Huang explicitly puts cooling/power-delivery inside the AI-factory layer; the same ticker spans Energy + Infrastructure.

**Layer 4 — Models (foundation models)** — AI models that understand language, biology, chemistry, physics, finance, medicine and the physical world — beyond LLMs into protein/chemical AI, simulation, robotics. The hardest layer to map to PUBLIC tickers: the frontier labs (OpenAI, Anthropic, Google DeepMind/Gemini, Meta L
- `GOOGL` Alphabet (Google DeepMind) — Owns Gemini + DeepMind + TPU stack; the cleanest public pure-frontier-model proxy.
- `META` Meta Platforms — Open-weight Llama family + massive in-house AI compute; public proxy for an open-model strategy.
- `MSFT` Microsoft — Deep OpenAI partnership + in-house models + Azure model hosting; public exposure to OpenAI-class models.
- `OpenAI — private, no ticker` OpenAI ⚠️named-only(no clean US ticker) — Frontier-model leader but PRIVATE; no public ticker. Reference editorially via MSFT; do not assign a symbol.
- `Anthropic — private, no ticker` Anthropic ⚠️named-only(no clean US ticker) — Frontier lab (Claude), PRIVATE; no public ticker (IPO speculated, not listed). Do not invent a symbol.

**Layer 5 — Applications (where value is captured)** — The top of the cake — 'where economic value is created': copilots, agents, workflow automation, autonomous systems, drug-discovery platforms, robotics. Embodies the same underlying stack in different forms (self-driving cars, humanoid robots, legal/code copilots). Broadest and mo
- `MSFT` Microsoft — Copilot across M365/Dynamics + GitHub Copilot; the broadest enterprise AI-application distribution.
- `CRM` Salesforce — Einstein / Agentforce — agentic AI embedded in the dominant CRM workflow.
- `NOW` ServiceNow — Now Assist — AI agents automating IT/enterprise service workflows.
- `PLTR` Palantir — AIP operationalizes models against enterprise/government data — applied-AI deployment platform.
- `SNOW` Snowflake — Data + AI app platform (Cortex) where enterprises build/run AI on their own data.
- `TSLA` Tesla — Physical-AI application — FSD autonomy + Optimus humanoid robotics; Huang's 'self-driving cars and humanoid robots' example.
- `DDOG` Datadog — Observability for AI apps + AI-driven monitoring; picks-and-shovels of running AI applications in production.

### Framework: three-layer

_Mapped TSMC's three-layer (上/中/下游) semiconductor value chain to real, US-listed or ADR tickers that exist in Tickwind's universe, with the three chokepoints (EUV / advanced packaging-CoWoS / HBM) broken out as their own groups. Framework grounded via WebSearch: Medium/ibinterviewquestions/Wooptix for the structure; TrendForce/Digitimes/Astute/TweakTown for 2026 CoWoS+HBM facts; Yahoo Finance/StockAnalysis for ticker verification. ANTI-HALLUCINATION: the layer structure is editorial only — I authored ZERO figures; every price/fundamental/market-share number must be pulled live from Go endpoints_

**Upstream · EDA & IP (设计工具与IP)** — Software and silicon-IP every chip design starts from. Cadence + Synopsys are a near-duopoly in EDA (place-and-route, verification, simulation); Arm licenses CPU/GPU/NPU architecture in ~99% of phones and a rising share of data-center silicon. High-margin recurring-license toll-b
- `CDNS` Cadence Design Systems — EDA leader (verification/implementation); essential tooling for every advanced tape-out, recurring-license model.
- `SNPS` Synopsys — Other half of the EDA duopoly plus large silicon-IP portfolio; gatekeeper to advanced-node design.
- `ARM` Arm Holdings (ADR) — Architecture-IP licensor in ~99% of smartphone CPUs and rising in AI/data-center; royalty per chip shipped.

**Upstream · Equipment & Materials (设备与材料)** — Capital equipment and process tools that build the wafer — deposition, etch, metrology/inspection — plus materials and subsystems. AMAT/LRCX dominate deposition+etch, KLAC dominates process control; ENTG/MKSI/ONTO/CAMT cover materials and advanced-packaging inspection. ASML lives
- `AMAT` Applied Materials — Broadest wafer-fab-equipment vendor (deposition/etch/implant); benefits from every fab buildout.
- `LRCX` Lam Research — Etch & deposition leader; critical for 3D NAND and advanced logic, levered to HBM/packaging capex.
- `KLAC` KLA Corporation — Near-monopoly in process-control/inspection metrology; yield-critical at advanced nodes.
- `ENTG` Entegris — Ultra-pure materials, filtration, advanced-packaging materials; consumable razor-blade exposure to wafer starts.
- `MKSI` MKS Instruments — Process subsystems (vacuum, RF power, lasers, photonics) used across litho/etch/deposition tools.
- `ONTO` Onto Innovation — Metrology & macro-defect inspection with strong advanced-packaging (HBM/CoWoS) exposure.
- `CAMT` Camtek — Inspection/metrology pure-play levered to HBM and advanced packaging; upgraded on HBM-inspection growth.

**Midstream · Foundry & IDM (代工与制造)** — The factory layer — contract foundries that fabricate chips designed by others, plus integrated device manufacturers that design and build their own. TSMC (TSM) is the dominant pure-play with ~60% foundry share and a monopoly on leading-edge (2nm booked through 2027); UMC is a tr
- `TSM` Taiwan Semiconductor (TSMC, ADR) — Dominant pure-play foundry (~60% share), monopoly on leading-edge nodes; the spine of the whole chain.
- `UMC` United Microelectronics (ADR) — No.2 Taiwan pure-play foundry focused on mature/specialty nodes; cyclical capacity proxy.
- `GFS` GlobalFoundries — Specialty/mature-node foundry (RF, auto, IoT); US-listed alternative to leading-edge exposure.
- `INTC` Intel — Largest Western IDM pivoting to external foundry (Intel Foundry); turnaround/optionality play on the manufacturing layer.

**Midstream · Advanced Packaging & OSAT (先进封装与测试) — CHOKEPOINT** **[CHOKEPOINT]** — CHOKEPOINT. The 2.5D/3D packaging step (CoWoS, SoIC, hybrid bonding) that stitches logic + HBM onto an interposer — now the single tightest bottleneck for AI accelerators. TSMC dominates CoWoS and is sold out; ASX (ASE) and AMKR are the merchant OSATs adding capacity; ONTO/CAMT (
- `TSM` Taiwan Semiconductor (CoWoS, ADR) — Owns/dominates CoWoS advanced packaging; capacity ramping to ~130k wpm in 2026 with NVDA booking ~60% — the bottleneck itself.
- `ASX` ASE Technology (ADR) — World's largest OSAT, expanding advanced-packaging/fan-out capacity to relieve the CoWoS squeeze. NOTE: ticker is ASX on NYSE, NOT 'ASE'.
- `AMKR` Amkor Technology — No.2 merchant OSAT and the key US-listed advanced-packaging name (incl. planned US packaging fab); direct CoWoS-overflow beneficiary.

**Cross-cut · HBM Memory (高带宽HBM) — CHOKEPOINT** **[CHOKEPOINT]** — CHOKEPOINT. High-bandwidth memory is the second hard bottleneck for AI accelerators — a 3-player oligopoly (SK Hynix ~60%+ share, Micron, Samsung) all qualified as HBM4 suppliers for NVIDIA's 2026 Rubin platform. Supply is sold out / allocated. MU is the only clean US-listed pure
- `MU` Micron Technology — Only major US-listed HBM/DRAM maker; HBM3E/HBM4 qualified for NVIDIA, gaining share — cleanest US HBM proxy.
- `000660.KS` SK Hynix ⚠️named-only(no clean US ticker) — HBM leader (~60%+ share), primary NVIDIA supplier. Trades on Korea Exchange — confirm Tickwind KR coverage/exact symbol before surfacing; may be out-of-universe.
- `005930.KS` Samsung Electronics ⚠️named-only(no clean US ticker) — HBM4 supplier scaling capacity. Korea-listed (no liquid US common; only OTC pink GDR) — verify KR coverage/symbol; likely out-of-universe.

**EUV Lithography (极紫外光刻) — CHOKEPOINT** **[CHOKEPOINT]** — CHOKEPOINT. The single hardest monopoly in the entire chain: ASML is the ONLY maker of EUV (and High-NA EUV) lithography systems, without which no sub-7nm logic or leading-edge DRAM can be produced. One name, one bottleneck — the purest 'owns-the-chokepoint' position in semicondu
- `ASML` ASML Holding (ADR) — Sole supplier of EUV and High-NA EUV lithography; absolute gate on leading-edge logic and advanced DRAM/HBM — monopoly chokepoint.

**Downstream · Fabless Chip Design (无厂芯片设计)** — The value-capture layer — fabless designers of the GPUs, CPUs and custom ASICs that drive AI demand, all outsourcing fab to TSMC. NVDA leads AI GPUs; AMD is the No.2 GPU + server CPU challenger; AVGO and MRVL design the custom AI ASICs and networking silicon for hyperscalers. The
- `NVDA` NVIDIA — Dominant AI-GPU designer; books ~60% of TSMC CoWoS and the largest HBM allocation — the demand center of the whole chain.
- `AMD` Advanced Micro Devices — No.2 AI-GPU (Instinct MI series) and server-CPU challenger; reserves meaningful CoWoS/HBM capacity.
- `AVGO` Broadcom — Leading custom-AI-ASIC partner (Google/Meta) plus networking silicon; ~15% of 2026 CoWoS capacity.
- `MRVL` Marvell Technology — Custom AI silicon for AWS/Microsoft plus data-center interconnect; secured tens of thousands of CoWoS wafers.

**Downstream · Connectivity, Optical & Systems (连接·光模块·系统)** — The layer that turns chips into AI racks — high-speed connectivity (SerDes/retimers), optical interconnect, power/cooling, and rack integration. CRDO/ALAB are connectivity pure-plays (retimers/active cables); COHR/LITE/FN supply the optical/transceiver stack; VRT supplies power &
- `CRDO` Credo Technology — High-speed connectivity (AECs/SerDes/retimers) for AI back-end networking; triple-digit revenue growth on AI capex.
- `ALAB` Astera Labs — PCIe/CXL/Ethernet connectivity (retimers, smart fabric) pure-play; core AI-rack interconnect.
- `COHR` Coherent — Optical transceivers, lasers and photonics for AI-datacenter interconnect; vertically integrated optical supplier.
- `LITE` Lumentum Holdings — Optical/photonic chips and components for cloud/AI interconnect; levered to the optical bottleneck inside AI clusters.
- `FN` Fabrinet — Optical-module manufacturing/packaging for the transceiver supply chain (incl. co-packaged optics); record optical-comms revenue.
- `VRT` Vertiv Holdings — Power distribution and liquid cooling for AI data centers; thermal/power enabler of dense GPU racks.
- `SMCI` Super Micro Computer — Rack-scale GPU server integration with fast time-to-market on new NVIDIA platforms; assembly/integration layer.

### Framework: chokepoint

_Researched and verified (June 2026 web-grounded) the AI "卡脖子" chokepoint thesis as a 13-layer stack. The tightest necks are non-substitutable single-vendor or sub-vendor monopolies at the top of the funnel — ASML (100% of production EUV) plus its irreplaceable sub-suppliers Zeiss optics, TRUMPF lasers, and Lasertec (90%+ EUV photomask inspection); then the WFE oligopoly (AMAT/LRCX/KLAC, with KLA ~60%+ of metrology/inspection); then TSMC's twin monopolies in leading-edge foundry AND CoWoS advanced packaging (sold out through 2026); then the HBM 3-way oligopoly (SK Hynix ~62% / Micron ~21% / Sam_

**Layer 1 — EUV Lithography (the narrowest neck)** **[CHOKEPOINT]** — The single hardest chokepoint in the entire AI stack: no sub-7nm logic, no HBM-grade DRAM, no leading-edge anything without EUV. ASML is the ONLY company on earth that makes production EUV scanners (100% share); Nikon/Canon make none. High-NA EUV ($350M+/tool) extends the monopol
- `ASML` ASML Holding N.V. — 100% monopoly on production EUV lithography — every sub-7nm AI chip and HBM die is gated by an ASML scanner. Clean Nasdaq ADR/NY registry shares, in Tickwind universe. Moat: extreme/decade+.
- `LSRCY` Lasertec Corp (EUV photomask actinic inspection) ⚠️named-only(no clean US ticker) — 90%+ monopoly on actinic EUV photomask defect inspection — the only tool that can see EUV-wavelength mask defects; a sub-neck inside the EUV neck. CAUTION: primary listing is Tokyo (6920.T); US presence is OTC ADR 'LSRCY' only — NOT a clean NYSE/Nasdaq line. Flag as uncertain/OTC for Tickwind; verify the OTC ticker resolves in the Go universe before using, otherwise present name-only with no ticker.
- `` Carl Zeiss SMT (EUV optics/mirrors) + TRUMPF (EUV CO2 drive laser) ⚠️named-only(no clean US ticker) — Both are irreplaceable single-source EUV sub-suppliers (Zeiss = the only EUV mirror optics; TRUMPF = the only high-power EUV light-source laser). BOTH ARE PRIVATE / not separately listed — Zeiss SMT is part of privately-held Carl Zeiss AG; TRUMPF is private. NO investable ticker. Include as editorial 'who owns the neck' context only; do NOT map to any symbol.

**Layer 2 — Wafer-Fab Equipment (deposition / etch / process-control oligopoly)** **[CHOKEPOINT]** — Below EUV sit the other must-have fab tools. A tight US oligopoly: Applied Materials leads deposition (~21% of total WFE), Lam Research dominates dry etch (~45% of the etch market, critical for 3D NAND/advanced logic), and KLA owns process control with 60%+ of metrology/inspectio
- `AMAT` Applied Materials — Largest WFE vendor; leads deposition/CVD and materials-engineering steps required at every node. US-listed (Nasdaq). High moat.
- `LRCX` Lam Research — ~45% of the global etch market; dry-etch monopoly-adjacent for advanced logic and 3D NAND; rising advanced-packaging exposure. US-listed (Nasdaq). High moat.
- `KLAC` KLA Corporation — 60%+ of metrology & inspection — the near-monopoly 'process-control gate'; effectively a single-vendor chokepoint within WFE. US-listed (Nasdaq). High/extreme moat in its niche.

**Layer 3 — Leading-Edge Foundry** **[CHOKEPOINT]** — The manufacturing neck: at 3nm/2nm, TSMC is the de-facto sole high-volume merchant foundry for cutting-edge AI silicon (NVIDIA, AMD, Broadcom, Apple, Arm's own AGI chip all fab here). Samsung Foundry trails badly on yield; Intel Foundry is unproven at scale. NVIDIA reserved the m
- `TSM` Taiwan Semiconductor Manufacturing (TSMC) — Near-monopoly on leading-edge (3nm/2nm) merchant foundry — the physical factory for essentially all top AI accelerators. Clean NYSE ADR, in Tickwind universe. Very high moat; geopolitical tail-risk is the one caveat to surface editorially.
- `INTC` Intel (Intel Foundry) — Aspiring #2 leading-edge foundry and first High-NA EUV adopter, but NOT yet a proven chokepoint — include only as the 'challenger' contrast, not as a neck-owner. US-listed (Nasdaq).

**Layer 4 — Advanced Packaging / CoWoS** **[CHOKEPOINT]** — TSMC's SECOND monopoly and arguably the single most acute near-term AI bottleneck: CoWoS (Chip-on-Wafer-on-Substrate) is how GPU logic + HBM stacks get integrated. Capacity ramped ~35k→~75k→~110-130k wafers/mo (2024→2026) yet remains 'extremely tight and sold out through 2026' (T
- `TSM` TSMC (CoWoS) — Controls nearly all commercial-scale CoWoS advanced packaging; sold out through 2026. Same NYSE ADR as Layer 3 — note the dual-monopoly (foundry AND packaging) in the editorial copy.
- `AMKR` Amkor Technology — Largest US-listed OSAT; the primary 'arms-dealer' overflow beneficiary for advanced packaging steps TSMC outsources. US-listed (Nasdaq). Moderate moat — overflow player, not the neck-owner.
- `ASX` ASE Technology Holding ⚠️named-only(no clean US ticker) — World's largest OSAT (Taiwan); the other overflow packager. Trades as US ADR 'ASX' (NYSE) — verify it resolves in Tickwind's Go universe; include as overflow context, not neck-owner.

**Layer 5 — HBM Memory (3-way oligopoly)** **[CHOKEPOINT]** — Every AI GPU needs stacked High-Bandwidth Memory, and only three firms can make it: SK Hynix (~62%, ~2/3 of NVIDIA Rubin/HBM4 orders), Micron (~21%, overtook Samsung), Samsung (~17%, fighting back into HBM4). Sold out through 2026; the 'three-supplier oligopoly means no one start
- `MU` Micron Technology — Only one of the three HBM makers with a clean US primary listing (Nasdaq). ~21% HBM share, overtook Samsung; full HBM3E/HBM4 ramp into NVIDIA. The investable US proxy for the HBM neck. High moat.
- `` SK Hynix (HBM share leader ~62%) ⚠️named-only(no clean US ticker) — The dominant HBM supplier and the real neck-owner, BUT primary listing is Korea (000660.KS); only thin OTC ADR ('HXSCL') exists — NOT a clean US line and likely outside Tickwind's tradable universe. Present name-only as the share leader; do NOT assert a US ticker. (Tickwind has 'some HK/TW/KR' — verify whether the KS line is actually covered before mapping.)
- `` Samsung Electronics (HBM ~17%, HBM4 push) ⚠️named-only(no clean US ticker) — Third HBM supplier, but Samsung has NO clean US primary listing (Korea 005930.KS; US is OTC 'SSNLF' only). Name-only editorial; do NOT map to a US ticker.

**Layer 6 — Compute + Interconnect Moat (GPU / CUDA / NVLink vs custom ASIC)** **[CHOKEPOINT]** — The most visible 'neck' but a software/ecosystem one rather than a single physical bottleneck: NVIDIA holds ~80% of AI accelerators, defended by CUDA (5M+ developers, switching-cost lock-in) and proprietary NVLink scale-up fabric (4-5yr lead). The credible counter-neck is custom 
- `NVDA` NVIDIA — ~80% accelerator share; CUDA + NVLink is the deepest software/interconnect moat in AI. US-listed (Nasdaq). High moat — but framed as ecosystem lock-in, not a sole-source physical neck.
- `AVGO` Broadcom — Dominant custom-AI-ASIC co-design partner (the way hyperscalers escape NVIDIA) PLUS the Tomahawk switch-silicon leader — a chokepoint on two layers. US-listed (Nasdaq). High moat.
- `AMD` Advanced Micro Devices — The only credible merchant GPU alternative to NVIDIA (MI350X-class); contestant, not neck-owner. US-listed (Nasdaq).
- `MRVL` Marvell Technology — Second custom-ASIC co-design house + acquiring Celestial AI for optical interconnect; spans the ASIC and optical necks. US-listed (Nasdaq). Moderate-high moat (duopoly with AVGO in custom silicon).

**Layer 7 — In-Rack Connectivity Silicon (retimers / PCIe / signal integrity)** **[CHOKEPOINT]** — As GPUs scale to rack-scale (NVL72-class), signals can't travel far without retiming/conditioning — a fast-growing neck owned by two specialists. Astera Labs is the first/only high-volume PCIe Gen6 retimer (Aries) plus Scorpio fabric switches and Leo memory expanders; Credo leads
- `ALAB` Astera Labs — First/only high-volume PCIe Gen6 retimer (Aries) + Scorpio switch + Leo CXL memory; the in-rack connectivity neck. US-listed (Nasdaq), explicitly in Tickwind's example set. Moderate-high moat.
- `CRDO` Credo Technology — Active-electrical-cable (AEC) leader for in-rack copper + entering PCIe6 retimers; the other half of the connectivity duopoly. US-listed (Nasdaq). Moderate-high moat.

**Layer 8 — Optical Transceivers / Silicon Photonics** **[CHOKEPOINT]** — Scale-out beyond the rack needs 800G→1.6T optics, and supply is structurally short (McKinsey: 800G 40-60% undersupplied through 2027; 1.6T 30-40% short through 2029). Coherent is the volume leader (~25% transceiver share); Lumentum and Marvell round out the pure-play photonics se
- `COHR` Coherent Corp (formerly II-VI) — Volume leader in optical transceivers (~25% share), deep silicon-photonics + laser vertical integration; took $2B NVIDIA equity. US-listed (NYSE). Moderate moat.
- `LITE` Lumentum Holdings — Top pure-play optical-component/laser maker; $2B NVIDIA preferred investment. US-listed (Nasdaq). Moderate moat.
- `FN` Fabrinet — Contract manufacturer that assembles/tests a large share of the world's optical transceivers for COHR/LITE/NVIDIA — the 'picks-and-shovels' neck of photonics. US-listed (NYSE). Moderate moat (sticky, hard-to-qualify manufacturing).
- `` Innolight (Eoptolink peer, China transceiver leader) ⚠️named-only(no clean US ticker) — One of the largest 800G transceiver suppliers globally, but listed in China (Shenzhen, 300308.SZ) — NO US listing. Name-only context; do NOT map a US ticker.

**Layer 9 — AI Networking Switches (back-end fabric)** **[CHOKEPOINT]** — The AI cluster fabric layer. NVIDIA's Spectrum-X Ethernet surged (+647% YoY to $2.3B, ~26% share) and overtook Cisco/Arista, complementing its InfiniBand franchise; Arista (~19% datacenter Ethernet) is the merchant leader for hyperscaler front-end; Broadcom supplies the Tomahawk 
- `ANET` Arista Networks — Merchant datacenter-Ethernet leader (~19% share) for AI front-end fabric; key hyperscaler switch vendor. US-listed (NYSE). Moderate moat (challenged by NVDA Spectrum-X + white-box).
- `AVGO` Broadcom (Tomahawk switch silicon) — Tomahawk 6 = highest-bandwidth merchant switch chip (102.4 Tbps); the silicon inside most third-party AI switches — a deeper neck than the box makers. US-listed (Nasdaq). Already counted in Layer 6; note the cross-layer dominance.
- `NVDA` NVIDIA (Spectrum-X / InfiniBand) — Spectrum-X Ethernet + InfiniBand give NVIDIA the AI-tuned back-end fabric; overtook Cisco/Arista in 2025. US-listed (Nasdaq). Cross-layer with Layer 6.

**Layer 10 — Power Delivery ('the last inch')** **[CHOKEPOINT]** — An under-appreciated bottleneck: GPUs now draw 1,000W→2,500W+ (Rubin Ultra), and stepping that power down to ~0.7V at thousands of amps right at the die is a hard analog problem. Monolithic Power Systems is positioned to win ~70% of NVIDIA's Vera Rubin (VR200/R200) VRM content wi
- `MPWR` Monolithic Power Systems — Front-runner for ~70% of NVIDIA Vera Rubin power-delivery content; 800V-DC + Z-axis VRM is a prerequisite for 2,500W+ GPUs. US-listed (Nasdaq), in Tickwind's example set. Moderate-high moat at leading edge.
- `ADI` Analog Devices — Broad analog/power competitor with AI power-delivery exposure; contestant, not neck-owner. US-listed (Nasdaq).
- `VICR` Vicor Corp — Specialist in high-density point-of-load/lateral power-delivery modules for AI; smaller challenger to MPWR. US-listed (Nasdaq). Lower/contested moat.

**Layer 11 — Liquid Cooling / Thermal** — Not a true monopoly neck, but a critical capacity bottleneck: 30-50kW+ racks can't be air-cooled, forcing direct-to-chip/cold-plate/immersion. Vertiv is the share leader (~11%+ of datacenter liquid cooling; top-5 of Schneider/Vertiv/Rittal/Stulz/Boyd ~35% combined) and the closes
- `VRT` Vertiv Holdings — Share leader (~11%+) in datacenter liquid cooling + power/thermal; the most investable scaled pure-play. US-listed (NYSE), in Tickwind's example set. Moderate moat (fragmented market).
- `SMCI` Super Micro Computer — Named among top liquid-cooling/rack-integration players (DLC at rack scale); but a system integrator, not a neck-owner. US-listed (Nasdaq), in Tickwind's example set. Low/contested moat — include with caveat.

**Layer 12 — EDA Design-Tool Duopoly** **[CHOKEPOINT]** — You literally cannot design a modern AI chip without EDA software, and two firms own ~85% of it: Synopsys (~38%) and Cadence (~36%), with Siemens EDA the distant #3. Near-100% retention, 80-85% recurring subscription revenue, decade-long tool-flow lock-in. Both deepened moats via
- `SNPS` Synopsys — ~38% EDA share, #1; full design-to-silicon flow + Ansys simulation. The design-tool neck. US-listed (Nasdaq). Very high moat (subscription lock-in).
- `CDNS` Cadence Design Systems — ~36% EDA share, the other half of the duopoly; ~85% combined with SNPS. US-listed (Nasdaq). Very high moat.

**Layer 13 — Chip-IP Architecture (the instruction-set tollbooth)** **[CHOKEPOINT]** — Arm's architecture is the licensing tollbooth under most non-x86 AI silicon — NVIDIA Grace/Vera CPUs, hyperscaler custom CPUs, and now Arm's own 136-core Neoverse-based 'AGI' datacenter CPU (Meta lead customer; OpenAI/Cloudflare/others as partners) all pay Arm royalties. 21 licen
- `ARM` Arm Holdings — Architecture-license + royalty tollbooth under most AI CPUs (incl. NVIDIA Grace/Vera) and now its own AGI datacenter CPU. Clean Nasdaq ADR, explicitly in Tickwind's example set. Very high moat (ISA ecosystem lock-in).

### Framework: tenx-themes

_Mapped 4 sectors into 14 sub-themes and ~70 real, verified tickers. AI-beyond-megacaps is the deepest bench and splits cleanly into a "value-layer cake" (power/cooling → silicon-chokepoint → memory → optics/interconnect → edge → AI-software), where the chokepoint layers (ASML/TSM/AVGO; HBM trio; optics) carry the strongest moats. Aerospace/space ranges from quasi-investable (RKLB, ASTS, LUNR, primes like LMT/RTX/LHX) to highly speculative recent IPOs (FLY, VOYG, KRMN) and pre-revenue eVTOL (JOBY, ACHR). Quantum is the most speculative bucket — four pure-plays (IONQ, RGTI, QBTS, QUBT) trading o_

**AI · Power & Cooling (data-center physical layer)** **[CHOKEPOINT]** — The 'AI needs electricity and heat removal' trade. Liquid cooling is now default for AI racks; grid power is the gating constraint on buildout. Two distinct chokepoints: thermal/power-distribution hardware and the electricity generation behind it (nuclear is the marquee AI-power 
- `VRT` Vertiv Holdings — Closest pure-play in AI data-center liquid cooling + critical power; large multi-year backlog, default vendor for new AI rack designs.
- `SMCI` Super Micro Computer ⚠️named-only(no clean US ticker) — Liquid-cooled AI server racks at scale; high-beta NVIDIA-systems proxy (note: prior accounting/governance overhang — flag for diligence).
- `MOD` Modine Manufacturing — Thermal-management maker pivoting hard into data-center cooling; smaller-cap, higher torque to the cooling theme.
- `ETN` Eaton — Electrical power-distribution + power management for data centers; diversified industrial, lower-beta way to play AI power.
- `NVT` nVent Electric ⚠️named-only(no clean US ticker) — Electrical enclosures, connection and liquid-cooling protection for data centers; mid-cap pick-and-shovel.
- `CEG` Constellation Energy — Largest US nuclear fleet; direct hyperscaler power deals (Three Mile Island restart for Microsoft) make it the marquee AI-baseload-power name.
- `VST` Vistra — Second-largest competitive US nuclear fleet post Energy Harbor; levered to AI-driven power-price and PPA demand.
- `TLN` Talen Energy — Nuclear/IPP with data-center co-location deals; smaller, more concentrated AI-power bet.
- `OKLO` Oklo ⚠️named-only(no clean US ticker) — SPECULATIVE — pre-revenue SMR/fast-fission developer (Altman-backed) aiming to power data centers directly; pure theme/option value, no commercial reactor yet.
- `SMR` NuScale Power — SPECULATIVE — small modular reactor developer; grid-tied utility customers, still pre-commercial-deployment, theme-driven.

**AI · Silicon & Foundry chokepoints (non-NVDA)** **[CHOKEPOINT]** — The architectural and manufacturing bottlenecks of AI compute, excluding the obvious mega-cap GPU leader. Custom ASIC designers capture hyperscaler in-house-silicon spend; the foundry + lithography layer is a true monopoly/duopoly chokepoint that every AI chip must pass through.
- `AVGO` Broadcom — Dominant custom AI accelerator (XPU) + AI networking franchise serving hyperscalers; the #2 AI-silicon story after NVDA. (Large-cap, so '10x from here' is the speculative part, not the ticker.)
- `MRVL` Marvell Technology — Custom ASIC + data-center interconnect/optical DSP; smaller than AVGO so more torque to the custom-silicon ramp.
- `AMD` Advanced Micro Devices — Only credible merchant GPU alternative to NVIDIA plus AI-CPU; share-gain optionality in inference.
- `TSM` TSMC (ADR) — ADR. The foundry chokepoint — advanced nodes + CoWoS packaging that nearly every AI chip depends on; closest thing to a 'tax' on all AI silicon.
- `ASML` ASML (ADR) — ADR. EUV lithography monopoly — the single deepest chokepoint in the AI hardware stack; no leading-edge chip exists without it.
- `ARM` Arm Holdings (ADR) — ADR. CPU IP licensing across data-center and edge AI; royalty-model leverage to AI compute proliferation.
- `ALAB` Astera Labs — Connectivity silicon (PCIe/CXL retimers, smart fabric) for AI servers; small-cap pure-play riding PCIe 6 / rack-scale interconnect.
- `CRDO` Credo Technology — High-speed connectivity (active electrical cables, SerDes) for AI clusters; small-cap, high-growth interconnect chokepoint.

**AI · Memory (HBM supercycle)** **[CHOKEPOINT]** — High-bandwidth memory is a hard supply chokepoint — 2026 HBM capacity is sold/committed and shortages are flagged into 2027. A three-supplier oligopoly. Note: classic boom/bust cyclicality is the main risk to the '10x' framing.
- `MU` Micron Technology — Only US-listed pure HBM/DRAM maker in the oligopoly; direct HBM supplier to GPU vendors. The cleanest US-listed memory chokepoint play.
- `SNDK` SanDisk ⚠️named-only(no clean US ticker) — UNCERTAIN — NAND/flash spun out of Western Digital (2025); ticker SNDK is the storage-memory pure-play, but verify the symbol against live universe before listing.

**AI · Optics & Interconnect** **[CHOKEPOINT]** — 'Copper is running out of road' inside AI data centers — the shift to optical fiber + silicon photonics for intra/inter-rack bandwidth. NVIDIA's ~$4B combined stakes in two of these names (optics leaders) validated the chokepoint thesis.
- `COHR` Coherent — Optical transceivers + laser components for AI networking; received a multibillion-dollar NVIDIA equity stake + purchase commitments.
- `LITE` Lumentum Holdings — Datacom optics / laser supplier; also received a large NVIDIA investment + capacity commitments — core silicon-photonics beneficiary.
- `AAOI` Applied Optoelectronics ⚠️named-only(no clean US ticker) — SPECULATIVE — small-cap optical transceiver maker; high-beta optics-rally name, lumpier execution than COHR/LITE.
- `COHR` (see above) — Listed once; COHR/LITE are the two anchor optics chokepoints in this sub-theme.

**AI · Edge & on-device inference** — AI 'leaving the data center' — inference on cameras, vehicles, robots, industrial gear. Lower-conviction than the data-center layers (smaller TAM today, more competitive), but the highest-torque small-caps if edge inference scales.
- `AMBA` Ambarella — Edge computer-vision SoCs (automotive/cameras/robotics); small-cap pure-play on on-device inference.
- `LSCC` Lattice Semiconductor — Low-power FPGAs for edge AI + AI-server management/security. NOTE: correct ticker is LSCC — a search source wrongly returned LRCX (that is Lam Research).
- `QCOM` Qualcomm — On-device AI across smartphones/auto/PC; diversified, lower-beta edge-AI exposure (not a 10x candidate but the scaled edge-AI incumbent).
- `SOUN` SoundHound AI ⚠️named-only(no clean US ticker) — SPECULATIVE — voice-AI software; very high P/S, narrative-driven, small revenue base. Lottery-style edge-AI-application name.

**AI · Software & Applications** — The application/orchestration layer capturing AI's enterprise spend (the ~$2T+ 2026 AI-spend wave). Higher-multiple than infra; '10x' here is about durable share-of-AI-budget, with valuation (not technology) as the main risk.
- `PLTR` Palantir Technologies — AI ops/decision platform (AIP) with rapid commercial + government growth; the marquee AI-software name — but already richly valued, so '10x' is the speculative part.
- `CRWD` CrowdStrike — AI-native cybersecurity platform; more durable/predictable growth than PLTR — the 'quality compounder' AI-software pick.
- `SNOW` Snowflake ⚠️named-only(no clean US ticker) — Data platform positioned as the substrate for enterprise AI workloads; consumption model leveraged to AI data demand.
- `NOW` ServiceNow — Enterprise workflow platform embedding agentic AI; large-cap, lower-beta AI-software compounder.

**Aerospace & Space · Launch & space infrastructure** — 航天/space buildout: launch cadence + satellite manufacturing + lunar/in-space services. Ranges from quasi-investable scaled names to highly speculative 2025-IPO debutants with little trading history. Treat newly-public names as venture-like.
- `RKLB` Rocket Lab — Most diversified small/medium-cap space pure-play: launch + satellite systems + defense (missile-warning sat contract). The anchor non-SpaceX launch name.
- `LUNR` Intuitive Machines ⚠️named-only(no clean US ticker) — Lunar landers + NASA/defense backlog; closest to profitability among new-space names, but execution-/contract-timing risk.
- `FLY` Firefly Aerospace ⚠️named-only(no clean US ticker) — SPECULATIVE — launch + lunar; IPO'd Aug 2025, large backlog but very short trading history and pre-scale economics. Verify ticker FLY against live universe.
- `VOYG` Voyager Technologies ⚠️named-only(no clean US ticker) — SPECULATIVE — Starlab commercial space station + in-space infrastructure; IPO'd June 2025, pre-revenue-at-scale, theme/option value.
- `KRMN` Karman Holdings ⚠️named-only(no clean US ticker) — SPECULATIVE — payload/propulsion systems for missiles + launch; IPO'd Feb 2025, profitable-ish defense supplier but short public history.

**Aerospace & Space · Satellites & direct-to-device** — Satellite connectivity and Earth-observation as recurring-revenue space businesses. ASTS is the highest-conviction 10x-style asymmetric bet here (direct-to-phone TAM); others are data/services plays.
- `ASTS` AST SpaceMobile ⚠️named-only(no clean US ticker) — Direct satellite-to-standard-smartphone connectivity; binary but enormous TAM if it works at scale — the canonical asymmetric space bet. Pre-meaningful-revenue.
- `PL` Planet Labs — Large Earth-observation fleet with recurring data revenue + growing backlog; the 'space data subscription' pick.
- `GSAT` Globalstar ⚠️named-only(no clean US ticker) — SPECULATIVE — satellite connectivity with a large anchor-customer (Apple emergency SOS) relationship; lumpy, customer-concentration risk. Verify ticker.

**Aerospace & Space · Defense-space primes & contractors** — The de-risked, cash-flowing side of the space/defense theme — primes and government-services contractors. Not 10x candidates individually, but the 'quality anchor' for a space 专区 and beneficiaries of rising space/missile-defense budgets.
- `LMT` Lockheed Martin — Defense prime with major space + missile-defense exposure; quality anchor, dividend, low-beta space proxy.
- `RTX` RTX Corporation — Defense prime (missiles/sensors/space sensors); diversified aerospace-defense anchor.
- `NOC` Northrop Grumman — Space systems + strategic missile/defense prime; strong space-segment exposure among primes.
- `LHX` L3Harris Technologies — Space sensors/electronics + defense tech; higher-margin electronics tilt vs. pure primes.
- `KTOS` Kratos Defense & Security ⚠️named-only(no clean US ticker) — SPECULATIVE-GROWTH — drones, hypersonics propulsion, satcom; revenue inflection expected as prototypes move to full-rate production. Highest-torque defense-space mid-cap.
- `LDOS` Leidos Holdings — Government IT/services supporting space (ground systems, mission software) for NASA/Space Force; lower-beta services anchor.

**Aerospace & Space · eVTOL / advanced air mobility** — Electric vertical-takeoff aircraft for urban air mobility + defense. Pre-revenue, certification-gated — the most speculative aerospace sub-theme; binary on FAA type-certification and commercial launch.
- `JOBY` Joby Aviation ⚠️named-only(no clean US ticker) — SPECULATIVE — leading eVTOL developer, well-capitalized; pre-commercial-revenue, certification-dependent. Binary outcome.
- `ACHR` Archer Aviation ⚠️named-only(no clean US ticker) — SPECULATIVE — eVTOL (Midnight) with FAA Means-of-Compliance progress + Palantir defense-systems tie-up; pre-revenue, certification-gated.

**Quantum Computing · Pure-plays** — 量子 — the most speculative bucket in this entire map. Four pre-profit pure-plays trading purely on theme/sentiment at extreme P/S multiples (triple-digit to ~800x per mid-2026 data). Real long-term TAM, but lottery-ticket risk/reward; size accordingly and lead the 专区 copy with the
- `IONQ` IonQ — SPECULATIVE — trapped-ion approach; largest pure-play by revenue and the only one with positive 2026 YTD at time of research. Still pre-profit, theme-driven.
- `RGTI` Rigetti Computing ⚠️named-only(no clean US ticker) — SPECULATIVE — superconducting-qubit approach; tiny revenue, extreme valuation multiple, headline-driven.
- `QBTS` D-Wave Quantum ⚠️named-only(no clean US ticker) — SPECULATIVE — quantum annealing for optimization workloads (closest to commercial use-cases); pre-profit, extreme P/S.
- `QUBT` Quantum Computing Inc. ⚠️named-only(no clean US ticker) — SPECULATIVE — photonic quantum-as-a-service; smallest/most speculative of the four, minimal revenue.

**Quantum Computing · Big-tech & enabling exposure** — The non-pure-play way to hold quantum: mega-caps with leading quantum research programs (de-risked, quantum is option value not the thesis), plus the cryo/control supply chain. Lower torque, far lower blow-up risk than the pure-plays.
- `IBM` IBM — Most advanced enterprise quantum roadmap among mega-caps (quantum hardware + cloud access); quantum is upside option on a profitable base.
- `GOOGL` Alphabet — Leading quantum research (error-correction milestones); quantum is a tiny option embedded in a mega-cap — the 'safe' quantum exposure.
- `NVDA` NVIDIA — Quantum-classical hybrid / GPU-accelerated quantum simulation (CUDA-Q) + control-systems partnerships; an enabling-layer angle, not a quantum-hardware bet.

**Medical · AI drug discovery & precision medicine** — 医疗 + AI: platforms using ML to compress drug discovery, plus AI-driven diagnostics. Mostly pre-profit / clinical-stage — high optionality, high cash-burn risk. The structurally exciting but financially riskiest medical sub-theme.
- `RXRX` Recursion Pharmaceuticals ⚠️named-only(no clean US ticker) — SPECULATIVE — image-based AI drug-discovery platform (absorbed Exscientia in 2024); pre-profit, pipeline-/partnership-dependent.
- `SDGR` Schrödinger — Physics-based computational drug-design software + internal pipeline; dual software-revenue + biotech-optionality model, still loss-making.
- `TEM` Tempus AI ⚠️named-only(no clean US ticker) — SPECULATIVE — AI precision-medicine/diagnostics platform (genomic + clinical data); high-growth, not yet profitable.
- `ABSI` Absci ⚠️named-only(no clean US ticker) — SPECULATIVE — generative-AI antibody design; small-cap, pre-commercial, partnership-milestone-driven.
- `RLAY` Relay Therapeutics ⚠️named-only(no clean US ticker) — SPECULATIVE — motion-based / computational drug discovery; clinical-stage, binary on trial readouts.

**Medical · GLP-1 / obesity ecosystem** — The metabolic-disease/obesity-drug supercycle. Anchored by two profitable mega-caps (the duopoly), with the asymmetric upside in oral-GLP-1 challengers and ecosystem suppliers. The most fundamentally-grounded medical sub-theme (real, large revenue today).
- `LLY` Eli Lilly — Co-leader of GLP-1 (Mounjaro/Zepbound) pulling ahead on oral orforglipron; profitable mega-cap — the quality anchor of the obesity theme.
- `NVO` Novo Nordisk (ADR) — ADR. Co-leader (semaglutide/Wegovy/Ozempic) defending its franchise with oral amycretin; profitable, but competitive-pressure risk vs LLY.
- `VKTX` Viking Therapeutics ⚠️named-only(no clean US ticker) — SPECULATIVE — dual GIPR/GLP-1 agonist VK2735 (oral + injectable) heading to Phase III; the highest-profile clinical-stage challenger + frequent M&A-target speculation. Binary on trials.

**Medical · Med-tech & surgical robotics** — Devices + robotics with AI augmentation. ISRG is the scaled robotic-surgery leader; the diversified med-tech majors are lower-beta 'quality' anchors. Not classic 10x candidates but the durable, profitable backbone of a medical 专区.
- `ISRG` Intuitive Surgical — da Vinci robotic-surgery franchise with razor-and-blade economics + AI-assisted next-gen platform; the category-defining med-tech compounder.
- `MDT` Medtronic — Diversified med-tech major investing in surgical robotics (Hugo) + AI; lower-beta diversified anchor.
- `SYK` Stryker — Orthopedic + surgical robotics (Mako) leader; quality med-tech compounder with robotics tailwind.
- `BSX` Boston Scientific ⚠️named-only(no clean US ticker) — High-growth med-tech major (electrophysiology/structural heart); among the fastest-growing large-cap device names.

**Medical · Genomics & gene editing** — 基因 — CRISPR/gene-editing and the broader genomics toolchain. Clinical-stage editors are binary (trial-readout-driven) and largely pre-profit; VRTX is the de-risked anchor (commercial CRISPR therapy partner). The highest-variance medical sub-theme after AI-drug-discovery.
- `CRSP` CRISPR Therapeutics ⚠️named-only(no clean US ticker) — SPECULATIVE — first approved CRISPR therapy (Casgevy, with VRTX) + pipeline; commercial-but-early, pipeline-dependent.
- `NTLA` Intellia Therapeutics ⚠️named-only(no clean US ticker) — SPECULATIVE — in-vivo gene editing; positive Phase 3 (hereditary angioedema, in-vivo first) but pre-commercial. Binary on launch/trials.
- `BEAM` Beam Therapeutics ⚠️named-only(no clean US ticker) — SPECULATIVE — base-editing platform (more precise than cut-CRISPR); clinical-stage, pre-revenue, binary.
- `VRTX` Vertex Pharmaceuticals — De-risked anchor: profitable mega-cap, commercial CRISPR partner (Casgevy) + dominant CF franchise + non-opioid pain launch. The 'quality' genomics holding, not a moonshot.
- `EDIT` Editas Medicine ⚠️named-only(no clean US ticker) — SPECULATIVE — gene-editing, smallest/most troubled of the editors; very high risk, frequent strategic uncertainty. Verify still-listed before featuring.
