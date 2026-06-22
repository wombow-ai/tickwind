'use client';

import {useEffect, useState} from 'react';
import LocalLink from '@/components/LocalLink';
import {getFundamentals, getQuote, type Fundamentals, type Quote} from '@/lib/api';
import {useLang} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, fmtCompactUSD, tok} from '@/lib/ui';

type Loaded = {f: Fundamentals | null; q: Quote | null};

/**
 * CompareTable renders the side-by-side metric table for two tickers, fetched CLIENT-SIDE
 * (getFundamentals + getQuote per ticker). It is deliberately a client component: the SSR
 * fetch of these endpoints through the Cloudflare tunnel is unreliable at Vercel build time
 * (it baked the page as the route loader), so the page renders its crawlable chrome server-side
 * and self-heals the numbers here in the browser, where the API is reachable. Every number is
 * Go-computed (anti-hallucination-safe); the table declares NO winner — just the figures.
 */
export function CompareTable({a, b}: {a: string; b: string}) {
  const dark = useDark();
  const t = tok(dark);
  const zh = useLang().lang === 'zh';
  const [la, setLa] = useState<Loaded | null>(null);
  const [lb, setLb] = useState<Loaded | null>(null);

  useEffect(() => {
    const c = new AbortController();
    const load = async (tk: string): Promise<Loaded> => {
      const [fR, qR] = await Promise.allSettled([
        getFundamentals(tk, c.signal),
        getQuote(tk, c.signal),
      ]);
      return {f: fR.status === 'fulfilled' ? fR.value : null, q: qR.status === 'fulfilled' ? qR.value : null};
    };
    load(a).then(r => !c.signal.aborted && setLa(r));
    load(b).then(r => !c.signal.aborted && setLb(r));
    return () => c.abort();
  }, [a, b]);

  if (!la || !lb) {
    return <div className={cx('h-72 rounded-2xl', t.skel)} />;
  }

  const dash = '—';
  const num = (v: number | null | undefined, digits = 2) =>
    v === null || v === undefined || !isFinite(v) ? dash : v.toFixed(digits);
  const usd = (v: number | null | undefined) =>
    v === null || v === undefined || !isFinite(v) || v === 0 ? dash : fmtCompactUSD(v);
  const dayChange = (q: Quote | null): {text: string; pos: boolean | null} => {
    if (!q || !q.prev_close || q.prev_close <= 0 || !q.price) return {text: dash, pos: null};
    const pct = (q.price / q.prev_close - 1) * 100;
    return {text: `${pct > 0 ? '+' : ''}${pct.toFixed(2)}%`, pos: pct >= 0};
  };
  const dcA = dayChange(la.q);
  const dcB = dayChange(lb.q);
  const peVal = (l: Loaded) => (l.f && l.f.pe === null ? (zh ? '亏损' : 'loss') : num(l.f?.pe));

  type Row = {label: string; a: string; b: string; aTone?: boolean | null; bTone?: boolean | null};
  const rows: Row[] = [
    {label: zh ? '股价' : 'Price', a: la.q?.price ? `$${num(la.q.price)}` : dash, b: lb.q?.price ? `$${num(lb.q.price)}` : dash},
    {label: zh ? '当日涨跌' : 'Day change', a: dcA.text, b: dcB.text, aTone: dcA.pos, bTone: dcB.pos},
    {label: zh ? '市值' : 'Market cap', a: usd(la.f?.market_cap), b: usd(lb.f?.market_cap)},
    {label: zh ? '市盈率 (P/E)' : 'P/E', a: peVal(la), b: peVal(lb)},
    {label: zh ? '市净率 (P/B)' : 'P/B', a: num(la.f?.pb), b: num(lb.f?.pb)},
    {label: zh ? '营收' : 'Revenue', a: usd(la.f?.revenue), b: usd(lb.f?.revenue)},
    {label: zh ? '净利润' : 'Net income', a: usd(la.f?.net_income), b: usd(lb.f?.net_income), aTone: la.f ? la.f.net_income >= 0 : null, bTone: lb.f ? lb.f.net_income >= 0 : null},
    {label: zh ? '摊薄每股收益' : 'Diluted EPS', a: la.f ? `$${num(la.f.eps_diluted)}` : dash, b: lb.f ? `$${num(lb.f.eps_diluted)}` : dash},
  ];

  const noData = !la.f && !la.q?.price && !lb.f && !lb.q?.price;
  if (noData) {
    return (
      <p className={cx('rounded-2xl border p-6 text-center text-[13px]', t.border, t.sub)}>
        {zh ? '暂无这两只股票的对比数据。' : 'No comparison data available for these two stocks.'}
      </p>
    );
  }

  const toneClass = (tone: boolean | null | undefined) =>
    tone === null || tone === undefined
      ? cx('text-slate-900 dark:text-slate-100')
      : tone
        ? cx(dark ? 'text-emerald-300' : 'text-emerald-600')
        : cx(dark ? 'text-rose-300' : 'text-rose-500');

  const nameA = la.f?.name || a;
  const nameB = lb.f?.name || b;

  return (
    <div className={cx('overflow-hidden rounded-2xl border', t.border)}>
      <table className="w-full text-[13.5px]">
        <thead>
          <tr className={dark ? 'bg-slate-900/50' : 'bg-slate-50'}>
            <th className={cx('px-3 py-2.5 text-left text-[11px] font-semibold uppercase tracking-wide', t.faint)}>
              {zh ? '指标' : 'Metric'}
            </th>
            {[{t: a, n: nameA}, {t: b, n: nameB}].map(s => (
              <th key={s.t} className="px-3 py-2.5 text-right">
                <LocalLink href={`/stock/${encodeURIComponent(s.t)}`} className={cx('font-bold hover:underline', t.text)}>
                  {s.t}
                </LocalLink>
                <span className={cx('block max-w-[140px] truncate text-[10.5px] font-normal', t.faint)} title={s.n}>
                  {s.n}
                </span>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={r.label} className={i % 2 ? (dark ? 'bg-slate-900/20' : 'bg-slate-50/40') : ''}>
              <td className={cx('px-3 py-2.5', t.sub)}>{r.label}</td>
              <td className={cx('px-3 py-2.5 text-right font-semibold tabular-nums', toneClass(r.aTone))}>{r.a}</td>
              <td className={cx('px-3 py-2.5 text-right font-semibold tabular-nums', toneClass(r.bTone))}>{r.b}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
