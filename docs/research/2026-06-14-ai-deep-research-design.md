# AI Deep Research (研报) — design (2026-06-14, owner 重磅)

Upgrade the shipped **R2 research** (`/v1/stocks/{t}/research` — `internal/research` Assemble→FactSheet, `enrich.ComposeReport` LLM-prose-only, 6 sections, anti-hallucination contract, daily cap; front `ResearchReport.tsx`) into a **paid, deeper, login-gated "AI Deep Research report"**.

## Owner spec
1. **Separate module**; entry = a button at the **top-right of the AI Digest module** on the stock page → navigates to the research report view.
2. **Engineering-fixed data first** (Go-owned numbers) + LLM analysis **harness-constrained**; **fixed output styling — charts, tables, data, source/原文 links**.
3. Prompt (system/user) **modeled on the Fable 5 system-prompt techniques** (below).
4. **Stronger model** (OpenRouter) → because pricier: **must be logged-in + GLOBAL per-user quota = 1 generation/day site-wide** (NOT per-stock).

## Non-negotiable: keep the anti-hallucination contract
R2 already enforces it and it MUST hold: **Go owns every number** (FactSheet, injected from structured sources); the LLM material contains only formatted strings (no raw values); the LLM writes **only prose**; stray numeric keys are ignored (`TestComposeNeverMutatesNumbers`). The deeper report keeps this — a stronger model writes richer prose over the SAME Go-owned facts; it never computes/asserts a number.

## Legal-safe data (per the monetization legal研报)
Since this is the PAID feature: **prioritize public-domain sources** — SEC EDGAR (financials/filings/insider/13F-ish), Treasury, FINRA (display) — which are clearly safe to monetize. Keep Yahoo OUT of the paid report's data (Yahoo removal is a pre-paywall must-fix). Attribution for CoinGecko/alternative.me if any crypto context.

## The LLM harness — Fable-5-derived techniques (→ system/user prompt)
1. **Hierarchical sections** by concern: `<research_standards>` `<citation_discipline>` `<hedging_requirements>` `<scope_boundaries>` `<format_rules>` `<self_check>` — isolate constraints, don't flat-list.
2. **Format discipline via negation + positive**: prose for analysis; a table/chart ONLY when it materially clarifies (financial trends, peer/valuation comparison); no over-formatting, no gratuitous bullet lists in analysis.
3. **Anti-fabrication**: never interpolate/invent a number, estimate, target price, or analyst consensus; every figure comes from the injected Go facts; if a fact is absent → say "数据不足/not disclosed", never guess. (Reinforces the existing contract.)
4. **Citation integrity**: attribute to the source ("据 [公司] 最新 10-Q…"); ≤~20 words any single quote, one per source; paraphrase over verbatim; each section's claims tie to the FactSheet citations (deep-linked, F3 anchors already exist).
5. **Scope boundaries**: analytical, NOT investment advice; no Buy/Sell/目标价; flag assumptions lacking support; don't project >N years without naming it a scenario.
6. **Hedging/uncertainty**: modal language ("数据显示/suggests" not "will"); ranges over false-precision point values; separate disclosed facts from inference.
7. **Self-check / pre-submission checklist** (in-prompt): every figure sourced? no recommendation? no fabricated number? hedged where uncertain? — the model re-reads before finalizing. (Optionally a 2nd "self-critique pass" agent that deletes unsupported claims — cf. the TradingAgents research already in the roadmap: bull/bear two-round debate → synthesis + self-critique, all within the numbers-from-Go contract.)
8. **Tone**: analytical detachment, neutral descriptors (headwinds/tailwinds/secular), no marketing language.

## Architecture (build on R2, incremental)
- **Data layer**: reuse `research.Assemble` FactSheet (6 sections) + extend with the depth a long report needs — multi-period financial TABLES (revenue/margins/EPS trends from EDGAR XBRL, several years), a peer/valuation comparison table (if cheap), the indicator panel, smart-money + sentiment sections. All Go-injected. Add CHART specs the frontend renders (e.g. reuse KLineChart + a fundamentals-trend sparkline/bar) — the report references chart data the front already has or fetches.
- **Compose**: a `ComposeDeepReport(material, lang)` (stronger OpenRouter model, the Fable-5 harness above) → richer per-section prose + an executive overview; longer token budget than the digest. Keep `Compose` degradation (LLM off → data-only, no error).
- **Gating**: **login required** (Supabase `auth` exists) + a **global per-user daily quota = 1** — store-backed counter keyed by (user_id, ET-day), checked/decremented server-side at generate time; a cached report for that user/day is re-served free (no new quota spend). Distinct from the existing per-stock anonymous digest cap.
- **Frontend**: a dedicated report view (route or expandable) reached from the AI Digest module's top-right button; **fixed styling** — exec summary → sections, each with: Go-fact tables, a chart where it clarifies, prose, and source/原文 links (citations w/ deep anchors). Bilingual single-locale. "AI 生成 · 数字来自公开数据 · 非投资建议".
- **Monetization (later, gated on owner's Yahoo decision)**: the 1/day free quota is the free tier; paid = more/day or PDF — wire the quota now, paywall later.

## Incremental implementation plan (each: opus agent → I review + ship + verify)
1. **Deep-report compose harness** (Go): `ComposeDeepReport` + the Fable-5 system/user prompts + stronger-model routing (OpenRouter, configurable model id), over the existing FactSheet; richer prose + exec overview; contract tests still green (numbers never mutated). API: extend `/v1/stocks/{t}/research` with a `depth=deep` mode OR a new `/v1/stocks/{t}/deep-research` (decide: reuse w/ flag is leaner). NO gating yet (behind a flag / data-only safe).
2. **Login + per-user 1/day quota** (Go + store): per-user daily counter; deep report requires a valid Supabase token; cached per (user, ticker, day) re-served free. 401 for anon, 429 over quota.
3. **Fixed-styling report view + entry button** (web): the report page/view + AI-Digest top-right button; tables + charts + citations + bilingual; defensive.
4. **(Later) Monetization paywall** — gated on Yahoo removal + owner go.

## Open questions for owner (later, non-blocking)
- Exact model id (OpenRouter) for the deep report (cost vs quality) — I'll pick a strong, cost-reasonable default (e.g. a top-tier model) and note the per-report token estimate.
- Whether the 1/day free quota becomes the paid boundary as-is, or free=0/paid-only.

*(Fable 5 prompt-technique source: github asgeirtj/system_prompts_leaks, fetched 2026-06-14. Builds on R2 design: docs/research/2026-06-13-r2-ai-research-design.md.)*
