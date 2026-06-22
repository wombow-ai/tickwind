'use client';

import {Layers} from 'lucide-react';
import {useEffect, useState} from 'react';
import Link from '@/components/LocalLink';
import {getFactorScreen, type FactorRank} from '@/lib/api';

/**
 * Client-self-healing factor leaderboard. The pSEO page SSR-fetches best-effort (for crawlable rows +
 * JSON-LD when the tunnel cooperates), but Vercel's server-side fetch through the Cloudflare tunnel is
 * unreliable (a cold serverless function hits a cold-tunnel reset/timeout) AND the ScorecardCache is
 * cold after every backend restart — so the SSR fetch frequently baked an empty "warming up" page that
 * then stuck in the ISR cache. This component renders the SSR `initial` rows immediately (SEO when
 * present) AND re-fetches from the browser on mount (a reliable path), so USERS always see the live
 * leaderboard regardless of what the SSR fetch returned. Deploy-gotcha #7: never DEPEND on SSR-fetching
 * dynamic, per-deploy-volatile data through the tunnel — self-heal client-side.
 */
export function FactorLeaderboard({
  factor,
  initial,
  initialPopulation,
  zh,
}: {
  factor: string;
  initial: FactorRank[];
  initialPopulation: number;
  zh: boolean;
}) {
  const [results, setResults] = useState<FactorRank[]>(initial);
  const [population, setPopulation] = useState(initialPopulation);
  const [loading, setLoading] = useState(initial.length === 0); // only show a loader when SSR gave nothing

  useEffect(() => {
    const c = new AbortController();
    getFactorScreen(factor, 100, c.signal)
      .then(r => {
        if (c.signal.aborted) return;
        if (r.results && r.results.length > 0) {
          setResults(r.results);
          setPopulation(r.population ?? r.results.length);
        }
        setLoading(false);
      })
      .catch(() => setLoading(false));
    return () => c.abort();
  }, [factor]);

  return (
    <>
      {population > 0 && (
        <p className="mb-3 text-[12px] text-slate-500 dark:text-slate-400">
          {zh ? `相对 ${population} 只个股排名` : `Ranked across ${population} stocks`}
        </p>
      )}
      {results.length > 0 ? (
        <div className="tw-fade overflow-hidden rounded-2xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-950">
          <div className="flex items-center gap-3 border-b border-slate-200 px-4 py-2 text-[11px] font-semibold uppercase tracking-wide text-slate-400 dark:border-slate-800 dark:text-slate-500">
            <span className="w-8 text-right tabular-nums">#</span>
            <span className="w-24">{zh ? '代码' : 'Ticker'}</span>
            <span className="flex-1">{zh ? '百分位' : 'Percentile'}</span>
            <span className="w-12 text-right tabular-nums">{zh ? '分位' : 'Pct'}</span>
          </div>
          {results.map((r, i) => (
            <Row key={r.ticker} r={r} rank={i + 1} last={i === results.length - 1} />
          ))}
        </div>
      ) : loading ? (
        <div className="h-40 animate-pulse rounded-2xl bg-slate-100 dark:bg-slate-900" />
      ) : (
        <div className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-10 text-center dark:border-slate-800 dark:bg-slate-900">
          <Layers size={22} className="mx-auto mb-2 text-slate-300 dark:text-slate-600" />
          <p className="text-[14px] font-semibold text-slate-700 dark:text-slate-200">
            {zh ? '排行榜正在生成' : 'Leaderboard is warming up'}
          </p>
          <p className="mt-1 text-[12.5px] text-slate-500 dark:text-slate-400">
            {zh ? '因子分布每小时刷新一次,稍后再来查看。' : 'The factor distribution rebuilds hourly — check back shortly.'}
          </p>
          <Link
            href="/screen"
            className="mt-3 inline-block rounded-lg bg-indigo-600 px-3 py-1.5 text-[12.5px] font-semibold text-white transition hover:bg-indigo-700 dark:bg-indigo-500 dark:hover:bg-indigo-600"
          >
            {zh ? '打开完整筛选器 →' : 'Open the full screener →'}
          </Link>
        </div>
      )}
    </>
  );
}

/** One ranked row → an internal link into the stock page, with a NEUTRAL percentile bar (no green/red
 * good-bad cue — the bar length IS the percentile, never a verdict). */
function Row({r, rank, last}: {r: FactorRank; rank: number; last: boolean}) {
  const pct = Math.max(0, Math.min(100, r.percentile));
  return (
    <Link
      href={`/stock/${encodeURIComponent(r.ticker)}`}
      className={`flex items-center gap-3 px-4 py-2.5 text-[13.5px] transition hover:bg-slate-50 dark:hover:bg-slate-900 ${
        last ? '' : 'border-b border-slate-200 dark:border-slate-800'
      }`}
    >
      <span className="w-8 text-right font-semibold tabular-nums text-slate-400 dark:text-slate-500">{rank}</span>
      <span className="w-24 font-bold text-slate-900 dark:text-slate-100">{r.ticker}</span>
      <span className="flex-1">
        <span className="block h-2 w-full overflow-hidden rounded-full bg-slate-200 dark:bg-slate-800">
          <span className="block h-full rounded-full bg-indigo-500/70 dark:bg-indigo-400/70" style={{width: `${pct}%`}} />
        </span>
      </span>
      <span className="w-12 text-right font-semibold tabular-nums text-slate-900 dark:text-slate-100">{Math.round(pct)}</span>
    </Link>
  );
}
