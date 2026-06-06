'use client';

import {Lock, Plus, Search, Wind} from 'lucide-react';
import Link from 'next/link';
import {useEffect, useMemo, useState} from 'react';
import {
  addToWatchlist,
  getBarsBatch,
  getStock,
  getWatchlist,
  removeFromWatchlist,
  type Security,
} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {POPULAR_TICKERS, SUGGESTED_TICKERS} from '@/lib/config';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, tok} from '@/lib/ui';
import {useQuotes} from '@/lib/useQuotes';
import {StockCard} from '@/components/StockCard';
import {useToast} from '@/components/ui/Toast';

/** Guesses a listing market from a ticker suffix. */
function guessMarket(ticker: string): string {
  if (ticker.endsWith('.HK')) return 'HK';
  if (ticker.endsWith('.KS') || ticker.endsWith('.KQ')) return 'KR';
  return 'US';
}

/** A minimal security used until the real one resolves. */
function placeholder(ticker: string): Security {
  return {ticker, name: ticker, market: guessMarket(ticker)};
}

/** The data-first home board: a watchlist (auth) or popular stocks (anon). */
export function Board() {
  const {user, loading: authLoading, getToken} = useAuth();
  const {toast} = useToast();
  const dark = useDark();
  const t = tok(dark);
  const isAuthed = !!user;

  const [tickers, setTickers] = useState<string[]>([...POPULAR_TICKERS]);
  const [securities, setSecurities] = useState<Record<string, Security>>({});
  const [barsMap, setBarsMap] = useState<Record<string, number[]>>({});
  const [listLoading, setListLoading] = useState(false);
  const [adding, setAdding] = useState('');

  const quotes = useQuotes(tickers);
  const tickerKey = tickers.join(',');

  // Load the list: the user's watchlist when signed in, else popular tickers.
  useEffect(() => {
    if (authLoading) return;
    if (!isAuthed) {
      setTickers([...POPULAR_TICKERS]);
      setListLoading(false);
      return;
    }
    let active = true;
    setListLoading(true);
    (async () => {
      try {
        const token = await getToken();
        const res = await getWatchlist(token);
        if (active) setTickers(res.tickers);
      } catch {
        if (active) setTickers([]);
      } finally {
        if (active) setListLoading(false);
      }
    })();
    return () => {
      active = false;
    };
  }, [authLoading, isAuthed, getToken]);

  // Resolve security metadata for any unresolved tickers.
  useEffect(() => {
    const controller = new AbortController();
    for (const ticker of tickers) {
      if (securities[ticker]) continue;
      getStock(ticker, controller.signal).then(
        sec => setSecurities(prev => ({...prev, [ticker]: sec})),
        () =>
          setSecurities(prev =>
            prev[ticker] ? prev : {...prev, [ticker]: placeholder(ticker)},
          ),
      );
    }
    return () => controller.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tickerKey]);

  // Batched trend sparklines for the whole board (one request).
  useEffect(() => {
    if (tickers.length === 0) {
      setBarsMap({});
      return;
    }
    const controller = new AbortController();
    getBarsBatch(tickers, controller.signal).then(
      r => setBarsMap(r.bars),
      () => setBarsMap({}),
    );
    return () => controller.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tickerKey]);

  async function add(raw: string) {
    const ticker = raw.trim().toUpperCase();
    if (!ticker) return;
    if (!isAuthed) {
      toast('Log in to build your watchlist');
      return;
    }
    if (tickers.includes(ticker)) {
      toast(`${ticker} is already on your watchlist`);
      return;
    }
    setTickers(prev => [...prev, ticker]); // optimistic
    setAdding('');
    try {
      const token = await getToken();
      const res = await addToWatchlist(token, ticker);
      setTickers(res.tickers);
      toast(`Added ${ticker}`, {tone: 'ok'});
    } catch {
      setTickers(prev => prev.filter(x => x !== ticker));
      toast(`Couldn't add ${ticker}`);
    }
  }

  async function remove(ticker: string) {
    const prev = tickers;
    setTickers(p => p.filter(x => x !== ticker)); // optimistic
    try {
      const token = await getToken();
      const res = await removeFromWatchlist(token, ticker);
      setTickers(res.tickers);
      toast(`Removed ${ticker}`, {
        action: {label: 'Undo', fn: () => add(ticker)},
      });
    } catch {
      setTickers(prev);
      toast(`Couldn't remove ${ticker}`);
    }
  }

  const cards = useMemo(
    () => tickers.map(tk => securities[tk] ?? placeholder(tk)),
    [tickers, securities],
  );

  return (
    <div>
      <header className="mb-7 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 className={cx('text-[26px] font-bold tracking-tight', t.text)}>
            {isAuthed ? 'Your watchlist' : 'Popular stocks'}
          </h1>
          <p className={cx('mt-1 text-[13.5px]', t.sub)}>
            {isAuthed
              ? `${tickers.length} ${tickers.length === 1 ? 'stock' : 'stocks'} · live across every session`
              : 'Live prices across every session — log in to build your own.'}
          </p>
        </div>

        {isAuthed ? (
          <form
            onSubmit={e => {
              e.preventDefault();
              add(adding);
            }}
            className={cx(
              'flex items-center gap-2 rounded-xl border p-1.5 sm:w-72',
              t.card,
              t.border,
              t.soft,
            )}
          >
            <Search size={16} className={cx('ml-1.5', t.faint)} />
            <input
              value={adding}
              onChange={e => setAdding(e.target.value)}
              placeholder="Add a ticker…"
              className={cx(
                'flex-1 bg-transparent text-[13.5px] uppercase tracking-wide outline-none',
                dark
                  ? 'text-slate-100 placeholder:text-slate-500'
                  : 'text-slate-900 placeholder:text-slate-400',
              )}
            />
            <button
              type="submit"
              className={cx(
                'inline-flex h-7 w-7 items-center justify-center rounded-lg',
                btnPrimary(dark),
              )}
              aria-label="Add ticker"
            >
              <Plus size={16} />
            </button>
          </form>
        ) : (
          <Link
            href="/login"
            className={cx(
              'inline-flex items-center gap-1.5 self-start rounded-full border px-3.5 py-2 text-[13px] font-semibold sm:self-auto',
              t.border,
              t.ghost,
              t.text,
            )}
          >
            <Lock size={14} className={t.sub} /> Log in to customize
          </Link>
        )}
      </header>

      {listLoading && tickers.length === 0 ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {[0, 1, 2, 3, 4, 5].map(i => (
            <div
              key={i}
              className={cx('h-[132px] rounded-2xl border', t.card, t.border, t.skel)}
            />
          ))}
        </div>
      ) : isAuthed && tickers.length === 0 ? (
        <div
          className={cx(
            'flex flex-col items-center rounded-3xl border p-12 text-center',
            t.card,
            t.border,
            t.soft,
          )}
        >
          <div
            className="mb-5 flex items-center justify-center rounded-2xl"
            style={{
              width: 72,
              height: 72,
              background: dark ? 'rgba(20,184,166,.12)' : 'rgba(13,148,136,.08)',
            }}
          >
            <Wind className={dark ? 'text-teal-300' : 'text-teal-600'} size={30} />
          </div>
          <h3 className={cx('text-[17px] font-semibold', t.text)}>
            Your board is calm and empty
          </h3>
          <p className={cx('mt-1.5 max-w-sm text-[13.5px]', t.sub)}>
            Add your first ticker and watch the price, news and filings flow in —
            across every session.
          </p>
          <div className="mt-5 flex flex-wrap items-center justify-center gap-2">
            {SUGGESTED_TICKERS.map(s => (
              <button
                key={s}
                onClick={() => add(s)}
                className={cx(
                  'inline-flex items-center gap-1.5 rounded-full border px-3.5 py-1.5 text-[13px] font-semibold',
                  t.border,
                  t.ghost,
                  t.text,
                )}
              >
                <Plus size={13} /> {s}
              </button>
            ))}
          </div>
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {cards.map(sec => (
            <StockCard
              key={sec.ticker}
              security={sec}
              quote={quotes.get(sec.ticker)}
              closes={barsMap[sec.ticker]}
              onRemove={isAuthed ? () => remove(sec.ticker) : undefined}
            />
          ))}
        </div>
      )}

      {!isAuthed && (
        <p className={cx('mt-6 text-center text-[12px]', t.faint)}>
          Showing popular US stocks.{' '}
          <Link href="/signup" className={cx('font-semibold', t.accentText)}>
            Create a free account
          </Link>{' '}
          to track your own.
        </p>
      )}
    </div>
  );
}
