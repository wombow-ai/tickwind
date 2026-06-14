'use client';

import Link from '@/components/LocalLink';
import {useCallback, useEffect, useMemo, useState} from 'react';
import {getHoldings, type Holding} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, fmtPrice, marketCurrency, tok} from '@/lib/ui';
import {useQuotes} from '@/lib/useQuotes';

type Tokens = ReturnType<typeof tok>;

function guessMarket(ticker: string): string {
  if (ticker.endsWith('.HK')) return 'HK';
  if (ticker.endsWith('.KS') || ticker.endsWith('.KQ')) return 'KR';
  if (ticker.endsWith('.TW') || ticker.endsWith('.TWO')) return 'TW';
  return 'US';
}

/**
 * The signed-in user's portfolio: total + per-stock positions, valued live.
 * Login is handled by the surrounding shell (`/me`), so this assumes a user.
 */
export function PortfolioView() {
  const {getToken} = useAuth();
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [holdings, setHoldings] = useState<Holding[]>([]);
  const [loaded, setLoaded] = useState(false);

  const load = useCallback(() => {
    getToken().then(token =>
      getHoldings(token).then(
        r => {
          setHoldings(r.holdings ?? []);
          setLoaded(true);
        },
        () => setLoaded(true),
      ),
    );
  }, [getToken]);
  useEffect(() => {
    load();
  }, [load]);

  const tickers = useMemo(() => holdings.map(h => h.ticker), [holdings]);
  const quotes = useQuotes(tickers);

  // Totals are summed nominally (US-first; mixed-currency portfolios are a known
  // v1 simplification). Rows without a live price are excluded from the total.
  let totalValue = 0;
  let totalCost = 0;
  let totalDayPL = 0; // today's P&L: Σ (price − prev_close) × shares (nominal-currency, US-first)
  let totalPrevValue = 0; // prior-close value of priced rows, for the day-% denominator
  let priced = 0;
  for (const h of holdings) {
    totalCost += h.shares * h.avg_cost;
    const q = quotes.get(h.ticker);
    if (q && q.price > 0) {
      totalValue += h.shares * q.price;
      priced++;
      if (q.prev_close && q.prev_close > 0) {
        totalDayPL += h.shares * (q.price - q.prev_close);
        totalPrevValue += h.shares * q.prev_close;
      }
    }
  }
  const totalPL = totalValue - totalCost;
  const totalPLPct = totalCost > 0 ? (totalPL / totalCost) * 100 : 0;
  const totalDayPLPct = totalPrevValue > 0 ? (totalDayPL / totalPrevValue) * 100 : 0;
  const up = totalPL >= 0;
  const dayUp = totalDayPL >= 0;
  const plColor = (good: boolean) =>
    good
      ? dark
        ? 'text-emerald-400'
        : 'text-emerald-600'
      : dark
        ? 'text-rose-400'
        : 'text-rose-500';
  const plCol = plColor(up);
  const dayCol = plColor(dayUp);

  if (!loaded) {
    return <div className={cx('h-40 rounded-3xl border', t.card, t.border, t.skel)} />;
  }
  if (holdings.length === 0) {
    return (
      <div className={cx('rounded-3xl border p-8 text-center', t.card, t.border, t.soft)}>
        <p className={cx('text-[14px] font-semibold', t.text)}>{tr('portfolio.empty')}</p>
        <p className={cx('mt-1 text-[13.5px]', t.sub)}>{tr('portfolio.emptySub')}</p>
      </div>
    );
  }

  return (
    <>
      <div className={cx('mb-5 grid grid-cols-2 gap-3 rounded-2xl border p-4 sm:grid-cols-4', t.card, t.border, t.soft)}>
        <Sum t={t} label={tr('portfolio.total')} value={fmtPrice('$', totalValue)} />
        <Sum t={t} label={tr('portfolio.totalCost')} value={fmtPrice('$', totalCost)} />
        <div className="flex flex-col">
          <span className={cx('text-[11px]', t.faint)}>{tr('portfolio.dayPL')}</span>
          <span className={cx('text-[15px] font-semibold tabular-nums', dayCol)}>
            {dayUp ? '+' : '−'}
            {fmtPrice('$', Math.abs(totalDayPL))} ({dayUp ? '+' : '−'}
            {Math.abs(totalDayPLPct).toFixed(2)}%)
          </span>
        </div>
        <div className="flex flex-col">
          <span className={cx('text-[11px]', t.faint)}>{tr('portfolio.totalPL')}</span>
          <span className={cx('text-[15px] font-semibold tabular-nums', plCol)}>
            {up ? '+' : '−'}
            {fmtPrice('$', Math.abs(totalPL))} ({up ? '+' : '−'}
            {Math.abs(totalPLPct).toFixed(2)}%)
          </span>
        </div>
      </div>

      <div className={cx('overflow-hidden rounded-2xl border', t.card, t.border, t.soft)}>
        <div
          className={cx(
            'grid grid-cols-12 gap-2 border-b px-4 py-2 text-[10.5px] font-semibold uppercase tracking-wide',
            t.border,
            t.faint,
          )}
        >
          <span className="col-span-3">{tr('portfolio.ticker')}</span>
          <span className="col-span-2 text-right">{tr('holdings.shares')}</span>
          <span className="col-span-2 text-right">{tr('holdings.avgCost')}</span>
          <span className="col-span-2 text-right">{tr('portfolio.price')}</span>
          <span className="col-span-3 text-right">{tr('holdings.value')}</span>
        </div>
        {holdings.map(h => {
          const q = quotes.get(h.ticker);
          const cur = marketCurrency(guessMarket(h.ticker));
          const price = q?.price ?? 0;
          const has = price > 0;
          const value = h.shares * price;
          const cost = h.shares * h.avg_cost;
          const pl = value - cost;
          const rUp = pl >= 0;
          const rCol = plColor(rUp);
          // Today's move (vs prior close) and this row's share of the portfolio.
          const dayPct = q?.prev_close && q.prev_close > 0 ? (price / q.prev_close - 1) * 100 : null;
          const alloc = totalValue > 0 ? (value / totalValue) * 100 : 0;
          return (
            <div
              key={h.id}
              className={cx('grid grid-cols-12 items-center gap-2 border-b px-4 py-2.5 text-[13px] last:border-0', t.border)}
            >
              <Link
                href={`/stock/${encodeURIComponent(h.ticker)}`}
                className={cx('col-span-3 font-bold hover:opacity-80', t.text)}
              >
                {h.ticker}
              </Link>
              <span className={cx('col-span-2 text-right tabular-nums', t.sub)}>{h.shares}</span>
              <span className={cx('col-span-2 text-right tabular-nums', t.sub)}>
                {fmtPrice(cur, h.avg_cost)}
              </span>
              <span className="col-span-2 flex flex-col items-end">
                <span className={cx('tabular-nums', t.sub)}>{has ? fmtPrice(cur, price) : '—'}</span>
                {dayPct !== null && (
                  <span className={cx('text-[11px] tabular-nums', plColor(dayPct >= 0))}>
                    {dayPct >= 0 ? '+' : '−'}
                    {Math.abs(dayPct).toFixed(2)}%
                  </span>
                )}
              </span>
              <span className="col-span-3 flex flex-col items-end">
                {has ? (
                  <>
                    <span className={cx('font-semibold tabular-nums', t.text)}>{fmtPrice(cur, value)}</span>
                    <span className="text-[11px] tabular-nums">
                      <span className={rCol}>
                        {rUp ? '+' : '−'}
                        {Math.abs(cost > 0 ? (pl / cost) * 100 : 0).toFixed(1)}%
                      </span>
                      <span className={cx('ml-1.5', t.faint)} title={tr('portfolio.allocation')}>
                        {tr('portfolio.allocShort')} {alloc.toFixed(1)}%
                      </span>
                    </span>
                  </>
                ) : (
                  <span className={t.faint}>—</span>
                )}
              </span>
            </div>
          );
        })}
      </div>
      {priced < holdings.length && (
        <p className={cx('mt-2 text-[11px]', t.faint)}>{tr('portfolio.someUnpriced')}</p>
      )}
      <p className={cx('mt-3 text-[10.5px]', t.faint)}>{tr('portfolio.disclaimer')}</p>
    </>
  );
}

function Sum({t, label, value}: {t: Tokens; label: string; value: string}) {
  return (
    <div className="flex flex-col">
      <span className={cx('text-[11px]', t.faint)}>{label}</span>
      <span className={cx('text-[15px] font-semibold tabular-nums', t.text)}>{value}</span>
    </div>
  );
}
