# AI Deep Research — development plan (flagship PAID feature)

> Owner (2026-06): this is the **most important** feature. Develop it carefully —
> multi-agent subagents + `/loop` small-steps, research → build → check → polish.
> No time limit; do NOT rush it into one session. Use free/redistribution-safe
> data only ([[tickwind-paid-ai-free-sources]]).

## Status snapshot (end of the 2026-06-18/19 session)

**Shipped + deploying this session (all gated green):**
- Real-time price deep-optimization incr 1+2 (LIVE): bulk poller (`alpaca.SnapshotQuotesLive`, cycle 20-60s→~1-2s) + `OverviewTab` live quotes + list-view WS subscribe (`POST /v1/live/subscribe`) + `alpacaws.Streamer.RefreshBase` (5-min base refresh). Commits `d60861e`, `d5ea56e`.
- 5 UI refinements (LIVE): Futu list view (`StockRow`+`useStockListView`/`StockListToggle` on HomeHub+Board), research back-arrow dedup, Filings&Money reorder, News/My 2-col. Commit `bc8aecb`.
- **#1a** dup 5-yr K-line on the deep page — FIXED (`DeepResearchView.tsx:676` `technical||valuation` → `technical`). Commit `3f31144`.
- **#3** Research-tab Deep Research entry — ADDED (exported `DeepEntry`, used in StockView Research tab). Commit `3f31144`.
- **Per-stock AI Digest → Deep Research FUNNEL** (cost): `AISummaryCard` no longer calls the LLM per view; it's now a static funnel to the deep page (reuses `deep.title`/`deep.subtitle`+`DeepEntry`). Market AI summary stays only on the home page. Resolves the "digest loads ~15s then disappears" report. Commit `ff32167`. (`getSummary` now unused by the frontend; endpoint left in place.)

## Diagnosis (root causes found this session)

1. **AI was fully down** = OpenRouter $5 credits **exhausted** ($5.14 used). Owner topped up OpenRouter ($5) + funded DeepSeek (¥10) → AI back. A failed/empty compose reports as `prose_status=llm_disabled` (misleading — it's not truly disabled).
2. **Deep research showed no AI prose** even when the model responds: `LLM_DEEP_MODEL=deepseek/deepseek-r1` is a **reasoning model**; its output (`<think>`/reasoning around the JSON) breaks `enrich.parseSectionProse` (tolerates only a fence + one `{…}` span) → 0 sections → `composeDeepBackground` caches nothing → data-only. The prose RENDER path is fine (`DeepResearchView` overview.prose@295, sec.prose@701, bull/bear@302-307/703-708).
3. **The "Claude Fable 5 harness" was never wired** — it exists only as code comments/naming (`enrich.go` composeDeepPrompt is a generic hierarchical prompt; the client is OpenAI-Chat-Completions only, can't speak Anthropic Messages API).
4. Caps: `researchDailyCap=80` (normal /research prose), `summaryDailyCap=150`, `movementDailyCap=120`, `materialEventsDailyCap=80`, deep quota `DEEP_RESEARCH_MONTHLY_LIMIT=1`/user/month. The normal /research cap (80) had been hit today (fresh composes did NO LLM call, ~1.9s).

## Plan — Increment B: cost-optimal API routing + Claude Fable 5 deep harness

**Routing decision (owner: "看情况和场景使用"):**
- **High-volume / routine LLM** (home morning briefing, news translation, movement explainer, 8-K material-events summaries, normal /research prose) → **DeepSeek direct** (`https://api.deepseek.com`, `deepseek-chat`) — cheapest, uses the new ¥10 key.
- **Deep Research** (low-volume, 1/user/month, premium PAID) → **Claude Opus 4.8 via OpenRouter** (`anthropic/claude-opus-4.8`) — quality, uses the $5.

> **2026-06-19 FINDING (step-1 model verification, live):** `anthropic/claude-fable-5`
> returns **404 "Claude Fable 5 is not available"** on OpenRouter for this account
> (Fable/Mythos requires special Anthropic access). Live-probed the alternatives with
> the real key: **`anthropic/claude-opus-4.8` ✅ works** (provider Anthropic),
> `anthropic/claude-sonnet-4.6` ✅ works, `deepseek/deepseek-r1` ✅ works.
> **Chosen: Opus 4.8** — the strongest available Claude, and at $5/M in + $25/M out it
> is actually CHEAPER than Fable 5 would have been ($10/$50). Est. ~$0.12–0.17/report
> (2–4k in + 3–6k out); ~$5 ≈ ~30–40 reports, fine at the 1/user/month quota pre-paywall.
> Cheaper alternative if more reports/$ wanted: **Sonnet 4.6** — a one-line `.env` swap
> (`LLM_DEEP_MODEL`). All code is env-driven, so the model is not hardcoded anywhere.

**Keys (already staged on the VPS `.env`, NEVER in git):**
- `DEEPSEEK_API_KEY=sk-ef6…` (staged this session) → will become the default `LLM_API_KEY` (DeepSeek direct).
- The current `LLM_API_KEY=sk-or-…` is the **OpenRouter** key → will become `LLM_DEEP_API_KEY` (must copy it BEFORE overwriting `LLM_API_KEY`).

### Implementation steps (post-compact)
1. **Verify the model first** — invoke the `claude-api` skill (knowledge cutoff Jan 2026): confirm the exact Claude Fable 5 model id (`claude-fable-5`?) + that OpenRouter exposes `anthropic/claude-fable-5` + pricing. If not on OpenRouter → native Anthropic client (Route B) or a fallback (Claude Sonnet). Do NOT hardcode from memory.
2. **Backend two-client routing** (`internal/enrich/enrich.go` + `internal/config/config.go`): add `LLM_DEEP_BASE_URL` + `LLM_DEEP_API_KEY` config; in `enrich.New`, build a separate deep client when set; `ComposeDeepReport` uses the deep client (deep model + deep base/key), everything else uses the default client.
3. **Claude compatibility**: drop/guard `response_format:{type:"json_object"}` for the deep call (`enrich.go:601` — Anthropic rejects it); rely on the prompt + a hardened parser.
4. **Harden the parser** (`enrich.go` parseSectionProse@646 + stripFence@770): strip a leading `<think>…</think>`, prefer the LAST balanced `{…}` span — recovers prose from reasoning models + is robust for Claude's fenced output.
5. **Claude-idiomatic deep harness** (`enrich.go` composeDeepPrompt): keep the XML-tagged sections + the anti-hallucination self_check; add an explicit `<output_contract>` naming the required keys (overview/valuation/fundamentals/technical/flows/sentiment + bull/bear); ask for a single fenced ```json block; consider Anthropic prompt-caching on the static system prompt.
6. **VPS `.env`** (one careful SSH; copy OpenRouter key first):
   - `LLM_DEEP_API_KEY` = (current `LLM_API_KEY` value, the OpenRouter key)
   - `LLM_DEEP_BASE_URL=https://openrouter.ai/api/v1`
   - `LLM_DEEP_MODEL=anthropic/claude-opus-4.8` (per step-1 finding: Fable 5 is 404/gated; Opus 4.8 is the chosen, cheaper-than-Fable premium Claude)
   - `LLM_BASE_URL=https://api.deepseek.com`
   - `LLM_API_KEY=$DEEPSEEK_API_KEY` (the staged DeepSeek key)
   - `LLM_MODEL=deepseek-chat`
   - then `cd /root/tickwind && docker compose up -d api` (recreates, re-reads .env; no rebuild needed for env-only) — BUT the code changes (steps 2-5) need a full deploy (`/root/deploy-ptr.sh`) first.
7. **Gate + verify**: Go `gofmt/build/vet/test -race`; web build. Deploy. **Verify business result** ([[verify-business-result]]): logged-in deep research on a fresh ticker produces real Claude prose (overview + per-section + bull/bear), `model` reflects Claude; routine AI (briefing/movement/normal research) works on DeepSeek-direct; check OpenRouter + DeepSeek usage to confirm the right model billed where. Watch the SSH flakiness ([[racknerd-ssh-flakiness]]) + the cold-tunnel reset ([[tickwind-cold-research-3s-reset]]).

### Open product decisions to surface
- Deep model: Claude Fable 5 (premium) vs DeepSeek-reasoner (cheap) — leaning Fable 5 (owner asked; low-volume). Confirm cost acceptable.
- The normal /research-tab report (free, LLM, cap 80) is ALSO a per-stock LLM cost — owner only removed the Overview digest; revisit if it's too costly (could also funnel, or keep cheap on DeepSeek-direct).
- Deep quota (1/user/month) — with Fable 5, keep free-1/month or adjust before any paywall.

## Pointers
- Render path: `web/src/components/DeepResearchView.tsx` (prose renders fine). Route: `app/[locale]/(main)/stock/[ticker]/research/page.tsx`. Entry: `DeepEntry` (AISummaryCard.tsx, exported).
- Backend: `getResearchDeep`/`composeDeepBackground` (`internal/api/api.go` ~2713/2824), `internal/research` ComposeDeep/DeepModel, `internal/enrich` ComposeDeepReport.
- Full investigation transcript: this session's workflow `wf_443c347e-5a0`.
