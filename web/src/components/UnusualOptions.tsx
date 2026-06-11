'use client';

import {Activity} from 'lucide-react';
import Link from 'next/link';
import {useEffect, useState} from 'react';
import {getUnusualOptions, type UnusualContract} from '@/lib/api';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, timeAgo, tok} from '@/lib/ui';

type Status = 'loading' | 'ready' | 'empty';

function fmtN(n: number): string {
  if (n >= 1e6) return `${(n / 1e6).toFixed(2)}M`;
  if (n >= 1e3) return `${(n / 1e3).toFixed(1)}K`;
  return String(n);
}

/**
 * Whole-market unusual options-activity board: the contracts trading the most
 * volume today across heavily-optioned US names, with their volume/OI ratio —
 * a quick read on where the options crowd is piling in. Delayed ≈15 min (Cboe).
 */
export function UnusualOptions() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const [rows, setRows] = useState<UnusualContract[]>([]);
  const [at, setAt] = useState('');
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    getUnusualOptions(c.signal).then(
      r => {
        setRows(r.contracts ?? []);
        setAt(r.updated_at);
        setStatus((r.contracts ?? []).length ? 'ready' : 'empty');
      },
      () => setStatus('empty'),
    );
    return () => c.abort();
  }, []);

  const locale = lang === 'zh' ? 'zh-CN' : 'en-US';
  const expLabel = (d: string) =>
    new Date(d + 'T00:00:00Z').toLocaleDateString(locale, {month: 'short', day: 'numeric'});

  return (
    <div className="mx-auto max-w-3xl">
      <header className="mb-4">
        <h1 className={cx('flex items-center gap-2 text-[22px] font-bold tracking-tight', t.text)}>
          <Activity size={20} className={dark ? 'text-indigo-300' : 'text-indigo-500'} />
          {tr('unusual.title')}
        </h1>
        <p className={cx('mt-1 text-[13px]', t.sub)}>
          {tr('unusual.subtitle')}
          {at && status === 'ready' ? ` · ${timeAgo(at)} ${tr('common.ago')}` : ''} · {tr('options.delayed')}
        </p>
      </header>

      {status === 'loading' && <div className={cx('h-72 rounded-2xl', t.skel)} />}
      {status === 'empty' && (
        <p className={cx('rounded-2xl border p-8 text-center text-[13px]', t.card, t.border, t.soft, t.sub)}>
          {tr('unusual.empty')}
        </p>
      )}

      {status === 'ready' && (
        <div className={cx('overflow-x-auto rounded-2xl border', t.card, t.border, t.soft)}>
          <table className="w-full text-[12.5px] tabular-nums">
            <thead>
              <tr className={cx('text-left', t.faint)}>
                <th className="px-3 py-2 font-medium">{tr('unusual.ticker')}</th>
                <th className="px-2 py-2 font-medium">{tr('options.kind')}</th>
                <th className="px-2 py-2 text-right font-medium">{tr('options.strike')}</th>
                <th className="px-2 py-2 font-medium">{tr('options.expiry')}</th>
                <th className="px-2 py-2 text-right font-medium">{tr('options.vol')}</th>
                <th className="px-2 py-2 text-right font-medium">{tr('options.oi')}</th>
                <th className="px-2 py-2 text-right font-medium">{tr('unusual.volOI')}</th>
                <th className="px-3 py-2 text-right font-medium">{tr('options.iv')}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((c, i) => {
                const call = c.type === 'C';
                return (
                  <tr key={`${c.ticker}-${c.type}-${c.strike}-${c.expiry}-${i}`} className={cx('border-t', t.hair)}>
                    <td className="px-3 py-2">
                      <Link href={`/stock/${encodeURIComponent(c.ticker)}`} className={cx('font-bold', t.accentText)}>
                        {c.ticker}
                      </Link>
                    </td>
                    <td className="px-2 py-2">
                      <span
                        className={cx(
                          'rounded px-1 py-0.5 text-[10px] font-bold',
                          call
                            ? dark ? 'bg-emerald-500/15 text-emerald-300' : 'bg-emerald-50 text-emerald-600'
                            : dark ? 'bg-rose-500/15 text-rose-300' : 'bg-rose-50 text-rose-600',
                        )}
                      >
                        {call ? tr('options.call') : tr('options.put')}
                      </span>
                    </td>
                    <td className={cx('px-2 py-2 text-right font-semibold', t.text)}>${c.strike}</td>
                    <td className={cx('px-2 py-2', t.sub)}>{expLabel(c.expiry)}</td>
                    <td className={cx('px-2 py-2 text-right font-semibold', t.text)}>{fmtN(c.volume)}</td>
                    <td className={cx('px-2 py-2 text-right', t.sub)}>{fmtN(c.oi)}</td>
                    <td className={cx('px-2 py-2 text-right font-semibold', c.vol_oi >= 1 ? (dark ? 'text-amber-300' : 'text-amber-600') : t.sub)}>
                      {c.vol_oi > 0 ? `${c.vol_oi.toFixed(1)}×` : '—'}
                    </td>
                    <td className={cx('px-3 py-2 text-right', t.sub)}>
                      {c.iv > 0 ? `${(c.iv * 100).toFixed(0)}%` : '—'}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
