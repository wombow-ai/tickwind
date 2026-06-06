'use client';

import {
  Check,
  Clipboard,
  FileText,
  Link2,
  Lock,
  MessageSquare,
  Newspaper,
  Plus,
} from 'lucide-react';
import Link from 'next/link';
import {useCallback, useEffect, useState} from 'react';
import {
  addToWatchlist,
  clipLink,
  getBars,
  getClips,
  getFilings,
  getNews,
  getSocial,
  getStock,
  getWatchlist,
  type Clip,
  type Filing,
  type NewsItem,
  type Post,
  type Security,
} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, marketCurrency, timeAgo, tok} from '@/lib/ui';
import {useQuotes} from '@/lib/useQuotes';
import {
  ChangeLine,
  MarketBadge,
  PriceTag,
  SessionBadge,
  Sparkline,
} from '@/components/ui/atoms';
import {EmptyState, ErrorState, FeedSkeleton} from '@/components/ui/states';
import {useToast} from '@/components/ui/Toast';
import {TimelineItem} from '@/components/TimelineItem';

type Status = 'loading' | 'ready' | 'error';
interface Feed<T> {
  status: Status;
  items: T[];
}

function guessMarket(ticker: string): string {
  if (ticker.endsWith('.HK')) return 'HK';
  if (ticker.endsWith('.KS') || ticker.endsWith('.KQ')) return 'KR';
  return 'US';
}

const TABS_ANON = ['News', 'Discussion', 'Filings'] as const;
const TABS_AUTH = ['News', 'Discussion', 'Saved links', 'Filings'] as const;

/** Full per-stock view: live header, add-to-watchlist, and source feeds. */
export function StockView({ticker}: {ticker: string}) {
  const norm = ticker.toUpperCase();
  const {user, getToken} = useAuth();
  const isAuthed = !!user;
  const {toast} = useToast();
  const dark = useDark();
  const t = tok(dark);
  const cur = marketCurrency(guessMarket(norm));

  const [security, setSecurity] = useState<Security>({
    ticker: norm,
    name: norm,
    market: guessMarket(norm),
  });
  const quotes = useQuotes([norm]);
  const quote = quotes.get(norm);
  const [bars, setBars] = useState<number[]>([]);

  const [news, setNews] = useState<Feed<NewsItem>>({status: 'loading', items: []});
  const [social, setSocial] = useState<Feed<Post>>({status: 'loading', items: []});
  const [filings, setFilings] = useState<Feed<Filing>>({
    status: 'loading',
    items: [],
  });
  const [clips, setClips] = useState<Feed<Clip>>({status: 'ready', items: []});

  const tabs = isAuthed ? TABS_AUTH : TABS_ANON;
  const [tab, setTab] = useState<string>('News');
  const [clipUrl, setClipUrl] = useState('');
  const [inList, setInList] = useState(false);

  // Resolve security metadata.
  useEffect(() => {
    const c = new AbortController();
    getStock(norm, c.signal).then(setSecurity, () => {});
    return () => c.abort();
  }, [norm]);

  // Trend sparkline (recent daily closes); empty when unavailable.
  useEffect(() => {
    const c = new AbortController();
    getBars(norm, c.signal).then(
      r => setBars(r.closes),
      () => setBars([]),
    );
    return () => c.abort();
  }, [norm]);

  // Public feeds.
  const loadNews = useCallback(() => {
    setNews(f => ({...f, status: 'loading'}));
    getNews(norm, 15).then(
      r => setNews({status: 'ready', items: r.news}),
      () => setNews({status: 'error', items: []}),
    );
  }, [norm]);
  const loadSocial = useCallback(() => {
    setSocial(f => ({...f, status: 'loading'}));
    getSocial(norm, 20).then(
      r => setSocial({status: 'ready', items: r.posts}),
      () => setSocial({status: 'error', items: []}),
    );
  }, [norm]);
  const loadFilings = useCallback(() => {
    setFilings(f => ({...f, status: 'loading'}));
    getFilings(norm, 15).then(
      r => setFilings({status: 'ready', items: r.filings}),
      () => setFilings({status: 'error', items: []}),
    );
  }, [norm]);

  useEffect(() => {
    loadNews();
    loadSocial();
    loadFilings();
  }, [loadNews, loadSocial, loadFilings]);

  // Private: clips + watchlist membership.
  const loadClips = useCallback(async () => {
    setClips(f => ({...f, status: 'loading'}));
    try {
      const token = await getToken();
      const r = await getClips(token, norm);
      setClips({status: 'ready', items: r.clips});
    } catch {
      setClips({status: 'error', items: []});
    }
  }, [norm, getToken]);

  useEffect(() => {
    if (isAuthed) {
      loadClips();
    } else {
      setClips({status: 'ready', items: []});
    }
  }, [isAuthed, loadClips]);

  useEffect(() => {
    if (!isAuthed) {
      setInList(false);
      return;
    }
    let active = true;
    (async () => {
      try {
        const token = await getToken();
        const r = await getWatchlist(token);
        if (active) setInList(r.tickers.includes(norm));
      } catch {
        /* ignore */
      }
    })();
    return () => {
      active = false;
    };
  }, [isAuthed, norm, getToken]);

  // Reset to a valid tab if auth state hid the active one.
  useEffect(() => {
    if (!tabs.includes(tab as never)) setTab('News');
  }, [tabs, tab]);

  async function addWatch() {
    if (inList) return;
    setInList(true); // optimistic
    try {
      const token = await getToken();
      await addToWatchlist(token, norm);
      toast(`Added ${norm} to your watchlist`, {tone: 'ok'});
    } catch {
      setInList(false);
      toast(`Couldn't add ${norm}`);
    }
  }

  async function saveClip() {
    const url = clipUrl.trim();
    if (!url) return;
    setClipUrl('');
    toast('Saving link…');
    try {
      const token = await getToken();
      const created = await clipLink(token, norm, url);
      setClips(f => ({status: 'ready', items: [created, ...f.items]}));
      toast('Link saved', {tone: 'ok'});
    } catch {
      toast("Couldn't save that link");
    }
  }

  return (
    <div className="mx-auto max-w-4xl">
      {/* header */}
      <div
        className="relative mb-6 overflow-hidden rounded-3xl border p-6"
        style={{
          background: dark
            ? 'radial-gradient(500px 200px at 90% -20%, rgba(45,212,191,.14), transparent)'
            : 'radial-gradient(500px 200px at 90% -20%, rgba(45,212,191,.18), transparent)',
        }}
      >
        <div
          className={cx('absolute inset-0 -z-10 rounded-3xl border', t.card, t.border, t.soft)}
        />
        <div className="flex flex-col gap-6 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <div className="mb-1.5 flex items-center gap-2.5">
              <h1 className={cx('text-[22px] font-bold tracking-tight', t.text)}>
                {security.name}
              </h1>
              <MarketBadge mkt={security.market} />
            </div>
            <div className="mb-4 flex items-center gap-2.5">
              <span className={cx('text-[13px] font-semibold tabular-nums', t.sub)}>
                {security.market}: {norm}
              </span>
              {quote && <SessionBadge session={quote.session} />}
            </div>
            {quote ? (
              <PriceTag value={quote.price} cur={cur} size="lg" />
            ) : (
              <span className={cx('text-4xl font-semibold tabular-nums sm:text-5xl', t.faint)}>
                {cur}—
              </span>
            )}
            <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1">
              {quote?.prev_close ? (
                <ChangeLine
                  chg={quote.price - quote.prev_close}
                  pct={
                    ((quote.price - quote.prev_close) / quote.prev_close) * 100
                  }
                  cur={cur}
                  size="lg"
                />
              ) : null}
              {quote ? (
                <span className={cx('inline-flex items-center gap-1.5 text-[11px]', t.faint)}>
                  {quote.session === 'regular' && (
                    <span
                      className="tw-livedot rounded-full"
                      style={{width: 6, height: 6, background: '#10B981'}}
                    />
                  )}
                  via {quote.source} · {timeAgo(quote.at)} ago
                </span>
              ) : (
                <span className={cx('text-[11px]', t.faint)}>Waiting for a price…</span>
              )}
            </div>
            {bars.length >= 2 && (
              <div className="mt-4">
                <Sparkline
                  values={bars}
                  up={bars[bars.length - 1] >= bars[0]}
                  w={320}
                  h={56}
                />
              </div>
            )}
          </div>

          <div className="shrink-0">
            {isAuthed ? (
              <button
                onClick={addWatch}
                className={cx(
                  'inline-flex items-center gap-1.5 rounded-full px-4 py-2 text-[13px] font-semibold',
                  inList ? cx('border', t.border, t.sub) : btnPrimary(dark),
                )}
              >
                {inList ? (
                  <>
                    <Check size={15} /> On your watchlist
                  </>
                ) : (
                  <>
                    <Plus size={15} /> Add to watchlist
                  </>
                )}
              </button>
            ) : (
              <Link
                href="/login"
                className={cx(
                  'inline-flex items-center gap-1.5 rounded-full px-4 py-2 text-[13px] font-semibold',
                  btnPrimary(dark),
                )}
              >
                <Plus size={15} /> Add to watchlist
              </Link>
            )}
          </div>
        </div>
      </div>

      {/* login gate */}
      {!isAuthed && (
        <div
          className={cx(
            'mb-6 flex items-center gap-3 rounded-2xl border p-4',
            t.border,
            dark ? 'bg-teal-500/5' : 'bg-teal-50/70',
          )}
        >
          <span
            className="flex shrink-0 items-center justify-center rounded-xl"
            style={{
              width: 38,
              height: 38,
              background: dark ? 'rgba(20,184,166,.14)' : 'rgba(13,148,136,.1)',
            }}
          >
            <Lock size={17} className={dark ? 'text-teal-300' : 'text-teal-600'} />
          </span>
          <div className="min-w-0 flex-1">
            <p className={cx('text-[13.5px] font-semibold', t.text)}>
              Log in to add {norm} and save links
            </p>
            <p className={cx('text-[12px]', t.sub)}>
              Keep a watchlist and clip posts from anywhere — free.
            </p>
          </div>
          <Link
            href="/login"
            className={cx(
              'shrink-0 rounded-full px-3.5 py-1.5 text-[12.5px] font-semibold',
              btnPrimary(dark),
            )}
          >
            Log in
          </Link>
        </div>
      )}

      {/* tabs */}
      <div className="mb-4">
        <div
          className={cx(
            'inline-flex items-center gap-1 overflow-x-auto rounded-xl border p-1',
            t.border,
            t.surf2,
          )}
        >
          {tabs.map(tb => (
            <button
              key={tb}
              onClick={() => setTab(tb)}
              className={cx(
                'whitespace-nowrap rounded-lg px-3 py-1.5 text-[13px] font-medium transition',
                tab === tb
                  ? dark
                    ? 'bg-slate-700 text-white'
                    : 'bg-white text-slate-900 shadow-sm'
                  : t.sub,
              )}
            >
              {tb}
            </button>
          ))}
        </div>
      </div>

      <div className="min-h-[280px]">
        {tab === 'News' && (
          <FeedList
            feed={news}
            onRetry={loadNews}
            empty={{
              label: 'No news yet',
              sub: `When headlines about ${norm} appear, they'll land here first.`,
              icon: Newspaper,
            }}
            render={n => (
              <TimelineItem key={n.id} entry={{kind: 'news', item: n}} />
            )}
          />
        )}
        {tab === 'Discussion' && (
          <FeedList
            feed={social}
            onRetry={loadSocial}
            empty={{
              label: 'No chatter right now',
              sub: `Posts from StockTwits and Reddit about ${norm} will show up here.`,
              icon: MessageSquare,
            }}
            render={p => (
              <TimelineItem key={p.id} entry={{kind: 'disc', item: p}} />
            )}
          />
        )}
        {tab === 'Filings' && (
          <FeedList
            feed={filings}
            onRetry={loadFilings}
            empty={{
              label: 'No recent filings',
              sub: 'Filings appear here as soon as they hit SEC EDGAR.',
              icon: FileText,
            }}
            render={f => (
              <TimelineItem
                key={f.accession_no}
                entry={{kind: 'filing', item: f}}
              />
            )}
          />
        )}
        {tab === 'Saved links' && isAuthed && (
          <div className="tw-fade">
            <form
              onSubmit={e => {
                e.preventDefault();
                saveClip();
              }}
              className={cx(
                'mb-4 flex items-center gap-2 rounded-2xl border p-2',
                t.card,
                t.border,
                t.soft,
              )}
            >
              <Clipboard size={16} className={cx('ml-1.5', t.sub)} />
              <input
                value={clipUrl}
                onChange={e => setClipUrl(e.target.value)}
                placeholder="Paste an X / Xiaohongshu / TikTok link…"
                className={cx(
                  'flex-1 bg-transparent text-[13.5px] outline-none',
                  dark
                    ? 'text-slate-100 placeholder:text-slate-500'
                    : 'text-slate-900 placeholder:text-slate-400',
                )}
              />
              <button
                type="submit"
                className={cx(
                  'rounded-lg px-3 py-1.5 text-[12.5px] font-semibold',
                  btnPrimary(dark),
                )}
              >
                Save
              </button>
            </form>
            <FeedList
              feed={clips}
              onRetry={loadClips}
              empty={{
                label: 'No saved links yet',
                sub: `Found something good elsewhere? Paste the link to clip it onto ${norm}.`,
                icon: Link2,
              }}
              render={c => (
                <TimelineItem key={c.id} entry={{kind: 'clip', item: c}} />
              )}
            />
          </div>
        )}
      </div>
    </div>
  );
}

/** Renders a feed's loading / error / empty / ready states uniformly. */
function FeedList<T>({
  feed,
  onRetry,
  empty,
  render,
}: {
  feed: Feed<T>;
  onRetry: () => void;
  empty: {label: string; sub: string; icon: typeof Newspaper};
  render: (item: T) => React.ReactNode;
}) {
  if (feed.status === 'loading') return <FeedSkeleton />;
  if (feed.status === 'error') return <ErrorState onRetry={onRetry} />;
  if (feed.items.length === 0) {
    return <EmptyState label={empty.label} sub={empty.sub} icon={empty.icon} />;
  }
  return <div className="tw-fade">{feed.items.map(render)}</div>;
}
