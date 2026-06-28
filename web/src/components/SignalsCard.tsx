'use client';

import {Activity, ArrowRight, Minus, TrendingDown, TrendingUp} from 'lucide-react';
import type {LucideIcon} from 'lucide-react';
import Link from '@/components/LocalLink';
import {useEffect, useState} from 'react';
import {
  getIndicatorSignals,
  type IndicatorSignal,
  type IndicatorSignalsResponse,
  trackEvent,
} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

type Tokens = ReturnType<typeof tok>;
type Status = 'loading' | 'ready' | 'hidden';
type Direction = IndicatorSignal['direction'];

/**
 * Per-direction presentation: icon + colour classes (dark / light). Bullish is
 * green, bearish is rose, neutral is slate — a disclosed posture, never a buy/sell
 * instruction (the copy and the trust line keep that boundary explicit).
 */
const DIR_META: Record<Direction, {icon: LucideIcon; dark: string; light: string}> = {
  bullish: {
    icon: TrendingUp,
    dark: 'border-emerald-500/30 bg-emerald-500/[0.08] text-emerald-300',
    light: 'border-emerald-200 bg-emerald-50 text-emerald-700',
  },
  bearish: {
    icon: TrendingDown,
    dark: 'border-rose-500/30 bg-rose-500/[0.08] text-rose-300',
    light: 'border-rose-200 bg-rose-50 text-rose-700',
  },
  neutral: {
    icon: Minus,
    dark: 'border-slate-600/40 bg-slate-500/[0.08] text-slate-300',
    light: 'border-slate-200 bg-slate-50 text-slate-600',
  },
};

/**
 * SignalsCard renders the deterministic posture signals for a ticker (the paid
 * "signals" layer). Every row is a Go-computed rule with a visible `basis`
 * (indicator + value + threshold) — anti-hallucination-safe, no advice/targets.
 * When the signals paywall is live and the viewer is not Pro the list is a teaser
 * and we surface an honest "unlock N more" upsell. Hides itself entirely when the
 * ticker has no computable signals (404).
 */
export function SignalsCard({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {getToken} = useAuth();

  const [data, setData] = useState<IndicatorSignalsResponse | null>(null);
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
        // anonymous viewer — fetch without a token (free tier)
      }
      try {
        const r = await getIndicatorSignals(ticker, token, c.signal);
        if (cancelled) return;
        if (!r) {
          setStatus('hidden');
          return;
        }
        setData(r);
        setStatus('ready');
      } catch {
        if (!cancelled) setStatus('hidden');
      }
    })();
    return () => {
      cancelled = true;
      c.abort();
    };
  }, [ticker, getToken]);

  // Funnel: a free viewer hit the indicator-signals Pro wall.
  useEffect(() => {
    if (data?.paywall_locked) void (async () => trackEvent('paywall_view', 'indicators', await getToken()))();
  }, [data?.paywall_locked, getToken]);

  if (status === 'hidden') return null;

  if (status === 'loading' || !data) {
    return (
      <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
        <div className={cx('mb-3 h-4 w-20 rounded', t.skel)} />
        <div className="space-y-2">
          {[0, 1].map(i => (
            <div key={i} className={cx('h-12 rounded-lg', t.skel)} />
          ))}
        </div>
      </section>
    );
  }

  const {signals, as_of: asOf, paywall_locked: locked, total_signals: total} = data;
  const more = Math.max(0, total - signals.length);

  return (
    <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <h2 className={cx('flex items-center gap-1.5 text-[14px] font-bold', t.text)}>
          <Activity size={15} className={dark ? 'text-violet-300' : 'text-violet-500'} />
          {tr('signals.title')}
        </h2>
        {asOf && (
          <span className={cx('ml-auto text-[10.5px]', t.faint)}>
            {tr('signals.asOf').replace('{d}', asOf)}
          </span>
        )}
      </div>

      {signals.length === 0 ? (
        <p className={cx('text-[12.5px]', t.sub)}>{tr('signals.empty')}</p>
      ) : (
        <ul className="space-y-2">
          {signals.map((sig, i) => {
            const meta = DIR_META[sig.direction] ?? DIR_META.neutral;
            const Icon = meta.icon;
            return (
              <li
                key={`${sig.id}-${i}`}
                className={cx(
                  'flex items-start gap-2.5 rounded-lg border p-2.5',
                  dark ? meta.dark : meta.light,
                )}
              >
                <Icon size={15} className="mt-0.5 shrink-0" />
                <div className="min-w-0 flex-1">
                  <p className="text-[13px] font-semibold">{sig.label}</p>
                  <p className={cx('mt-0.5 text-[11.5px] tabular-nums', t.faint)}>{sig.basis}</p>
                </div>
                <span className="shrink-0 text-[10.5px] font-semibold uppercase tracking-wide opacity-80">
                  {tr(`signals.dir.${sig.direction}`)}
                </span>
              </li>
            );
          })}
        </ul>
      )}

      {signals.length > 0 && (
        <p className={cx('mt-3 text-[11px] leading-snug', t.faint)}>{tr('signals.trust')}</p>
      )}

      {locked && (
        <div
          className={cx(
            'mt-3 rounded-xl border p-3.5',
            dark ? 'border-violet-500/30 bg-violet-500/[0.06]' : 'border-violet-200 bg-violet-50/60',
          )}
        >
          <p className={cx('text-[13px] font-bold', t.text)}>{tr('signals.locked.title')}</p>
          <p className={cx('mt-1 text-[12px]', t.sub)}>{tr('signals.locked.body')}</p>
          <Link
            href="/pro"
            className={cx(
              'mt-2.5 inline-flex items-center gap-1 rounded-full px-3.5 py-1.5 text-[12px] font-semibold text-white',
              dark ? 'bg-violet-500 hover:bg-violet-400' : 'bg-violet-600 hover:bg-violet-500',
            )}
          >
            {more > 0
              ? tr('signals.locked.cta').replace('{n}', String(more))
              : tr('deep.paywall.cta')}
            <ArrowRight size={13} />
          </Link>
        </div>
      )}
    </section>
  );
}
