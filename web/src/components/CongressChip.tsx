'use client';

import {Landmark} from 'lucide-react';
import Link from 'next/link';
import {useEffect, useState} from 'react';
import {getStockCongress, type CongressTrade} from '@/lib/api';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

type Status = 'loading' | 'ready' | 'hidden';

// How many trades to show inline before collapsing the rest into a "+N more".
const MAX_SHOWN = 6;

/** Normalizes a disclosed direction into buy / sell / exchange / other. */
function side(type: string): 'buy' | 'sell' | 'exchange' | 'other' {
  const x = type.toLowerCase();
  if (x.includes('purchase') || x.includes('buy')) return 'buy';
  if (x.includes('sale') || x.includes('sell')) return 'sell';
  if (x.includes('exchange')) return 'exchange';
  return 'other';
}

/**
 * "Recent congressional trades" block on the stock detail page: which U.S. House
 * members disclosed buying or selling this ticker, each linking to the member's
 * detail page. Sourced facts (House Clerk PTRs), never advice. Hides entirely
 * (no fake data) when the ticker has no disclosed trades — mirroring EarningsChip
 * / ShortChip's hide-on-empty pattern.
 */
export function CongressChip({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const [trades, setTrades] = useState<CongressTrade[]>([]);
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    getStockCongress(ticker, c.signal).then(
      r => {
        const list = r.trades ?? [];
        setTrades(list);
        setStatus(list.length > 0 ? 'ready' : 'hidden');
      },
      () => setStatus('hidden'), // 404 / no data → hide
    );
    return () => c.abort();
  }, [ticker]);

  if (status === 'hidden') return null;
  if (status === 'loading') {
    return <div className={cx('mb-6 h-9 w-72 rounded-full', t.skel)} />;
  }

  const locale = lang === 'zh' ? 'zh-CN' : 'en-US';
  const fmtDate = (raw: string) => {
    const d = new Date(raw);
    return Number.isNaN(d.getTime())
      ? raw
      : d.toLocaleDateString(locale, {month: 'short', day: 'numeric', year: 'numeric'});
  };

  const shown = trades.slice(0, MAX_SHOWN);
  const extra = trades.length - shown.length;

  return (
    <div
      className={cx(
        'mb-6 rounded-2xl border p-4',
        t.card,
        t.border,
        t.soft,
      )}
    >
      <div className="mb-3 flex items-center gap-2">
        <Landmark size={15} className={dark ? 'text-sky-300' : 'text-sky-600'} />
        <span className={cx('text-[13px] font-semibold', t.text)}>{tr('cchip.title')}</span>
      </div>
      <ul className="flex flex-col gap-2">
        {shown.map((trade, i) => {
          const s = side(trade.type);
          const sideLabel =
            s === 'buy' ? tr('cchip.buy') : s === 'sell' ? tr('cchip.sell') : s === 'exchange' ? tr('cchip.exchange') : trade.type;
          const sideColor =
            s === 'buy'
              ? dark
                ? 'bg-emerald-500/15 text-emerald-300'
                : 'bg-emerald-50 text-emerald-600'
              : s === 'sell'
                ? dark
                  ? 'bg-rose-500/15 text-rose-300'
                  : 'bg-rose-50 text-rose-600'
                : dark
                  ? 'bg-slate-800 text-slate-300'
                  : 'bg-slate-100 text-slate-600';
          return (
            <li
              key={`${trade.slug}-${trade.tx_date}-${i}`}
              className="flex flex-wrap items-center gap-x-2.5 gap-y-1 text-[12.5px]"
            >
              <Link
                href={`/congress/member/${trade.slug}`}
                className={cx('font-semibold hover:underline', t.accentText)}
              >
                {trade.member}
              </Link>
              <span className={cx('rounded-md px-1.5 py-0.5 text-[10.5px] font-bold', sideColor)}>
                {sideLabel}
              </span>
              {trade.amount_range && (
                <span className={cx('tabular-nums', t.sub)}>{trade.amount_range}</span>
              )}
              <span className={cx('ml-auto tabular-nums', t.faint)}>{fmtDate(trade.tx_date)}</span>
            </li>
          );
        })}
      </ul>
      {extra > 0 && (
        <p className={cx('mt-2 text-[11.5px]', t.faint)}>
          {tr('cchip.more').replace('{n}', String(extra))}
        </p>
      )}
    </div>
  );
}
