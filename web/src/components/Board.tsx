'use client';

import {Lock, MessageSquare, Newspaper, Plus, Wind} from 'lucide-react';
import type {LucideIcon} from 'lucide-react';
import Link from 'next/link';
import {useCallback, useEffect, useMemo, useState} from 'react';
import {
  addToWatchlist,
  getBarsBatch,
  getNewsBatch,
  getSocialBatch,
  getStock,
  getWatchlist,
  removeFromWatchlist,
  type NewsItem,
  type Post,
  type Security,
} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {POPULAR_TICKERS, SUGGESTED_TICKERS} from '@/lib/config';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, tok} from '@/lib/ui';
import {useQuotes} from '@/lib/useQuotes';
import {SearchBox} from '@/components/SearchBox';
import {StockCard} from '@/components/StockCard';
import {TimelineItem} from '@/components/TimelineItem';
import {EmptyState, ErrorState, FeedSkeleton} from '@/components/ui/states';
import {useToast} from '@/components/ui/Toast';

type Status = 'loading' | 'ready' | 'error';
interface Feed<T> {
  status: Status;
  items: T[];
}

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

/**
 * Data-first board: a compact strip of stocks over aggregated News and
 * Discussion feeds. `markets` shows a popular set (the public home `/`);
 * `watchlist` shows the signed-in user's own tickers (`/watchlist`).
 */
export function Board({variant = 'markets'}: {variant?: 'markets' | 'watchlist'}) {
  const {user, loading: authLoading, getToken} = useAuth();
  const {toast} = useToast();
  const dark = useDark();
  const t = tok(dark);
  const isAuthed = !!user;
  const watchlistMode = variant === 'watchlist';

  const [tickers, setTickers] = useState<string[]>([...POPULAR_TICKERS]);
  const [securities, setSecurities] = useState<Record<string, Security>>({});
  const [barsMap, setBarsMap] = useState<Record<string, number[]>>({});
  const [listLoading, setListLoading] = useState(false);
  // Watchlist page only: the feeds are optional and can be focused on one stock.
  const [feedsOpen, setFeedsOpen] = useState(true);
  const [focusTicker, setFocusTicker] = useState<string | null>(null);

  const [news, setNews] = useState<Feed<NewsItem>>({status: 'loading', items: []});
  const [social, setSocial] = useState<Feed<Post>>({status: 'loading', items: []});

  const quotes = useQuotes(tickers);
  const tickerKey = tickers.join(',');
  // Which tickers the feeds aggregate over: a single focused stock, else all.
  const feedTickers =
    watchlistMode && focusTicker && tickers.includes(focusTicker)
      ? [focusTicker]
      : tickers;
  const feedKey = feedTickers.join(',');
  const feedsVisible = !watchlistMode || feedsOpen;

  // Markets → a popular set; Watchlist → the signed-in user's own tickers.
  useEffect(() => {
    if (!watchlistMode) {
      setTickers([...POPULAR_TICKERS]);
      setListLoading(false);
      return;
    }
    if (authLoading) return;
    if (!isAuthed) {
      setTickers([]);
      setListLoading(false);
      return;
    }
    let active = true;
    setListLoading(true);
    (async () => {
      try {
        const token = await getToken();
        const res = await getWatchlist(token);
        if (active) setTickers(res.tickers ?? []);
      } catch {
        if (active) setTickers([]);
      } finally {
        if (active) setListLoading(false);
      }
    })();
    return () => {
      active = false;
    };
  }, [watchlistMode, authLoading, isAuthed, getToken]);

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

  // Batched trend sparklines for the strip (one request).
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

  // Aggregated News + Discussion feeds across the tracked tickers.
  const loadNews = useCallback(() => {
    if (feedTickers.length === 0) {
      setNews({status: 'ready', items: []});
      return;
    }
    setNews(f => ({...f, status: 'loading'}));
    getNewsBatch(feedTickers).then(
      r => setNews({status: 'ready', items: r.news ?? []}),
      () => setNews({status: 'error', items: []}),
    );
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [feedKey]);
  const loadSocial = useCallback(() => {
    if (feedTickers.length === 0) {
      setSocial({status: 'ready', items: []});
      return;
    }
    setSocial(f => ({...f, status: 'loading'}));
    getSocialBatch(feedTickers).then(
      r => setSocial({status: 'ready', items: r.posts ?? []}),
      () => setSocial({status: 'error', items: []}),
    );
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [feedKey]);

  useEffect(() => {
    loadNews();
    loadSocial();
  }, [loadNews, loadSocial]);

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
      toast(`Removed ${ticker}`, {action: {label: 'Undo', fn: () => add(ticker)}});
    } catch {
      setTickers(prev);
      toast(`Couldn't remove ${ticker}`);
    }
  }

  const cards = useMemo(
    () => tickers.map(tk => securities[tk] ?? placeholder(tk)),
    [tickers, securities],
  );
  const showEmptyWatchlist =
    watchlistMode && isAuthed && !listLoading && tickers.length === 0;
  const needLogin = watchlistMode && !authLoading && !isAuthed;

  return (
    <div>
      <header className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 className={cx('text-[26px] font-bold tracking-tight', t.text)}>
            {watchlistMode ? 'Your watchlist' : 'Markets today'}
          </h1>
          <p className={cx('mt-1 text-[13.5px]', t.sub)}>
            {watchlistMode
              ? 'Your stocks — prices, news and chatter in one place.'
              : 'Live prices, then the news and chatter across the market.'}
          </p>
        </div>

        {watchlistMode && isAuthed ? (
          <SearchBox
            onSelect={add}
            placeholder="Add a stock…"
            size="md"
            className="sm:w-72"
          />
        ) : !watchlistMode && !isAuthed ? (
          <Link
            href="/login"
            className={cx(
              'inline-flex items-center gap-1.5 self-start rounded-full border px-3.5 py-2 text-[13px] font-semibold sm:self-auto',
              t.border,
              t.ghost,
              t.text,
            )}
          >
            <Lock size={14} className={t.sub} /> Log in
          </Link>
        ) : null}
      </header>

      {needLogin && (
        <div
          className={cx(
            'flex flex-col items-center rounded-3xl border p-12 text-center',
            t.card,
            t.border,
            t.soft,
          )}
        >
          <div
            className="mb-4 flex items-center justify-center rounded-2xl"
            style={{
              width: 64,
              height: 64,
              background: dark ? 'rgba(20,184,166,.12)' : 'rgba(13,148,136,.08)',
            }}
          >
            <Lock className={dark ? 'text-teal-300' : 'text-teal-600'} size={26} />
          </div>
          <h3 className={cx('text-[16px] font-semibold', t.text)}>
            Log in to see your watchlist
          </h3>
          <p className={cx('mt-1.5 max-w-sm text-[13.5px]', t.sub)}>
            Track your own stocks and clip links from anywhere — free.
          </p>
          <Link
            href="/login"
            className={cx(
              'mt-4 rounded-full px-4 py-2 text-[13px] font-semibold',
              btnPrimary(dark),
            )}
          >
            Log in
          </Link>
        </div>
      )}

      {/* Stock strip */}
      {!needLogin &&
        (showEmptyWatchlist ? (
        <div
          className={cx(
            'mb-8 flex flex-col items-center rounded-3xl border p-10 text-center',
            t.card,
            t.border,
            t.soft,
          )}
        >
          <div
            className="mb-4 flex items-center justify-center rounded-2xl"
            style={{
              width: 64,
              height: 64,
              background: dark ? 'rgba(20,184,166,.12)' : 'rgba(13,148,136,.08)',
            }}
          >
            <Wind className={dark ? 'text-teal-300' : 'text-teal-600'} size={28} />
          </div>
          <h3 className={cx('text-[16px] font-semibold', t.text)}>
            Your board is calm and empty
          </h3>
          <p className={cx('mt-1.5 max-w-sm text-[13.5px]', t.sub)}>
            Add a ticker to follow its price, news and chatter.
          </p>
          <div className="mt-4 flex flex-wrap items-center justify-center gap-2">
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
        <div className="mb-8 flex gap-4 overflow-x-auto pb-2">
          {(listLoading && tickers.length === 0
            ? [...Array(4)].map((_, i) => ({ticker: `skeleton-${i}`}))
            : cards
          ).map((sec, i) =>
            'name' in sec ? (
              <div key={sec.ticker} className="w-[270px] shrink-0">
                <StockCard
                  security={sec as Security}
                  quote={quotes.get(sec.ticker)}
                  closes={barsMap[sec.ticker]}
                  onRemove={
                    watchlistMode && isAuthed
                      ? () => remove(sec.ticker)
                      : undefined
                  }
                />
              </div>
            ) : (
              <div
                key={i}
                className={cx(
                  'h-[150px] w-[270px] shrink-0 rounded-2xl border',
                  t.card,
                  t.border,
                  t.skel,
                )}
              />
            ),
          )}
        </div>
        ))}

      {/* Watchlist: feeds are optional and can be focused on a single stock. */}
      {watchlistMode && !needLogin && tickers.length > 0 && (
        <div className="mb-4 flex flex-wrap items-center gap-2">
          <button
            onClick={() => setFeedsOpen(o => !o)}
            aria-pressed={feedsOpen}
            className={cx(
              'inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-[12.5px] font-semibold',
              t.border,
              t.ghost,
              t.text,
            )}
          >
            {feedsOpen ? 'Hide news & discussion' : 'Show news & discussion'}
          </button>
          {feedsOpen && (
            <div className="flex flex-wrap items-center gap-1.5">
              {[null, ...tickers].map(tk => {
                const active = focusTicker === tk;
                return (
                  <button
                    key={tk ?? '__all'}
                    onClick={() => setFocusTicker(tk)}
                    aria-pressed={active}
                    className={cx(
                      'rounded-full px-2.5 py-1 text-[12px] font-semibold transition',
                      active
                        ? btnPrimary(dark)
                        : cx('border hover:opacity-80', t.border, t.sub),
                    )}
                  >
                    {tk ?? 'All'}
                  </button>
                );
              })}
            </div>
          )}
        </div>
      )}

      {/* News + Discussion */}
      {!needLogin && feedsVisible && (
        <div className="grid gap-6 md:grid-cols-2">
        <FeedColumn
          title="News"
          icon={Newspaper}
          feed={news}
          onRetry={loadNews}
          empty={{
            label: 'No news yet',
            sub: 'Headlines about the stocks you follow will land here.',
            icon: Newspaper,
          }}
          render={(n, last) => (
            <TimelineItem
              key={`${n.ticker}:${n.id}`}
              entry={{kind: 'news', item: n}}
              showTicker
              last={last}
            />
          )}
        />
        <FeedColumn
          title="Discussion"
          icon={MessageSquare}
          feed={social}
          onRetry={loadSocial}
          empty={{
            label: 'No chatter yet',
            sub: 'Posts from StockTwits and Reddit will show up here.',
            icon: MessageSquare,
          }}
          render={(p, last) => (
            <TimelineItem
              key={`${p.ticker}:${p.id}`}
              entry={{kind: 'disc', item: p}}
              showTicker
              last={last}
            />
          )}
        />
      </div>
      )}

      {!watchlistMode && !isAuthed && (
        <p className={cx('mt-8 text-center text-[12px]', t.faint)}>
          Showing popular US stocks.{' '}
          <Link href="/signup" className={cx('font-semibold', t.accentText)}>
            Create a free account
          </Link>{' '}
          to follow your own.
        </p>
      )}
    </div>
  );
}

/** One titled feed column with loading / error / empty / list states. */
function FeedColumn<T>({
  title,
  icon: Icon,
  feed,
  onRetry,
  empty,
  render,
}: {
  title: string;
  icon: LucideIcon;
  feed: Feed<T>;
  onRetry: () => void;
  empty: {label: string; sub: string; icon: LucideIcon};
  render: (item: T, last: boolean) => React.ReactNode;
}) {
  const dark = useDark();
  const t = tok(dark);
  return (
    <section className="min-w-0">
      <h2 className={cx('mb-3 flex items-center gap-2 text-[15px] font-bold', t.text)}>
        <Icon size={16} className={dark ? 'text-teal-300' : 'text-teal-600'} />
        {title}
      </h2>
      {feed.status === 'loading' ? (
        <FeedSkeleton />
      ) : feed.status === 'error' ? (
        <ErrorState onRetry={onRetry} />
      ) : feed.items.length === 0 ? (
        <EmptyState label={empty.label} sub={empty.sub} icon={empty.icon} />
      ) : (
        <div className="tw-fade">
          {feed.items.map((item, i) => render(item, i === feed.items.length - 1))}
        </div>
      )}
    </section>
  );
}
