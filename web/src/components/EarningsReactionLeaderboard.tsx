'use client';

import {Activity} from 'lucide-react';
import {useEffect, useState} from 'react';
import Link from '@/components/LocalLink';
import {getEarningsReactionScreen, type ReactionRank} from '@/lib/api';

/**
 * Client-self-healing earnings-reaction leaderboard. The pSEO page SSR-fetches best-effort (crawlable
 * rows + JSON-LD when the tunnel cooperates), but Vercel's server-side fetch through the Cloudflare
 * tunnel is unreliable and the reaction cache is cold after each backend restart — so the SSR fetch can
 * bake an empty page that sticks in the ISR cache. This renders the SSR `initial` rows when present
 * (SEO) + re-fetches from the browser on mount (the reliable path), so users always see the live
 * ranking. Deploy-gotcha #7 — never DEPEND on SSR-fetching dynamic, per-deploy-volatile data.
 */
export function EarningsReactionLeaderboard({
  view,
  initial,
  initialTotal,
  zh,
}: {
  view: string;
  initial: ReactionRank[];
  initialTotal: number;
  zh: boolean;
}) {
  const [results, setResults] = useState<ReactionRank[]>(initial);
  const [total, setTotal] = useState(initialTotal);
  const [loading, setLoading] = useState(initial.length === 0);

  useEffect(() => {
    const c = new AbortController();
    getEarningsReactionScreen(view, 100, c.signal)
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

  return (
    <>
      {total > 0 && (
        <p className="mb-3 text-[12px] text-slate-500 dark:text-slate-400">
          {zh
            ? `相对 ${total} 只个股排名 · 财报前后约 2 个交易日的价格变动`
            : `Ranked across ${total} stocks · the ~2-session price move around each earnings`}
        </p>
      )}
      {results.length > 0 ? (
        <div className="tw-fade overflow-hidden rounded-2xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-950">
          <div className="flex items-center gap-2 border-b border-slate-200 px-4 py-2 text-[11px] font-semibold uppercase tracking-wide text-slate-400 dark:border-slate-800 dark:text-slate-500">
            <span className="w-7 text-right tabular-nums">#</span>
            <span className="w-16">{zh ? '代码' : 'Ticker'}</span>
            <span className="flex-1 text-right tabular-nums">{zh ? '典型波动' : 'Typical move'}</span>
            <span className="w-16 text-right tabular-nums">{zh ? '上涨率' : 'Up-rate'}</span>
            <span className="w-9 text-right tabular-nums" title={zh ? '样本数(历次财报)' : 'sample count (past earnings)'}>
              n
            </span>
          </div>
          {results.map((r, i) => (
            <Row key={r.ticker} r={r} rank={i + 1} last={i === results.length - 1} />
          ))}
        </div>
      ) : loading ? (
        <div className="h-40 animate-pulse rounded-2xl bg-slate-100 dark:bg-slate-900" />
      ) : (
        <div className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-10 text-center dark:border-slate-800 dark:bg-slate-900">
          <Activity size={22} className="mx-auto mb-2 text-slate-300 dark:text-slate-600" />
          <p className="text-[14px] font-semibold text-slate-700 dark:text-slate-200">
            {zh ? '排行榜正在生成' : 'Leaderboard is warming up'}
          </p>
          <p className="mt-1 text-[12.5px] text-slate-500 dark:text-slate-400">
            {zh ? '财报反应每 12 小时刷新一次,稍后再来查看。' : 'Earnings reactions rebuild every 12 hours — check back shortly.'}
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

/** One ranked row → an internal link into the stock page. Every figure is a Go-computed DISCLOSED
 * HISTORICAL statistic: the typical move is a non-directional MAGNITUDE (how big, not up/down), the
 * up-rate is how often it rose, and n is the sample count (the basis) — never a forecast or rating. */
function Row({r, rank, last}: {r: ReactionRank; rank: number; last: boolean}) {
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
        ±{r.avg_abs_move.toFixed(1)}%
      </span>
      <span className="w-16 text-right font-medium tabular-nums text-slate-600 dark:text-slate-300">
        {Math.round(r.up_rate * 100)}%
      </span>
      <span className="w-9 text-right tabular-nums text-slate-400 dark:text-slate-500">{r.samples}</span>
    </Link>
  );
}
