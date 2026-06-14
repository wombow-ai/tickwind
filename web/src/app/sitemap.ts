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

import {LOCALES} from '@/lib/locale';

/**
 * Path-based bilingual hreflang alternates for an un-prefixed sitemap `path`
 * (e.g. `/hot`). Every page is served at `/en${path}` and `/zh${path}`, so each
 * entry advertises both language variants + x-default — letting search engines
 * index and language-target both.
 */
function langAlt(path: string): {languages: Record<string, string>} {
  const suffix = path === '/' ? '' : path;
  const en = `${SITE_URL}/en${suffix}`;
  const zh = `${SITE_URL}/zh${suffix}`;
  return {languages: {en, 'zh-CN': zh, 'x-default': en}};
}

/** An indexable page keyed by its locale-less `path` (e.g. `/hot`). */
type Page = {
  path: string;
  changeFrequency: MetadataRoute.Sitemap[number]['changeFrequency'];
  priority: number;
};

/**
 * Sitemap of the public, indexable pages: the hub + section pages, and a stock
 * page per indexable ticker. Per-user/auth routes are intentionally omitted.
 * Path-based i18n: each page is emitted twice (`/en${path}` and `/zh${path}`),
 * and every entry carries the en / zh / x-default hreflang alternates.
 */
export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const now = new Date();
  const staticPages: Page[] = [
    {path: '/', changeFrequency: 'hourly', priority: 1},
    {path: '/opportunities', changeFrequency: 'daily', priority: 0.7},
    {path: '/smart-money', changeFrequency: 'daily', priority: 0.7},
    {path: '/hot', changeFrequency: 'hourly', priority: 0.7},
    {path: '/screen', changeFrequency: 'daily', priority: 0.6},
    {path: '/calendar/earnings', changeFrequency: 'daily', priority: 0.6},
    {path: '/calendar/ipo', changeFrequency: 'daily', priority: 0.6},
    {path: '/news', changeFrequency: 'hourly', priority: 0.6},
    {path: '/discussion', changeFrequency: 'hourly', priority: 0.6},
    {path: '/calendar/macro', changeFrequency: 'daily', priority: 0.6},
    {path: '/unusual', changeFrequency: 'daily', priority: 0.6},
    {path: '/indicators', changeFrequency: 'monthly', priority: 0.6},
    {path: '/guide', changeFrequency: 'weekly', priority: 0.6},
    {path: '/announcements', changeFrequency: 'weekly', priority: 0.5},
  ];
  const guidePages: Page[] = GUIDES.map(g => ({
    path: `/guide/${g.slug}`,
    changeFrequency: 'monthly',
    priority: 0.6,
  }));
  const [tickers, memberSlugs, funds, indicators] = await Promise.all([
    indexableTickers(),
    congressMemberSlugs(),
    fundSlugs(),
    indicatorSlugs(),
  ]);
  const stockPages: Page[] = tickers.map(ticker => ({
    path: `/stock/${encodeURIComponent(ticker)}`,
    changeFrequency: 'hourly',
    priority: 0.8,
  }));
  const memberPages: Page[] = memberSlugs.map(slug => ({
    path: `/congress/member/${encodeURIComponent(slug)}`,
    changeFrequency: 'daily',
    priority: 0.6,
  }));
  const fundPages: Page[] = funds.map(slug => ({
    path: `/fund/${encodeURIComponent(slug)}`,
    changeFrequency: 'weekly',
    priority: 0.6,
  }));
  const indicatorPages: Page[] = indicators.map(slug => ({
    path: `/indicators/${encodeURIComponent(slug)}`,
    changeFrequency: 'monthly',
    priority: 0.6,
  }));
  const pages: Page[] = [
    ...staticPages,
    ...guidePages,
    ...stockPages,
    ...memberPages,
    ...fundPages,
    ...indicatorPages,
  ];
  // Emit one entry per (page × locale), each advertising both language variants.
  return pages.flatMap(p =>
    LOCALES.map(locale => ({
      url: `${SITE_URL}/${locale}${p.path === '/' ? '' : p.path}`,
      lastModified: now,
      changeFrequency: p.changeFrequency,
      priority: p.priority,
      alternates: langAlt(p.path),
    })),
  );
}
