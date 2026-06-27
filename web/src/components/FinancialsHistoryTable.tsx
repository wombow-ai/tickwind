'use client';

import {Fragment, useEffect, useState} from 'react';
import {getFundamentals, type Fundamentals, type QuarterValue, type YearValue} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, fmtCompactUSD, tok} from '@/lib/ui';

type Status = 'loading' | 'ready' | 'hidden';
type Hist = NonNullable<Fundamentals['history']>;

// The DOLLAR lines, in display order. The income-statement lines are split-immune flows; the
// balance-sheet lines are year-END instants (annual only — no `_q`, so they hide in Quarterly).
const ROWS: {key: keyof Hist; label: string; group: 'income' | 'balance'}[] = [
  {key: 'revenue', label: 'fhist.revenue', group: 'income'},
  {key: 'gross_profit', label: 'fhist.grossProfit', group: 'income'},
  {key: 'operating_income', label: 'fhist.operatingIncome', group: 'income'},
  {key: 'net_income', label: 'fhist.netIncome', group: 'income'},
  {key: 'operating_cash_flow', label: 'fhist.operatingCashFlow', group: 'income'},
  {key: 'total_assets', label: 'fhist.totalAssets', group: 'balance'},
  {key: 'total_liabilities', label: 'fhist.totalLiabilities', group: 'balance'},
  {key: 'stockholders_equity', label: 'fhist.equity', group: 'balance'},
];

// One normalized period point: a sort/align KEY (year string for annual, end-date for quarterly —
// both YYYY-prefixed so a lexical sort is chronological), a display LABEL, and the value.
type Pt = {key: string; label: string; val: number; derived?: boolean};

function seriesFor(hist: Hist, rowKey: keyof Hist, quarterly: boolean): Pt[] {
  if (quarterly) {
    const q = (hist[`${rowKey}_q` as keyof Hist] as QuarterValue[] | undefined) ?? [];
    return q.map(qv => ({key: qv.end, label: qv.label, val: qv.val, derived: qv.derived}));
  }
  const a = (hist[rowKey] as YearValue[] | undefined) ?? [];
  return a.map(yv => ({key: String(yv.year), label: `FY${yv.fy}`, val: yv.val}));
}

const hasAnnual = (h: Hist) => ROWS.some(r => ((h[r.key] as YearValue[] | undefined)?.length ?? 0) > 0);
const hasQuarterly = (h: Hist) =>
  ROWS.some(r => ((h[`${r.key}_q` as keyof Hist] as QuarterValue[] | undefined)?.length ?? 0) > 0);

/**
 * A tiny inline trend sparkline (oldest → newest, left → right) for one row's series. Neutral
 * single color — it shows the SHAPE of the reported history, not a buy/sell judgment. Renders
 * nothing for fewer than two points. Pure SVG, no deps.
 */
function Sparkline({values, dark}: {values: number[]; dark: boolean}) {
  if (values.length < 2) return null;
  const w = 56;
  const h = 16;
  const pad = 1.5;
  const min = Math.min(...values);
  const max = Math.max(...values);
  const range = max - min || 1;
  const pts = values
    .map((v, i) => {
      const x = pad + (i / (values.length - 1)) * (w - 2 * pad);
      const y = h - pad - ((v - min) / range) * (h - 2 * pad);
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(' ');
  return (
    <svg width={w} height={h} viewBox={`0 0 ${w} ${h}`} className="shrink-0" aria-hidden>
      <polyline
        points={pts}
        fill="none"
        stroke={dark ? '#2dd4bf' : '#0d9488'}
        strokeWidth="1.25"
        strokeLinejoin="round"
        strokeLinecap="round"
      />
    </svg>
  );
}

/**
 * Multi-year + multi-quarter financial-history table on the stock detail page: the headline DOLLAR
 * lines (revenue / gross profit / operating income / net income / operating cash flow) from SEC
 * XBRL, via GET /v1/stocks/{t}/fundamentals (history field). An Annual ⇄ Quarterly toggle switches
 * between the ≤10 fiscal-year series and the last ~8 standalone single quarters. Every value is a
 * real reported figure (a derived Q4 = full year − 9-month YTD); a missing cell shows "—" (never
 * interpolated). Per-share metrics are excluded (a decade of EPS would mix pre/post-split bases).
 * Hides for non-US / funds / no history. Newest period first; the label column is pinned so it
 * stays visible while the periods scroll horizontally on narrow screens.
 */
export function FinancialsHistoryTable({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [hist, setHist] = useState<Hist | null>(null);
  const [status, setStatus] = useState<Status>('loading');
  const [quarterly, setQuarterly] = useState(false);

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    getFundamentals(ticker, c.signal).then(
      f => {
        const h = f.history;
        if (h && (hasAnnual(h) || hasQuarterly(h))) {
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

  const annualOK = hasAnnual(hist);
  const quarterlyOK = hasQuarterly(hist);
  // If the chosen view has no data (e.g. an only-cumulative filer with no quarterly), fall back.
  const q = quarterly ? quarterlyOK : !annualOK && quarterlyOK;

  // Display rows: income lines (backend), then computed MARGINS (% of revenue — real reported
  // values divided, never fabricated; a missing period is simply skipped), then the balance-sheet
  // lines (backend). Each row carries its number format + group.
  const base = ROWS.map(r => ({
    id: r.key as string,
    label: r.label,
    group: r.group as 'income' | 'margin' | 'balance',
    format: 'usd' as 'usd' | 'pct',
    data: seriesFor(hist, r.key, q),
  }));
  const revByKey = new Map((base.find(r => r.id === 'revenue')?.data ?? []).map(p => [p.key, p.val]));
  const marginOf = (numId: string, label: string) => ({
    id: `m_${numId}`,
    label,
    group: 'margin' as const,
    format: 'pct' as const,
    data: (base.find(r => r.id === numId)?.data ?? [])
      .map((p): Pt | null => {
        const rv = revByKey.get(p.key);
        return rv && rv !== 0 ? {key: p.key, label: p.label, val: p.val / rv, derived: p.derived} : null;
      })
      .filter((x): x is Pt => x !== null),
  });
  const rows = [
    ...base.filter(r => r.group === 'income'),
    marginOf('gross_profit', 'fhist.grossMargin'),
    marginOf('operating_income', 'fhist.operatingMargin'),
    marginOf('net_income', 'fhist.netMargin'),
    ...base.filter(r => r.group === 'balance'),
  ].filter(r => r.data.length > 0);
  // Column set = union of period keys across the kept rows, NEWEST first (YYYY-prefixed → lexical).
  const colLabel = new Map<string, string>();
  for (const r of rows) for (const p of r.data) colLabel.set(p.key, p.label);
  const cols = [...colLabel.keys()].sort((a, b) => (a < b ? 1 : a > b ? -1 : 0));

  const neg = dark ? 'text-rose-400' : 'text-rose-500';
  const pin = cx('sticky left-0 z-10', t.card); // pinned label column keeps its card background

  const Toggle = ({value, label}: {value: boolean; label: string}) => (
    <button
      type="button"
      onClick={() => setQuarterly(value)}
      aria-pressed={q === value}
      className={cx('rounded-md px-2 py-0.5 text-[11.5px] font-semibold transition', q === value ? cx(t.chip, t.chipText) : t.faint)}
    >
      {label}
    </button>
  );

  return (
    <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <h2 className={cx('text-[14px] font-bold', t.text)}>{tr('fhist.title')}</h2>
        {annualOK && quarterlyOK && (
          <div className={cx('inline-flex gap-1 rounded-lg border p-0.5', t.card, t.border)}>
            <Toggle value={false} label={tr('fhist.annual')} />
            <Toggle value={true} label={tr('fhist.quarterly')} />
          </div>
        )}
        <span className={cx('ml-auto text-[10.5px]', t.faint)}>{tr('fund.source')}</span>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-[12.5px] tabular-nums">
          <thead>
            <tr>
              <th className={cx(pin, 'py-1.5 pr-3 text-left')} />
              {cols.map((k, i) => (
                <th
                  key={k}
                  className={cx('whitespace-nowrap px-2.5 py-1.5 text-right font-semibold', i === 0 ? t.text : t.faint)}
                >
                  {colLabel.get(k)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((r, idx) => {
              const m = new Map(r.data.map(p => [p.key, p]));
              // A group divider ("Margins" / "Balance sheet") before the first row of a new section.
              const headerKey =
                r.group === 'margin' ? 'fhist.margins' : r.group === 'balance' ? 'fhist.balanceSheet' : null;
              const showHeader = headerKey && (idx === 0 || rows[idx - 1].group !== r.group);
              return (
                <Fragment key={r.id}>
                  {showHeader && (
                    <tr>
                      <td
                        colSpan={cols.length + 1}
                        className={cx('pt-3 pb-1 text-left text-[10.5px] font-bold uppercase tracking-wide', t.faint)}
                      >
                        {tr(headerKey)}
                      </td>
                    </tr>
                  )}
                  <tr className={cx('border-t', t.border)}>
                    <td className={cx(pin, 'py-1.5 pr-3 text-left font-medium', t.sub)}>
                      <span className="flex items-center gap-2">
                        <span className="whitespace-nowrap">{tr(r.label)}</span>
                        <Sparkline values={r.data.map(p => p.val)} dark={dark} />
                      </span>
                    </td>
                    {cols.map((k, i) => {
                      const p = m.get(k);
                      return (
                        <td
                          key={k}
                          title={p?.derived ? tr('fhist.derivedNote') : undefined}
                          className={cx(
                            'whitespace-nowrap px-2.5 py-1.5 text-right',
                            p != null && p.val < 0 ? neg : t.text,
                            i === 0 && 'font-semibold',
                          )}
                        >
                          {p == null ? (
                            '—'
                          ) : (
                            <>
                              {r.format === 'pct' ? `${(p.val * 100).toFixed(1)}%` : fmtCompactUSD(p.val)}
                              {p.derived && <sup className={cx('ml-0.5', t.faint)}>†</sup>}
                            </>
                          )}
                        </td>
                      );
                    })}
                  </tr>
                </Fragment>
              );
            })}
          </tbody>
        </table>
      </div>
      {q && rows.some(r => r.data.some(p => p.derived)) && (
        <p className={cx('mt-2 text-[10.5px]', t.faint)}>{tr('fhist.derivedNote')}</p>
      )}
    </section>
  );
}
