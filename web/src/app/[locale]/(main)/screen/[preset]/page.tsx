import type {Metadata} from 'next';
import Link from '@/components/LocalLink';
import {notFound} from 'next/navigation';
import {SlidersHorizontal} from 'lucide-react';
import {getScreen, type ScreenResult} from '@/lib/api';
import {SITE_URL, langAlternates} from '@/lib/config';
import {isLocale, LOCALES} from '@/lib/locale';
import {ogImageMeta} from '@/lib/og';
import {SCREEN_PRESETS, presetByKey} from '@/lib/presets';

// Dynamic intraday movers → ISR: re-fetch the ranked list every 10 minutes so the
// page stays fresh without a deploy, while serving cached HTML in between.
export const revalidate = 600;

/** Pre-render every preset × locale (≈9 × 2) at build time. */
export function generateStaticParams(): {locale: string; preset: string}[] {
  return LOCALES.flatMap(locale =>
    SCREEN_PRESETS.map(p => ({locale, preset: p.key})),
  );
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string; preset: string}>;
}): Promise<Metadata> {
  const {locale, preset} = await params;
  const p = presetByKey(preset);
  if (!p) return {title: 'Screener'};
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';
  const title = zh ? p.titleZh : p.titleEn;
  const desc = zh ? p.descZh : p.descEn;
  const path = `/screen/${p.key}`;
  return {
    // Locale-matched browser-tab title (English-default per the owner principle).
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
          eyebrow: zh ? '美股筛选' : 'Screener',
          title,
          subtitle: desc.slice(0, 54),
        }),
      ],
    },
  };
}

/**
 * Curated screener landing page (pSEO): one preset's fixed filters run against
 * the whole-US universe (delayed IEX quotes), rendered as a ranked, internally-
 * linked list of stocks. Server-rendered single-locale (chosen from the route
 * segment) so /en and /zh are distinct, crawlable HTML. Best-effort fetch — a
 * slow/down API or an empty (e.g. off-hours session) result renders the empty
 * state, never a 500. Unknown preset slug → notFound().
 */
export default async function ScreenPresetRoute({
  params,
}: {
  params: Promise<{locale: string; preset: string}>;
}) {
  const {locale, preset} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';
  const p = presetByKey(preset);
  if (!p) notFound();

  const title = zh ? p.titleZh : p.titleEn;
  const desc = zh ? p.descZh : p.descEn;
  const path = `/screen/${p.key}`;

  // Best-effort fetch: any failure → empty list (the page still renders + ISR
  // refills on the next revalidate). Never throws to the route.
  let results: ScreenResult[] = [];
  try {
    const r = await getScreen(p.params, AbortSignal.timeout(8000));
    results = r.results ?? [];
  } catch {
    results = [];
  }

  // JSON-LD: an ItemList of the ranked tickers (each item a locale-prefixed
  // /stock URL) + a BreadcrumbList (Tickwind → Screener → preset). All `item`
  // URLs locale-prefixed to match the canonical — the FIXED guide/indicators
  // pattern (NOT the old bare-path bug).
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

  // Other presets (excluding self) for an internal-linking hub at the bottom.
  const others = SCREEN_PRESETS.filter(o => o.key !== p.key);

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
          <SlidersHorizontal size={20} className="text-sky-600 dark:text-sky-300" />
          {title}
        </h1>
        <p className="mt-1.5 text-[13.5px] leading-relaxed text-slate-600 dark:text-slate-300">
          {desc}
        </p>
      </header>

      {results.length > 0 ? (
        <div className="tw-fade overflow-hidden rounded-2xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-950">
          {/* header row */}
          <div className="flex items-center gap-3 border-b border-slate-200 px-4 py-2 text-[11px] font-semibold uppercase tracking-wide text-slate-400 dark:border-slate-800 dark:text-slate-500">
            <span className="w-8 text-right tabular-nums">#</span>
            <span className="w-24">{zh ? '代码' : 'Ticker'}</span>
            <span className="flex-1 text-right tabular-nums">{zh ? '价格' : 'Price'}</span>
            <span className="w-24 text-right tabular-nums">{zh ? '涨跌幅' : 'Change'}</span>
          </div>
          {results.map((r, i) => (
            <Row key={r.ticker} r={r} rank={i + 1} last={i === results.length - 1} />
          ))}
        </div>
      ) : (
        <div className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-10 text-center dark:border-slate-800 dark:bg-slate-900">
          <SlidersHorizontal size={22} className="mx-auto mb-2 text-slate-300 dark:text-slate-600" />
          <p className="text-[14px] font-semibold text-slate-700 dark:text-slate-200">
            {zh ? '暂无符合条件的个股' : 'No matching stocks right now'}
          </p>
          <p className="mt-1 text-[12.5px] text-slate-500 dark:text-slate-400">
            {zh
              ? '稍后再来查看,或使用完整筛选器自定义条件。'
              : 'Check back shortly, or use the full screener to set your own filters.'}
          </p>
          <Link
            href="/screen"
            className="mt-3 inline-block rounded-lg bg-sky-600 px-3 py-1.5 text-[12.5px] font-semibold text-white transition hover:bg-sky-700 dark:bg-sky-500 dark:hover:bg-sky-600"
          >
            {zh ? '打开完整筛选器 →' : 'Open the full screener →'}
          </Link>
        </div>
      )}

      <p className="mt-4 text-center text-[11px] text-slate-400 dark:text-slate-500">
        {zh ? '数据延迟 · 仅供参考 · 非投资建议' : 'Delayed data · For reference only · Not investment advice'}
      </p>

      {/* Cross-link hub: the other presets, for internal linking. */}
      <section className="mt-8">
        <h2 className="mb-2.5 text-[15px] font-bold text-slate-900 dark:text-slate-100">
          {zh ? '更多筛选榜单' : 'More screener lists'}
        </h2>
        <div className="grid gap-2 sm:grid-cols-2">
          {others.map(o => (
            <Link
              key={o.key}
              href={`/screen/${o.key}`}
              className="block rounded-xl border border-slate-200 px-3 py-2.5 hover:border-sky-300 hover:bg-slate-50 dark:border-slate-800 dark:hover:border-sky-500/40 dark:hover:bg-slate-900"
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

/** One ranked result row → an internal link into the stock page. */
function Row({r, rank, last}: {r: ScreenResult; rank: number; last: boolean}) {
  const pos = r.change_pct != null && r.change_pct >= 0;
  const chgColor =
    r.change_pct == null
      ? 'text-slate-400 dark:text-slate-500'
      : pos
        ? 'text-emerald-600 dark:text-emerald-400'
        : 'text-rose-500 dark:text-rose-400';
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
      <span className="flex-1 text-right font-semibold tabular-nums text-slate-900 dark:text-slate-100">
        ${r.price.toFixed(2)}
      </span>
      <span className={`w-24 text-right font-semibold tabular-nums ${chgColor}`}>
        {r.change_pct == null ? '—' : `${pos ? '+' : ''}${r.change_pct.toFixed(2)}%`}
      </span>
    </Link>
  );
}
