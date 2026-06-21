import type {Metadata} from 'next';
import Link from '@/components/LocalLink';
import {notFound} from 'next/navigation';
import {LayoutGrid} from 'lucide-react';
import {SITE_URL, langAlternates} from '@/lib/config';
import {isLocale, LOCALES} from '@/lib/locale';
import {ogImageMeta} from '@/lib/og';
import {STOCK_DIRECTORY_LETTERS, tickersForLetter} from '@/lib/pseo';
import {LetterGrid} from '@/components/StocksDirectory';

// Mirrors the /stocks hub + sitemap cadence: the per-letter bucket only shifts as
// the price universe is swept, so an hourly ISR window keeps it fresh deploy-free.
export const revalidate = 3600;

/**
 * Below this many tickers a letter page is treated as THIN (e.g. a sparse letter
 * like Q/X/Z, or an empty/error fetch) → noindex (follow). Fail-open: only a
 * definitively-sparse letter is deindexed; a real, populated letter stays
 * indexable. Mirrors the `/stock/[ticker]` thin-content guard's intent.
 */
const MIN_TICKERS_FOR_INDEX = 3;

/** Pre-render every letter a..z × locale (= 52 pages). */
export function generateStaticParams(): {locale: string; letter: string}[] {
  return LOCALES.flatMap(locale =>
    STOCK_DIRECTORY_LETTERS.map(letter => ({locale, letter})),
  );
}

/** Narrows an arbitrary segment to a supported a..z directory letter. */
function isLetter(x: string): boolean {
  return x.length === 1 && x >= 'a' && x <= 'z';
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string; letter: string}>;
}): Promise<Metadata> {
  const {locale, letter} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';
  const lc = letter.toLowerCase();

  // Out-of-range segment (not a..z) → keep it out of the index; notFound() in the
  // body serves the 404 page.
  if (!isLetter(lc)) {
    return {
      title: zh ? '个股目录 · Tickwind' : 'Stock directory · Tickwind',
      robots: {index: false, follow: true},
    };
  }

  const up = lc.toUpperCase();
  const tickers = await tickersForLetter(lc);
  const path = `/stocks/${lc}`;
  const title = zh
    ? `以 ${up} 开头的美股代码 | Tickwind`
    : `US Stocks Starting With ${up} | Tickwind`;
  const description = zh
    ? `按字母浏览:所有以 ${up} 开头、有实时报价的美股代码,每个代码链接到其实时价格、SEC 文件、基本面与新闻页面。延迟数据,仅供参考,不构成投资建议。`
    : `Every quote-bearing US stock ticker starting with ${up} — each links to its live price, SEC filings, fundamentals and news. Delayed data, for reference only, not investment advice.`;

  return {
    title: {absolute: title},
    description,
    alternates: langAlternates(path, loc),
    // noindex-when-thin: a sparse/empty letter is kept out of the index but still
    // followable. A real, populated letter gets the default (indexable).
    ...(tickers.length < MIN_TICKERS_FOR_INDEX ? {robots: {index: false, follow: true}} : {}),
    openGraph: {
      type: 'website',
      title,
      description: description.slice(0, 110),
      url: `${SITE_URL}/${loc}${path}`,
      images: [
        ogImageMeta({
          lang: loc,
          eyebrow: zh ? '个股目录' : 'Stock directory',
          title: zh ? `以 ${up} 开头的美股` : `US stocks starting with ${up}`,
          subtitle: zh ? '点击任意代码查看详情' : 'Pick a ticker for its full page',
        }),
      ],
    },
  };
}

/**
 * One A–Z letter page (pSEO): lists every quote-bearing ticker starting with the
 * letter, each an internal link into `/stock/{t}`. Server-rendered single-locale
 * (chosen from the route segment) so /en and /zh are distinct, crawlable HTML.
 * Company names are intentionally NOT fetched (they're absent from the universe
 * endpoint and a per-ticker name fetch would be thousands of calls — the /stock
 * page already carries the name). Best-effort fetch — a slow/down or empty API
 * renders the graceful empty state (+ noindex via generateMetadata), never a 500.
 * An out-of-range segment (not a..z) → notFound().
 */
export default async function StocksLetterRoute({
  params,
}: {
  params: Promise<{locale: string; letter: string}>;
}) {
  const {locale, letter} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';
  const lc = letter.toLowerCase();
  if (!isLetter(lc)) notFound();

  const up = lc.toUpperCase();
  const path = `/stocks/${lc}`;
  // Best-effort: any failure → empty list → graceful empty state (ISR refills on
  // the next revalidate). Never throws to the route.
  const tickers = await tickersForLetter(lc);

  // JSON-LD: a CollectionPage wrapping an ItemList of this letter's tickers (each
  // a locale-prefixed /stock URL) + a BreadcrumbList (Tickwind → All stocks →
  // letter). All `item`/`url` locale-prefixed to match the canonical (the FIXED
  // pattern, NOT the old bare-path bug).
  const pageUrl = `${SITE_URL}/${loc}${path}`;
  const ld = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'CollectionPage',
        name: zh ? `以 ${up} 开头的美股` : `US stocks starting with ${up}`,
        url: pageUrl,
        mainEntity: {
          '@type': 'ItemList',
          name: zh ? `以 ${up} 开头的美股代码` : `US stock tickers starting with ${up}`,
          numberOfItems: tickers.length,
          itemListElement: tickers.map((tk, i) => ({
            '@type': 'ListItem',
            position: i + 1,
            name: tk,
            url: `${SITE_URL}/${loc}/stock/${encodeURIComponent(tk)}`,
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
            item: `${SITE_URL}/${loc}/stocks`,
          },
          {'@type': 'ListItem', position: 3, name: up, item: pageUrl},
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
        <span className="mx-1.5">/</span>
        <Link href="/stocks" className="hover:underline">
          {zh ? '个股目录' : 'All stocks'}
        </Link>
      </nav>

      <header className="mb-4">
        <h1 className="flex items-center gap-2 text-[24px] font-bold tracking-tight text-slate-900 dark:text-slate-100">
          <LayoutGrid size={20} className="text-sky-600 dark:text-sky-300" />
          {zh ? `以 ${up} 开头的美股` : `US stocks starting with ${up}`}
        </h1>
        <p className="mt-1.5 text-[13.5px] leading-relaxed text-slate-600 dark:text-slate-300">
          {zh
            ? `所有以 ${up} 开头、有实时报价的美股代码。点击任意代码查看其实时价格、SEC 文件、基本面与讨论。`
            : `Every quote-bearing US stock ticker starting with ${up}. Pick a ticker for its live price, SEC filings, fundamentals and discussion.`}
        </p>
      </header>

      {/* A–Z jump bar — at the TOP and sticky under the TopNav, so you can switch letters
          without scrolling past a long ticker grid (it used to sit at the very bottom). */}
      <nav
        aria-label={zh ? '按字母跳转' : 'Jump to letter'}
        className="sticky top-14 z-10 mb-5 rounded-xl border border-slate-200 bg-white/85 px-2.5 py-2.5 backdrop-blur dark:border-slate-800 dark:bg-slate-950/80"
      >
        <div className="flex flex-wrap justify-center gap-1.5 sm:justify-start">
          {STOCK_DIRECTORY_LETTERS.map(other => (
            <Link
              key={other}
              href={`/stocks/${other}`}
              aria-current={other === lc ? 'page' : undefined}
              className={`flex h-8 w-8 items-center justify-center rounded-lg text-[13px] font-semibold uppercase transition ${
                other === lc
                  ? 'bg-sky-600 text-white shadow-sm dark:bg-sky-500'
                  : 'text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800'
              }`}
            >
              {other}
            </Link>
          ))}
        </div>
      </nav>

      {/* Self-healing grid: server-rendered when the universe baked OK (SEO), client-filled
          from the live universe when an ill-timed build baked it empty. */}
      <LetterGrid letter={lc} initial={tickers} zh={zh} />

      <p className="mt-8 text-center text-[11px] text-slate-400 dark:text-slate-500">
        {zh
          ? '数据延迟 · 仅供参考 · 非投资建议'
          : 'Delayed data · For reference only · Not investment advice'}
      </p>
    </article>
  );
}
