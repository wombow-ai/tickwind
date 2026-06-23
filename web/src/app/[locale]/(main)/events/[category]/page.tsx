import type {Metadata} from 'next';
import Link from '@/components/LocalLink';
import {notFound} from 'next/navigation';
import {Newspaper} from 'lucide-react';
import {getMaterialFeed, type MaterialFeedEvent} from '@/lib/api';
import {SITE_URL, langAlternates} from '@/lib/config';
import {isLocale, LOCALES} from '@/lib/locale';
import {ogImageMeta} from '@/lib/og';
import {EVENT_CATEGORIES, eventCategoryByKey} from '@/lib/eventCategories';
import {MaterialFeedList} from '@/components/MaterialFeedList';

export const revalidate = 1800;

/** Pre-render every category × locale at build time. */
export function generateStaticParams(): {locale: string; category: string}[] {
  return LOCALES.flatMap(locale => EVENT_CATEGORIES.map(c => ({locale, category: c.key})));
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string; category: string}>;
}): Promise<Metadata> {
  const {locale, category} = await params;
  const c = eventCategoryByKey(category);
  if (!c) return {title: 'Material events'};
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';
  const title = zh ? c.titleZh : c.titleEn;
  const desc = zh ? c.descZh : c.descEn;
  const path = `/events/${c.key}`;
  return {
    title: {absolute: `${title} · Tickwind`},
    description: desc,
    alternates: langAlternates(path, loc),
    openGraph: {
      type: 'website',
      title,
      description: desc.slice(0, 110),
      url: `${SITE_URL}/${loc}${path}`,
      images: [
        ogImageMeta({
          lang: loc,
          eyebrow: zh ? '重大事件' : 'Material events',
          title,
          subtitle: desc.slice(0, 54),
        }),
      ],
    },
  };
}

/**
 * One material-events CATEGORY feed (pSEO): recent 8-K filings of a single high-signal item type
 * (leadership change / material agreement / new debt / bankruptcy / restatement) across the tracked
 * universe. Server-rendered single-locale, best-effort SSR fetch (the client component self-heals);
 * facts only, no advice. Unknown category → notFound().
 */
export default async function EventCategoryRoute({
  params,
}: {
  params: Promise<{locale: string; category: string}>;
}) {
  const {locale, category} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';
  const c = eventCategoryByKey(category);
  if (!c) notFound();

  const title = zh ? c.titleZh : c.titleEn;
  const desc = zh ? c.descZh : c.descEn;
  const path = `/events/${c.key}`;

  let events: MaterialFeedEvent[] = [];
  try {
    const r = await getMaterialFeed(c.item, 80, AbortSignal.timeout(8000));
    events = r.events ?? [];
  } catch {
    events = [];
  }

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
        '@type': 'ItemList',
        name: title,
        description: desc,
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
          {'@type': 'ListItem', position: 3, name: title, item: `${SITE_URL}/${loc}${path}`},
        ],
      },
    ],
  };

  const others = EVENT_CATEGORIES.filter(o => o.key !== c.key);

  return (
    <article className="mx-auto max-w-3xl">
      <script type="application/ld+json" dangerouslySetInnerHTML={{__html: JSON.stringify(ld)}} />

      <nav className="mb-4 text-[12px] text-slate-500 dark:text-slate-400" aria-label="Breadcrumb">
        <Link href="/" className="hover:underline">
          {zh ? '首页' : 'Home'}
        </Link>
        <span className="mx-1.5">/</span>
        <Link href="/events" className="hover:underline">
          {zh ? '重大事件' : 'Material events'}
        </Link>
      </nav>

      <header className="mb-4">
        <h1 className="flex items-center gap-2 text-[24px] font-bold tracking-tight text-slate-900 dark:text-slate-100">
          <Newspaper size={20} className="text-amber-600 dark:text-amber-300" />
          {title}
        </h1>
        <p className="mt-1.5 text-[13.5px] leading-relaxed text-slate-600 dark:text-slate-300">{desc}</p>
      </header>

      <MaterialFeedList item={c.item} initial={events} zh={zh} />

      <p className="mt-4 text-center text-[11px] text-slate-400 dark:text-slate-500">
        {zh
          ? `数据源:SEC EDGAR 8-K 第 ${c.item} 项 · 非投资建议`
          : `Source: SEC EDGAR 8-K item ${c.item} · Not investment advice`}
      </p>

      <section className="mt-8">
        <h2 className="mb-2.5 text-[15px] font-bold text-slate-900 dark:text-slate-100">
          {zh ? '更多事件类别' : 'More event categories'}
        </h2>
        <div className="grid gap-2 sm:grid-cols-2">
          {others.map(o => (
            <Link
              key={o.key}
              href={`/events/${o.key}`}
              className="block rounded-xl border border-slate-200 px-3 py-2.5 hover:border-amber-300 hover:bg-slate-50 dark:border-slate-800 dark:hover:border-amber-500/40 dark:hover:bg-slate-900"
            >
              <div className="text-[13px] font-semibold text-slate-800 dark:text-slate-100">{zh ? o.titleZh : o.titleEn}</div>
            </Link>
          ))}
          <Link
            href="/events"
            className="block rounded-xl border border-dashed border-slate-300 px-3 py-2.5 text-[13px] font-medium text-slate-600 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-900"
          >
            {zh ? '全部重大事件 →' : 'All material events →'}
          </Link>
        </div>
      </section>
    </article>
  );
}
