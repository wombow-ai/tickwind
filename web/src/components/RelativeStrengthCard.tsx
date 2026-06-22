'use client';

import {Gauge} from 'lucide-react';
import {useEffect, useState} from 'react';
import {getRelativeStrength, type RelativeStrength, type RelStrengthWindow} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

type Tokens = ReturnType<typeof tok>;
type Status = 'loading' | 'ready' | 'hidden';

const pct = (v: number) => `${v > 0 ? '+' : ''}${v.toFixed(1)}%`;
const pp = (v: number) => `${v > 0 ? '+' : ''}${v.toFixed(1)}`;

/**
 * RelativeStrengthCard shows a ticker's trailing performance vs the S&P 500 (SPY) over 1M/3M/6M/1Y
 * — the EXCESS return (stock minus SPY, in percentage points) headlined per window, with the raw
 * stock/SPY returns underneath for context. Every number is Go-computed
 * (GET /v1/stocks/{t}/relative-strength); it is a DISCLOSED HISTORICAL STATISTIC, never a forecast
 * or advice (the disclaimer says so). Hides itself when there is too little history.
 */
export function RelativeStrengthCard({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [data, setData] = useState<RelativeStrength | null>(null);
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    getRelativeStrength(ticker, c.signal).then(rs => {
      if (c.signal.aborted) return;
      if (rs && rs.windows.length > 0) {
        setData(rs);
        setStatus('ready');
      } else {
        setStatus('hidden');
      }
    });
    return () => c.abort();
  }, [ticker]);

  if (status === 'hidden') return null;

  return (
    <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-1 flex flex-wrap items-center gap-2">
        <h2 className={cx('flex items-center gap-1.5 text-[14px] font-bold', t.text)}>
          <Gauge size={15} className={dark ? 'text-sky-300' : 'text-sky-500'} />
          {tr('relstr.title')}
        </h2>
        {data && (
          <span className={cx('text-[10.5px]', t.faint)}>
            {tr('relstr.asof').replace('{d}', data.as_of)}
          </span>
        )}
      </div>
      <p className={cx('mb-3 text-[11.5px]', t.sub)}>{tr('relstr.sub')}</p>

      {status === 'loading' || !data ? (
        <div className={cx('h-28 rounded-xl', t.skel)} />
      ) : (
        <WindowGrid windows={data.windows} benchmark={data.benchmark} dark={dark} t={t} tr={tr} />
      )}

      <p className={cx('mt-3 text-[11px] leading-snug', t.faint)}>{tr('relstr.disclaimer')}</p>
    </section>
  );
}

function WindowGrid({
  windows,
  benchmark,
  dark,
  t,
  tr,
}: {
  windows: RelStrengthWindow[];
  benchmark: string;
  dark: boolean;
  t: Tokens;
  tr: (k: string) => string;
}) {
  const up = dark ? 'text-emerald-300' : 'text-emerald-600';
  const down = dark ? 'text-rose-300' : 'text-rose-500';
  return (
    <div className={cx('grid gap-2', windows.length >= 4 ? 'grid-cols-4' : 'grid-cols-2 sm:grid-cols-4')}>
      {windows.map(w => {
        const outperf = w.relative >= 0;
        return (
          <div
            key={w.label}
            className={cx('rounded-xl border p-2.5 text-center', t.border, dark ? 'bg-slate-900/40' : 'bg-slate-50')}
          >
            <div className={cx('text-[10.5px] font-semibold uppercase tracking-wide', t.faint)}>{w.label}</div>
            {/* headline: excess return vs the benchmark (percentage points) */}
            <div className={cx('mt-0.5 text-[18px] font-bold tabular-nums leading-tight', outperf ? up : down)}>
              {pp(w.relative)}
            </div>
            <div className={cx('text-[9.5px]', t.faint)}>{tr('relstr.vs').replace('{b}', benchmark)}</div>
            {/* context: the raw stock and benchmark returns */}
            <div className={cx('mt-1.5 space-y-0.5 text-[10.5px] tabular-nums', t.sub)}>
              <div className="flex items-center justify-between">
                <span className={t.faint}>{tr('relstr.stock')}</span>
                <span>{pct(w.stock_return)}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className={t.faint}>{benchmark}</span>
                <span>{pct(w.benchmark_return)}</span>
              </div>
            </div>
          </div>
        );
      })}
    </div>
  );
}
