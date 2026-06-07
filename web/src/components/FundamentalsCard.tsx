'use client';

import {useEffect, useState} from 'react';
import {getFundamentals, type Fundamentals} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, fmtCompactUSD, tok} from '@/lib/ui';

type Status = 'loading' | 'ready' | 'hidden';

/**
 * Compact fundamentals card on the stock detail page: market cap · P/E · revenue
 * · net income (+ EPS, P/B), from free SEC XBRL via GET /v1/stocks/{t}/fundamentals.
 * P/E shows "亏损/Loss" for loss-makers (null pe). The whole card hides for
 * non-US / unknown tickers or when the endpoint 404s (no XBRL data).
 */
export function FundamentalsCard({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [data, setData] = useState<Fundamentals | null>(null);
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    getFundamentals(ticker, c.signal).then(
      f => {
        setData(f);
        setStatus('ready');
      },
      () => setStatus('hidden'), // 404 / non-US / no data → hide the card
    );
    return () => c.abort();
  }, [ticker]);

  if (status === 'hidden') return null;

  if (status === 'loading' || !data) {
    return (
      <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
        <div className={cx('mb-3 h-4 w-20 rounded', t.skel)} />
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
          {Array.from({length: 6}).map((_, i) => (
            <div key={i} className={cx('h-11 rounded-lg', t.skel)} />
          ))}
        </div>
      </section>
    );
  }

  const dash = '—';
  const neg = dark ? 'text-rose-400' : 'text-rose-500';
  const cells: {label: string; value: string; negative?: boolean}[] = [
    {label: tr('fund.marketCap'), value: data.market_cap !== null ? fmtCompactUSD(data.market_cap) : dash},
    {label: tr('fund.pe'), value: data.pe !== null ? data.pe.toFixed(1) : tr('fund.loss')},
    {label: tr('fund.revenue'), value: fmtCompactUSD(data.revenue)},
    {label: tr('fund.netIncome'), value: fmtCompactUSD(data.net_income), negative: data.net_income < 0},
    {label: tr('fund.eps'), value: `$${data.eps_diluted.toFixed(2)}`, negative: data.eps_diluted < 0},
    {label: tr('fund.pb'), value: data.pb !== null ? data.pb.toFixed(2) : dash},
  ];

  return (
    <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <h2 className={cx('text-[14px] font-bold', t.text)}>{tr('fund.title')}</h2>
        {data.period && (
          <span className={cx('rounded-md px-1.5 py-0.5 text-[10.5px] font-semibold', t.chip, t.chipText)}>
            {data.period}
          </span>
        )}
        <span className={cx('ml-auto text-[10.5px]', t.faint)}>{tr('fund.source')}</span>
      </div>
      <div className="grid grid-cols-2 gap-x-4 gap-y-3 sm:grid-cols-3 lg:grid-cols-6">
        {cells.map(c => (
          <div key={c.label} className="min-w-0">
            <div className={cx('truncate text-[11px]', t.faint)}>{c.label}</div>
            <div className={cx('mt-0.5 truncate text-[15px] font-bold tabular-nums', c.negative ? neg : t.text)}>
              {c.value}
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}
