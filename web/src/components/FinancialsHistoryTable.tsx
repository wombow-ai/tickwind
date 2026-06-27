'use client';

import {useEffect, useState} from 'react';
import {getFundamentals, type Fundamentals, type YearValue} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, fmtCompactUSD, tok} from '@/lib/ui';

type Status = 'loading' | 'ready' | 'hidden';
type Hist = NonNullable<Fundamentals['history']>;

// The split-immune DOLLAR lines, in display order, with their i18n label keys.
const ROWS: {key: keyof Hist; label: string}[] = [
  {key: 'revenue', label: 'fhist.revenue'},
  {key: 'gross_profit', label: 'fhist.grossProfit'},
  {key: 'operating_income', label: 'fhist.operatingIncome'},
  {key: 'net_income', label: 'fhist.netIncome'},
  {key: 'operating_cash_flow', label: 'fhist.operatingCashFlow'},
];

/**
 * Multi-year (≤10 fiscal years) financial-history table on the stock detail page: the headline
 * DOLLAR lines (revenue / gross profit / operating income / net income / operating cash flow)
 * year-by-year from SEC XBRL, via GET /v1/stocks/{t}/fundamentals (history field). Every value is
 * a real reported 10-K figure; a missing year shows "—" (never interpolated). Per-share metrics
 * are excluded (a decade of EPS would mix pre/post-split bases). Hides for non-US / funds / no
 * history, mirroring the FundamentalsCard hide-on-empty pattern. Newest year first; the label
 * column is pinned so it stays visible while the years scroll horizontally on narrow screens.
 */
export function FinancialsHistoryTable({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [hist, setHist] = useState<Hist | null>(null);
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    getFundamentals(ticker, c.signal).then(
      f => {
        const h = f.history;
        // Gate on the SAME union the table renders (ROWS), so the hide path and the row filter
        // can never disagree (e.g. a gross-profit-only filer).
        if (h && ROWS.some(r => (h[r.key]?.length ?? 0) > 0)) {
          setHist(h);
          setStatus('ready');
        } else {
          setStatus('hidden');
        }
      },
      () => setStatus('hidden'), // 404 / non-US / no data → hide
    );
    return () => c.abort();
  }, [ticker]);

  if (status === 'hidden') return null;
  if (status === 'loading' || !hist) {
    return (
      <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
        <div className={cx('mb-3 h-4 w-28 rounded', t.skel)} />
        <div className={cx('h-40 rounded-lg', t.skel)} />
      </section>
    );
  }

  // Keep only the lines that have data; build the column set = union of all their years, newest-first.
  const rows = ROWS.map(r => ({...r, data: (hist[r.key] ?? []) as YearValue[]})).filter(r => r.data.length > 0);
  const yearSet = new Set<number>();
  // year (end-date year) is the alignment KEY; fy is the company-designated DISPLAY label (they
  // differ only for an off-calendar FYE) — keep them paired so labels match the snapshot card.
  const yearToFy = new Map<number, number>();
  for (const r of rows)
    for (const yv of r.data) {
      yearSet.add(yv.year);
      yearToFy.set(yv.year, yv.fy);
    }
  const years = [...yearSet].sort((a, b) => b - a); // newest first

  const neg = dark ? 'text-rose-400' : 'text-rose-500';
  const pin = cx('sticky left-0 z-10', t.card); // pinned label column keeps its card background

  return (
    <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-3 flex items-center gap-2">
        <h2 className={cx('text-[14px] font-bold', t.text)}>{tr('fhist.title')}</h2>
        <span className={cx('ml-auto text-[10.5px]', t.faint)}>{tr('fund.source')}</span>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-[12.5px] tabular-nums">
          <thead>
            <tr>
              <th className={cx(pin, 'py-1.5 pr-3 text-left')} />
              {years.map((y, i) => (
                <th
                  key={y}
                  className={cx('whitespace-nowrap px-2.5 py-1.5 text-right font-semibold', i === 0 ? t.text : t.faint)}
                >
                  FY{yearToFy.get(y) ?? y}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map(r => {
              const m = new Map(r.data.map(yv => [yv.year, yv.val]));
              return (
                <tr key={r.key} className={cx('border-t', t.border)}>
                  <td className={cx(pin, 'py-1.5 pr-3 text-left font-medium', t.sub)}>{tr(r.label)}</td>
                  {years.map((y, i) => {
                    const v = m.get(y);
                    return (
                      <td
                        key={y}
                        className={cx(
                          'whitespace-nowrap px-2.5 py-1.5 text-right',
                          v != null && v < 0 ? neg : t.text,
                          i === 0 && 'font-semibold',
                        )}
                      >
                        {v == null ? '—' : fmtCompactUSD(v)}
                      </td>
                    );
                  })}
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </section>
  );
}
