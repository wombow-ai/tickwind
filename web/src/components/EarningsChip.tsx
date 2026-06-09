'use client';

import {CalendarClock} from 'lucide-react';
import {useEffect, useState} from 'react';
import {getStockEarnings, type Earning} from '@/lib/api';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

type Status = 'loading' | 'ready' | 'hidden';

// Finnhub reporting-time codes → i18n keys (bmo before open, amc after close,
// dmh during market hours). Empty/unknown codes render no label.
const HOUR_KEY: Record<string, string> = {
  bmo: 'earnings.bmo',
  amc: 'earnings.amc',
  dmh: 'earnings.dmh',
};

/**
 * Slim "next earnings" chip on the stock detail page: the nearest upcoming
 * earnings date (+ a pre/after-market label and the consensus EPS estimate when
 * present), from GET /v1/stocks/{t}/earnings. Hides entirely when there is no
 * upcoming row (404 / non-US / every row already in the past), mirroring the
 * FundamentalsCard hide-on-empty pattern so no fake data is ever shown.
 */
export function EarningsChip({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const [next, setNext] = useState<Earning | null>(null);
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    getStockEarnings(ticker, 12, c.signal).then(
      r => {
        const today = new Date();
        today.setHours(0, 0, 0, 0);
        const upcoming = (r.earnings ?? [])
          .map(e => ({e, ts: new Date(e.date).getTime()}))
          .filter(({ts}) => !Number.isNaN(ts) && ts >= today.getTime())
          .sort((a, b) => a.ts - b.ts);
        if (upcoming.length === 0) {
          setStatus('hidden');
          return;
        }
        setNext(upcoming[0].e);
        setStatus('ready');
      },
      () => setStatus('hidden'), // 404 / non-US / no data → hide
    );
    return () => c.abort();
  }, [ticker]);

  if (status === 'hidden') return null;
  if (status === 'loading' || !next) {
    return <div className={cx('mb-6 h-9 w-56 rounded-full', t.skel)} />;
  }

  const locale = lang === 'zh' ? 'zh-CN' : 'en-US';
  const dateStr = new Date(next.date).toLocaleDateString(locale, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  });
  const hourLabel =
    next.hour && HOUR_KEY[next.hour] ? tr(HOUR_KEY[next.hour]) : '';

  return (
    <div className="mb-6">
      <span
        className={cx(
          'inline-flex items-center gap-2 rounded-full border px-3.5 py-1.5 text-[12.5px]',
          t.card,
          t.border,
          t.soft,
        )}
      >
        <CalendarClock
          size={14}
          className={dark ? 'text-teal-300' : 'text-teal-600'}
        />
        <span className={cx('font-semibold', t.sub)}>{tr('earnings.next')}</span>
        <span className={cx('font-bold tabular-nums', t.text)}>{dateStr}</span>
        {hourLabel && (
          <span
            className={cx(
              'rounded-md px-1.5 py-0.5 text-[10.5px] font-semibold',
              t.chip,
              t.chipText,
            )}
          >
            {hourLabel}
          </span>
        )}
        {typeof next.eps_estimate === 'number' && (
          <span className={cx('tabular-nums text-[11.5px]', t.faint)}>
            {tr('earnings.estEps')} ${next.eps_estimate.toFixed(2)}
          </span>
        )}
      </span>
    </div>
  );
}
