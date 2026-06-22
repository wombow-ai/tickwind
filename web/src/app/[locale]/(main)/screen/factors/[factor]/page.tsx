import type {Metadata} from 'next';
import Link from '@/components/LocalLink';
import {notFound} from 'next/navigation';
import {Layers} from 'lucide-react';
import {getFactorScreen, type FactorRank} from '@/lib/api';
import {SITE_URL, langAlternates} from '@/lib/config';
import {isLocale, LOCALES} from '@/lib/locale';
import {ogImageMeta} from '@/lib/og';
import {FACTOR_PRESETS, factorByKey} from '@/lib/factors';

// The factor population is rebuilt hourly (fundamentals barely move intraday); ISR re-fetches the
// ranked leaderboard every 30 minutes so the page self-heals an empty/cold bake without a deploy.
export const revalidate = 1800;

/** Pre-render every factor × locale (4 × 2) at build time. */
export function generateStaticParams(): {locale: string; factor: string}[] {
  return LOCALES.flatMap(locale => FACTOR_PRESETS.map(p => ({locale, factor: p.key})));
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string; factor: string}>;
}): Promise<Metadata> {
  const {locale, factor} = await params;
  const p = factorByKey(factor);
  if (!p) return {title: 'Factor screener'};
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';
  const title = zh ? p.titleZh : p.titleEn;
  const desc = zh ? p.descZh : p.descEn;
  const path = `/screen/factors/${p.key}`;
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
          eyebrow: zh ? '因子排行' : 'Factor screen',
          title,
          subtitle: desc.slice(0, 54),
        }),
      ],
    },
  };
}

/**
 * Factor-leaderboard landing page (pSEO): every tracked stock ranked by one factor's PERCENTILE
 * (value | growth | quality | momentum) vs the whole tracked universe — the market-wide view of the
 * free per-stock multi-factor scorecard. Server-rendered single-locale (chosen from the route
 * segment) so /en and /zh are distinct, crawlable HTML. Best-effort fetch — a slow/down API or a
 * cold (not-yet-scanned) population renders the empty state, never a 500; ISR refills. Every number
 * is Go-computed and DESCRIPTIVE — no rating, no advice. Unknown factor slug → notFound().
 */
export default async function FactorScreenRoute({
  params,
}: {
  params: Promise<{locale: string; factor: string}>;
}) {
  const {locale, factor} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';
  const p = factorByKey(factor);
  if (!p) notFound();

  const title = zh ? p.titleZh : p.titleEn;
  const desc = zh ? p.descZh : p.descEn;
  const basis = zh ? p.basisZh : p.basisEn;
  const path = `/screen/factors/${p.key}`;

  // Best-effort fetch: any failure → empty list (the page still renders + ISR refills). Never throws.
  let results: FactorRank[] = [];
  let population = 0;
  try {
    const r = await getFactorScreen(p.key, 100, AbortSignal.timeout(8000));
    results = r.results ?? [];
    population = r.population ?? 0;
  } catch {
    results = [];
  }

  // JSON-LD: an ItemList of the ranked tickers (locale-prefixed /stock URLs) + a BreadcrumbList.
  const ld = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'ItemList',
        name: title,
        description: desc,
        numberOfItems: results.length,
        itemListElement: results.map((r, i) => ({
          '@type': 'ListItem',
          position: i + 1,
          name: r.ticker,
          url: `${SITE_URL}/${loc}/stock/${encodeURIComponent(r.ticker)}`,
        })),
      },
      {
        '@type': 'BreadcrumbList',
        itemListElement: [
          {'@type': 'ListItem', position: 1, name: 'Tickwind', item: `${SITE_URL}/${loc}`},
          {
            '@type': 'ListItem',
            position: 2,
            name: zh ? '美股筛选' : 'Screener',
            item: `${SITE_URL}/${loc}/screen`,
          },
          {'@type': 'ListItem', position: 3, name: title, item: `${SITE_URL}/${loc}${path}`},
        ],
      },
    ],
  };

  const others = FACTOR_PRESETS.filter(o => o.key !== p.key);

  return (
    <article className="mx-auto max-w-3xl">
      <script type="application/ld+json" dangerouslySetInnerHTML={{__html: JSON.stringify(ld)}} />

      <nav className="mb-4 text-[12px] text-slate-500 dark:text-slate-400" aria-label="Breadcrumb">
        <Link href="/" className="hover:underline">
          {zh ? '首页' : 'Home'}
        </Link>
        <span className="mx-1.5">/</span>
        <Link href="/screen" className="hover:underline">
          {zh ? '美股筛选' : 'Screener'}
        </Link>
      </nav>

      <header className="mb-4">
        <h1 className="flex items-center gap-2 text-[24px] font-bold tracking-tight text-slate-900 dark:text-slate-100">
          <Layers size={20} className="text-indigo-500 dark:text-indigo-300" />
          {title}
        </h1>
        <p className="mt-1.5 text-[13.5px] leading-relaxed text-slate-600 dark:text-slate-300">
          {desc}
        </p>
        <p className="mt-1 text-[12px] text-slate-500 dark:text-slate-400">
          {basis}
          {population > 0 && (
            <>
              {' · '}
              {zh
                ? `相对 ${population} 只个股排名`
                : `ranked across ${population} stocks`}
            </>
          )}
        </p>
      </header>

      {results.length > 0 ? (
        <div className="tw-fade overflow-hidden rounded-2xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-950">
          <div className="flex items-center gap-3 border-b border-slate-200 px-4 py-2 text-[11px] font-semibold uppercase tracking-wide text-slate-400 dark:border-slate-800 dark:text-slate-500">
            <span className="w-8 text-right tabular-nums">#</span>
            <span className="w-24">{zh ? '代码' : 'Ticker'}</span>
            <span className="flex-1">{zh ? '百分位' : 'Percentile'}</span>
            <span className="w-12 text-right tabular-nums">{zh ? '分位' : 'Pct'}</span>
          </div>
          {results.map((r, i) => (
            <Row key={r.ticker} r={r} rank={i + 1} last={i === results.length - 1} />
          ))}
        </div>
      ) : (
        <div className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-10 text-center dark:border-slate-800 dark:bg-slate-900">
          <Layers size={22} className="mx-auto mb-2 text-slate-300 dark:text-slate-600" />
          <p className="text-[14px] font-semibold text-slate-700 dark:text-slate-200">
            {zh ? '排行榜正在生成' : 'Leaderboard is warming up'}
          </p>
          <p className="mt-1 text-[12.5px] text-slate-500 dark:text-slate-400">
            {zh
              ? '因子分布每小时刷新一次,稍后再来查看。'
              : 'The factor distribution rebuilds hourly — check back shortly.'}
          </p>
          <Link
            href="/screen"
            className="mt-3 inline-block rounded-lg bg-indigo-600 px-3 py-1.5 text-[12.5px] font-semibold text-white transition hover:bg-indigo-700 dark:bg-indigo-500 dark:hover:bg-indigo-600"
          >
            {zh ? '打开完整筛选器 →' : 'Open the full screener →'}
          </Link>
        </div>
      )}

      <p className="mt-4 text-center text-[11px] text-slate-400 dark:text-slate-500">
        {zh
          ? '描述性百分位 · 非评级 · 非投资建议'
          : 'Descriptive percentile · Not a rating · Not investment advice'}
      </p>

      {/* Cross-link hub: the other factor leaderboards, for internal linking. */}
      <section className="mt-8">
        <h2 className="mb-2.5 text-[15px] font-bold text-slate-900 dark:text-slate-100">
          {zh ? '更多因子排行' : 'More factor leaderboards'}
        </h2>
        <div className="grid gap-2 sm:grid-cols-2">
          {others.map(o => (
            <Link
              key={o.key}
              href={`/screen/factors/${o.key}`}
              className="block rounded-xl border border-slate-200 px-3 py-2.5 hover:border-indigo-300 hover:bg-slate-50 dark:border-slate-800 dark:hover:border-indigo-500/40 dark:hover:bg-slate-900"
            >
              <div className="text-[13px] font-semibold text-slate-800 dark:text-slate-100">
                {zh ? o.titleZh : o.titleEn}
              </div>
            </Link>
          ))}
          <Link
            href="/screen"
            className="block rounded-xl border border-dashed border-slate-300 px-3 py-2.5 text-[13px] font-medium text-slate-600 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-900"
          >
            {zh ? '自定义筛选器 →' : 'Custom screener →'}
          </Link>
        </div>
      </section>
    </article>
  );
}

/**
 * One ranked row → an internal link into the stock page, with a NEUTRAL percentile bar (no
 * green/red good-bad cue — that would read as a rating; the bar length IS the percentile).
 */
function Row({r, rank, last}: {r: FactorRank; rank: number; last: boolean}) {
  const pct = Math.max(0, Math.min(100, r.percentile));
  return (
    <Link
      href={`/stock/${encodeURIComponent(r.ticker)}`}
      className={`flex items-center gap-3 px-4 py-2.5 text-[13.5px] transition hover:bg-slate-50 dark:hover:bg-slate-900 ${
        last ? '' : 'border-b border-slate-200 dark:border-slate-800'
      }`}
    >
      <span className="w-8 text-right font-semibold tabular-nums text-slate-400 dark:text-slate-500">
        {rank}
      </span>
      <span className="w-24 font-bold text-slate-900 dark:text-slate-100">{r.ticker}</span>
      <span className="flex-1">
        <span className="block h-2 w-full overflow-hidden rounded-full bg-slate-200 dark:bg-slate-800">
          <span
            className="block h-full rounded-full bg-indigo-500/70 dark:bg-indigo-400/70"
            style={{width: `${pct}%`}}
          />
        </span>
      </span>
      <span className="w-12 text-right font-semibold tabular-nums text-slate-900 dark:text-slate-100">
        {Math.round(pct)}
      </span>
    </Link>
  );
}
