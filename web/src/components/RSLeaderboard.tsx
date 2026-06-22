'use client';

import {TrendingUp} from 'lucide-react';
import {useEffect, useState} from 'react';
import Link from '@/components/LocalLink';
import {getRSScreen, type RSRank} from '@/lib/api';

/**
 * Client-self-healing relative-strength leaderboard. The pSEO page SSR-fetches best-effort (crawlable
 * rows + JSON-LD when the tunnel cooperates), but Vercel's server-side fetch through the Cloudflare
 * tunnel is unreliable and the RS cache is cold after each backend restart — so the SSR fetch can bake
 * an empty page that sticks in the ISR cache. This renders the SSR `initial` rows when present (SEO) +
 * re-fetches from the browser on mount (the reliable path), so users always see the live ranking.
 * Deploy-gotcha #7 — never DEPEND on SSR-fetching dynamic, per-deploy-volatile data through the tunnel.
 */
export function RSLeaderboard({
  window: win,
  initial,
  initialTotal,
  zh,
}: {
  window: string;
  initial: RSRank[];
  initialTotal: number;
  zh: boolean;
}) {
  const [results, setResults] = useState<RSRank[]>(initial);
  const [total, setTotal] = useState(initialTotal);
  const [loading, setLoading] = useState(initial.length === 0);

  useEffect(() => {
    const c = new AbortController();
    getRSScreen(win, 100, c.signal)
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
  }, [win]);

  return (
    <>
      {total > 0 && (
        <p className="mb-3 text-[12px] text-slate-500 dark:text-slate-400">
          {zh ? `相对 ${total} 只个股排名 · 基准:标普 500 (SPY)` : `Ranked across ${total} stocks · benchmark: S&P 500 (SPY)`}
        </p>
      )}
      {results.length > 0 ? (
        <div className="tw-fade overflow-hidden rounded-2xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-950">
          <div className="flex items-center gap-3 border-b border-slate-200 px-4 py-2 text-[11px] font-semibold uppercase tracking-wide text-slate-400 dark:border-slate-800 dark:text-slate-500">
            <span className="w-8 text-right tabular-nums">#</span>
            <span className="w-24">{zh ? '代码' : 'Ticker'}</span>
            <span className="flex-1 text-right tabular-nums">{zh ? '超额收益' : 'Excess vs SPY'}</span>
            <span className="w-24 text-right tabular-nums">{zh ? '个股收益' : 'Stock'}</span>
          </div>
          {results.map((r, i) => (
            <Row key={r.ticker} r={r} rank={i + 1} last={i === results.length - 1} />
          ))}
        </div>
      ) : loading ? (
        <div className="h-40 animate-pulse rounded-2xl bg-slate-100 dark:bg-slate-900" />
      ) : (
        <div className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-10 text-center dark:border-slate-800 dark:bg-slate-900">
          <TrendingUp size={22} className="mx-auto mb-2 text-slate-300 dark:text-slate-600" />
          <p className="text-[14px] font-semibold text-slate-700 dark:text-slate-200">
            {zh ? '排行榜正在生成' : 'Leaderboard is warming up'}
          </p>
          <p className="mt-1 text-[12.5px] text-slate-500 dark:text-slate-400">
            {zh ? '相对强弱每 45 分钟刷新一次,稍后再来查看。' : 'Relative strength rebuilds every 45 minutes — check back shortly.'}
          </p>
          <Link
            href="/screen"
            className="mt-3 inline-block rounded-lg bg-teal-600 px-3 py-1.5 text-[12.5px] font-semibold text-white transition hover:bg-teal-700 dark:bg-teal-500 dark:hover:bg-teal-600"
          >
            {zh ? '打开完整筛选器 →' : 'Open the full screener →'}
          </Link>
        </div>
      )}
    </>
  );
}

/** One ranked row → an internal link into the stock page. The EXCESS return (the ranking metric) is
 * colored by direction (green = outperformed SPY, red = underperformed) — a factual historical change,
 * not a rating; the stock's own return is shown faint for context. */
function Row({r, rank, last}: {r: RSRank; rank: number; last: boolean}) {
  const exPos = r.relative >= 0;
  const exColor = exPos ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-500 dark:text-rose-400';
  return (
    <Link
      href={`/stock/${encodeURIComponent(r.ticker)}`}
      className={`flex items-center gap-3 px-4 py-2.5 text-[13.5px] transition hover:bg-slate-50 dark:hover:bg-slate-900 ${
        last ? '' : 'border-b border-slate-200 dark:border-slate-800'
      }`}
    >
      <span className="w-8 text-right font-semibold tabular-nums text-slate-400 dark:text-slate-500">{rank}</span>
      <span className="w-24 font-bold text-slate-900 dark:text-slate-100">{r.ticker}</span>
      <span className={`flex-1 text-right font-semibold tabular-nums ${exColor}`}>
        {exPos ? '+' : ''}
        {r.relative.toFixed(1)}pp
      </span>
      <span className="w-24 text-right font-medium tabular-nums text-slate-500 dark:text-slate-400">
        {r.stock_return >= 0 ? '+' : ''}
        {r.stock_return.toFixed(1)}%
      </span>
    </Link>
  );
}
