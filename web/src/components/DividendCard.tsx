'use client';

import {Coins} from 'lucide-react';
import {useEffect, useState} from 'react';
import {getDividend, type DividendView} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

type Status = 'loading' | 'ready' | 'hidden';

/**
 * DividendCard surfaces a stock's dividend profile — yield, payout ratio, dividends-per-share, FCF
 * coverage, and YoY dividend change — for the income investor (figures that otherwise sit buried among
 * the ~160 indicators). Every number is Go-computed from the company's SEC-filed annual figures
 * (GET /v1/stocks/{t}/dividend); it is DESCRIPTIVE — there is deliberately no "dividend-safety grade"
 * (the disclaimer says so). Hides itself for a non-payer / when nothing is computable. Client-fetched
 * (per deploy-gotcha #7 — never SSR-fetch the API through the tunnel).
 */
export function DividendCard({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [d, setD] = useState<DividendView | null>(null);
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    getDividend(ticker, c.signal).then(r => {
      if (c.signal.aborted) return;
      if (r) {
        setD(r);
        setStatus('ready');
      } else {
        setStatus('hidden');
      }
    });
    return () => c.abort();
  }, [ticker]);

  if (status === 'hidden') return null;

  const pct = (v?: number) => (v == null ? null : `${v.toFixed(1)}%`);
  const cells: {key: string; label: string; value: string | null; tone?: 'up' | 'down'}[] = d
    ? [
        {key: 'yield', label: tr('div.yield'), value: pct(d.yield)},
        {key: 'payout', label: tr('div.payout'), value: pct(d.payout_ratio)},
        {key: 'dps', label: tr('div.dps'), value: d.dps == null ? null : `$${d.dps.toFixed(2)}`},
        {key: 'coverage', label: tr('div.coverage'), value: d.fcf_coverage == null ? null : `${d.fcf_coverage.toFixed(1)}×`},
        {
          key: 'growth',
          label: tr('div.growth'),
          value: d.yoy_growth == null ? null : `${d.yoy_growth >= 0 ? '+' : ''}${d.yoy_growth.toFixed(1)}%`,
          tone: (d.yoy_growth == null ? undefined : d.yoy_growth >= 0 ? 'up' : 'down') as 'up' | 'down' | undefined,
        },
      ].filter(c => c.value != null)
    : [];

  return (
    <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-1 flex flex-wrap items-center gap-2">
        <h2 className={cx('flex items-center gap-1.5 text-[14px] font-bold', t.text)}>
          <Coins size={15} className={dark ? 'text-amber-300' : 'text-amber-500'} />
          {tr('div.title')}
        </h2>
        {d?.period && <span className={cx('text-[10.5px]', t.faint)}>{d.period}</span>}
      </div>
      <p className={cx('mb-3 text-[11.5px]', t.sub)}>{tr('div.sub')}</p>

      {status === 'loading' || !d ? (
        <div className={cx('h-20 rounded-xl', t.skel)} />
      ) : (
        <div className="grid grid-cols-2 gap-x-4 gap-y-3 sm:grid-cols-3">
          {cells.map(c => (
            <div key={c.key}>
              <div className={cx('text-[11px]', t.sub)}>{c.label}</div>
              <div
                className={cx(
                  'text-[15px] font-semibold tabular-nums',
                  c.tone === 'up'
                    ? dark ? 'text-emerald-400' : 'text-emerald-600'
                    : c.tone === 'down'
                      ? dark ? 'text-rose-400' : 'text-rose-500'
                      : t.text,
                )}
              >
                {c.value}
              </div>
            </div>
          ))}
        </div>
      )}

      <p className={cx('mt-3 text-[11px] leading-snug', t.faint)}>{tr('div.disclaimer')}</p>
    </section>
  );
}
