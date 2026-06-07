# Storage + tiered retention plan (verified 2026-06)

## Current state
Two backends behind `store.Store`, routed by `store.Split{Market,User}`. **Five
Market-side tables grow unbounded — pure upsert, zero TTL/DELETE** (de-facto
"keep everything forever"):
- `news` (`published`), `social` (`created_at`) — largest by bytes.
- `seen_form4` (`filed_date`) — fastest row churn (one row per Form-4 scanned).
- `insider_buys` (`filed_date`), `filings` (`filed_at`) — slower.
Self-bounding (no work): `quotes`/`signals` (one row/key, overwritten), `hotlist`
(replaced wholesale each cycle). `watchlist`/`clips` = User-side, low volume.

Key fact: **KOL/Serenity posts live in the `social` table with `source="substack"`**
→ the protect-rule maps to a real column value (no schema change).

## Design — a `Pruner` (optional, type-asserted; Split → Market)
```go
type Pruner interface {
  PruneNews(ctx, before, hotBefore time.Time) (int64, error)
  PruneSocial(ctx, before, hotBefore time.Time, protectSources []string) (int64, error)
  PruneFilings(ctx, before) (int64, error)
  PruneInsiderBuys(ctx, before) (int64, error)
  PruneSeenForm4(ctx, before) (int64, error)
  CapPerTicker(ctx, table string, n int) (int64, error) // backstop, table∈{news,social}
}
```
Postgres = `pool.Exec` DELETEs (return RowsAffected); memory = locked map sweeps
(also fixes dev-time growth); Split forwards to `Market`, no-op if unsupported.

### Per-table windows (env-tunable; 0 disables)
| table | rule | default |
|---|---|---|
| news | delete `published < now-N`, keep hot-list tickers longer | 60d (hot 120d) |
| social | delete `created_at < now-N`, **except `source=ANY(protect)`** + hot longer | 30d (hot 90d) |
| filings | delete `filed_at < now-N` | 730d |
| insider_buys | delete `filed_date < now-N` (reader window 30d) | 90d |
| seen_form4 | delete `filed_date < now-N` (reader window max(backfill+7,40)d) | 60d |
| news/social cap | `CapPerTicker` newest-N backstop | 500 |

### Protect-rules
1. **Never prune `source="substack"`** (KOL/Serenity rail) — `AND source <> ALL($protect)`.
2. **Hot-list tickers keep the longer window** — `AND NOT (ticker IN (SELECT ticker FROM hotlist) AND ts >= $hotBefore)`. (Per-post engagement isn't stored — `store.Post` has no score/upvotes column; heat lives in hotlist/signals at ticker granularity. True per-post retention would need a `social.score` column — deferred.)
3. **Keep filings + insider_buys** on long/cheap windows.

### "Unattended" tickers
The ingest set (`WATCHLIST ∪ seeds ∪ AllWatchlistTickers`, cap 200) is the gate —
the scheduler only ingests attended tickers, so unattended ones barely accumulate.
**Use prune-after-the-fact** (the time windows naturally drain dropped/once-hot
tickers); no need to enumerate unattended tickers. Don't skip-at-ingest (you want
a short tail for just-trending names); cap is only a backstop.

### Mechanism
`internal/ingest/prune.go` — a `Pruner` goroutine mirroring `GuruIngestor.Run`
(initial pass + ticker). Cadence `PRUNE_EVERY=6h`, off the request path. Config
knobs: `RETAIN_NEWS_DAYS`/`_HOT_DAYS`, `RETAIN_SOCIAL_DAYS`/`_HOT_DAYS`,
`RETAIN_FILINGS_DAYS`, `RETAIN_INSIDER_DAYS`, `RETAIN_SEEN_FORM4_DAYS`,
`PROTECT_SOCIAL_SOURCES`(=substack), `CAP_NEWS/SOCIAL_PER_TICKER`, `PRUNE_EVERY`.

### Effect
Moves `news`/`social`/`seen_form4` from linear-in-time growth → bounded steady
state (seen_form4 ~85-95% reduction). Fits the Oracle free VM + Supabase free tier.

### Build checklist — ✅ BUILT (2026-06)
store.Pruner iface → postgres impls (pool.Exec DELETEs) → memory impls (map sweeps + hotTickerSet()) → Split forwards to Market → config knobs (RetentionConfig) → internal/ingest/prune.go (Pruner goroutine, initial pass + 6h ticker) → wired in main (type-asserted) → prune_test.go (memory: window/hot/protected/cap; Split routes to Market). All green; deployed.

---

## OTC / pink-sheet finding (SIVEF, LPKFF)
- SEC's `company_tickers_exchange.json` has a 2,539-row **OTC bucket of SEC-reporting foreign shares** — **already in our index** (FetchUS doesn't filter exchange). Verified live: `RTNTF`(Rio Tinto), `IFNNY`(Infineon) **are searchable**.
- **SIVEF / LPKFF are NOT SEC-registered** (foreign ordinaries) → structurally absent from any EDGAR-derived file. No free + redistribution-safe source carries them: Yahoo returns them (SIVEF $7.77, LPKFF $23.34) but its ToS is **commercial-gray**; OTC Markets is authoritative but **~$400/mo + $1,500 setup**.
- **Verdict:** can't add SIVEF/LPKFF on $0-clean. Options: (a) accept Yahoo gray ToS (prototype-only), (b) pay OTC Markets, (c) skip. We already cover SEC-reporting OTC/ADRs for free.
