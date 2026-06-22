'use client';

import {CalendarClock} from 'lucide-react';
import {useEffect, useState} from 'react';
import {getEarningsReaction, type EarningsEvent, type EarningsReaction} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

type Tokens = ReturnType<typeof tok>;
type Status = 'loading' | 'ready' | 'hidden';

const signedPct = (v: number) => `${v > 0 ? '+' : ''}${v.toFixed(1)}%`;
const TRACK = 34; // px half-height for the tallest bar on each side of the zero line

/**
 * EarningsReactionCard shows how a stock has historically MOVED around its past earnings
 * announcements — headline aggregates (typical magnitude, % positive, average) plus a mini
 * timeline of each report's reaction (up/down bar around a zero line). Every number is Go-computed
 * (GET /v1/stocks/{t}/earnings-reaction); it is a DISCLOSED HISTORICAL STATISTIC, never a forecast
 * or advice (the disclaimer says so). Hides itself when there is too little earnings history.
 */
export function EarningsReactionCard({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [data, setData] = useState<EarningsReaction | null>(null);
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    getEarningsReaction(ticker, c.signal).then(er => {
      if (c.signal.aborted) return;
      if (er && er.events.length > 0) {
        setData(er);
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
          <CalendarClock size={15} className={dark ? 'text-violet-300' : 'text-violet-500'} />
          {tr('er.title')}
        </h2>
        {data && (
          <span className={cx('text-[10.5px]', t.faint)}>
            {tr('er.reports').replace('{n}', String(data.samples))}
          </span>
        )}
      </div>
      <p className={cx('mb-3 text-[11.5px]', t.sub)}>{tr('er.sub')}</p>

      {status === 'loading' || !data ? (
        <div className={cx('h-28 rounded-xl', t.skel)} />
      ) : (
        <>
          <Summary data={data} dark={dark} t={t} tr={tr} />
          <ReactionBars events={data.events} dark={dark} t={t} tr={tr} />
        </>
      )}

      <p className={cx('mt-3 text-[11px] leading-snug', t.faint)}>{tr('er.disclaimer')}</p>
    </section>
  );
}

function Summary({data, dark, t, tr}: {data: EarningsReaction; dark: boolean; t: Tokens; tr: (k: string) => string}) {
  const up = dark ? 'text-emerald-300' : 'text-emerald-600';
  const down = dark ? 'text-rose-300' : 'text-rose-500';
  const tiles: {label: string; value: string; tone?: string}[] = [
    {label: tr('er.typical'), value: `±${data.avg_abs_move.toFixed(1)}%`},
    {label: tr('er.positive'), value: `${Math.round(data.up_rate * 100)}%`},
    {label: tr('er.avg'), value: signedPct(data.avg_move), tone: data.avg_move >= 0 ? up : down},
  ];
  return (
    <div className="mb-3 grid grid-cols-3 gap-2">
      {tiles.map(tile => (
        <div
          key={tile.label}
          className={cx('rounded-xl border p-2.5 text-center', t.border, dark ? 'bg-slate-900/40' : 'bg-slate-50')}
        >
          <div className={cx('text-[10.5px] font-semibold uppercase tracking-wide', t.faint)}>{tile.label}</div>
          <div className={cx('mt-0.5 text-[18px] font-bold tabular-nums leading-tight', tile.tone ?? t.text)}>
            {tile.value}
          </div>
        </div>
      ))}
    </div>
  );
}

function ReactionBars({events, dark, t, tr}: {events: EarningsEvent[]; dark: boolean; t: Tokens; tr: (k: string) => string}) {
  // Oldest → newest (left → right), so the most recent report is at the right edge.
  const ordered = [...events].reverse();
  const maxAbs = Math.max(0.01, ...ordered.map(e => Math.abs(e.move)));
  const up = dark ? 'bg-emerald-400' : 'bg-emerald-500';
  const down = dark ? 'bg-rose-400' : 'bg-rose-500';

  return (
    <div className="flex items-stretch gap-1">
      {ordered.map(e => {
        const pos = e.move >= 0;
        const barH = Math.round((Math.abs(e.move) / maxAbs) * TRACK);
        const title = tr('er.tip').replace('{date}', e.date).replace('{move}', signedPct(e.move));
        return (
          <div key={e.date} className="flex flex-1 flex-col items-center" title={title}>
            <div className="flex w-full items-end justify-center" style={{height: TRACK}}>
              {pos && <div className={cx('w-2 rounded-t', up)} style={{height: barH}} />}
            </div>
            <div className={cx('h-px w-full', dark ? 'bg-slate-700' : 'bg-slate-200')} />
            <div className="flex w-full items-start justify-center" style={{height: TRACK}}>
              {!pos && <div className={cx('w-2 rounded-b', down)} style={{height: barH}} />}
            </div>
          </div>
        );
      })}
    </div>
  );
}
