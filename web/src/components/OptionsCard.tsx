'use client';

import {Layers} from 'lucide-react';
import {useEffect, useState} from 'react';
import {getOptions, type OptionsView} from '@/lib/api';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

type Status = 'loading' | 'ready' | 'hidden';

/** 91297 → "91.3K"; 1_234_567 → "1.23M". */
function fmtN(n: number): string {
  if (n >= 1e6) return `${(n / 1e6).toFixed(2)}M`;
  if (n >= 1e3) return `${(n / 1e3).toFixed(1)}K`;
  return String(n);
}

/**
 * Per-stock options overview: put/call ratios (volume & OI), nearest-expiry
 * max pain, and the open-interest leaders — from Cboe's ~15-min delayed feed
 * (clearly labeled). Hides entirely for symbols with no listed options
 * (non-US etc.), mirroring ShortChip/EarningsChip.
 */
export function OptionsCard({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const [v, setV] = useState<OptionsView | null>(null);
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    getOptions(ticker, c.signal).then(
      r => {
        setV(r);
        setStatus('ready');
      },
      () => setStatus('hidden'),
    );
    return () => c.abort();
  }, [ticker]);

  if (status === 'hidden') return null;
  if (status === 'loading' || !v) {
    return <div className={cx('mb-6 h-44 rounded-2xl', t.skel)} />;
  }

  const bearish = (r: number) => r > 1;
  const ratioColor = (r: number) =>
    bearish(r)
      ? dark
        ? 'text-rose-400'
        : 'text-rose-500'
      : dark
        ? 'text-emerald-400'
        : 'text-emerald-600';
  const locale = lang === 'zh' ? 'zh-CN' : 'en-US';
  const expLabel = (d: string) =>
    d
      ? new Date(d + 'T00:00:00Z').toLocaleDateString(locale, {month: 'short', day: 'numeric'})
      : '';

  return (
    <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <h2 className={cx('flex items-center gap-1.5 text-[14px] font-bold', t.text)}>
          <Layers size={15} className={dark ? 'text-indigo-300' : 'text-indigo-500'} />
          {tr('options.title')}
        </h2>
        <span className={cx('ml-auto text-[10.5px]', t.faint)}>{tr('options.delayed')}</span>
      </div>

      <div className="mb-3 grid grid-cols-3 gap-2">
        <Metric label={tr('options.pcVol')} value={v.pc_volume.toFixed(2)} color={ratioColor(v.pc_volume)} t={t} />
        <Metric label={tr('options.pcOI')} value={v.pc_oi.toFixed(2)} color={ratioColor(v.pc_oi)} t={t} />
        <Metric
          label={tr('options.maxPain')}
          value={v.max_pain ? `$${v.max_pain.toFixed(0)}` : '—'}
          sub={v.expiry ? expLabel(v.expiry) : undefined}
          t={t}
        />
      </div>

      {v.top_oi.length > 0 && (
        <div>
          <div className={cx('mb-1 text-[11px] font-semibold', t.faint)}>{tr('options.topOI')}</div>
          <div className="overflow-x-auto">
            <table className="w-full text-[11.5px] tabular-nums">
              <thead>
                <tr className={cx('text-left', t.faint)}>
                  <th className="py-1 pr-2 font-medium">{tr('options.kind')}</th>
                  <th className="py-1 pr-2 text-right font-medium">{tr('options.strike')}</th>
                  <th className="py-1 pr-2 font-medium">{tr('options.expiry')}</th>
                  <th className="py-1 pr-2 text-right font-medium">{tr('options.oi')}</th>
                  <th className="py-1 pr-2 text-right font-medium">{tr('options.vol')}</th>
                  <th className="py-1 text-right font-medium">{tr('options.iv')}</th>
                </tr>
              </thead>
              <tbody>
                {v.top_oi.map(c => {
                  const call = c.type === 'C';
                  return (
                    <tr key={c.contract} className={cx('border-t', t.hair)}>
                      <td className="py-1 pr-2">
                        <span
                          className={cx(
                            'rounded px-1 py-0.5 text-[10px] font-bold',
                            call
                              ? dark
                                ? 'bg-emerald-500/15 text-emerald-300'
                                : 'bg-emerald-50 text-emerald-600'
                              : dark
                                ? 'bg-rose-500/15 text-rose-300'
                                : 'bg-rose-50 text-rose-600',
                          )}
                        >
                          {call ? tr('options.call') : tr('options.put')}
                        </span>
                      </td>
                      <td className={cx('py-1 pr-2 text-right font-semibold', t.text)}>${c.strike}</td>
                      <td className={cx('py-1 pr-2', t.sub)}>{expLabel(c.expiry)}</td>
                      <td className={cx('py-1 pr-2 text-right font-semibold', t.text)}>{fmtN(c.oi)}</td>
                      <td className={cx('py-1 pr-2 text-right', t.sub)}>{fmtN(c.volume)}</td>
                      <td className={cx('py-1 text-right', t.sub)}>{(c.iv * 100).toFixed(0)}%</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </section>
  );
}

function Metric({
  label,
  value,
  sub,
  color,
  t,
}: {
  label: string;
  value: string;
  sub?: string;
  color?: string;
  t: ReturnType<typeof tok>;
}) {
  return (
    <div className={cx('rounded-xl border px-2.5 py-2', t.border)}>
      <div className={cx('text-[10.5px]', t.faint)}>{label}</div>
      <div className={cx('text-[15px] font-bold tabular-nums', color ?? t.text)}>{value}</div>
      {sub && <div className={cx('text-[10px] tabular-nums', t.faint)}>{sub}</div>}
    </div>
  );
}
