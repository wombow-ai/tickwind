'use client';

import {ArrowRight, Minus, ScanLine, TrendingDown, TrendingUp} from 'lucide-react';
import type {LucideIcon} from 'lucide-react';
import Link from '@/components/LocalLink';
import {useEffect, useState} from 'react';
import {
  getScreenSignals,
  type IndicatorSignal,
  type ScreenSignalsResponse,
} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

type Tokens = ReturnType<typeof tok>;
type Status = 'loading' | 'ready';
type Direction = IndicatorSignal['direction'];

const DIR_ICON: Record<Direction, LucideIcon> = {
  bullish: TrendingUp,
  bearish: TrendingDown,
  neutral: Minus,
};

function dirChip(dark: boolean, d: Direction): string {
  switch (d) {
    case 'bullish':
      return dark
        ? 'border-emerald-500/30 bg-emerald-500/[0.08] text-emerald-300'
        : 'border-emerald-200 bg-emerald-50 text-emerald-700';
    case 'bearish':
      return dark
        ? 'border-rose-500/30 bg-rose-500/[0.08] text-rose-300'
        : 'border-rose-200 bg-rose-50 text-rose-700';
    default:
      return dark
        ? 'border-slate-600/40 bg-slate-500/[0.08] text-slate-300'
        : 'border-slate-200 bg-slate-50 text-slate-600';
  }
}

// Filter presets. Values map to the API query (direction / signal id); labels are dict keys.
const DIRECTIONS: {v: string; k: string}[] = [
  {v: '', k: 'sigscreen.dir.all'},
  {v: 'bullish', k: 'signals.dir.bullish'},
  {v: 'bearish', k: 'signals.dir.bearish'},
  {v: 'neutral', k: 'signals.dir.neutral'},
];
const SIGNAL_TYPES: {v: string; k: string}[] = [
  {v: '', k: 'sigscreen.sig.all'},
  {v: 'technical.ma-cross', k: 'sigscreen.sig.cross'},
  {v: 'technical.rsi', k: 'sigscreen.sig.rsi'},
  {v: 'technical.macd', k: 'sigscreen.sig.macd'},
  {v: 'technical.stochastic-kdj', k: 'sigscreen.sig.kdj'},
];

/**
 * The deterministic signals SCREENER: scan the universe for stocks whose Go-computed
 * signals match a direction / signal-type filter. Every match shows the signals that
 * triggered it, each with its traceable basis — never advice, never LLM-invented.
 */
export function SignalsScreen() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {getToken} = useAuth();

  const [direction, setDirection] = useState('');
  const [sigType, setSigType] = useState('');
  const [data, setData] = useState<ScreenSignalsResponse | null>(null);
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    let cancelled = false;
    setStatus('loading');
    (async () => {
      let token: string | null = null;
      try {
        token = await getToken();
      } catch {
        // anonymous — fetch without a token
      }
      try {
        const r = await getScreenSignals({direction, signal: sigType, limit: 100}, token, c.signal);
        if (cancelled) return;
        setData(r);
      } catch {
        if (!cancelled) setData({count: 0, results: []});
      } finally {
        if (!cancelled) setStatus('ready');
      }
    })();
    return () => {
      cancelled = true;
      c.abort();
    };
  }, [direction, sigType, getToken]);

  const locked = data?.paywall_locked ?? false;
  const results = data?.results ?? [];

  const selectCx = cx(
    'rounded-lg border px-3 py-1.5 text-[12.5px] font-medium',
    t.card,
    t.border,
    t.text,
  );

  return (
    <div className="mx-auto max-w-3xl">
      <header className="mb-4">
        <h1 className={cx('flex items-center gap-2 text-[22px] font-bold tracking-tight', t.text)}>
          <ScanLine size={20} className={dark ? 'text-violet-300' : 'text-violet-500'} />
          {tr('sigscreen.title')}
        </h1>
        <p className={cx('mt-1 text-[13px]', t.sub)}>{tr('sigscreen.subtitle')}</p>
      </header>

      <div className="mb-4 flex flex-wrap items-center gap-2">
        <select
          aria-label={tr('sigscreen.filter.direction')}
          value={direction}
          onChange={e => setDirection(e.target.value)}
          className={selectCx}
        >
          {DIRECTIONS.map(o => (
            <option key={o.k} value={o.v}>
              {tr(o.k)}
            </option>
          ))}
        </select>
        <select
          aria-label={tr('sigscreen.filter.signal')}
          value={sigType}
          onChange={e => setSigType(e.target.value)}
          className={selectCx}
        >
          {SIGNAL_TYPES.map(o => (
            <option key={o.k} value={o.v}>
              {tr(o.k)}
            </option>
          ))}
        </select>
        {!locked && status === 'ready' && (
          <span className={cx('ml-auto text-[11.5px]', t.faint)}>
            {tr('sigscreen.count').replace('{n}', String(results.length))}
          </span>
        )}
      </div>

      {locked ? (
        <ProLock dark={dark} t={t} tr={tr} />
      ) : status === 'loading' ? (
        <div className={cx('h-72 rounded-2xl', t.skel)} />
      ) : results.length === 0 ? (
        <p
          className={cx(
            'rounded-2xl border p-8 text-center text-[13px]',
            t.card,
            t.border,
            t.soft,
            t.sub,
          )}
        >
          {tr('sigscreen.empty')}
        </p>
      ) : (
        <ul className="space-y-2">
          {results.map(m => (
            <li
              key={m.ticker}
              className={cx('rounded-xl border p-3', t.card, t.border, t.soft)}
            >
              <div className="flex items-center justify-between gap-2">
                <Link
                  href={`/stock/${m.ticker}`}
                  className={cx('text-[15px] font-bold tracking-tight hover:underline', t.text)}
                >
                  {m.ticker}
                </Link>
              </div>
              <ul className="mt-2 space-y-1.5">
                {m.signals.map((s, i) => {
                  const Icon = DIR_ICON[s.direction] ?? Minus;
                  return (
                    <li key={`${s.id}-${i}`} className="flex items-start gap-2">
                      <span
                        className={cx(
                          'inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-[11px] font-semibold',
                          dirChip(dark, s.direction),
                        )}
                      >
                        <Icon size={12} />
                        {s.label}
                      </span>
                      <span className={cx('mt-0.5 text-[11.5px] tabular-nums', t.faint)}>{s.basis}</span>
                    </li>
                  );
                })}
              </ul>
            </li>
          ))}
        </ul>
      )}

      <p className={cx('mt-4 text-[11px] leading-snug', t.faint)}>{tr('sigscreen.trust')}</p>
    </div>
  );
}

/** Whole-screen Pro lock (the screener is Pro-only when the paywall is live). */
function ProLock({dark, t, tr}: {dark: boolean; t: Tokens; tr: (k: string) => string}) {
  return (
    <section
      className={cx(
        'rounded-2xl border p-6 text-center',
        dark ? 'border-violet-500/30 bg-violet-500/[0.06]' : 'border-violet-200 bg-violet-50/60',
      )}
    >
      <h2 className={cx('text-[16px] font-bold', t.text)}>{tr('sigscreen.locked.title')}</h2>
      <p className={cx('mx-auto mt-1.5 max-w-md text-[12.5px]', t.sub)}>{tr('sigscreen.locked.body')}</p>
      <Link
        href="/pro"
        className={cx(
          'mt-3 inline-flex items-center gap-1 rounded-full px-4 py-1.5 text-[12.5px] font-semibold text-white',
          dark ? 'bg-violet-500 hover:bg-violet-400' : 'bg-violet-600 hover:bg-violet-500',
        )}
      >
        {tr('sigscreen.locked.cta')}
        <ArrowRight size={13} />
      </Link>
    </section>
  );
}
