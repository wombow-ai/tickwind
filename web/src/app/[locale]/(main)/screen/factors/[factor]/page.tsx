import type {Metadata} from 'next';
import Link from '@/components/LocalLink';
import {notFound} from 'next/navigation';
import {Layers} from 'lucide-react';
import {getFactorScreen, type FactorRank} from '@/lib/api';
import {SITE_URL, langAlternates} from '@/lib/config';
import {isLocale, LOCALES} from '@/lib/locale';
import {ogImageMeta} from '@/lib/og';
import {FACTOR_PRESETS, factorByKey} from '@/lib/factors';
import {FactorLeaderboard} from '@/components/FactorLeaderboard';

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
        <p className="mt-1 text-[12px] text-slate-500 dark:text-slate-400">{basis}</p>
      </header>

      {/* The leaderboard self-heals client-side (see FactorLeaderboard) — the SSR `results` seed the
          crawlable rows + JSON-LD when the tunnel cooperates, but the browser re-fetch guarantees
          users always see the live ranking even when the SSR fetch baked empty. */}
      <FactorLeaderboard factor={p.key} initial={results} initialPopulation={population} zh={zh} />

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
