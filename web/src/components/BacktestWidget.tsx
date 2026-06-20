'use client';

import {ArrowRight, FlaskConical} from 'lucide-react';
import Link from '@/components/LocalLink';
import {useEffect, useState} from 'react';
import {getBacktest, type SignalBacktestResponse} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

type Tokens = ReturnType<typeof tok>;
type Status = 'loading' | 'ready';

const RULES: {v: string; k: string}[] = [
  {v: 'golden_cross', k: 'backtest.rule.golden'},
  {v: 'death_cross', k: 'backtest.rule.death'},
  {v: 'macd_bullish_cross', k: 'backtest.rule.macdBull'},
  {v: 'macd_bearish_cross', k: 'backtest.rule.macdBear'},
  {v: 'rsi_oversold', k: 'backtest.rule.rsiOversold'},
  {v: 'rsi_overbought', k: 'backtest.rule.rsiOverbought'},
];
const HORIZONS = [10, 20, 60];

function signColor(dark: boolean, v: number): string {
  if (v > 0) return dark ? 'text-emerald-300' : 'text-emerald-600';
  if (v < 0) return dark ? 'text-rose-300' : 'text-rose-600';
  return '';
}

/**
 * BacktestWidget replays a deterministic signal rule over a ticker's daily history and
 * shows how it performed — win rate, average forward return, trade count, and a
 * buy-and-hold baseline. Every number is Go-computed over public price data; it is a
 * disclosed HISTORICAL statistic, never a prediction or advice (the disclaimer says so).
 * Hides itself when there is no usable history.
 */
export function BacktestWidget({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {getToken} = useAuth();

  const [rule, setRule] = useState('golden_cross');
  const [horizon, setHorizon] = useState(20);
  const [data, setData] = useState<SignalBacktestResponse | null>(null);
  const [status, setStatus] = useState<Status>('loading');
  const [hidden, setHidden] = useState(false);

  useEffect(() => {
    const c = new AbortController();
    let cancelled = false;
    setStatus('loading');
    (async () => {
      let token: string | null = null;
      try {
        token = await getToken();
      } catch {
        // anonymous
      }
      try {
        const r = await getBacktest(ticker, rule, horizon, token, c.signal);
        if (cancelled) return;
        if (r === null) {
          // No history for this rule/ticker; keep the card but show an empty note —
          // unless it's the very first load with nothing at all, then hide.
          setData({ticker, result: undefined});
        } else {
          setData(r);
        }
      } catch {
        if (!cancelled) setHidden(true);
      } finally {
        if (!cancelled) setStatus('ready');
      }
    })();
    return () => {
      cancelled = true;
      c.abort();
    };
  }, [ticker, rule, horizon, getToken]);

  if (hidden) return null;

  const locked = data?.paywall_locked ?? false;
  const result = data?.result;

  const selectCx = cx(
    'rounded-lg border px-2.5 py-1.5 text-[12.5px] font-medium',
    t.card,
    t.border,
    t.text,
  );

  return (
    <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <h2 className={cx('flex items-center gap-1.5 text-[14px] font-bold', t.text)}>
          <FlaskConical size={15} className={dark ? 'text-violet-300' : 'text-violet-500'} />
          {tr('backtest.title')}
        </h2>
      </div>

      {!locked && (
        <div className="mb-3 flex flex-wrap items-center gap-2">
          <select
            aria-label={tr('backtest.rule')}
            value={rule}
            onChange={e => setRule(e.target.value)}
            className={selectCx}
          >
            {RULES.map(o => (
              <option key={o.v} value={o.v}>
                {tr(o.k)}
              </option>
            ))}
          </select>
          <select
            aria-label={tr('backtest.horizon')}
            value={horizon}
            onChange={e => setHorizon(Number(e.target.value))}
            className={selectCx}
          >
            {HORIZONS.map(h => (
              <option key={h} value={h}>
                {tr('backtest.days').replace('{n}', String(h))}
              </option>
            ))}
          </select>
        </div>
      )}

      {locked ? (
        <ProLock dark={dark} t={t} tr={tr} />
      ) : status === 'loading' ? (
        <div className={cx('h-24 rounded-xl', t.skel)} />
      ) : !result || result.trades === 0 ? (
        <p className={cx('text-[12.5px]', t.sub)}>{tr('backtest.empty')}</p>
      ) : (
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
          <Stat t={t} label={tr('backtest.winRate')} value={`${Math.round(result.win_rate * 100)}%`} />
          <Stat
            t={t}
            label={tr('backtest.avgReturn').replace('{n}', String(result.horizon))}
            value={`${result.avg_return > 0 ? '+' : ''}${result.avg_return.toFixed(2)}%`}
            valueClass={signColor(dark, result.avg_return)}
          />
          <Stat t={t} label={tr('backtest.trades')} value={String(result.trades)} />
          <Stat
            t={t}
            label={tr('backtest.baseline')}
            value={`${result.baseline > 0 ? '+' : ''}${result.baseline.toFixed(2)}%`}
            valueClass={signColor(dark, result.baseline)}
          />
        </div>
      )}

      <p className={cx('mt-3 text-[11px] leading-snug', t.faint)}>{tr('backtest.disclaimer')}</p>
    </section>
  );
}

function Stat({
  t,
  label,
  value,
  valueClass,
}: {
  t: Tokens;
  label: string;
  value: string;
  valueClass?: string;
}) {
  return (
    <div className={cx('rounded-xl border p-2.5', t.border)}>
      <div className={cx('text-[10.5px] uppercase tracking-wide', t.faint)}>{label}</div>
      <div className={cx('mt-0.5 text-[17px] font-bold tabular-nums', valueClass || t.text)}>{value}</div>
    </div>
  );
}

function ProLock({dark, t, tr}: {dark: boolean; t: Tokens; tr: (k: string) => string}) {
  return (
    <div
      className={cx(
        'rounded-xl border p-3.5',
        dark ? 'border-violet-500/30 bg-violet-500/[0.06]' : 'border-violet-200 bg-violet-50/60',
      )}
    >
      <p className={cx('text-[13px] font-bold', t.text)}>{tr('backtest.locked.title')}</p>
      <p className={cx('mt-1 text-[12px]', t.sub)}>{tr('backtest.locked.body')}</p>
      <Link
        href="/pro"
        className={cx(
          'mt-2.5 inline-flex items-center gap-1 rounded-full px-3.5 py-1.5 text-[12px] font-semibold text-white',
          dark ? 'bg-violet-500 hover:bg-violet-400' : 'bg-violet-600 hover:bg-violet-500',
        )}
      >
        {tr('backtest.locked.cta')}
        <ArrowRight size={13} />
      </Link>
    </div>
  );
}
