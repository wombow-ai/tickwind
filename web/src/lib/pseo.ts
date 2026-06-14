/**
 * Shared pSEO ticker-universe helpers, used by both the sitemap and the
 * `/stock/[ticker]` page's `generateStaticParams`. Kept DRY here so the two
 * stay in sync; every fetch is best-effort (short timeout + graceful fallback)
 * so a slow/down API can never break a build or sitemap generation.
 */

import {getHot, getOpportunities, getScreen} from '@/lib/api';
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
 * The *quote-bearing* ticker universe: every symbol the screener returns (it
 * iterates the live price cache and drops anything without a usable price), so
 * each one has real, ingestible content — the natural "not thin" set. We use
 * the highest sort + a high `limit`; the backend currently caps `/v1/screen` at
 * 200 rows, so this returns up to 200 quote-bearing tickers (still a meaningful
 * expansion over the ~popular set). Deduped against `POPULAR_TICKERS` by the
 * caller via a Set. Best-effort: a slow/down API yields `[]`.
 *
 * @param limit upper bound requested from the API (the server clamps it).
 */
export async function quoteBearingTickers(limit = 5000): Promise<string[]> {
  try {
    const r = await getScreen({limit}, AbortSignal.timeout(8000));
    const out: string[] = [];
    for (const row of r.results ?? []) {
      if (row?.ticker) out.push(row.ticker);
    }
    return out;
  } catch {
    // API hiccup → no expansion this build; the popular set still ships.
    return [];
  }
}
