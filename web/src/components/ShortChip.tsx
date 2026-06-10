'use client';

import {Flame} from 'lucide-react';
import {useEffect, useState} from 'react';
import {getShort, type ShortInterest} from '@/lib/api';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

type Status = 'loading' | 'ready' | 'hidden';

// Heuristic squeeze flags: days-to-cover ≥ 5 sessions of average volume, or a
// ≥ 20% jump in the short position since the prior settlement. Display-only
// (sourced FINRA facts stay on the chip); never advice.
const SQUEEZE_DTC = 5;
const SQUEEZE_CHG = 20;

/** 58_930_916 → "58.9M"; 1_234_567_890 → "1.23B". */
function fmtQty(n: number): string {
  if (n >= 1e9) return `${(n / 1e9).toFixed(2)}B`;
  if (n >= 1e6) return `${(n / 1e6).toFixed(1)}M`;
  if (n >= 1e3) return `${(n / 1e3).toFixed(0)}K`;
  return String(n);
}

/**
 * Short-pressure chip on the stock detail page: FINRA consolidated short
 * interest for the latest twice-monthly settlement — days-to-cover, the short
 * position and its change vs the prior period, plus a "squeeze risk" badge
 * when pressure runs hot. Hides entirely (no fake data) when the symbol has no
 * row, mirroring EarningsChip.
 */
export function ShortChip({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const [si, setSi] = useState<ShortInterest | null>(null);
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    getShort(ticker, c.signal).then(
      r => {
        setSi(r.short);
        setStatus('ready');
      },
      () => setStatus('hidden'), // 404 / no data → hide
    );
    return () => c.abort();
  }, [ticker]);

  if (status === 'hidden') return null;
  if (status === 'loading' || !si) {
    return <div className={cx('mb-6 h-9 w-72 rounded-full', t.skel)} />;
  }

  const locale = lang === 'zh' ? 'zh-CN' : 'en-US';
  const asOf = new Date(si.settlement_date + 'T00:00:00Z').toLocaleDateString(locale, {
    month: 'short',
    day: 'numeric',
  });
  const chgUp = si.change_pct >= 0;
  const squeeze = si.days_to_cover >= SQUEEZE_DTC || si.change_pct >= SQUEEZE_CHG;

  return (
    <div className="mb-6">
      <span
        className={cx(
          'inline-flex flex-wrap items-center gap-x-2 gap-y-1 rounded-full border px-3.5 py-1.5 text-[12.5px]',
          t.card,
          t.border,
          t.soft,
        )}
      >
        <Flame size={14} className={dark ? 'text-orange-300' : 'text-orange-500'} />
        <span className={cx('font-semibold', t.sub)}>{tr('short.title')}</span>
        <span className={cx('tabular-nums', t.faint)}>{tr('short.dtc')}</span>
        <span className={cx('font-bold tabular-nums', t.text)}>{si.days_to_cover.toFixed(2)}</span>
        <span className={cx('tabular-nums', t.faint)}>{tr('short.qty')}</span>
        <span className={cx('font-bold tabular-nums', t.text)}>{fmtQty(si.short_qty)}</span>
        <span
          className={cx(
            'font-semibold tabular-nums',
            chgUp ? (dark ? 'text-rose-400' : 'text-rose-500') : dark ? 'text-emerald-400' : 'text-emerald-600',
          )}
        >
          {chgUp ? '+' : ''}
          {si.change_pct.toFixed(1)}%
        </span>
        {squeeze && (
          <span
            className={cx(
              'rounded-md px-1.5 py-0.5 text-[10.5px] font-bold',
              dark ? 'bg-rose-500/15 text-rose-300' : 'bg-rose-50 text-rose-600',
            )}
          >
            {tr('short.risk')}
          </span>
        )}
        <span className={cx('text-[11px]', t.faint)}>
          {tr('short.asof').replace('{d}', asOf)} · FINRA
        </span>
      </span>
    </div>
  );
}
