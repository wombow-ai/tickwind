'use client';

import {CalendarClock} from 'lucide-react';
import {useEffect, useState} from 'react';
import {getStockEarnings, getEarningsReaction, type Earning} from '@/lib/api';
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
 * Earnings-timing chip on the stock detail page: the NEXT upcoming earnings date
 * (+ a pre/after-market slot and the consensus EPS estimate, from Finnhub via
 * GET /v1/stocks/{t}/earnings) AND the LAST reported date (the most recent SEC
 * 8-K item 2.02 filing, from the earnings-reaction history). Both are real dates,
 * never a forecast. Hides entirely when NEITHER is available (404 / non-US / no
 * filings), mirroring the FundamentalsCard hide-on-empty pattern so no fake data
 * is ever shown.
 */
export function EarningsChip({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const [next, setNext] = useState<Earning | null>(null);
  const [lastDate, setLastDate] = useState<string | null>(null);
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    Promise.all([
      getStockEarnings(ticker, 12, c.signal).catch(() => null), // 404 → null
      getEarningsReaction(ticker, c.signal), // resolves null on any error
    ]).then(([earn, react]) => {
      if (c.signal.aborted) return;
      const today = new Date();
      today.setHours(0, 0, 0, 0);
      const upcoming = (earn?.earnings ?? [])
        .map(e => ({e, ts: Date.parse(e.date)}))
        .filter(({ts}) => !Number.isNaN(ts) && ts >= today.getTime())
        .sort((a, b) => a.ts - b.ts);
      const nextE = upcoming.length ? upcoming[0].e : null;
      const last = react?.events?.[0]?.date ?? null; // most recent reported date
      if (!nextE && !last) {
        setStatus('hidden');
        return;
      }
      setNext(nextE);
      setLastDate(last);
      setStatus('ready');
    });
    return () => c.abort();
  }, [ticker]);

  if (status === 'hidden') return null; // no margin when absent → no dangling gap
  if (status === 'loading') {
    return (
      <div className="mb-6">
        <div className={cx('h-9 w-64 rounded-full', t.skel)} />
      </div>
    );
  }

  const locale = lang === 'zh' ? 'zh-CN' : 'en-US';
  // Both dates are CALENDAR dates (no time-of-day): the SEC filing date is "YYYY-MM-DD"
  // and the Finnhub next-date is a UTC-midnight RFC3339 timestamp. Slice BOTH to 10 chars
  // and parse at LOCAL midnight, so the UTC-midnight next-date never renders the prior day
  // in a negative-offset (US) timezone — and the chip always matches the calendar.
  const fmt = (s: string) =>
    new Date(s.slice(0, 10) + 'T00:00:00').toLocaleDateString(locale, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    });
  const hourLabel = next?.hour && HOUR_KEY[next.hour] ? tr(HOUR_KEY[next.hour]) : '';

  return (
    <div className="mb-6">
      <span
        className={cx(
          'inline-flex min-h-9 flex-wrap items-center gap-x-2 gap-y-1 rounded-full border px-3.5 py-1 text-[12.5px]',
          t.card,
          t.border,
          t.soft,
        )}
      >
        <CalendarClock size={14} className={dark ? 'text-teal-300' : 'text-teal-600'} />
      {next && (
        <>
          <span className={cx('font-semibold', t.sub)}>{tr('earnings.next')}</span>
          <span className={cx('font-bold tabular-nums', t.text)}>{fmt(next.date)}</span>
          {hourLabel && (
            <span className={cx('rounded-md px-1.5 py-0.5 text-[10.5px] font-semibold', t.chip, t.chipText)}>
              {hourLabel}
            </span>
          )}
          {typeof next.eps_estimate === 'number' && (
            <span className={cx('tabular-nums text-[11.5px]', t.faint)}>
              {tr('earnings.estEps')} ${next.eps_estimate.toFixed(2)}
            </span>
          )}
        </>
      )}
      {next && lastDate && (
        <span className={t.faint} aria-hidden>
          ·
        </span>
      )}
      {lastDate && (
        <>
          <span className={cx('font-semibold', t.sub)}>{tr('earnings.last')}</span>
          <span className={cx('font-bold tabular-nums', t.text)}>{fmt(lastDate)}</span>
        </>
      )}
      </span>
    </div>
  );
}
