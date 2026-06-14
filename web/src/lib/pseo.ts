/**
 * Shared pSEO ticker-universe helpers, used by both the sitemap and the
 * `/stock/[ticker]` page's `generateStaticParams`. Kept DRY here so the two
 * stay in sync; every fetch is best-effort (short timeout + graceful fallback)
 * so a slow/down API can never break a build or sitemap generation.
 */

import {getHot, getOpportunities, getUniverseSymbols} from '@/lib/api';
import {POPULAR_TICKERS} from '@/lib/config';

/**
 * The *popular* ticker subset: the curated `POPULAR_TICKERS` ∪ every live-board
 * ticker (hot / surging / WSB / opportunities). These are the highest-traffic,
 * guaranteed-has-real-data names (~hundreds). This is the set we PRE-RENDER
 * (`generateStaticParams`) — small enough that `npm run build` stays bounded;
 * everything else stays dynamic ISR.
 *
 * Best-effort: a slow/down API just yields the static popular list, never an
 * error — so the build never breaks.
 */
export async function popularTickers(): Promise<string[]> {
  const set = new Set<string>(POPULAR_TICKERS);
  const signal = AbortSignal.timeout(5000);
  const results = await Promise.allSettled([
    getHot('hot', 40, signal),
    getHot('surging', 40, signal),
    getHot('wsb', 40, signal),
    getOpportunities(40, signal),
  ]);
  for (const r of results) {
    if (r.status !== 'fulfilled') continue;
    // getOpportunities returns {stocks: OpportunityStock[]}; getHot returns
    // {stocks: HotStock[]} — both expose a `ticker` per row.
    for (const s of (r.value as {stocks?: {ticker?: string}[]}).stocks ?? []) {
      if (s?.ticker) set.add(s.ticker);
    }
  }
  return [...set];
}

/**
 * The *quote-bearing* ticker universe (~6,700): every symbol the server has a
 * live price for, via `GET /v1/universe/symbols` — each has real, ingestible
 * content (live price + indicators + 52w range), the natural "not thin" set.
 * This is the full price universe (NOT capped at 200 like `/v1/screen`), a
 * strict subset of `/v1/symbols`' ~16k full index (the ~9,400 quote-less names
 * are excluded). Deduped against `POPULAR_TICKERS` by the caller via a Set.
 * Best-effort: a slow/down API yields `[]` (the popular set still ships).
 *
 * NOTE: this universe (the Alpaca snapshot) currently excludes S&P mega-caps
 * (AAPL/MSFT/…); they are covered in the sitemap via {@link popularTickers}.
 */
export async function quoteBearingTickers(): Promise<string[]> {
  try {
    return await getUniverseSymbols(AbortSignal.timeout(8000));
  } catch {
    // API hiccup → no expansion this build; the popular set still ships.
    return [];
  }
}
