'use client';

import {Briefcase} from 'lucide-react';
import Link from 'next/link';
import {useEffect, useState} from 'react';
import {getWhales, type WhaleHolder} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, fmtCompactUSD, tok} from '@/lib/ui';

type Status = 'loading' | 'ready' | 'hidden';

// How many holders to show inline before collapsing the rest into a "+N more".
const MAX_SHOWN = 8;

/**
 * "Institutional whales" block on the stock detail page: which famous 13F fund
 * managers hold this ticker, each linking to the fund's /fund/{slug} page, with
 * the position's value, portfolio weight, and quarter-over-quarter move (colored
 * like the 13F board). The reverse of the whale-holdings board. Sourced facts
 * (SEC 13F filings, ~45-day lag), never advice. Hides entirely (no fake data)
 * when no tracked fund holds the ticker — mirroring CongressChip's hide-on-empty.
 */
export function WhalesChip({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [holders, setHolders] = useState<WhaleHolder[]>([]);
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    getWhales(ticker, c.signal).then(
      r => {
        const list = r.holders ?? [];
        setHolders(list);
        setStatus(list.length > 0 ? 'ready' : 'hidden');
      },
      () => setStatus('hidden'), // network / error → hide gracefully
    );
    return () => c.abort();
  }, [ticker]);

  if (status === 'hidden') return null;
  if (status === 'loading') {
    return <div className={cx('mb-6 h-9 w-72 rounded-full', t.skel)} />;
  }

  const shown = holders.slice(0, MAX_SHOWN);
  const extra = holders.length - shown.length;

  // QoQ change → label + color, matching the 13F board's palette.
  const changeStyle = (change: string): string => {
    switch (change) {
      case 'new':
        return dark ? 'bg-sky-500/15 text-sky-300' : 'bg-sky-50 text-sky-600';
      case 'add':
        return dark ? 'bg-emerald-500/15 text-emerald-300' : 'bg-emerald-50 text-emerald-600';
      case 'trim':
        return dark ? 'bg-rose-500/15 text-rose-300' : 'bg-rose-50 text-rose-600';
      default:
        return dark ? 'bg-slate-800 text-slate-400' : 'bg-slate-100 text-slate-500';
    }
  };

  return (
    <div className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-3 flex items-center gap-2">
        <Briefcase size={15} className={dark ? 'text-violet-300' : 'text-violet-600'} />
        <span className={cx('text-[13px] font-semibold', t.text)}>{tr('whales.title')}</span>
      </div>
      <ul className="flex flex-col gap-2">
        {shown.map((h, i) => (
          <li
            key={`${h.fund_slug}-${i}`}
            className="flex flex-wrap items-center gap-x-2.5 gap-y-1 text-[12.5px]"
          >
            <Link
              href={`/fund/${encodeURIComponent(h.fund_slug)}`}
              className={cx('font-semibold hover:underline', t.accentText)}
            >
              {h.fund_name}
            </Link>
            <span className={cx('hidden text-[11px] sm:inline', t.faint)}>{h.manager}</span>
            <span className={cx('rounded-md px-1.5 py-0.5 text-[10.5px] font-bold', changeStyle(h.change))}>
              {tr(`13f.change.${h.change}`)}
            </span>
            <span className={cx('ml-auto flex items-center gap-2 tabular-nums', t.sub)}>
              <span className={cx('font-semibold', t.text)}>{fmtCompactUSD(h.value)}</span>
              <span className={cx('hidden text-[11px] sm:inline', t.faint)}>
                {tr('whales.weight').replace('{w}', h.weight.toFixed(1))}
              </span>
            </span>
          </li>
        ))}
      </ul>
      {extra > 0 && (
        <p className={cx('mt-2 text-[11.5px]', t.faint)}>
          {tr('whales.more').replace('{n}', String(extra))}
        </p>
      )}
    </div>
  );
}
