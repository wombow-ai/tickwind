import type {Metadata} from 'next';
import Link from '@/components/LocalLink';
import {LayoutGrid} from 'lucide-react';
import {SITE_URL, langAlternates} from '@/lib/config';
import {isLocale, LOCALES} from '@/lib/locale';
import {ogImageMeta} from '@/lib/og';
import {
  bucketByFirstLetter,
  quoteBearingTickers,
  STOCK_DIRECTORY_LETTERS,
} from '@/lib/pseo';

// The directory tracks the quote-bearing universe (~6,700), which only shifts as
// the price universe is swept — a long ISR window (hourly) keeps the counts
// reasonably fresh without a deploy, matching the sitemap's cadence.
export const revalidate = 3600;

/** Pre-render the hub × each locale (= 2 pages). */
export function generateStaticParams(): {locale: string}[] {
  return LOCALES.map(locale => ({locale}));
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string}>;
}): Promise<Metadata> {
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';
  const title = zh
    ? '美股代码大全 · A–Z 个股目录 | Tickwind'
    : 'Browse All US Stocks · A–Z Directory | Tickwind';
  const description = zh
    ? '按首字母 A–Z 浏览美股个股目录 —— 收录全部有实时报价的美股代码,每个代码链接到其实时价格、SEC 文件、基本面与新闻页面。延迟数据,仅供参考,不构成投资建议。'
    : 'Browse the full A–Z directory of US stocks with live quotes — every quote-bearing ticker links to its live price, SEC filings, fundamentals and news. Delayed data, for reference only, not investment advice.';
  return {
    // The hub is always indexable (the per-letter pages carry the thin guard).
    title: {absolute: title},
    description,
    alternates: langAlternates('/stocks', loc),
    openGraph: {
      type: 'website',
      title,
      description: description.slice(0, 110),
      url: `${SITE_URL}/${loc}/stocks`,
      images: [
        ogImageMeta({
          lang: loc,
          eyebrow: zh ? '个股目录' : 'Stock directory',
          title: zh ? '美股代码大全 · A–Z' : 'All US stocks · A–Z',
          subtitle: zh ? '按首字母浏览全部美股' : 'Browse every US stock by first letter',
        }),
      ],
    },
  };
}

/**
 * The A–Z stock directory hub (pSEO): a crawlable index of 26 letter links over
 * the quote-bearing universe, plus per-letter ticker counts. Server-rendered
 * single-locale (chosen from the route segment) so /en and /zh are distinct,
 * indexable HTML. Aids Google's crawl discovery + internal linking for the
 * thousands of `/stock/{t}` pages and gives a "browse all stocks" UX.
 * Best-effort fetch — a slow/down API renders the letters with zero counts
 * (the per-letter pages still render their own empty state), never a 500.
 */
export default async function StocksHubRoute({
  params,
}: {
  params: Promise<{locale: string}>;
}) {
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';

  // Best-effort: any failure → empty universe → letters render with 0 counts
  // (ISR refills on the next revalidate). Never throws to the route.
  const tickers = await quoteBearingTickers();
  const buckets = bucketByFirstLetter(tickers);
  const total = [...buckets.values()].reduce((n, arr) => n + arr.length, 0);

  // JSON-LD: a CollectionPage wrapping an ItemList of the 26 letter pages (each a
  // locale-prefixed /stocks/{letter} URL) + a BreadcrumbList (Tickwind → All
  // stocks). All `item`/`url` locale-prefixed to match the canonical (the FIXED
  // guide/indicators pattern, NOT the old bare-path bug).
  const hubUrl = `${SITE_URL}/${loc}/stocks`;
  const ld = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'CollectionPage',
        name: zh ? '美股代码大全 · A–Z' : 'All US stocks · A–Z',
        url: hubUrl,
        mainEntity: {
          '@type': 'ItemList',
          name: zh ? '美股个股目录(按首字母)' : 'US stock directory by first letter',
          numberOfItems: STOCK_DIRECTORY_LETTERS.length,
          itemListElement: STOCK_DIRECTORY_LETTERS.map((letter, i) => ({
            '@type': 'ListItem',
            position: i + 1,
            name: letter.toUpperCase(),
            url: `${SITE_URL}/${loc}/stocks/${letter}`,
          })),
        },
      },
      {
        '@type': 'BreadcrumbList',
        itemListElement: [
          {'@type': 'ListItem', position: 1, name: 'Tickwind', item: `${SITE_URL}/${loc}`},
          {
            '@type': 'ListItem',
            position: 2,
            name: zh ? '个股目录' : 'All stocks',
            item: hubUrl,
          },
        ],
      },
    ],
  };

  return (
    <article className="mx-auto max-w-3xl">
      <script type="application/ld+json" dangerouslySetInnerHTML={{__html: JSON.stringify(ld)}} />

      <nav className="mb-4 text-[12px] text-slate-500 dark:text-slate-400" aria-label="Breadcrumb">
        <Link href="/" className="hover:underline">
          {zh ? '首页' : 'Home'}
        </Link>
      </nav>

      <header className="mb-5">
        <h1 className="flex items-center gap-2 text-[24px] font-bold tracking-tight text-slate-900 dark:text-slate-100">
          <LayoutGrid size={20} className="text-sky-600 dark:text-sky-300" />
          {zh ? '美股代码大全 · A–Z' : 'Browse all US stocks · A–Z'}
        </h1>
        <p className="mt-1.5 text-[13.5px] leading-relaxed text-slate-600 dark:text-slate-300">
          {zh
            ? '按首字母浏览全部有实时报价的美股个股。点击任意字母查看该字母下的所有代码,每个代码链接到其实时价格、SEC 文件、基本面与讨论页面。'
            : 'Browse every US stock with a live quote by first letter. Pick a letter to see all its tickers — each links to its live price, SEC filings, fundamentals and discussion.'}
        </p>
        {total > 0 && (
          <p className="mt-2 text-[12.5px] text-slate-500 dark:text-slate-400">
            {zh
              ? `共收录 ${total.toLocaleString()} 只有报价的美股`
              : `${total.toLocaleString()} quote-bearing US stocks indexed`}
          </p>
        )}
      </header>

      {/* The crawlable A–Z index: 26 letter links, each into /stocks/{letter}. */}
      <section>
        <h2 className="mb-2.5 text-[12px] font-semibold uppercase tracking-wide text-slate-400 dark:text-slate-500">
          {zh ? '按首字母浏览' : 'Browse by letter'}
        </h2>
        <div className="grid grid-cols-3 gap-2 sm:grid-cols-4 md:grid-cols-6">
          {STOCK_DIRECTORY_LETTERS.map(letter => {
            const count = buckets.get(letter)?.length ?? 0;
            return (
              <Link
                key={letter}
                href={`/stocks/${letter}`}
                className="flex flex-col items-center justify-center gap-0.5 rounded-xl border border-slate-200 px-3 py-3 transition hover:border-sky-300 hover:bg-slate-50 dark:border-slate-800 dark:hover:border-sky-500/40 dark:hover:bg-slate-900"
              >
                <span className="text-[18px] font-bold uppercase text-slate-900 dark:text-slate-100">
                  {letter}
                </span>
                <span className="text-[11px] tabular-nums text-slate-400 dark:text-slate-500">
                  {count > 0 ? count.toLocaleString() : '—'}
                </span>
              </Link>
            );
          })}
        </div>
      </section>

      <p className="mt-6 text-center text-[11px] text-slate-400 dark:text-slate-500">
        {zh
          ? '数据延迟 · 仅供参考 · 非投资建议'
          : 'Delayed data · For reference only · Not investment advice'}
      </p>
    </article>
  );
}
