import type {Metadata} from 'next';
import Link from '@/components/LocalLink';
import {notFound} from 'next/navigation';
import {Scale} from 'lucide-react';
import {getFundamentals, getQuote, type Fundamentals, type Quote} from '@/lib/api';
import {COMPARE_PAIRS, pairSlug, parsePair} from '@/lib/compare';
import {SITE_URL, langAlternates} from '@/lib/config';
import {isLocale} from '@/lib/locale';
import {fmtCompactUSD} from '@/lib/ui';
import {ogImageMeta} from '@/lib/og';

// Fundamentals are quarterly + the quote snapshot is for the crawlable view; a 30-min ISR window
// keeps the page fresh enough without hammering the API on every hit.
export const revalidate = 1800;

/** Prerender the curated rivalries (both locales); any other fetchable pair renders on-demand. */
export function generateStaticParams(): {locale: string; pair: string}[] {
  const out: {locale: string; pair: string}[] = [];
  for (const loc of ['en', 'zh']) {
    for (const [a, b] of COMPARE_PAIRS) out.push({locale: loc, pair: pairSlug(a, b)});
  }
  return out;
}

type Loaded = {ticker: string; f: Fundamentals | null; q: Quote | null};

/** Best-effort fetch of one ticker's fundamentals + quote (a failure → null, never throws). */
async function load(ticker: string): Promise<Loaded> {
  const [fR, qR] = await Promise.allSettled([
    getFundamentals(ticker, AbortSignal.timeout(8000)),
    getQuote(ticker, AbortSignal.timeout(8000)),
  ]);
  return {
    ticker,
    f: fR.status === 'fulfilled' ? fR.value : null,
    q: qR.status === 'fulfilled' ? qR.value : null,
  };
}

function hasData(l: Loaded): boolean {
  return l.f !== null || (l.q !== null && l.q.price > 0);
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
 * Side-by-side stock comparison (pSEO): two stocks' price + Go-computed fundamentals (market cap,
 * P/E, P/B, revenue, net income, EPS) in one crawlable table, for high-intent "X vs Y" queries.
 * Every number is sourced from the public quote + SEC-XBRL fundamentals endpoints — NOTHING is a
 * recommendation and the page never declares a "winner" (just the figures, side by side). An
 * unparseable slug 404s; a pair where NEITHER ticker resolves to data → notFound() (kept out of
 * the index). The crawlable core is server-rendered per the active locale.
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

  const [la, lb] = await Promise.all([load(a), load(b)]);
  if (!hasData(la) && !hasData(lb)) notFound(); // neither side has anything → keep out of the index

  const path = `/compare/${pairSlug(a, b)}`;
  const nameA = la.f?.name || a;
  const nameB = lb.f?.name || b;

  const dash = '—';
  const num = (v: number | null | undefined, digits = 2) =>
    v === null || v === undefined || !isFinite(v) ? dash : v.toFixed(digits);
  const usd = (v: number | null | undefined) =>
    v === null || v === undefined || !isFinite(v) || v === 0 ? dash : fmtCompactUSD(v);
  const dayChange = (q: Quote | null): string => {
    if (!q || !q.prev_close || q.prev_close <= 0 || !q.price) return dash;
    const pct = (q.price / q.prev_close - 1) * 100;
    return `${pct > 0 ? '+' : ''}${pct.toFixed(2)}%`;
  };

  type Row = {label: string; a: string; b: string};
  const rows: Row[] = [
    {label: zh ? '股价' : 'Price', a: la.q?.price ? `$${num(la.q.price)}` : dash, b: lb.q?.price ? `$${num(lb.q.price)}` : dash},
    {label: zh ? '当日涨跌' : 'Day change', a: dayChange(la.q), b: dayChange(lb.q)},
    {label: zh ? '市值' : 'Market cap', a: usd(la.f?.market_cap), b: usd(lb.f?.market_cap)},
    {label: zh ? '市盈率 (P/E)' : 'P/E', a: la.f && la.f.pe === null ? (zh ? '亏损' : 'loss') : num(la.f?.pe), b: lb.f && lb.f.pe === null ? (zh ? '亏损' : 'loss') : num(lb.f?.pe)},
    {label: zh ? '市净率 (P/B)' : 'P/B', a: num(la.f?.pb), b: num(lb.f?.pb)},
    {label: zh ? '营收' : 'Revenue', a: usd(la.f?.revenue), b: usd(lb.f?.revenue)},
    {label: zh ? '净利润' : 'Net income', a: usd(la.f?.net_income), b: usd(lb.f?.net_income)},
    {label: zh ? '摊薄每股收益' : 'Diluted EPS', a: la.f ? `$${num(la.f.eps_diluted)}` : dash, b: lb.f ? `$${num(lb.f.eps_diluted)}` : dash},
  ];

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

  // Related rivalries (excluding this pair) for crawl discovery + browse utility.
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

      <section className="mb-7 overflow-hidden rounded-2xl border border-slate-200 dark:border-slate-800">
        <table className="w-full text-[13.5px]">
          <thead>
            <tr className="bg-slate-50 dark:bg-slate-900/50">
              <th className="px-3 py-2.5 text-left text-[11px] font-semibold uppercase tracking-wide text-slate-400 dark:text-slate-500">
                {zh ? '指标' : 'Metric'}
              </th>
              {[
                {t: a, n: nameA},
                {t: b, n: nameB},
              ].map(s => (
                <th key={s.t} className="px-3 py-2.5 text-right">
                  <Link href={`/stock/${encodeURIComponent(s.t)}`} className="font-bold text-slate-900 hover:underline dark:text-slate-100">
                    {s.t}
                  </Link>
                  <span className="block max-w-[140px] truncate text-[10.5px] font-normal text-slate-400 dark:text-slate-500" title={s.n}>
                    {s.n}
                  </span>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((r, i) => (
              <tr key={r.label} className={i % 2 ? 'bg-slate-50/40 dark:bg-slate-900/20' : ''}>
                <td className="px-3 py-2.5 text-slate-500 dark:text-slate-400">{r.label}</td>
                <td className="px-3 py-2.5 text-right font-semibold tabular-nums text-slate-900 dark:text-slate-100">{r.a}</td>
                <td className="px-3 py-2.5 text-right font-semibold tabular-nums text-slate-900 dark:text-slate-100">{r.b}</td>
              </tr>
            ))}
          </tbody>
        </table>
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
