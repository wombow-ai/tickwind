import type {Metadata} from 'next';
import Link from '@/components/LocalLink';
import {notFound} from 'next/navigation';
import {Landmark} from 'lucide-react';
import {
  getCongressBacktest,
  getCongressMember,
  type Backtest,
  type MemberResponse,
  type MemberTx,
} from '@/lib/api';
import {SITE_URL, langAlternates} from '@/lib/config';
import {isLocale} from '@/lib/locale';
import {ogImageMeta} from '@/lib/og';
import {FollowTradeSim} from '@/components/FollowTradeSim';

// SSR with ISR: a member's disclosure history changes at most daily, so cache an
// hour. This is the rare pSEO exception — "{member} holdings" deserves its own
// indexable URL (per the plan), so the detail lives at /congress/member/{slug}.
export const revalidate = 3600;

/** Builds the English / Chinese browser-tab titles for a member. */
function titles(name: string): {en: string; zh: string} {
  return {
    en: `${name} — Stock Trades & Holdings · Tickwind`,
    zh: `${name} 持仓与交易 · 国会山股神 · 潮汐 Tickwind`,
  };
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string; slug: string}>;
}): Promise<Metadata> {
  const {locale, slug} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  let m: MemberResponse | null = null;
  try {
    m = await getCongressMember(slug, AbortSignal.timeout(5000));
  } catch {
    m = null; // API hiccup → fall through to a generic title; the page still renders/notFound
  }
  if (!m) return {title: 'Congress member'};
  const tt = titles(m.name);
  const path = `/congress/member/${slug}`;
  return {
    // Locale-matched tab title; Chinese keywords in description/keywords.
    title: {absolute: loc === 'zh' ? tt.zh : tt.en},
    description: `${m.name}（美国国会议员${m.state ? ` · ${m.state}` : ''}）依《STOCK 法案》披露的股票买卖与持仓记录，逐条对应官方申报。Track ${m.name}'s disclosed U.S. congressional stock trades. 公开数据，不构成投资建议。`,
    keywords: [
      `${m.name} 持仓`,
      `${m.name} 交易`,
      `${m.name} stock trades`,
      '国会山股神',
      '美国国会议员股票交易',
      'congress stock tracker',
    ],
    alternates: langAlternates(path, loc),
    openGraph: {
      type: 'profile',
      title: loc === 'zh' ? tt.zh : tt.en,
      url: `${SITE_URL}/${loc}${path}`,
      images: [
        ogImageMeta({
          lang: loc,
          eyebrow: loc === 'zh' ? '国会交易' : 'Congress trades',
          title: loc === 'zh' ? `${m.name} 持仓` : `${m.name} holdings`,
          subtitle:
            loc === 'zh'
              ? '美国国会议员股票买卖披露 · 国会山股神'
              : 'U.S. lawmaker stock-trade disclosures · Congress tracker',
        }),
      ],
    },
  };
}

/** Normalizes a disclosed direction into buy / sell / exchange / other. */
function side(type: string): 'buy' | 'sell' | 'exchange' | 'other' {
  const x = (type ?? '').toLowerCase();
  if (x.includes('purchase') || x.includes('buy')) return 'buy';
  if (x.includes('sale') || x.includes('sell')) return 'sell';
  if (x.includes('exchange')) return 'exchange';
  return 'other';
}

/** Formats a member's TxDate for both locales (falls back to the raw string). */
function fmtDate(raw: string): {en: string; zh: string} {
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) return {en: raw, zh: raw};
  const opts: Intl.DateTimeFormatOptions = {year: 'numeric', month: 'short', day: 'numeric'};
  return {en: d.toLocaleDateString('en-US', opts), zh: d.toLocaleDateString('zh-CN', opts)};
}

/**
 * Member detail page (pSEO): a U.S. House member's disclosed stock trades from
 * the public-domain House Clerk PTR dataset. Server-rendered so crawlers get the
 * full table; only the active locale's chrome (chosen from the route segment) is
 * emitted, the per-locale tab title set in generateMetadata, so /en and /zh are
 * distinct single-language HTML. Unknown slug → notFound().
 */
export default async function MemberRoute({params}: {params: Promise<{locale: string; slug: string}>}) {
  const {locale, slug} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';
  let m: MemberResponse | null = null;
  try {
    m = await getCongressMember(slug, AbortSignal.timeout(5000));
  } catch {
    // A transient API error shouldn't hard-404 a real member; show the empty
    // shell so ISR can refill on the next request. (notFound only on a real 404.)
    m = {slug, name: slug, state: '', transactions: []};
  }
  if (!m) notFound();

  // Follow-trade simulation (historical replay). Best-effort + SSR: a slow/failed
  // backtest fetch just hides the section rather than breaking the page.
  let bt: Backtest | null = null;
  try {
    const res = await getCongressBacktest(slug, AbortSignal.timeout(8000));
    bt = res?.backtest ?? null;
  } catch {
    bt = null;
  }

  const txs = m.transactions ?? [];

  const ld = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'BreadcrumbList',
        itemListElement: [
          {'@type': 'ListItem', position: 1, name: 'Tickwind', item: `${SITE_URL}/${loc}`},
          {'@type': 'ListItem', position: 2, name: zh ? '聪明钱' : 'Smart money', item: `${SITE_URL}/${loc}/smart-money`},
          {
            '@type': 'ListItem',
            position: 3,
            name: m.name,
            item: `${SITE_URL}/${loc}/congress/member/${slug}`,
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
          Tickwind
        </Link>
        <span className="mx-1.5">/</span>
        <Link href="/smart-money?tab=congress" className="hover:underline">
          {zh ? '国会交易' : 'Congress'}
        </Link>
      </nav>

      <header className="mb-4">
        <h1 className="flex items-center gap-2 text-[24px] font-bold tracking-tight text-slate-900 dark:text-slate-100">
          <Landmark size={20} className="text-sky-600 dark:text-sky-300" />
          {m.name}
        </h1>
        {m.state && (
          <span className="mt-2 inline-block rounded-full bg-slate-100 px-2.5 py-0.5 text-[11px] font-semibold text-slate-500 dark:bg-slate-800 dark:text-slate-300">
            {m.state}
          </span>
        )}
      </header>

      <div className="mb-5 rounded-xl border border-slate-200 bg-slate-50 p-3 text-[12px] text-slate-500 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-400">
        {zh
          ? '公开数据（美国众议院书记官）。申报最多滞后 45 天，且仅标注金额区间 —— 非实时交易，亦非投资建议。'
          : 'Public data (U.S. House Clerk). Disclosures lag up to 45 days and show only amount ranges — not real-time trades, and not investment advice.'}
      </div>

      {bt && <FollowTradeSim bt={bt} memberName={m.name} />}

      <h2 className="mb-3 text-[15px] font-bold text-slate-900 dark:text-slate-100">
        {zh ? '披露交易' : 'Disclosed trades'}
      </h2>

      {txs.length === 0 ? (
        <div className="rounded-2xl border border-slate-200 px-6 py-10 text-center dark:border-slate-800">
          <p className="text-[14px] font-semibold text-slate-900 dark:text-slate-100">
            {zh ? '暂无披露交易' : 'No disclosed trades'}
          </p>
          <p className="mt-1 text-[12.5px] text-slate-500 dark:text-slate-400">
            {zh
              ? '该议员目前还没有定期交易报告（PTR）在档。'
              : 'No Periodic Transaction Reports are on file for this member yet.'}
          </p>
        </div>
      ) : (
        <div className="overflow-x-auto rounded-2xl border border-slate-200 dark:border-slate-800">
          <table className="w-full border-collapse text-left text-[13px]">
            <thead>
              <tr className="border-b border-slate-200 text-[11.5px] font-semibold uppercase tracking-wide text-slate-400 dark:border-slate-800 dark:text-slate-500">
                <th className="px-3 py-2.5 font-semibold">{zh ? '日期' : 'Date'}</th>
                <th className="px-3 py-2.5 font-semibold">{zh ? '方向' : 'Type'}</th>
                <th className="px-3 py-2.5 font-semibold">{zh ? '资产' : 'Asset'}</th>
                <th className="px-3 py-2.5 text-right font-semibold">{zh ? '金额区间' : 'Amount'}</th>
              </tr>
            </thead>
            <tbody>
              {txs.map((tx, i) => (
                <TxRow key={`${tx.tx_date}-${tx.ticker}-${tx.asset}-${i}`} tx={tx} zh={zh} />
              ))}
            </tbody>
          </table>
        </div>
      )}

      <p className="mt-6 text-center text-[11px] text-slate-400 dark:text-slate-500">
        {zh
          ? '数据来源：美国众议院书记官金融披露（公有领域）。非投资建议。'
          : 'Source: U.S. House Clerk financial disclosures (public domain). Not investment advice.'}
      </p>
    </article>
  );
}

/** One disclosure row: date · buy/sell (green/red) · asset (+ ticker link) · amount. */
function TxRow({tx, zh}: {tx: MemberTx; zh: boolean}) {
  const s = side(tx.type);
  const date = fmtDate(tx.tx_date);
  const sideCls =
    s === 'buy'
      ? 'bg-emerald-50 text-emerald-600 dark:bg-emerald-500/15 dark:text-emerald-300'
      : s === 'sell'
        ? 'bg-rose-50 text-rose-600 dark:bg-rose-500/15 dark:text-rose-300'
        : 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300';
  return (
    <tr className="border-b border-slate-100 last:border-0 dark:border-slate-800/60">
      <td className="whitespace-nowrap px-3 py-2.5 tabular-nums text-slate-500 dark:text-slate-400">
        {zh ? date.zh : date.en}
      </td>
      <td className="px-3 py-2.5">
        <span className={`rounded-md px-1.5 py-0.5 text-[11px] font-bold ${sideCls}`}>
          {s === 'buy'
            ? zh
              ? '买入'
              : 'Buy'
            : s === 'sell'
              ? zh
                ? '卖出'
                : 'Sell'
              : s === 'exchange'
                ? zh
                  ? '换股'
                  : 'Exchange'
                : tx.type}
        </span>
      </td>
      <td className="px-3 py-2.5 text-slate-800 dark:text-slate-100">
        <span>{tx.asset}</span>
        {tx.ticker && (
          <Link
            href={`/stock/${encodeURIComponent(tx.ticker)}`}
            className="ml-1.5 font-semibold text-teal-700 hover:underline dark:text-teal-300"
          >
            {tx.ticker}
          </Link>
        )}
      </td>
      <td className="whitespace-nowrap px-3 py-2.5 text-right tabular-nums text-slate-600 dark:text-slate-300">
        {tx.amount_range}
      </td>
    </tr>
  );
}
