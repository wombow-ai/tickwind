'use client';

import {useEffect, useState} from 'react';
import Link from '@/components/LocalLink';
import {getEtfHoldings, type EtfHoldings} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

type Status = 'loading' | 'ready' | 'hidden' | 'note';

/**
 * Top-holdings panel for an ETF/fund detail page: the largest positions (name + % of net
 * assets) from the fund's latest SEC Form N-PORT filing, via GET /v1/etf/{t}/holdings. Every
 * figure is Go-parsed from the official filing (no estimate, no advice). The whole panel hides
 * for a non-fund ticker (the endpoint 404s), so it is safe to mount on every stock page.
 */
export function EtfHoldingsPanel({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [data, setData] = useState<EtfHoldings | null>(null);
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    getEtfHoldings(ticker, c.signal).then(
      d => {
        if (!d.holdings || d.holdings.length === 0) {
          // A KNOWN ETF with no SEC filing yet → show a brief note; otherwise hide.
          setStatus(d.no_filing ? 'note' : 'hidden');
          return;
        }
        setData(d);
        setStatus('ready');
      },
      () => setStatus('hidden'), // 404 (not a fund) / error → hide the panel
    );
    return () => c.abort();
  }, [ticker]);

  if (status === 'hidden') return null;

  if (status === 'note') {
    return (
      <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
        <h2 className={cx('mb-1.5 text-[14px] font-bold', t.text)}>{tr('etf.title')}</h2>
        <p className={cx('text-[12.5px] leading-relaxed', t.faint)}>{tr('etf.noFiling')}</p>
      </section>
    );
  }

  if (status === 'loading' || !data) {
    return (
      <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
        <div className={cx('mb-3 h-4 w-28 rounded', t.skel)} />
        <div className="space-y-2">
          {Array.from({length: 6}).map((_, i) => (
            <div key={i} className={cx('h-7 rounded-lg', t.skel)} />
          ))}
        </div>
      </section>
    );
  }

  const maxPct = Math.max(...data.holdings.map(h => h.pct_val), 0.01);

  return (
    <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <h2 className={cx('text-[14px] font-bold', t.text)}>{tr('etf.title')}</h2>
        {data.as_of && (
          <span className={cx('rounded-md px-1.5 py-0.5 text-[10.5px] font-semibold', t.chip, t.chipText)}>
            {data.as_of.slice(0, 10)}
          </span>
        )}
        <span className={cx('ml-auto text-[10.5px]', t.faint)}>{tr('etf.source')}</span>
      </div>
      <ol className="space-y-1.5">
        {data.holdings.map((h, i) => (
          <li key={h.cusip ?? `${i}-${h.name}`} className="flex items-center gap-3">
            <span className={cx('w-5 shrink-0 text-right text-[11px] tabular-nums', t.faint)}>{i + 1}</span>
            {h.ticker ? (
              <Link
                href={`/stock/${h.ticker}`}
                className={cx('min-w-0 flex-1 truncate text-[13px] hover:underline', t.text)}
              >
                {h.name}
                <span className={cx('ml-1.5 text-[11px] font-semibold', dark ? 'text-teal-400' : 'text-teal-600')}>
                  {h.ticker}
                </span>
              </Link>
            ) : (
              <span className={cx('min-w-0 flex-1 truncate text-[13px]', t.text)}>{h.name}</span>
            )}
            <span
              className="hidden h-1.5 w-24 shrink-0 overflow-hidden rounded-full sm:block"
              style={{background: dark ? 'rgba(255,255,255,0.08)' : 'rgba(15,23,42,0.06)'}}
              aria-hidden
            >
              <span
                className={cx('block h-full rounded-full', dark ? 'bg-teal-400' : 'bg-teal-500')}
                style={{width: `${Math.max(4, (h.pct_val / maxPct) * 100)}%`}}
              />
            </span>
            <span className={cx('w-14 shrink-0 text-right text-[13px] font-bold tabular-nums', t.text)}>
              {h.pct_val.toFixed(2)}%
            </span>
          </li>
        ))}
      </ol>
      <p className={cx('mt-3 text-[10.5px] leading-relaxed', t.faint)}>{tr('etf.note')}</p>
    </section>
  );
}
