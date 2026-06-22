'use client';

import {SlidersHorizontal} from 'lucide-react';
import {useEffect, useState} from 'react';
import Link from '@/components/LocalLink';
import {getScreen, type ScreenResult} from '@/lib/api';
import {presetByKey} from '@/lib/presets';

/**
 * Client-self-healing screener list for the /screen/[preset] pSEO pages. The page SSR-fetches
 * best-effort (crawlable rows + JSON-LD when the tunnel cooperates), but Vercel's server-side fetch
 * through the Cloudflare tunnel is unreliable (a cold serverless fn hits a cold-tunnel reset/timeout —
 * a direct browser fetch gets the data, Vercel's SSR doesn't), so it frequently bakes an empty
 * "No matching stocks" page that then sticks in the ISR cache. This re-fetches from the browser on
 * mount so users always see the live movers. A session preset that is GENUINELY empty off-hours still
 * shows the empty state (the client fetch also returns empty). Deploy-gotcha #7 — never DEPEND on
 * SSR-fetching dynamic data through the tunnel.
 */
export function ScreenList({presetKey, initial, zh}: {presetKey: string; initial: ScreenResult[]; zh: boolean}) {
  const [results, setResults] = useState<ScreenResult[]>(initial);
  const [loading, setLoading] = useState(initial.length === 0); // loader only when SSR gave nothing

  useEffect(() => {
    const p = presetByKey(presetKey);
    if (!p) {
      setLoading(false);
      return;
    }
    const c = new AbortController();
    getScreen(p.params, c.signal)
      .then(r => {
        if (c.signal.aborted) return;
        if (r.results && r.results.length > 0) setResults(r.results);
        setLoading(false);
      })
      .catch(() => setLoading(false));
    return () => c.abort();
  }, [presetKey]);

  if (results.length > 0) {
    return (
      <div className="tw-fade overflow-hidden rounded-2xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-950">
        <div className="flex items-center gap-3 border-b border-slate-200 px-4 py-2 text-[11px] font-semibold uppercase tracking-wide text-slate-400 dark:border-slate-800 dark:text-slate-500">
          <span className="w-8 text-right tabular-nums">#</span>
          <span className="w-24">{zh ? '代码' : 'Ticker'}</span>
          <span className="flex-1 text-right tabular-nums">{zh ? '价格' : 'Price'}</span>
          <span className="w-24 text-right tabular-nums">{zh ? '涨跌幅' : 'Change'}</span>
        </div>
        {results.map((r, i) => (
          <Row key={r.ticker} r={r} rank={i + 1} last={i === results.length - 1} />
        ))}
      </div>
    );
  }
  if (loading) {
    return <div className="h-40 animate-pulse rounded-2xl bg-slate-100 dark:bg-slate-900" />;
  }
  return (
    <div className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-10 text-center dark:border-slate-800 dark:bg-slate-900">
      <SlidersHorizontal size={22} className="mx-auto mb-2 text-slate-300 dark:text-slate-600" />
      <p className="text-[14px] font-semibold text-slate-700 dark:text-slate-200">
        {zh ? '暂无符合条件的个股' : 'No matching stocks right now'}
      </p>
      <p className="mt-1 text-[12.5px] text-slate-500 dark:text-slate-400">
        {zh ? '稍后再来查看,或使用完整筛选器自定义条件。' : 'Check back shortly, or use the full screener to set your own filters.'}
      </p>
      <Link
        href="/screen"
        className="mt-3 inline-block rounded-lg bg-sky-600 px-3 py-1.5 text-[12.5px] font-semibold text-white transition hover:bg-sky-700 dark:bg-sky-500 dark:hover:bg-sky-600"
      >
        {zh ? '打开完整筛选器 →' : 'Open the full screener →'}
      </Link>
    </div>
  );
}

/** One ranked result row → an internal link into the stock page. */
function Row({r, rank, last}: {r: ScreenResult; rank: number; last: boolean}) {
  const pos = r.change_pct != null && r.change_pct >= 0;
  const chgColor =
    r.change_pct == null
      ? 'text-slate-400 dark:text-slate-500'
      : pos
        ? 'text-emerald-600 dark:text-emerald-400'
        : 'text-rose-500 dark:text-rose-400';
  return (
    <Link
      href={`/stock/${encodeURIComponent(r.ticker)}`}
      className={`flex items-center gap-3 px-4 py-2.5 text-[13.5px] transition hover:bg-slate-50 dark:hover:bg-slate-900 ${
        last ? '' : 'border-b border-slate-200 dark:border-slate-800'
      }`}
    >
      <span className="w-8 text-right font-semibold tabular-nums text-slate-400 dark:text-slate-500">{rank}</span>
      <span className="w-24 font-bold text-slate-900 dark:text-slate-100">{r.ticker}</span>
      <span className="flex-1 text-right font-semibold tabular-nums text-slate-900 dark:text-slate-100">
        ${r.price.toFixed(2)}
      </span>
      <span className={`w-24 text-right font-semibold tabular-nums ${chgColor}`}>
        {r.change_pct == null ? '—' : `${pos ? '+' : ''}${r.change_pct.toFixed(2)}%`}
      </span>
    </Link>
  );
}
