import type {MetadataRoute} from 'next';
import {
  congressSlug,
  getCongress,
  getIndicators,
  getThirteenF,
  getTopics,
  indicatorSlug,
} from '@/lib/api';
import {SITE_URL} from '@/lib/config';
import {GUIDES} from '@/lib/guides';
import {SCREEN_PRESETS} from '@/lib/presets';
import {SIGNAL_SCREEN_PRESETS} from '@/lib/signalPresets';
import {ZONES} from '@/lib/zones';
import {popularTickers, quoteBearingTickers, STOCK_DIRECTORY_LETTERS} from '@/lib/pseo';

// Regenerate hourly so newly-trending tickers enter the sitemap without a deploy.
export const revalidate = 3600;

/** Hard ceiling on `/stock/*` entries PER LOCALE — a sanity cap so a future,
 *  larger quote universe can't bloat the sitemap (and we never dump the full
 *  ~16k SEC+Nasdaq listing, only quote-bearing names). */
// Measured cap for a young domain: the full quote-bearing universe is ~6,700,
// but we ramp indexable coverage gradually (popular set first, then alphabetical
// fill) rather than dumping every page at once. Lift toward ~6,700 as the domain
// gains crawl authority. The popular set is unioned FIRST so the highest-value
// names (incl. the mega-caps the Alpaca universe omits) are never sliced off.
const MAX_STOCK_URLS = 3000;

/**
 * The indexable stock universe: the popular set ∪ the quote-bearing price
 * universe (`/v1/universe/symbols` — every symbol with a usable live price, the
 * natural "has real content" set, so we avoid thin/delisted pages). Deduped,
 * popular-first, capped at {@link MAX_STOCK_URLS}. Both sub-fetches are
 * best-effort (short timeout + graceful fallback) so a slow/down API never
 * breaks sitemap generation; at worst we fall back to just the popular set.
 */
async function indexableTickers(): Promise<string[]> {
  const [popular, quoted] = await Promise.all([
    popularTickers(),
    quoteBearingTickers(),
  ]);
  const set = new Set<string>(popular);
  for (const t of quoted) set.add(t);
  return [...set].slice(0, MAX_STOCK_URLS);
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
 * The indexable trending topics: the key of every Hot Topic in the live snapshot,
 * backing the `/topic/{key}` pSEO pages. Topics are volatile (they trend through
 * the day) so this captures the build/revalidate-time set; ISR + dynamicParams
 * cover the rest. Tolerant — a slow/down API just yields an empty list.
 */
async function topicKeys(): Promise<string[]> {
  try {
    const r = await getTopics(AbortSignal.timeout(5000));
    return (r.topics ?? []).map(t => t.key);
  } catch {
    // API hiccup → skip topic pages this build; the rest of the sitemap stands.
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
    {path: '/screen/signals', changeFrequency: 'daily', priority: 0.6},
    {path: '/zone', changeFrequency: 'weekly', priority: 0.7},
    {path: '/calendar/earnings', changeFrequency: 'daily', priority: 0.6},
    {path: '/calendar/ipo', changeFrequency: 'daily', priority: 0.6},
    {path: '/news', changeFrequency: 'hourly', priority: 0.6},
    {path: '/discussion', changeFrequency: 'hourly', priority: 0.6},
    {path: '/calendar/macro', changeFrequency: 'daily', priority: 0.6},
    {path: '/unusual', changeFrequency: 'daily', priority: 0.6},
    {path: '/indicators', changeFrequency: 'monthly', priority: 0.6},
    {path: '/guide', changeFrequency: 'weekly', priority: 0.6},
    {path: '/stocks', changeFrequency: 'weekly', priority: 0.6},
    {path: '/announcements', changeFrequency: 'weekly', priority: 0.5},
    {path: '/contact', changeFrequency: 'yearly', priority: 0.3},
  ];
  // The A–Z stock directory: the hub (above) + one page per letter, aiding crawl
  // discovery of the thousands of `/stock/{t}` pages they internally link.
  const stockDirectoryPages: Page[] = STOCK_DIRECTORY_LETTERS.map(letter => ({
    path: `/stocks/${letter}`,
    changeFrequency: 'weekly',
    priority: 0.5,
  }));
  const guidePages: Page[] = GUIDES.map(g => ({
    path: `/guide/${g.slug}`,
    changeFrequency: 'monthly',
    priority: 0.6,
  }));
  // Curated screener landing pages (`/screen/{preset}`) — intraday movers, so a
  // daily change frequency mirrors the interactive /screen hub.
  const presetPages: Page[] = SCREEN_PRESETS.map(p => ({
    path: `/screen/${p.key}`,
    changeFrequency: 'daily',
    priority: 0.6,
  }));
  // Curated SIGNAL-screener landing pages (`/screen/signals/{preset}`).
  const signalPresetPages: Page[] = SIGNAL_SCREEN_PRESETS.map(p => ({
    path: `/screen/signals/${p.key}`,
    changeFrequency: 'daily',
    priority: 0.6,
  }));
  // Curated theme zones (`/zone/{key}`) — the AI flagship + 10x theme siblings.
  const zonePages: Page[] = ZONES.map(z => ({
    path: `/zone/${z.key}`,
    changeFrequency: 'weekly',
    priority: 0.7,
  }));
  const [tickers, memberSlugs, funds, indicators, topics] = await Promise.all([
    indexableTickers(),
    congressMemberSlugs(),
    fundSlugs(),
    indicatorSlugs(),
    topicKeys(),
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
  const topicPages: Page[] = topics.map(key => ({
    path: `/topic/${encodeURIComponent(key)}`,
    changeFrequency: 'daily',
    priority: 0.5,
  }));
  const pages: Page[] = [
    ...staticPages,
    ...guidePages,
    ...presetPages,
    ...signalPresetPages,
    ...zonePages,
    ...stockDirectoryPages,
    ...stockPages,
    ...memberPages,
    ...fundPages,
    ...indicatorPages,
    ...topicPages,
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
