'use client';

import {Percent} from 'lucide-react';
import {useEffect, useState} from 'react';
import {getRateCut, type RateCutMarket} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, timeAgo, tok} from '@/lib/ui';

type Tokens = ReturnType<typeof tok>;

/**
 * Rate-cut odds module on the home macro context (grouped with the Treasury
 * {@link MacroStrip} so the macro/rates signals sit together): what prediction
 * markets (Kalshi / Polymarket) price for the Fed's next move, broken out by cut
 * size as labeled probability bars per source. Self-hides while loading errors
 * out; shows an empty state when the feed is live but has no markets yet.
 * Prediction-market odds — explicitly not investment advice.
 */
export function RateCutOdds() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [markets, setMarkets] = useState<RateCutMarket[]>([]);
  const [updatedAt, setUpdatedAt] = useState<string>('');
  const [status, setStatus] = useState<'loading' | 'ready' | 'hidden'>('loading');

  useEffect(() => {
    const c = new AbortController();
    getRateCut(c.signal).then(
      r => {
        setMarkets(r.markets ?? []);
        setUpdatedAt(r.updated_at ?? '');
        setStatus('ready');
      },
      // Endpoint not deployed / network error → hide the whole section (the
      // contract guarantees 200 + [] when the source is merely empty).
      () => setStatus('hidden'),
    );
    return () => c.abort();
  }, []);

  if (status === 'hidden') return null;

  return (
    <section className={cx('mb-5 rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-3 flex items-center gap-2">
        <Percent size={18} className={dark ? 'text-emerald-300' : 'text-emerald-600'} />
        <h2 className={cx('text-[16px] font-bold tracking-tight', t.text)}>
          {tr('ratecut.title')}
        </h2>
        {updatedAt && (
          <span className={cx('ml-auto text-[10.5px]', t.faint)}>
            {tr('ratecut.updated').replace('{t}', timeAgo(updatedAt))}
          </span>
        )}
      </div>
      <p className={cx('mb-3 text-[12.5px]', t.sub)}>{tr('ratecut.blurb')}</p>

      {status === 'loading' && (
        <div className="space-y-2" aria-hidden>
          {[0, 1, 2].map(i => (
            <div key={i} className={cx('h-7 rounded-lg', t.skel)} style={{width: `${90 - i * 12}%`}} />
          ))}
        </div>
      )}

      {status === 'ready' && markets.length === 0 && (
        <p className={cx('py-4 text-center text-[12.5px]', t.faint)}>
          {tr('ratecut.empty')} · {tr('ratecut.emptySub')}
        </p>
      )}

      {status === 'ready' && markets.length > 0 && (
        <div className="space-y-4">
          {markets.map((m, i) => (
            <MarketBlock key={`${m.source}:${i}`} m={m} dark={dark} t={t} />
          ))}
        </div>
      )}

      <p className={cx('mt-3 text-[10.5px]', t.faint)}>{tr('ratecut.footer')}</p>
    </section>
  );
}

function MarketBlock({m, dark, t}: {m: RateCutMarket; dark: boolean; t: Tokens}) {
  const outcomes = [...(m.outcomes ?? [])].sort((a, b) => b.probability - a.probability);
  return (
    <div>
      <div className="mb-1.5 flex items-baseline gap-2">
        <span
          className={cx(
            'shrink-0 rounded-md px-1.5 py-0.5 text-[10.5px] font-bold uppercase tracking-wide',
            dark ? 'bg-slate-800 text-slate-300' : 'bg-slate-100 text-slate-600',
          )}
        >
          {m.source}
        </span>
        <span className={cx('min-w-0 truncate text-[12.5px] font-medium', t.sub)} title={m.question}>
          {m.question}
        </span>
      </div>
      <div className="space-y-1.5">
        {outcomes.map((o, i) => {
          const pct = Math.max(0, Math.min(100, o.probability * 100));
          const lead = i === 0; // the highest-probability outcome
          const fill = lead
            ? dark
              ? 'bg-emerald-400/70'
              : 'bg-emerald-400'
            : dark
              ? 'bg-sky-400/50'
              : 'bg-sky-300';
          return (
            <div key={o.label} className="flex items-center gap-2.5">
              <span className={cx('w-16 shrink-0 text-[12px] font-semibold tabular-nums', t.text)}>
                {o.label}
              </span>
              <div className={cx('relative h-5 flex-1 overflow-hidden rounded-md', dark ? 'bg-slate-800' : 'bg-slate-100')}>
                <div className={cx('h-full rounded-md', fill)} style={{width: `${pct}%`}} />
              </div>
              <span
                className={cx(
                  'w-12 shrink-0 text-right text-[12.5px] font-bold tabular-nums',
                  lead ? (dark ? 'text-emerald-300' : 'text-emerald-600') : t.sub,
                )}
              >
                {pct.toFixed(0)}%
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
