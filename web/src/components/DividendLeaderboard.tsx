'use client';

import {Coins} from 'lucide-react';
import {useEffect, useState} from 'react';
import Link from '@/components/LocalLink';
import {getDividendScreen, type DividendRank} from '@/lib/api';
import type {DividendMetric} from '@/lib/dividendViews';

/** Format one dividend metric off a row (null when the metric isn't computable for that name). */
function fmtMetric(metric: DividendMetric, r: DividendRank): string | null {
  switch (metric) {
    case 'yield':
      return r.yield == null ? null : `${r.yield.toFixed(1)}%`;
    case 'payout':
      return r.payout_ratio == null ? null : `${r.payout_ratio.toFixed(1)}%`;
    case 'growth':
      return r.yoy_growth == null ? null : `${r.yoy_growth >= 0 ? '+' : ''}${r.yoy_growth.toFixed(1)}%`;
    case 'coverage':
      return r.fcf_coverage == null ? null : `${r.fcf_coverage.toFixed(1)}×`;
  }
}

function metricLabel(metric: DividendMetric, zh: boolean): string {
  switch (metric) {
    case 'yield':
      return zh ? '股息率' : 'Yield';
    case 'payout':
      return zh ? '派息率' : 'Payout';
    case 'growth':
      return zh ? '同比增长' : 'YoY growth';
    case 'coverage':
      return zh ? '现金流覆盖' : 'FCF cover';
  }
}

/**
 * Client-self-healing dividend leaderboard. The pSEO page SSR-fetches best-effort (crawlable rows +
 * JSON-LD when the tunnel cooperates), but Vercel's server-side fetch through the Cloudflare tunnel is
 * unreliable and the dividend cache is cold after each backend restart — so the SSR fetch can bake an
 * empty page that sticks in the ISR cache. This renders the SSR `initial` rows when present (SEO) +
 * re-fetches from the browser on mount (the reliable path). Deploy-gotcha #7 — never DEPEND on
 * SSR-fetching dynamic, per-deploy-volatile data through the tunnel.
 *
 * Each row leads with the view's ranking metric (`primary`, amber) and shows the dividend yield (or the
 * payout ratio when yield IS the primary) for context. Every figure is a Go-computed DESCRIPTIVE
 * statistic — never a rating/advice.
 */
export function DividendLeaderboard({
  view,
  primary,
  initial,
  initialTotal,
  zh,
}: {
  view: string;
  primary: DividendMetric;
  initial: DividendRank[];
  initialTotal: number;
  zh: boolean;
}) {
  const [results, setResults] = useState<DividendRank[]>(initial);
  const [total, setTotal] = useState(initialTotal);
  const [loading, setLoading] = useState(initial.length === 0);

  useEffect(() => {
    const c = new AbortController();
    getDividendScreen(view, 100, c.signal)
      .then(r => {
        if (c.signal.aborted) return;
        if (r.results && r.results.length > 0) {
          setResults(r.results);
          setTotal(r.total ?? r.results.length);
        }
        setLoading(false);
      })
      .catch(() => setLoading(false));
    return () => c.abort();
  }, [view]);

  // Context column: the canonical yield, unless yield is already the ranked metric (then show payout).
  const secondary: DividendMetric = primary === 'yield' ? 'payout' : 'yield';

  return (
    <>
      {total > 0 && (
        <p className="mb-3 text-[12px] text-slate-500 dark:text-slate-400">
          {zh ? `相对 ${total} 只分红股排名 · 数据源:SEC 申报 + 实时价格` : `Ranked across ${total} payers · from SEC filings + live price`}
        </p>
      )}
      {results.length > 0 ? (
        <div className="tw-fade overflow-hidden rounded-2xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-950">
          <div className="flex items-center gap-2 border-b border-slate-200 px-4 py-2 text-[11px] font-semibold uppercase tracking-wide text-slate-400 dark:border-slate-800 dark:text-slate-500">
            <span className="w-7 text-right tabular-nums">#</span>
            <span className="w-16">{zh ? '代码' : 'Ticker'}</span>
            <span className="flex-1 text-right tabular-nums">{metricLabel(primary, zh)}</span>
            <span className="w-20 text-right tabular-nums">{metricLabel(secondary, zh)}</span>
          </div>
          {results.map((r, i) => (
            <Row key={r.ticker} r={r} rank={i + 1} primary={primary} secondary={secondary} last={i === results.length - 1} />
          ))}
        </div>
      ) : loading ? (
        <div className="h-40 animate-pulse rounded-2xl bg-slate-100 dark:bg-slate-900" />
      ) : (
        <div className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-10 text-center dark:border-slate-800 dark:bg-slate-900">
          <Coins size={22} className="mx-auto mb-2 text-slate-300 dark:text-slate-600" />
          <p className="text-[14px] font-semibold text-slate-700 dark:text-slate-200">
            {zh ? '排行榜正在生成' : 'Leaderboard is warming up'}
          </p>
          <p className="mt-1 text-[12.5px] text-slate-500 dark:text-slate-400">
            {zh ? '分红数据每小时刷新一次,稍后再来查看。' : 'Dividend profiles rebuild hourly — check back shortly.'}
          </p>
          <Link
            href="/screen"
            className="mt-3 inline-block rounded-lg bg-amber-600 px-3 py-1.5 text-[12.5px] font-semibold text-white transition hover:bg-amber-700 dark:bg-amber-500 dark:hover:bg-amber-600"
          >
            {zh ? '打开完整筛选器 →' : 'Open the full screener →'}
          </Link>
        </div>
      )}
    </>
  );
}

/** One ranked row → an internal link into the stock page. The primary metric (amber) is the column the
 * leaderboard is sorted by; the secondary is shown for context. Every figure is a Go-computed DISCLOSED
 * statistic from SEC filings — never a forecast or rating. */
function Row({
  r,
  rank,
  primary,
  secondary,
  last,
}: {
  r: DividendRank;
  rank: number;
  primary: DividendMetric;
  secondary: DividendMetric;
  last: boolean;
}) {
  return (
    <Link
      href={`/stock/${encodeURIComponent(r.ticker)}`}
      className={`flex items-center gap-2 px-4 py-2.5 text-[13.5px] transition hover:bg-slate-50 dark:hover:bg-slate-900 ${
        last ? '' : 'border-b border-slate-200 dark:border-slate-800'
      }`}
    >
      <span className="w-7 text-right font-semibold tabular-nums text-slate-400 dark:text-slate-500">{rank}</span>
      <span className="w-16 font-bold text-slate-900 dark:text-slate-100">{r.ticker}</span>
      <span className="flex-1 text-right font-semibold tabular-nums text-amber-600 dark:text-amber-400">
        {fmtMetric(primary, r) ?? '—'}
      </span>
      <span className="w-20 text-right font-medium tabular-nums text-slate-500 dark:text-slate-400">
        {fmtMetric(secondary, r) ?? '—'}
      </span>
    </Link>
  );
}
