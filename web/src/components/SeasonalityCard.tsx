'use client';

import {CalendarRange} from 'lucide-react';
import {useEffect, useState} from 'react';
import {getSeasonality, type Seasonality, type SeasonStat} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

type Tokens = ReturnType<typeof tok>;
type Status = 'loading' | 'ready' | 'hidden';

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
const TRACK = 60; // px half-height for the tallest bar each side of the zero line

/**
 * SeasonalityCard shows a ticker's month-of-year return seasonality — the historical average
 * monthly return for each calendar month (Jan…Dec) as an up/down bar around a zero line, over
 * the available years. Every number is Go-computed (GET /v1/stocks/{t}/seasonality); it is a
 * DISCLOSED HISTORICAL STATISTIC, never a forecast or advice (the disclaimer says so). Hides
 * itself when the ticker has too little history.
 */
export function SeasonalityCard({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [data, setData] = useState<Seasonality | null>(null);
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    getSeasonality(ticker, c.signal).then(s => {
      if (c.signal.aborted) return;
      if (s && s.months.length > 0) {
        setData(s);
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
          <CalendarRange size={15} className={dark ? 'text-amber-300' : 'text-amber-500'} />
          {tr('season.title')}
        </h2>
        {data && (
          <span className={cx('text-[10.5px]', t.faint)}>
            {tr('season.range').replace('{a}', String(data.from_year)).replace('{b}', String(data.to_year))}
          </span>
        )}
      </div>
      <p className={cx('mb-3 text-[11.5px]', t.sub)}>{tr('season.sub')}</p>

      {status === 'loading' || !data ? (
        <div className={cx('h-40 rounded-xl', t.skel)} />
      ) : (
        <MonthBars months={data.months} dark={dark} t={t} tr={tr} />
      )}

      <p className={cx('mt-3 text-[11px] leading-snug', t.faint)}>{tr('season.disclaimer')}</p>
    </section>
  );
}

function MonthBars({months, dark, t, tr}: {months: SeasonStat[]; dark: boolean; t: Tokens; tr: (k: string) => string}) {
  const byMonth = new Map(months.map(m => [m.month, m]));
  const maxAbs = Math.max(0.01, ...months.map(m => Math.abs(m.avg_return)));
  const up = dark ? 'bg-emerald-400' : 'bg-emerald-500';
  const down = dark ? 'bg-rose-400' : 'bg-rose-500';

  return (
    <div className="flex items-stretch gap-1">
      {Array.from({length: 12}, (_, i) => i + 1).map(m => {
        const s = byMonth.get(m);
        const avg = s?.avg_return ?? 0;
        const barH = s ? Math.round((Math.abs(avg) / maxAbs) * TRACK) : 0;
        const pos = avg >= 0;
        const title = s
          ? `${MONTHS[m - 1]}: ${tr('season.tip')
              .replace('{avg}', `${avg > 0 ? '+' : ''}${avg.toFixed(2)}%`)
              .replace('{win}', `${Math.round(s.win_rate * 100)}%`)
              .replace('{n}', String(s.years))}`
          : MONTHS[m - 1];
        return (
          <div key={m} className="flex flex-1 flex-col items-center" title={title}>
            {/* positive half (grows up from the zero line) */}
            <div className="flex w-full items-end justify-center" style={{height: TRACK}}>
              {s && pos && <div className={cx('w-2.5 rounded-t', up)} style={{height: barH}} />}
            </div>
            <div className={cx('h-px w-full', dark ? 'bg-slate-700' : 'bg-slate-200')} />
            {/* negative half (grows down from the zero line) */}
            <div className="flex w-full items-start justify-center" style={{height: TRACK}}>
              {s && !pos && <div className={cx('w-2.5 rounded-b', down)} style={{height: barH}} />}
            </div>
            <span className={cx('mt-1 text-[9.5px] tabular-nums', t.faint)}>{MONTHS[m - 1][0]}</span>
          </div>
        );
      })}
    </div>
  );
}
