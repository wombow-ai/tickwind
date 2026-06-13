import type {MetadataRoute} from 'next';
import {
  congressSlug,
  getCongress,
  getHot,
  getIndicators,
  getOpportunities,
  getThirteenF,
  indicatorSlug,
} from '@/lib/api';
import {POPULAR_TICKERS, SITE_URL} from '@/lib/config';
import {GUIDES} from '@/lib/guides';

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
 * The indexable Congress members: every filer of a Periodic Transaction Report
 * (filing_type "P") in the recent feed, deduped by their canonical slug. These
 * back the `/congress/member/{slug}` pSEO pages. Tolerant — a slow/down API just
 * yields an empty list, never breaking sitemap generation.
 */
async function congressMemberSlugs(): Promise<string[]> {
  const slugs = new Set<string>();
  try {
    const r = await getCongress(250, AbortSignal.timeout(5000));
    for (const f of r.filings ?? []) {
      if (f.filing_type === 'P' && f.name) slugs.add(congressSlug(f.name));
    }
  } catch {
    // API hiccup → skip member pages this build; the rest of the sitemap stands.
  }
  return [...slugs];
}

/**
 * The indexable 13F funds: every famous-manager slug on the whale-holdings
 * board. These back the `/fund/{slug}` pSEO pages. Tolerant — a slow/down API
 * just yields an empty list, never breaking sitemap generation.
 */
async function fundSlugs(): Promise<string[]> {
  const slugs = new Set<string>();
  try {
    const r = await getThirteenF(AbortSignal.timeout(5000));
    for (const f of r.funds ?? []) {
      if (f.slug) slugs.add(f.slug);
    }
  } catch {
    // API hiccup → skip fund pages this build; the rest of the sitemap stands.
  }
  return [...slugs];
}

/**
 * The indexable indicators: the slug of every stock-applicable catalog record,
 * backing the `/indicators/{slug}` pSEO pages. Real-data-driven — tolerant of a
 * slow/down API (yields an empty list, never breaking sitemap generation).
 */
async function indicatorSlugs(): Promise<string[]> {
  try {
    const r = await getIndicators({}, AbortSignal.timeout(5000));
    return r.indicators.map(ind => indicatorSlug(ind.id));
  } catch {
    // API hiccup → skip indicator pages this build; the rest of the sitemap stands.
    return [];
  }
}

/**
 * Bilingual hreflang alternates for a sitemap URL. Every page is served in both
 * languages via a `?lang=zh|en` param, so each entry advertises its en / zh
 * variants — letting search engines index and language-target both.
 */
function langAlt(url: string): {languages: Record<string, string>} {
  return {languages: {en: `${url}?lang=en`, 'zh-CN': `${url}?lang=zh`}};
}

/**
 * Sitemap of the public, indexable pages: the hub + section pages, and a stock
 * page per indexable ticker. Per-user/auth routes are intentionally omitted.
 * Every entry carries en / zh hreflang alternates (URL-level i18n via `?lang=`).
 */
export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const now = new Date();
  const staticPages: MetadataRoute.Sitemap = [
    {url: `${SITE_URL}/`, lastModified: now, changeFrequency: 'hourly', priority: 1},
    {url: `${SITE_URL}/opportunities`, lastModified: now, changeFrequency: 'daily', priority: 0.7},
    {url: `${SITE_URL}/smart-money`, lastModified: now, changeFrequency: 'daily', priority: 0.7},
    {url: `${SITE_URL}/hot`, lastModified: now, changeFrequency: 'hourly', priority: 0.7},
    {url: `${SITE_URL}/screen`, lastModified: now, changeFrequency: 'daily', priority: 0.6},
    {url: `${SITE_URL}/calendar/earnings`, lastModified: now, changeFrequency: 'daily', priority: 0.6},
    {url: `${SITE_URL}/calendar/ipo`, lastModified: now, changeFrequency: 'daily', priority: 0.6},
    {url: `${SITE_URL}/news`, lastModified: now, changeFrequency: 'hourly', priority: 0.6},
    {url: `${SITE_URL}/discussion`, lastModified: now, changeFrequency: 'hourly', priority: 0.6},
    {url: `${SITE_URL}/calendar/macro`, lastModified: now, changeFrequency: 'daily', priority: 0.6},
    {url: `${SITE_URL}/unusual`, lastModified: now, changeFrequency: 'daily', priority: 0.6},
    {url: `${SITE_URL}/indicators`, lastModified: now, changeFrequency: 'monthly', priority: 0.6},
    {url: `${SITE_URL}/guide`, lastModified: now, changeFrequency: 'weekly', priority: 0.6},
    {url: `${SITE_URL}/announcements`, lastModified: now, changeFrequency: 'weekly', priority: 0.5},
  ];
  const guidePages: MetadataRoute.Sitemap = GUIDES.map(g => ({
    url: `${SITE_URL}/guide/${g.slug}`,
    lastModified: now,
    changeFrequency: 'monthly',
    priority: 0.6,
  }));
  const [tickers, memberSlugs, funds, indicators] = await Promise.all([
    indexableTickers(),
    congressMemberSlugs(),
    fundSlugs(),
    indicatorSlugs(),
  ]);
  const stockPages: MetadataRoute.Sitemap = tickers.map(ticker => ({
    url: `${SITE_URL}/stock/${encodeURIComponent(ticker)}`,
    lastModified: now,
    changeFrequency: 'hourly',
    priority: 0.8,
  }));
  const memberPages: MetadataRoute.Sitemap = memberSlugs.map(slug => ({
    url: `${SITE_URL}/congress/member/${encodeURIComponent(slug)}`,
    lastModified: now,
    changeFrequency: 'daily',
    priority: 0.6,
  }));
  const fundPages: MetadataRoute.Sitemap = funds.map(slug => ({
    url: `${SITE_URL}/fund/${encodeURIComponent(slug)}`,
    lastModified: now,
    changeFrequency: 'weekly',
    priority: 0.6,
  }));
  const indicatorPages: MetadataRoute.Sitemap = indicators.map(slug => ({
    url: `${SITE_URL}/indicators/${encodeURIComponent(slug)}`,
    lastModified: now,
    changeFrequency: 'monthly',
    priority: 0.6,
  }));
  return [
    ...staticPages,
    ...guidePages,
    ...stockPages,
    ...memberPages,
    ...fundPages,
    ...indicatorPages,
  ].map(entry => ({
    ...entry,
    alternates: langAlt(entry.url),
  }));
}
