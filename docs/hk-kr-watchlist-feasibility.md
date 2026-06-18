# Follow-a-few KR + HK tickers — feasibility (researched 2026-06)

Owner wants to track a **handful of specific foreign names**, not whole markets:
- 🇰🇷 Korea: **SK Hynix `000660.KS`**, **Samsung Electronics `005930.KS`**.
- 🇭🇰 Hong Kong: **Tencent `0700.HK`**, **智谱 / Z.ai `02513.HK`**, **MiniMax `00100.HK`**.

## Headline: the two AI names ARE now listed (changed Jan 2026)
Earlier we assumed Zhipu/MiniMax were pre-IPO. They are not anymore — both
completed Hong Kong IPOs in the first week of **January 2026** (the "AI tigers"
wave):
- **智谱 / Z.ai** (Beijing Zhipu Huazhang) — **`02513.HK`**, debuted 8 Jan 2026
  (billed "first listed LLM company"; IPO HK$116.20).
- **MiniMax** — **`00100.HK`** (HKEX entity "MINIMAX GROUP INC. - W"; the `-W`
  suffix = weighted-voting-rights share class), debuted 9 Jan 2026.
  ⚠️ confirm zero-padding our quote source expects (`0100.HK` vs `00100.HK`).

## Per-ticker feasibility
| Ticker | Listed | Free + redistribution-clean? | How to reach it on $0 |
|---|---|---|---|
| `000660.KS` SK Hynix | ✅ KOSPI | prices: **no** (needs key) | **free KRX Open API key** (registration + auth key) — our existing KR path |
| `005930.KS` Samsung | ✅ KOSPI | prices: **no** | same free KRX key (one key covers both) |
| `0700.HK` Tencent | ✅ HKEX | **filings/symbols: yes**; quotes: **no** | HKEXnews filings + Securities List (free); price = gray or skip |
| `02513.HK` Z.ai | ✅ HKEX | same | same |
| `00100.HK` MiniMax | ✅ HKEX | same | same |

## The two walls
1. **Korea prices** still require the **free KRX Open API key** (`openapi.krx.co.kr`,
   registration-gated). Limiting to 2 tickers does **not** remove the key
   requirement — there's no clean free per-ticker KR price feed. Our `KRAdapter`
   is code-ready and inert until `KRX_API_KEY` is set; `koreaSeed` is now exactly
   `{005930.KS, 000660.KS}`. **Owner action: drop the free KRX key to go live.**
2. **Hong Kong prices** are **HKEX Market-Data Vendor-Licence-gated — real-time
   *and* 15-min delayed** — with no free redistribution tier, regardless of ticker
   count (the licence attaches to the *activity* of redistributing HKEX data, not
   the volume). What **is** free + clean: **HKEXnews filings/announcements** and the
   **HKEX Securities List** (symbol/name reference).

## Recommendation
- **KR:** ready now — just needs the owner's free KRX key. No code blocker.
- **HK:** ship the **free clean slice = filings-only** for `0700/02513/00100`
  (HKEXnews + Securities List) so the names aren't "given up on" — announcements,
  no live price. Live HK price needs one of: (a) accept a gray delayed-quote
  source (e.g. Yahoo `0700.HK`) — third-party-ToS risk, prototype-grade; (b) pay
  for an HKEX vendor licence; (c) skip price (filings only). **Owner decision.**
- Build cost for HK filings-only: a small `internal/hkex` client (HKEXnews
  list + Securities List) + an `HK` market in `market.Of` (`.HK` suffix) + an
  `HKAdapter` (filings only, `Quote` returns empty) wired like `TWAdapter`.

## 2026-06-18 — probe: HK filings is now BUILDABLE (the `prefix.do` "empty" was a param bug, NOT IP-gating)
Two read-only VPS probes (owner long-absent → bounded autonomous investigation, no build):
- **Residential proxy works** (egress IP: direct `104.168.38.21` → proxied `115.76.50.232`) — but it was NOT the unblock.
- **The real blocker was a stale query param.** `prefix.do?...&lang=en` (lowercase) returns 200 + an empty
  `callback(...)` (~2 bytes); **`lang=EN` (uppercase) returns the full JSONP** — e.g. `name=0700` →
  `callback({"more":"1","stockInfo":[{"stockId":7609,"code":"00700","name":"TENCENT"}]});` (`name=tencent` also
  matches). So **code→stockId resolution WORKS** — the long-standing "prefix.do empty from here / datacenter-IP-
  gated" note was a stale-param artifact, not IP-gating. (Two vars changed across the probes [lang + proxy], so
  whether it ALSO needs the proxy is unisolated — try direct first at build time, fall back to `config.ProxyClient`.)
- `titlesearch.xhtml` reachable (200, ~12 KB HTML). The static **HKEX `ListOfSecurities.xlsx` downloads** (200,
  1.4 MB, real xlsx) — a code→name reference / static stockId-map fallback.
- Not yet probed (recon at build time): `titleSearchServlet.do?...&stockId=<id>&...&lang=EN` = the filings list
  for a stockId (the page + prefix.do working strongly implies it works).

**Verdict: HK filings-only is feasible + buildable** for the owner's 3 names — Tencent `0700` (stockId **7609**),
智谱 `02513`, MiniMax `00100` — restoring value lost when Yahoo (their only quote source) was removed 2026-06-17.
**Build plan** (mirrors `TWAdapter`): a small `internal/hkex` client (`prefix.do`/static-list for code→stockId +
`titleSearchServlet.do` for filings) → an `HKAdapter` (filings only; `Quote` empty) wired into the ingest
scheduler + `market.Of` (`.HK`) → reuse the existing filings/material-events card on the frontend. Free +
redistribution-clean (HKEXnews announcements are public). **Owner decision — greenlight the build?** Surfaced
rather than auto-built: it's a deferred roadmap item + a genuine product/priority call. One word ("做 HK 财报")
starts it.
