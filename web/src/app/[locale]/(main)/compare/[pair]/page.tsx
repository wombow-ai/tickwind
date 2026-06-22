import type {Metadata} from 'next';
import Link from '@/components/LocalLink';
import {notFound} from 'next/navigation';
import {Scale} from 'lucide-react';
import {CompareTable} from '@/components/CompareTable';
import {COMPARE_PAIRS, pairSlug, parsePair} from '@/lib/compare';
import {SITE_URL, langAlternates} from '@/lib/config';
import {isLocale} from '@/lib/locale';
import {ogImageMeta} from '@/lib/og';

// The crawlable chrome (heading/links/meta) is static per slug; ISR keeps it cheap. The metric
// table is fetched CLIENT-SIDE (CompareTable) — the SSR fetch of those endpoints through the
// Cloudflare tunnel is unreliable at Vercel build time (it baked the page as the route loader),
// so we render no API call server-side and self-heal the numbers in the browser.
export const revalidate = 86400;

/** Prerender the curated rivalries (both locales); any other parseable pair renders on-demand. */
export function generateStaticParams(): {locale: string; pair: string}[] {
  const out: {locale: string; pair: string}[] = [];
  for (const loc of ['en', 'zh']) {
    for (const [a, b] of COMPARE_PAIRS) out.push({locale: loc, pair: pairSlug(a, b)});
  }
  return out;
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string; pair: string}>;
}): Promise<Metadata> {
  const {locale, pair} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';
  const parsed = parsePair(pair);
  if (!parsed) {
    return {title: zh ? '股票对比 · Tickwind' : 'Compare Stocks · Tickwind', robots: {index: false, follow: true}};
  }
  const [a, b] = parsed;
  const path = `/compare/${pairSlug(a, b)}`;
  const title = zh ? `${a} vs ${b} 对比 · 估值与基本面 | Tickwind` : `${a} vs ${b} — Stock Comparison | Tickwind`;
  const description = zh
    ? `${a} 与 ${b} 并排对比:市值、市盈率、市净率、营收、净利润、每股收益 —— 全部基于公开数据。延迟数据,仅供参考,不构成投资建议。`
    : `Compare ${a} vs ${b} side by side — market cap, P/E, P/B, revenue, net income and EPS, all from public data. Delayed data, for reference only, not investment advice.`;
  return {
    title: {absolute: title},
    description,
    alternates: langAlternates(path, loc),
    openGraph: {
      type: 'website',
      title,
      description: description.slice(0, 110),
      url: `${SITE_URL}/${loc}${path}`,
      images: [
        ogImageMeta({
          lang: loc,
          eyebrow: zh ? '股票对比' : 'Compare',
          title: `${a} vs ${b}`,
          subtitle: zh ? '估值与基本面并排对比' : 'Valuation & fundamentals, side by side',
        }),
      ],
    },
  };
}

/**
 * Side-by-side stock comparison (pSEO): two stocks' price + Go-computed fundamentals in one table,
 * for high-intent "X vs Y" queries. The crawlable core (h1 / breadcrumb / intro / related links /
 * metadata) is server-rendered per the active locale FROM THE SLUG (no API fetch, so it never
 * blanks); the metric table self-heals client-side. NOTHING is a recommendation — the page declares
 * no "winner". An unparseable slug 404s.
 */
export default async function CompareRoute({
  params,
}: {
  params: Promise<{locale: string; pair: string}>;
}) {
  const {locale, pair} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';
  const parsed = parsePair(pair);
  if (!parsed) notFound();
  const [a, b] = parsed;
  const path = `/compare/${pairSlug(a, b)}`;

  const ld = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'BreadcrumbList',
        itemListElement: [
          {'@type': 'ListItem', position: 1, name: 'Tickwind', item: `${SITE_URL}/${loc}`},
          {'@type': 'ListItem', position: 2, name: zh ? '股票对比' : 'Compare', item: `${SITE_URL}/${loc}/compare`},
          {'@type': 'ListItem', position: 3, name: `${a} vs ${b}`, item: `${SITE_URL}/${loc}${path}`},
        ],
      },
    ],
  };

  const related = COMPARE_PAIRS.filter(([x, y]) => pairSlug(x, y) !== pairSlug(a, b)).slice(0, 6);

  return (
    <article className="mx-auto max-w-3xl">
      <script type="application/ld+json" dangerouslySetInnerHTML={{__html: JSON.stringify(ld)}} />

      <nav className="mb-4 text-[12px] text-slate-500 dark:text-slate-400" aria-label="Breadcrumb">
        <Link href="/" className="hover:underline">
          {zh ? '首页' : 'Home'}
        </Link>
        <span className="mx-1.5">/</span>
        <Link href="/compare" className="hover:underline">
          {zh ? '股票对比' : 'Compare'}
        </Link>
      </nav>

      <header className="mb-5">
        <h1 className="flex flex-wrap items-center gap-x-2 gap-y-1 text-[24px] font-bold tracking-tight text-slate-900 dark:text-slate-100">
          <Scale size={20} className="text-teal-500 dark:text-teal-300" />
          {a} <span className="text-slate-400 dark:text-slate-500">vs</span> {b}
        </h1>
        <p className="mt-1.5 text-[13.5px] leading-relaxed text-slate-600 dark:text-slate-300">
          {zh
            ? `${a} 与 ${b} 的估值与基本面并排对比(公开数据)。`
            : `${a} and ${b} side by side — valuation & fundamentals (public data).`}
        </p>
      </header>

      <section className="mb-7">
        <CompareTable a={a} b={b} />
      </section>

      {related.length > 0 && (
        <section className="mb-7">
          <h2 className="mb-2.5 text-[12px] font-semibold uppercase tracking-wide text-slate-400 dark:text-slate-500">
            {zh ? '相关对比' : 'Related comparisons'}
          </h2>
          <div className="flex flex-wrap gap-2">
            {related.map(([x, y]) => (
              <Link
                key={pairSlug(x, y)}
                href={`/compare/${pairSlug(x, y)}`}
                className="rounded-lg border border-slate-200 px-2.5 py-1.5 text-[12.5px] font-medium text-slate-700 transition hover:border-teal-300 hover:bg-slate-50 dark:border-slate-800 dark:text-slate-200 dark:hover:border-teal-500/40 dark:hover:bg-slate-900"
              >
                {x} vs {y}
              </Link>
            ))}
          </div>
        </section>
      )}

      <p className="mt-6 text-center text-[11px] text-slate-400 dark:text-slate-500">
        {zh
          ? '数据延迟 · 公开数据 · 仅供参考 · 非投资建议(本页不评判孰优孰劣)'
          : 'Delayed data · Public sources · For reference only · Not investment advice (this page declares no winner)'}
      </p>
    </article>
  );
}
