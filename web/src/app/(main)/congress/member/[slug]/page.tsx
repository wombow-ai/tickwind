import type {Metadata} from 'next';
import Link from 'next/link';
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
import {ogImageMeta} from '@/lib/og';
import {LocalizedTitle} from '@/components/LocalizedTitle';
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
  params: Promise<{slug: string}>;
}): Promise<Metadata> {
  const {slug} = await params;
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
    // English-default tab title (LocalizedTitle swaps zh); Chinese keywords in
    // description/keywords for the targeting.
    title: {absolute: tt.en},
    description: `${m.name}（美国国会议员${m.state ? ` · ${m.state}` : ''}）依《STOCK 法案》披露的股票买卖与持仓记录，逐条对应官方申报。Track ${m.name}'s disclosed U.S. congressional stock trades. 公开数据，不构成投资建议。`,
    keywords: [
      `${m.name} 持仓`,
      `${m.name} 交易`,
      `${m.name} stock trades`,
      '国会山股神',
      '美国国会议员股票交易',
      'congress stock tracker',
    ],
    alternates: langAlternates(path),
    openGraph: {
      type: 'profile',
      title: tt.en,
      url: `${SITE_URL}${path}`,
      images: [
        ogImageMeta({
          eyebrow: '国会交易',
          title: `${m.name} 持仓`,
          subtitle: '美国国会议员股票买卖披露 · 国会山股神',
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
 * full table; bilingual chrome via the [data-i18n] CSS keyed to <html lang>, the
 * tab title swapped by LocalizedTitle. Unknown slug → notFound().
 */
export default async function MemberRoute({params}: {params: Promise<{slug: string}>}) {
  const {slug} = await params;
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

  const tt = titles(m.name);
  const txs = m.transactions ?? [];

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
            name: m.name,
            item: `${SITE_URL}/congress/member/${slug}`,
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
        <Link href="/smart-money?tab=congress" className="hover:underline">
          <span data-i18n="zh">国会交易</span>
          <span data-i18n="en">Congress</span>
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
        <span data-i18n="zh">
          公开数据（美国众议院书记官）。申报最多滞后 45 天，且仅标注金额区间 —— 非实时交易，亦非投资建议。
        </span>
        <span data-i18n="en">
          Public data (U.S. House Clerk). Disclosures lag up to 45 days and show only amount ranges —
          not real-time trades, and not investment advice.
        </span>
      </div>

      {bt && <FollowTradeSim bt={bt} />}

      <h2 className="mb-3 text-[15px] font-bold text-slate-900 dark:text-slate-100">
        <span data-i18n="zh">披露交易</span>
        <span data-i18n="en">Disclosed trades</span>
      </h2>

      {txs.length === 0 ? (
        <div className="rounded-2xl border border-slate-200 px-6 py-10 text-center dark:border-slate-800">
          <p className="text-[14px] font-semibold text-slate-900 dark:text-slate-100">
            <span data-i18n="zh">暂无披露交易</span>
            <span data-i18n="en">No disclosed trades</span>
          </p>
          <p className="mt-1 text-[12.5px] text-slate-500 dark:text-slate-400">
            <span data-i18n="zh">该议员目前还没有定期交易报告（PTR）在档。</span>
            <span data-i18n="en">No Periodic Transaction Reports are on file for this member yet.</span>
          </p>
        </div>
      ) : (
        <div className="overflow-x-auto rounded-2xl border border-slate-200 dark:border-slate-800">
          <table className="w-full border-collapse text-left text-[13px]">
            <thead>
              <tr className="border-b border-slate-200 text-[11.5px] font-semibold uppercase tracking-wide text-slate-400 dark:border-slate-800 dark:text-slate-500">
                <th className="px-3 py-2.5 font-semibold">
                  <span data-i18n="zh">日期</span>
                  <span data-i18n="en">Date</span>
                </th>
                <th className="px-3 py-2.5 font-semibold">
                  <span data-i18n="zh">方向</span>
                  <span data-i18n="en">Type</span>
                </th>
                <th className="px-3 py-2.5 font-semibold">
                  <span data-i18n="zh">资产</span>
                  <span data-i18n="en">Asset</span>
                </th>
                <th className="px-3 py-2.5 text-right font-semibold">
                  <span data-i18n="zh">金额区间</span>
                  <span data-i18n="en">Amount</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {txs.map((tx, i) => (
                <TxRow key={`${tx.tx_date}-${tx.ticker}-${tx.asset}-${i}`} tx={tx} />
              ))}
            </tbody>
          </table>
        </div>
      )}

      <p className="mt-6 text-center text-[11px] text-slate-400 dark:text-slate-500">
        <span data-i18n="zh">数据来源：美国众议院书记官金融披露（公有领域）。非投资建议。</span>
        <span data-i18n="en">
          Source: U.S. House Clerk financial disclosures (public domain). Not investment advice.
        </span>
      </p>
    </article>
  );
}

/** One disclosure row: date · buy/sell (green/red) · asset (+ ticker link) · amount. */
function TxRow({tx}: {tx: MemberTx}) {
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
        <span data-i18n="zh">{date.zh}</span>
        <span data-i18n="en">{date.en}</span>
      </td>
      <td className="px-3 py-2.5">
        <span className={`rounded-md px-1.5 py-0.5 text-[11px] font-bold ${sideCls}`}>
          {s === 'buy' ? (
            <>
              <span data-i18n="zh">买入</span>
              <span data-i18n="en">Buy</span>
            </>
          ) : s === 'sell' ? (
            <>
              <span data-i18n="zh">卖出</span>
              <span data-i18n="en">Sell</span>
            </>
          ) : s === 'exchange' ? (
            <>
              <span data-i18n="zh">换股</span>
              <span data-i18n="en">Exchange</span>
            </>
          ) : (
            tx.type
          )}
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
