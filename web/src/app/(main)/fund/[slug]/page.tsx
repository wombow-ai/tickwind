import type {Metadata} from 'next';
import Link from 'next/link';
import {notFound} from 'next/navigation';
import {Briefcase} from 'lucide-react';
import {getFund, type FundHoldings, type WhalePosition} from '@/lib/api';
import {SITE_URL, langAlternates} from '@/lib/config';
import {ogImageMeta} from '@/lib/og';
import {LocalizedTitle} from '@/components/LocalizedTitle';
import {ShareCardButton} from '@/components/ShareCardButton';
import {fmtCompactUSD} from '@/lib/ui';

// SSR with ISR: a fund's 13F holdings change at most quarterly (filed ~45 days
// after quarter-end), so an hour of caching is plenty. This is a pSEO page —
// "{fund/manager} 持仓" deserves its own indexable URL — living at /fund/{slug}.
export const revalidate = 3600;

/** Quarter-end date "2026-03-31" → "2026 Q1". */
function asOfQuarter(period: string): string {
  const m = /^(\d{4})-(\d{2})/.exec(period);
  if (!m) return period;
  return `${m[1]} Q${Math.ceil(+m[2] / 3)}`;
}

/** Builds the English / Chinese browser-tab titles for a fund. */
function titles(name: string, manager: string): {en: string; zh: string} {
  return {
    en: `${manager} — ${name} 13F Holdings · Tickwind`,
    zh: `${manager}（${name}）持仓 · 13F 大佬持仓 · 潮汐 Tickwind`,
  };
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{slug: string}>;
}): Promise<Metadata> {
  const {slug} = await params;
  let f: FundHoldings | null = null;
  try {
    f = await getFund(slug, AbortSignal.timeout(5000));
  } catch {
    f = null; // API hiccup → generic title; the page still renders / notFound
  }
  if (!f) return {title: 'Fund holdings'};
  const tt = titles(f.name, f.manager);
  const path = `/fund/${slug}`;
  return {
    // English-default tab title (LocalizedTitle swaps zh); Chinese keywords live
    // in description/keywords for the targeting.
    title: {absolute: tt.en},
    description: `${f.manager}（${f.name}）依 SEC 13F 披露的最新一季持仓明细 —— 逐只股票的市值、占组合权重与环比加减仓。Track ${f.manager}'s latest 13F portfolio holdings from SEC filings. 公开数据，滞后约 45 天，不构成投资建议。`,
    keywords: [
      `${f.manager} 持仓`,
      `${f.name} 持仓`,
      `${f.manager} 13F`,
      `${f.manager} portfolio`,
      `${f.manager} holdings`,
      '13F 大佬持仓',
      '机构持仓',
    ],
    alternates: langAlternates(path),
    openGraph: {
      type: 'profile',
      title: tt.en,
      url: `${SITE_URL}${path}`,
      images: [
        ogImageMeta({
          eyebrow: '13F 持仓',
          title: `${f.manager} 持仓`,
          subtitle: `${f.name} · SEC 13F 大佬持仓`,
        }),
      ],
    },
  };
}

/**
 * Fund detail page (pSEO): one famous manager's latest quarterly SEC 13F
 * holdings from the public-domain 13F dataset. Server-rendered so crawlers get
 * the full table; bilingual chrome via [data-i18n] CSS keyed to <html lang>, the
 * tab title swapped by LocalizedTitle. Unknown slug → notFound().
 */
export default async function FundRoute({params}: {params: Promise<{slug: string}>}) {
  const {slug} = await params;
  let f: FundHoldings | null = null;
  try {
    f = await getFund(slug, AbortSignal.timeout(5000));
  } catch {
    // A transient API error shouldn't hard-404 a real fund; render the shell so
    // ISR can refill on the next request. (notFound only on a real 404.)
    f = null;
  }
  if (!f) notFound();

  const tt = titles(f.name, f.manager);
  const positions = f.positions ?? [];

  // Share card: a 13F 大佬持仓 card for 小红书 / 微信. Subtitle lists the top
  // few positions (ticker + portfolio weight), which is the shareable hook.
  const topNames = positions
    .filter(p => p.ticker)
    .slice(0, 4)
    .map(p => `${p.ticker} ${p.pct.toFixed(1)}%`)
    .join(' · ');
  const shareCard = {
    eyebrow: '13F 大佬持仓',
    title: `${f.manager}（${f.name}）持仓`,
    subtitle: topNames ? `最新季 top: ${topNames}` : `${f.name} · SEC 13F 大佬持仓 · 滞后约 45 天`,
  };

  const ld = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'BreadcrumbList',
        itemListElement: [
          {'@type': 'ListItem', position: 1, name: 'Tickwind', item: SITE_URL},
          {'@type': 'ListItem', position: 2, name: 'Smart money', item: `${SITE_URL}/smart-money`},
          {
            '@type': 'ListItem',
            position: 3,
            name: `${f.manager} — ${f.name}`,
            item: `${SITE_URL}/fund/${slug}`,
          },
        ],
      },
    ],
  };

  return (
    <article className="mx-auto max-w-3xl">
      <LocalizedTitle en={tt.en} zh={tt.zh} />
      <script type="application/ld+json" dangerouslySetInnerHTML={{__html: JSON.stringify(ld)}} />

      <nav className="mb-4 text-[12px] text-slate-500 dark:text-slate-400" aria-label="Breadcrumb">
        <Link href="/" className="hover:underline">
          Tickwind
        </Link>
        <span className="mx-1.5">/</span>
        <Link href="/smart-money?tab=13f" className="hover:underline">
          <span data-i18n="zh">大佬持仓</span>
          <span data-i18n="en">Whale holdings</span>
        </Link>
      </nav>

      <header className="mb-4">
        <div className="flex items-start justify-between gap-3">
          <h1 className="flex items-center gap-2 text-[24px] font-bold tracking-tight text-slate-900 dark:text-slate-100">
            <Briefcase size={20} className="text-violet-600 dark:text-violet-300" />
            {f.manager}
          </h1>
          {/* propagation organ: save a branded 13F holdings card */}
          <ShareCardButton card={shareCard} />
        </div>
        <div className="mt-2 flex flex-wrap items-center gap-2">
          <span className="text-[13px] text-slate-500 dark:text-slate-400">{f.name}</span>
          <span className="inline-block rounded-full bg-slate-100 px-2.5 py-0.5 text-[11px] font-semibold text-slate-500 dark:bg-slate-800 dark:text-slate-300">
            <span data-i18n="zh">截至 </span>
            <span data-i18n="en">as of </span>
            {asOfQuarter(f.period)}
          </span>
          <span className="text-[11.5px] tabular-nums text-slate-400 dark:text-slate-500">
            · {fmtCompactUSD(f.value)}
            <span data-i18n="zh"> 组合 · {f.count} 只持仓</span>
            <span data-i18n="en"> portfolio · {f.count} positions</span>
          </span>
        </div>
      </header>

      <div className="mb-5 rounded-xl border border-slate-200 bg-slate-50 p-3 text-[12px] text-slate-500 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-400">
        <span data-i18n="zh">
          公开数据（SEC 13F-HR 申报）。每季度末快照，披露最多滞后约 45 天，且仅含美股多头（不含做空/期权）—— 非实时持仓，亦非投资建议。
        </span>
        <span data-i18n="en">
          Public data (SEC 13F-HR filings). A quarter-end snapshot disclosed up to ~45 days late, long
          U.S. equity positions only (no shorts or options) — not real-time holdings, and not investment
          advice.
        </span>
      </div>

      <h2 className="mb-3 text-[15px] font-bold text-slate-900 dark:text-slate-100">
        <span data-i18n="zh">最新一季持仓</span>
        <span data-i18n="en">Latest quarter holdings</span>
      </h2>

      {positions.length === 0 ? (
        <div className="rounded-2xl border border-slate-200 px-6 py-10 text-center dark:border-slate-800">
          <p className="text-[14px] font-semibold text-slate-900 dark:text-slate-100">
            <span data-i18n="zh">暂无持仓数据</span>
            <span data-i18n="en">No holdings yet</span>
          </p>
          <p className="mt-1 text-[12.5px] text-slate-500 dark:text-slate-400">
            <span data-i18n="zh">正在抓取最新 13F 申报 —— 稍后再来看看。</span>
            <span data-i18n="en">Fetching the latest 13F filings — check back shortly.</span>
          </p>
        </div>
      ) : (
        <div className="overflow-x-auto rounded-2xl border border-slate-200 dark:border-slate-800">
          <table className="w-full border-collapse text-left text-[13px]">
            <thead>
              <tr className="border-b border-slate-200 text-[11.5px] font-semibold uppercase tracking-wide text-slate-400 dark:border-slate-800 dark:text-slate-500">
                <th className="px-3 py-2.5 font-semibold">
                  <span data-i18n="zh">股票</span>
                  <span data-i18n="en">Stock</span>
                </th>
                <th className="px-3 py-2.5 text-right font-semibold">
                  <span data-i18n="zh">市值</span>
                  <span data-i18n="en">Value</span>
                </th>
                <th className="px-3 py-2.5 text-right font-semibold">
                  <span data-i18n="zh">占比</span>
                  <span data-i18n="en">Weight</span>
                </th>
                <th className="px-3 py-2.5 text-right font-semibold">
                  <span data-i18n="zh">环比</span>
                  <span data-i18n="en">QoQ</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {positions.map((p, i) => (
                <PositionRow key={`${p.ticker || p.issuer}-${i}`} p={p} />
              ))}
            </tbody>
          </table>
        </div>
      )}

      <p className="mt-6 text-center text-[11px] text-slate-400 dark:text-slate-500">
        <span data-i18n="zh">
          数据来源：SEC 13F-HR 申报（公有领域）；CUSIP→代码经 OpenFIGI 映射。非投资建议。
        </span>
        <span data-i18n="en">
          Source: SEC 13F-HR filings (public domain); CUSIP→ticker via OpenFIGI. Not investment advice.
        </span>
      </p>
    </article>
  );
}

/** One holding row: stock (+ ticker link) · value · weight · QoQ change. */
function PositionRow({p}: {p: WhalePosition}) {
  const changeCls =
    p.change === 'new'
      ? 'bg-sky-50 text-sky-600 dark:bg-sky-500/15 dark:text-sky-300'
      : p.change === 'add'
        ? 'bg-emerald-50 text-emerald-600 dark:bg-emerald-500/15 dark:text-emerald-300'
        : p.change === 'trim'
          ? 'bg-rose-50 text-rose-600 dark:bg-rose-500/15 dark:text-rose-300'
          : 'bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400';
  const changeLabel: Record<string, {en: string; zh: string}> = {
    new: {en: 'New', zh: '新建仓'},
    add: {en: 'Add', zh: '加仓'},
    trim: {en: 'Trim', zh: '减仓'},
    hold: {en: 'Hold', zh: '持有'},
  };
  const lbl = changeLabel[p.change] ?? changeLabel.hold;
  const pctSuffix =
    (p.change === 'add' || p.change === 'trim') && p.chg_pct !== 0
      ? ` ${p.chg_pct > 0 ? '+' : ''}${p.chg_pct.toFixed(0)}%`
      : '';
  return (
    <tr className="border-b border-slate-100 last:border-0 dark:border-slate-800/60">
      <td className="px-3 py-2.5">
        {p.ticker ? (
          <Link
            href={`/stock/${encodeURIComponent(p.ticker)}`}
            className="font-bold text-teal-700 hover:underline dark:text-teal-300"
          >
            {p.ticker}
          </Link>
        ) : (
          <span className="font-semibold text-slate-500 dark:text-slate-400">{p.issuer}</span>
        )}
        {p.ticker && (
          <span className="ml-1.5 hidden text-[11px] text-slate-400 sm:inline dark:text-slate-500">
            {p.issuer}
          </span>
        )}
      </td>
      <td className="whitespace-nowrap px-3 py-2.5 text-right font-semibold tabular-nums text-slate-800 dark:text-slate-100">
        {fmtCompactUSD(p.value)}
      </td>
      <td className="px-3 py-2.5 text-right tabular-nums text-slate-500 dark:text-slate-400">
        {p.pct.toFixed(1)}%
      </td>
      <td className="px-3 py-2.5 text-right">
        <span className={`rounded-md px-1.5 py-0.5 text-[10.5px] font-bold ${changeCls}`}>
          <span data-i18n="zh">{lbl.zh}</span>
          <span data-i18n="en">{lbl.en}</span>
          {pctSuffix}
        </span>
      </td>
    </tr>
  );
}
