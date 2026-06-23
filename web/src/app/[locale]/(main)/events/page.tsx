import type {Metadata} from 'next';
import Link from '@/components/LocalLink';
import {Newspaper} from 'lucide-react';
import {getMaterialFeed, type MaterialFeedEvent} from '@/lib/api';
import {SITE_URL, langAlternates} from '@/lib/config';
import {isLocale} from '@/lib/locale';
import {ogImageMeta} from '@/lib/og';
import {EVENT_CATEGORIES} from '@/lib/eventCategories';
import {MaterialFeedList} from '@/components/MaterialFeedList';

// The feed rebuilds hourly; ISR re-fetches every 30 min so the page self-heals an empty/cold bake
// without a deploy. The client component self-heals for users.
export const revalidate = 1800;

const TITLE_EN = 'Recent US Stock Material Events — SEC 8-K Filings';
const TITLE_ZH = '美股近期重大事件 — SEC 8-K 申报';
const DESC_EN =
  'Recent high-signal SEC 8-K filings — leadership changes, material agreements, new debt, bankruptcy, restatements — across tracked US stocks, newest first. Disclosed corporate-filing facts, not investment advice.';
const DESC_ZH =
  '近期高信号 SEC 8-K 申报 —— 高管变动、重大协议、新增债务、破产、财报重述等,覆盖追踪的美股,最新在前。披露性公司申报事实,非投资建议。';

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string}>;
}): Promise<Metadata> {
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';
  return {
    title: {absolute: `${zh ? TITLE_ZH : TITLE_EN} · Tickwind`},
    description: zh ? DESC_ZH : DESC_EN,
    alternates: langAlternates('/events', loc),
    openGraph: {
      type: 'website',
      title: zh ? TITLE_ZH : TITLE_EN,
      description: (zh ? DESC_ZH : DESC_EN).slice(0, 110),
      url: `${SITE_URL}/${loc}/events`,
      images: [
        ogImageMeta({
          lang: loc,
          eyebrow: zh ? '重大事件' : 'Material events',
          title: zh ? TITLE_ZH : TITLE_EN,
          subtitle: zh ? '高管变动 · 并购 · 债务 · 破产 · 重述' : 'Leadership · M&A · debt · bankruptcy · restatements',
        }),
      ],
    },
  };
}

/**
 * Market-wide material-events feed (pSEO): recent high-signal 8-K filings across the tracked universe.
 * Server-rendered single-locale (chosen from the route segment) so /en and /zh are distinct, crawlable
 * HTML. Best-effort SSR fetch — a slow/down API or a cold cache renders the empty state, never a 500;
 * the MaterialFeedList client component then self-heals for users. Facts only, no advice.
 */
export default async function EventsPage({
  params,
}: {
  params: Promise<{locale: string}>;
}) {
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';

  let events: MaterialFeedEvent[] = [];
  try {
    const r = await getMaterialFeed(undefined, 80, AbortSignal.timeout(8000));
    events = r.events ?? [];
  } catch {
    events = [];
  }

  // ItemList of the distinct stocks mentioned (internal-link discovery for crawlers).
  const seen = new Set<string>();
  const tickers: string[] = [];
  for (const e of events) {
    if (!seen.has(e.ticker)) {
      seen.add(e.ticker);
      tickers.push(e.ticker);
    }
  }
  const ld = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'CollectionPage',
        name: zh ? TITLE_ZH : TITLE_EN,
        description: zh ? DESC_ZH : DESC_EN,
      },
      {
        '@type': 'ItemList',
        numberOfItems: tickers.length,
        itemListElement: tickers.map((t, i) => ({
          '@type': 'ListItem',
          position: i + 1,
          name: t,
          url: `${SITE_URL}/${loc}/stock/${encodeURIComponent(t)}`,
        })),
      },
      {
        '@type': 'BreadcrumbList',
        itemListElement: [
          {'@type': 'ListItem', position: 1, name: 'Tickwind', item: `${SITE_URL}/${loc}`},
          {'@type': 'ListItem', position: 2, name: zh ? '重大事件' : 'Material events', item: `${SITE_URL}/${loc}/events`},
        ],
      },
    ],
  };

  return (
    <article className="mx-auto max-w-3xl">
      <script type="application/ld+json" dangerouslySetInnerHTML={{__html: JSON.stringify(ld)}} />

      <header className="mb-4">
        <h1 className="flex items-center gap-2 text-[24px] font-bold tracking-tight text-slate-900 dark:text-slate-100">
          <Newspaper size={20} className="text-amber-600 dark:text-amber-300" />
          {zh ? TITLE_ZH : TITLE_EN}
        </h1>
        <p className="mt-1.5 text-[13.5px] leading-relaxed text-slate-600 dark:text-slate-300">{zh ? DESC_ZH : DESC_EN}</p>
      </header>

      {/* Category filter rail. */}
      <nav className="mb-5 flex flex-wrap gap-2" aria-label={zh ? '事件类别' : 'Event categories'}>
        {EVENT_CATEGORIES.map(c => (
          <Link
            key={c.key}
            href={`/events/${c.key}`}
            className="rounded-full border border-slate-200 px-3 py-1 text-[12.5px] font-medium text-slate-700 hover:border-amber-300 hover:bg-amber-50 dark:border-slate-800 dark:text-slate-200 dark:hover:border-amber-500/40 dark:hover:bg-amber-500/10"
          >
            {zh ? c.titleZh.split(' — ')[0].replace(/\s*\([^)]*\)\s*$/, '') : categoryShort(c.titleEn)}
          </Link>
        ))}
      </nav>

      <MaterialFeedList initial={events} zh={zh} />

      <p className="mt-4 text-center text-[11px] text-slate-400 dark:text-slate-500">
        {zh
          ? '数据源:SEC EDGAR 8-K 当期报告 · 仅高信号事项(已剔除财报/附件等例行项)· 非投资建议'
          : 'Source: SEC EDGAR 8-K current reports · high-signal items only (routine earnings/exhibits excluded) · Not investment advice'}
      </p>
    </article>
  );
}

/** A short chip label from the full English category title (the part before the em-dash). */
function categoryShort(titleEn: string): string {
  return titleEn
    .split(' — ')[0]
    .replace(/^US Stock(s)? /, '')
    .replace(/\s*\([^)]*\)\s*$/, '')
    .trim();
}
