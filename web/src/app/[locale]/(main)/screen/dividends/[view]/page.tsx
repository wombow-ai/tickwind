import type {Metadata} from 'next';
import Link from '@/components/LocalLink';
import {notFound} from 'next/navigation';
import {Coins} from 'lucide-react';
import {getDividendScreen, type DividendRank} from '@/lib/api';
import {SITE_URL, langAlternates} from '@/lib/config';
import {isLocale, LOCALES} from '@/lib/locale';
import {ogImageMeta} from '@/lib/og';
import {DIVIDEND_VIEWS, dividendViewByKey} from '@/lib/dividendViews';
import {DividendLeaderboard} from '@/components/DividendLeaderboard';

// Dividend profiles are SEC-fundamentals + price-derived (rebuilt hourly); ISR re-fetches every 30 min
// so the page self-heals an empty/cold bake without a deploy. The client component self-heals for users.
export const revalidate = 1800;

/** Pre-render every view × locale (4 × 2) at build time. */
export function generateStaticParams(): {locale: string; view: string}[] {
  return LOCALES.flatMap(locale => DIVIDEND_VIEWS.map(v => ({locale, view: v.key})));
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string; view: string}>;
}): Promise<Metadata> {
  const {locale, view} = await params;
  const v = dividendViewByKey(view);
  if (!v) return {title: 'Dividend screener'};
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';
  const title = zh ? v.titleZh : v.titleEn;
  const desc = zh ? v.descZh : v.descEn;
  const path = `/screen/dividends/${v.key}`;
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
          eyebrow: zh ? '分红榜' : 'Dividends',
          title,
          subtitle: desc.slice(0, 54),
        }),
      ],
    },
  };
}

/**
 * Dividend leaderboard landing page (pSEO): every tracked payer ranked by one dividend metric — the
 * market-wide view of the per-stock dividend card. Server-rendered single-locale (chosen from the route
 * segment) so /en and /zh are distinct, crawlable HTML. Best-effort SSR fetch — a slow/down API or a
 * cold cache renders the empty state, never a 500; the DividendLeaderboard client component then
 * self-heals for users. Descriptive figures, no advice. Unknown view slug → notFound().
 */
export default async function DividendScreenRoute({
  params,
}: {
  params: Promise<{locale: string; view: string}>;
}) {
  const {locale, view} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';
  const v = dividendViewByKey(view);
  if (!v) notFound();

  const title = zh ? v.titleZh : v.titleEn;
  const desc = zh ? v.descZh : v.descEn;
  const path = `/screen/dividends/${v.key}`;

  // Best-effort fetch: any failure → empty (the client component self-heals + ISR refills). Never throws.
  let results: DividendRank[] = [];
  let total = 0;
  try {
    const r = await getDividendScreen(v.key, 100, AbortSignal.timeout(8000));
    results = r.results ?? [];
    total = r.total ?? 0;
  } catch {
    results = [];
  }

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
          {'@type': 'ListItem', position: 2, name: zh ? '美股筛选' : 'Screener', item: `${SITE_URL}/${loc}/screen`},
          {'@type': 'ListItem', position: 3, name: title, item: `${SITE_URL}/${loc}${path}`},
        ],
      },
    ],
  };

  const others = DIVIDEND_VIEWS.filter(o => o.key !== v.key);

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
          <Coins size={20} className="text-amber-600 dark:text-amber-300" />
          {title}
        </h1>
        <p className="mt-1.5 text-[13.5px] leading-relaxed text-slate-600 dark:text-slate-300">{desc}</p>
      </header>

      {/* The leaderboard self-heals client-side (see DividendLeaderboard) — the SSR `results` seed the
          crawlable rows + JSON-LD when the tunnel cooperates, but the browser re-fetch guarantees users
          always see the live ranking even when the SSR fetch baked empty. */}
      <DividendLeaderboard view={v.key} primary={v.primary} initial={results} initialTotal={total} zh={zh} />

      <p className="mt-4 text-center text-[11px] text-slate-400 dark:text-slate-500">
        {zh
          ? '股息率=年度分红 ÷ 市值 · 派息率=分红 ÷ 净利润 · 覆盖=自由现金流 ÷ 分红 · 在档历史统计 · 非投资建议'
          : 'Yield = dividends ÷ market cap · payout = dividends ÷ net income · cover = FCF ÷ dividends · as-filed · Not investment advice'}
      </p>

      {/* Cross-link hub: the other dividend views + the full screener, for internal linking. */}
      <section className="mt-8">
        <h2 className="mb-2.5 text-[15px] font-bold text-slate-900 dark:text-slate-100">
          {zh ? '更多分红榜' : 'More dividend views'}
        </h2>
        <div className="grid gap-2 sm:grid-cols-2">
          {others.map(o => (
            <Link
              key={o.key}
              href={`/screen/dividends/${o.key}`}
              className="block rounded-xl border border-slate-200 px-3 py-2.5 hover:border-amber-300 hover:bg-slate-50 dark:border-slate-800 dark:hover:border-amber-500/40 dark:hover:bg-slate-900"
            >
              <div className="text-[13px] font-semibold text-slate-800 dark:text-slate-100">{zh ? o.titleZh : o.titleEn}</div>
            </Link>
          ))}
          <Link
            href="/screen"
            className="block rounded-xl border border-dashed border-slate-300 px-3 py-2.5 text-[13px] font-medium text-slate-600 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-900"
          >
            {zh ? '美股筛选首页 →' : 'All screeners →'}
          </Link>
        </div>
      </section>
    </article>
  );
}
