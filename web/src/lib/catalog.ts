import catalogData from '@/data/indicatorCatalog.json';
import type {Indicator, IndicatorParams, IndicatorsResponse} from '@/lib/api';

/**
 * The indicator catalog as a BUNDLED static dataset, read synchronously with no network call.
 *
 * Why bundled: the catalog endpoint (`GET /v1/indicators`) is ~150KB, and Vercel's build AND
 * runtime cannot reliably fetch a payload that large through the Cloudflare tunnel — every
 * /indicators and /indicators/[id] page baked/served as the loading fallback ("Indicator ·
 * Tickwind", no content). The catalog is STATIC reference metadata (formulas/definitions, not
 * live market data), so bundling a copy makes those pSEO pages render reliably server-side.
 *
 * This module is imported ONLY by server components (the indicator library + detail pages), so
 * the JSON stays in the SERVER bundle and never ships to the browser. Client callers (the
 * picker) keep using `getIndicators` (a real fetch, which works fine in the browser via CORS).
 *
 * REGENERATE on a backend dataset change:
 *   curl -s https://api.tickwind.com/v1/indicators > web/src/data/indicatorCatalog.json
 */
const CATALOG = catalogData as unknown as IndicatorsResponse;

/** The full catalog filtered by the same params `getIndicators` accepts — sync, never throws. */
export function localCatalog(params: IndicatorParams = {}): IndicatorsResponse {
  let inds: Indicator[] = CATALOG.indicators;
  if (params.domain) inds = inds.filter(i => i.domain === params.domain);
  if (params.priority) inds = inds.filter(i => i.priority === params.priority);
  if (params.subcategory) inds = inds.filter(i => i.subcategory === params.subcategory);
  if (params.q) {
    const q = params.q.toLowerCase();
    inds = inds.filter(i =>
      `${i.id} ${i.name_en} ${i.name_zh} ${i.abbr ?? ''}`.toLowerCase().includes(q),
    );
  }
  return {...CATALOG, indicators: inds, count: inds.length};
}
