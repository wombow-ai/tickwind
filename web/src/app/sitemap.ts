import type {MetadataRoute} from 'next';
import {getHot, getOpportunities} from '@/lib/api';
import {POPULAR_TICKERS, SITE_URL} from '@/lib/config';

// Regenerate hourly so newly-trending tickers enter the sitemap without a deploy.
export const revalidate = 3600;

/**
 * The indexable stock universe: the popular set ∪ every live-board ticker
 * (hot / surging / WSB / opportunities). These all have real ingested data, so
 * we avoid thin-content pages while covering far more than the static popular
 * list. Fetched with a short timeout + graceful fallback so a slow/down API
 * never breaks sitemap generation.
 */
async function indexableTickers(): Promise<string[]> {
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
    for (const s of r.value.stocks ?? []) {
      if (s?.ticker) set.add(s.ticker);
    }
  }
  return [...set];
}

/**
 * Sitemap of the public, indexable pages: the hub + section pages, and a stock
 * page per indexable ticker. Per-user/auth routes are intentionally omitted.
 */
export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const now = new Date();
  const staticPages: MetadataRoute.Sitemap = [
    {url: `${SITE_URL}/`, lastModified: now, changeFrequency: 'hourly', priority: 1},
    {url: `${SITE_URL}/opportunities`, lastModified: now, changeFrequency: 'daily', priority: 0.7},
    {url: `${SITE_URL}/smart-money`, lastModified: now, changeFrequency: 'daily', priority: 0.7},
    {url: `${SITE_URL}/hot`, lastModified: now, changeFrequency: 'hourly', priority: 0.7},
    {url: `${SITE_URL}/screen`, lastModified: now, changeFrequency: 'daily', priority: 0.6},
    {url: `${SITE_URL}/earnings`, lastModified: now, changeFrequency: 'daily', priority: 0.6},
    {url: `${SITE_URL}/news`, lastModified: now, changeFrequency: 'hourly', priority: 0.6},
    {url: `${SITE_URL}/discussion`, lastModified: now, changeFrequency: 'hourly', priority: 0.6},
    {url: `${SITE_URL}/events`, lastModified: now, changeFrequency: 'daily', priority: 0.6},
    {url: `${SITE_URL}/briefing`, lastModified: now, changeFrequency: 'daily', priority: 0.6},
    {url: `${SITE_URL}/community`, lastModified: now, changeFrequency: 'daily', priority: 0.6},
    {url: `${SITE_URL}/announcements`, lastModified: now, changeFrequency: 'weekly', priority: 0.5},
  ];
  const stockPages: MetadataRoute.Sitemap = (await indexableTickers()).map(ticker => ({
    url: `${SITE_URL}/stock/${encodeURIComponent(ticker)}`,
    lastModified: now,
    changeFrequency: 'hourly',
    priority: 0.8,
  }));
  return [...staticPages, ...stockPages];
}
