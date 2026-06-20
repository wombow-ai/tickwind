'use client';

import {useEffect, useState} from 'react';
import {type Holding, getHoldings, getWatchlist} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {useQuotes} from '@/lib/useQuotes';
import {cx, tok} from '@/lib/ui';

// Inline portfolio widgets surfaced in chat (Product C). They render the user's OWN
// watchlist/holdings — fetched authed (the backend scopes to the user) + priced live via
// the shared quote stream. Numbers are real (Go-stored positions × live prices), computed
// here for display; the chat model never sees them (the contract holds).

const money = (n: number) => (n >= 0 ? '$' : '-$') + Math.abs(n).toLocaleString(undefined, {maximumFractionDigits: n >= 1000 ? 0 : 2});
const pct = (n: number) => `${n >= 0 ? '+' : ''}${(n * 100).toFixed(1)}%`;
const up = (n: number, dark: boolean) => (n > 0 ? (dark ? 'text-emerald-400' : 'text-emerald-600') : n < 0 ? (dark ? 'text-rose-400' : 'text-rose-600') : '');

export function ChatPortfolioWidget({type}: {type: string}) {
  if (type === 'holdings_pnl') return <HoldingsPnl />;
  if (type === 'watchlist_summary') return <WatchlistSummary />;
  if (type === 'portfolio_heatmap') return <PortfolioHeatmap />;
  return null;
}

function useHoldings(): {holdings: Holding[]; loaded: boolean} {
  const {getToken} = useAuth();
  const [holdings, setHoldings] = useState<Holding[]>([]);
  const [loaded, setLoaded] = useState(false);
  useEffect(() => {
    const c = new AbortController();
    (async () => {
      try {
        const r = await getHoldings(await getToken(), c.signal);
        setHoldings(r.holdings ?? []);
      } catch {
        /* empty */
      } finally {
        setLoaded(true);
      }
    })();
    return () => c.abort();
  }, [getToken]);
  return {holdings, loaded};
}

function Empty({text}: {text: string}) {
  const t = tok(useDark());
  return <div className={cx('my-2 rounded-xl border p-3 text-[12px]', t.border, t.faint)}>{text}</div>;
}

function HoldingsPnl() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {holdings, loaded} = useHoldings();
  const quotes = useQuotes(holdings.map(h => h.ticker));
  if (!loaded) return null;
  if (holdings.length === 0) return <Empty text={tr('chat.pw.noHoldings')} />;
  const rows = holdings.map(h => {
    const price = quotes.get(h.ticker)?.price ?? 0;
    const value = price * h.shares;
    const gainPct = h.avg_cost > 0 && price > 0 ? price / h.avg_cost - 1 : 0;
    return {h, price, value, gainPct};
  });
  const total = rows.reduce((s, r) => s + r.value, 0);
  return (
    <div className={cx('my-2 overflow-hidden rounded-xl border', t.border)}>
      <table className="w-full text-[11.5px]">
        <thead className={cx(t.faint, dark ? 'bg-slate-800/40' : 'bg-slate-50')}>
          <tr>
            <th className="px-2 py-1.5 text-left font-semibold">{tr('chat.pw.ticker')}</th>
            <th className="px-2 py-1.5 text-right font-semibold">{tr('chat.pw.value')}</th>
            <th className="px-2 py-1.5 text-right font-semibold">{tr('chat.pw.gain')}</th>
            <th className="px-2 py-1.5 text-right font-semibold">{tr('chat.pw.weight')}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map(r => (
            <tr key={r.h.ticker} className={cx('border-t', t.border)}>
              <td className={cx('px-2 py-1.5 font-semibold', t.text)}>{r.h.ticker}</td>
              <td className={cx('px-2 py-1.5 text-right tabular-nums', t.sub)}>{r.price > 0 ? money(r.value) : '—'}</td>
              <td className={cx('px-2 py-1.5 text-right tabular-nums', up(r.gainPct, dark))}>{r.price > 0 ? pct(r.gainPct) : '—'}</td>
              <td className={cx('px-2 py-1.5 text-right tabular-nums', t.faint)}>{total > 0 && r.price > 0 ? `${((r.value / total) * 100).toFixed(0)}%` : '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function WatchlistSummary() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {getToken} = useAuth();
  const [tickers, setTickers] = useState<string[]>([]);
  const [loaded, setLoaded] = useState(false);
  useEffect(() => {
    const c = new AbortController();
    (async () => {
      try {
        const r = await getWatchlist(await getToken(), c.signal);
        setTickers(r.tickers ?? []);
      } catch {
        /* empty */
      } finally {
        setLoaded(true);
      }
    })();
    return () => c.abort();
  }, [getToken]);
  const quotes = useQuotes(tickers);
  if (!loaded) return null;
  if (tickers.length === 0) return <Empty text={tr('chat.pw.noWatchlist')} />;
  return (
    <div className={cx('my-2 grid grid-cols-2 gap-1.5 sm:grid-cols-3', '')}>
      {tickers.map(tk => {
        const q = quotes.get(tk);
        const chg = q && q.prev_close ? q.price / q.prev_close - 1 : 0;
        return (
          <div key={tk} className={cx('rounded-lg border px-2 py-1.5', t.card, t.border)}>
            <div className={cx('text-[11.5px] font-bold', t.text)}>{tk}</div>
            <div className="flex items-baseline justify-between">
              <span className={cx('text-[11px] tabular-nums', t.sub)}>{q?.price ? money(q.price) : '—'}</span>
              <span className={cx('text-[10.5px] tabular-nums', up(chg, dark))}>{q?.prev_close ? pct(chg) : ''}</span>
            </div>
          </div>
        );
      })}
    </div>
  );
}

function PortfolioHeatmap() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {holdings, loaded} = useHoldings();
  const quotes = useQuotes(holdings.map(h => h.ticker));
  if (!loaded) return null;
  if (holdings.length === 0) return <Empty text={tr('chat.pw.noHoldings')} />;
  const tiles = holdings.map(h => {
    const price = quotes.get(h.ticker)?.price ?? 0;
    const gainPct = h.avg_cost > 0 && price > 0 ? price / h.avg_cost - 1 : 0;
    return {ticker: h.ticker, gainPct, value: price * h.shares};
  });
  const max = Math.max(...tiles.map(t2 => t2.value), 1);
  return (
    <div className="my-2 grid grid-cols-3 gap-1.5 sm:grid-cols-4">
      {tiles.map(tile => {
        const intensity = Math.min(0.5, 0.12 + (Math.abs(tile.gainPct) * 4)); // cap opacity
        const bg = tile.gainPct >= 0
          ? `rgba(16,185,129,${intensity})`
          : `rgba(244,63,94,${intensity})`;
        const big = tile.value > max * 0.5;
        return (
          <div
            key={tile.ticker}
            title={tile.value > 0 ? `${tile.ticker} · ${money(tile.value)} · ${pct(tile.gainPct)}` : tile.ticker}
            className={cx('flex flex-col items-center justify-center rounded-lg px-2 py-3 text-center', big ? 'col-span-2' : '')}
            style={{backgroundColor: bg}}
          >
            <span className={cx('text-[12px] font-bold', t.text)}>{tile.ticker}</span>
            <span className={cx('text-[11px] font-semibold tabular-nums', up(tile.gainPct, dark))}>{pct(tile.gainPct)}</span>
          </div>
        );
      })}
    </div>
  );
}
