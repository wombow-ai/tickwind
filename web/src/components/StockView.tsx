'use client';

import {
  Check,
  Clipboard,
  FileText,
  Link2,
  Loader2,
  Lock,
  MessageSquare,
  Newspaper,
  Plus,
  X,
} from 'lucide-react';
import Link from 'next/link';
import {useT} from '@/lib/i18n';
import {useCallback, useEffect, useState} from 'react';
import {
  addToWatchlist,
  clipLink,
  getBars,
  getClips,
  getFilings,
  getNews,
  getSignals,
  subscribeLive,
  getSocial,
  getStock,
  getWatchlist,
  removeFromWatchlist,
  type Clip,
  type Filing,
  type NewsItem,
  type Post,
  type Security,
  type Signal,
} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, fmtPrice, marketCurrency, timeAgo, tok} from '@/lib/ui';
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
import {PulseBar} from '@/components/PulseBar';
import {NotesPanel} from '@/components/NotesPanel';
import {AlertsPanel} from '@/components/AlertsPanel';
import {HoldingsPanel} from '@/components/HoldingsPanel';
import {KLineChart} from '@/components/KLineChart';
import {FundamentalsCard} from '@/components/FundamentalsCard';
import {IndicatorsPanel} from '@/components/IndicatorsPanel';
import {AISummaryCard} from '@/components/AISummaryCard';
import {EarningsChip} from '@/components/EarningsChip';
import {OptionsCard} from '@/components/OptionsCard';
import {ShortChip} from '@/components/ShortChip';
import {CongressChip} from '@/components/CongressChip';
import {WhalesChip} from '@/components/WhalesChip';
import {CommentsPanel} from '@/components/CommentsPanel';
import {ResearchReport} from '@/components/ResearchReport';
import {ShareCardButton} from '@/components/ShareCardButton';

type Status = 'loading' | 'ready' | 'error';
interface Feed<T> {
  status: Status;
  items: T[];
}

function guessMarket(ticker: string): string {
  if (ticker.endsWith('.HK')) return 'HK';
  if (ticker.endsWith('.KS') || ticker.endsWith('.KQ')) return 'KR';
  if (ticker.endsWith('.TW') || ticker.endsWith('.TWO')) return 'TW';
  return 'US';
}

const TABS_ANON = ['Research', 'News', 'Discussion', 'Comments', 'Filings'] as const;
const TABS_AUTH = ['Research', 'News', 'Discussion', 'Comments', 'Notes', 'Alerts', 'Holdings', 'Saved links', 'Filings'] as const;
// Tab keys stay English (they're the state values); only the display is translated.
const TAB_LABELS: Record<string, string> = {
  Research: 'research.tab',
  News: 'mod.news',
  Discussion: 'mod.discussion',
  Comments: 'comments.tab',
  Notes: 'nav.notes',
  Alerts: 'alerts.title',
  Holdings: 'holdings.title',
  'Saved links': 'stock.savedLinks',
  Filings: 'stock.filings',
};

/** Full per-stock view: live header, add-to-watchlist, and source feeds. */
export function StockView({ticker}: {ticker: string}) {
  const norm = ticker.toUpperCase();
  const {user, getToken} = useAuth();
  const isAuthed = !!user;
  const {toast} = useToast();
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const cur = marketCurrency(guessMarket(norm));

  const [security, setSecurity] = useState<Security>({
    ticker: norm,
    name: norm,
    market: guessMarket(norm),
  });
  const quotes = useQuotes([norm]);
  const quote = quotes.get(norm);
  const [bars, setBars] = useState<number[]>([]);
  const [signals, setSignals] = useState<Signal[]>([]);

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
  const [collecting, setCollecting] = useState(false);
  const [reload, setReload] = useState(0);

  // Resolve security metadata. A brand-new ticker (never ingested) 404s until the
  // on-add ingest lands (~seconds–1min); poll briefly, show a "collecting" state,
  // and refresh the feeds once it resolves so the page fills in on its own.
  useEffect(() => {
    const c = new AbortController();
    let tries = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const resolve = () => {
      getStock(norm, c.signal).then(
        s => {
          setSecurity(s);
          setCollecting(false);
          if (tries > 0) setReload(r => r + 1); // data just landed → refill feeds
        },
        () => {
          tries++;
          if (tries < 18) {
            setCollecting(true);
            timer = setTimeout(resolve, 5000); // poll while data is collected (~90s, outlasts the 60s server collect)
          } else {
            setCollecting(false); // give up after ~90s → show normal empty states
          }
        },
      );
    };
    resolve();
    return () => {
      c.abort();
      if (timer) clearTimeout(timer);
    };
  }, [norm]);

  // Ask the backend to stream this ticker in real time while it's open (#2b):
  // it joins the live WebSocket subscription so the price stays fresh. Best-effort.
  useEffect(() => {
    const c = new AbortController();
    subscribeLive(norm, c.signal).catch(() => {});
    return () => c.abort();
  }, [norm]);

  // Trend sparkline (recent daily closes); empty when unavailable.
  useEffect(() => {
    const c = new AbortController();
    getBars(norm, c.signal).then(
      r => setBars(r.closes ?? []),
      () => setBars([]),
    );
    return () => c.abort();
  }, [norm]);

  // Buzz / sentiment pulse (optional; empty when no signal source has data).
  useEffect(() => {
    const c = new AbortController();
    getSignals(norm, c.signal).then(
      r => setSignals(r.signals ?? []),
      () => setSignals([]),
    );
    return () => c.abort();
  }, [norm]);

  // Public feeds. Null lists (Go marshals an empty slice as null) are coerced to
  // [] so the feed never renders against `null.length`.
  const loadNews = useCallback(() => {
    setNews(f => ({...f, status: 'loading'}));
    getNews(norm, 15).then(
      r => setNews({status: 'ready', items: r.news ?? []}),
      () => setNews({status: 'error', items: []}),
    );
  }, [norm]);
  const loadSocial = useCallback(() => {
    setSocial(f => ({...f, status: 'loading'}));
    getSocial(norm, 20).then(
      r => setSocial({status: 'ready', items: r.posts ?? []}),
      () => setSocial({status: 'error', items: []}),
    );
  }, [norm]);
  const loadFilings = useCallback(() => {
    setFilings(f => ({...f, status: 'loading'}));
    getFilings(norm, 15).then(
      r => setFilings({status: 'ready', items: r.filings ?? []}),
      () => setFilings({status: 'error', items: []}),
    );
  }, [norm]);

  useEffect(() => {
    loadNews();
    loadSocial();
    loadFilings();
  }, [loadNews, loadSocial, loadFilings, reload]);

  // Private: clips + watchlist membership.
  const loadClips = useCallback(async () => {
    setClips(f => ({...f, status: 'loading'}));
    try {
      const token = await getToken();
      const r = await getClips(token, norm);
      setClips({status: 'ready', items: r.clips ?? []});
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

  async function toggleWatch() {
    if (inList) {
      setInList(false); // optimistic
      try {
        const token = await getToken();
        await removeFromWatchlist(token, norm);
        toast(`Removed ${norm} from your watchlist`, {tone: 'ok'});
      } catch {
        setInList(true);
        toast(`Couldn't remove ${norm}`);
      }
      return;
    }
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

  // Regular vs extended-hours split (Futu/Google style): the primary line shows
  // the regular-session price + day change; in pre/post/overnight sessions a
  // second line shows the extended price + its change vs the regular close.
  const regClose =
    quote && quote.regular_close && quote.regular_close > 0
      ? quote.regular_close
      : quote?.price ?? 0;
  // The regular-session figure shown as the primary line: the LIVE price during
  // regular hours, else the most-recent regular close (pre/post/overnight). Both
  // the big number and its day-change derive from this single value, so they
  // always agree — previously the big number used `price` while the change used
  // `regClose`, which disagreed whenever the snapshot's daily bar lagged the
  // latest trade.
  const regularPrice = quote?.session === 'regular' ? quote.price : regClose;
  const isExt =
    !!quote &&
    (quote.session === 'pre' || quote.session === 'post' || quote.session === 'overnight') &&
    regClose > 0 &&
    Math.abs(quote.price - regClose) > 1e-9;
  const primaryPrice = regularPrice;
  // The prior close to measure the regular-session day-change against. In
  // extended hours the live quote's prev_close can be anchored to regClose (the
  // thin-name overlay guard against phantom day-changes), which would zero out
  // the change — so fall back to the reliable daily bars: the close before the
  // most recent one. Keeps "正股" change real (last completed session) while the
  // separate extended line carries the pre/post delta.
  const priorClose =
    isExt && bars.length >= 2 ? bars[bars.length - 2] : quote?.prev_close ?? 0;

  // Share card (propagation organ): a branded snapshot of the ticker for 小红书 /
  // 微信. The big figure is the price; tone tilts green/red by day-change; the
  // "数据延迟" note rides on the subtitle. Only meaningful once a price exists.
  const dayChgPct = priorClose > 0 ? ((regularPrice - priorClose) / priorClose) * 100 : 0;
  const up = dayChgPct >= 0;
  const chgStr =
    priorClose > 0 ? `${up ? '+' : '−'}${Math.abs(dayChgPct).toFixed(2)}%` : '';
  const shareCard = {
    kind: 'stock' as const,
    eyebrow: security.market,
    title: norm,
    stat: fmtPrice(cur, regularPrice),
    subtitle: [security.name !== norm ? security.name : '', chgStr, '数据延迟']
      .filter(Boolean)
      .join(' · '),
    tone: (up ? 'up' : 'down') as 'up' | 'down',
  };

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
            {/* Ticker leads (it's what identifies the stock); the full name sits
                below in muted text — matching the home StockCard. */}
            <div className="mb-1 flex items-center gap-2.5">
              <h1 className={cx('text-[24px] font-bold tracking-tight', t.text)}>{norm}</h1>
              <MarketBadge mkt={security.market} />
              {quote && <SessionBadge session={quote.session} />}
            </div>
            <div className="mb-4">
              <span className={cx('text-[13px]', t.sub)}>
                {security.name !== norm ? security.name : ' '}
              </span>
            </div>
            {quote ? (
              <PriceTag value={primaryPrice} cur={cur} size="lg" />
            ) : (
              <span className={cx('text-4xl font-semibold tabular-nums sm:text-5xl', t.faint)}>
                {cur}—
              </span>
            )}
            <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1">
              {quote && priorClose > 0 ? (
                <ChangeLine
                  chg={regularPrice - priorClose}
                  pct={((regularPrice - priorClose) / priorClose) * 100}
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
                  {(timeAgo(quote.at) === 'now'
                    ? tr('quote.lastTradeNow')
                    : tr('quote.lastTrade').replace('{t}', timeAgo(quote.at))
                  ).replace('{src}', quote.source)}
                </span>
              ) : (
                <span className={cx('text-[11px]', t.faint)}>{tr('stock.waitingPrice')}</span>
              )}
            </div>
            {/* extended-hours line: pre/post/overnight price vs the regular close */}
            {isExt && quote && (
              <div className="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-0.5">
                <span className={cx('text-[12px] font-semibold', t.faint)}>
                  {tr(`session.${quote.session}`)}
                </span>
                <span className={cx('text-[15px] font-bold tabular-nums', t.text)}>
                  {cur}
                  {quote.price.toFixed(2)}
                </span>
                <ChangeLine
                  chg={quote.price - regClose}
                  pct={((quote.price - regClose) / regClose) * 100}
                  cur={cur}
                />
              </div>
            )}
          </div>

          {/* right column: watchlist action + the price-trend sparkline (fills
              the space so the action button isn't left floating alone) */}
          <div className="flex shrink-0 flex-col items-stretch gap-4 sm:items-end">
            {isAuthed ? (
              <button
                onClick={toggleWatch}
                aria-label={inList ? tr('stock.removeWatch') : tr('stock.addWatch')}
                className={cx(
                  'group inline-flex items-center justify-center gap-1.5 rounded-full px-4 py-2 text-[13px] font-semibold transition',
                  inList
                    ? cx('border hover:border-rose-300 hover:text-rose-500', t.border, t.sub)
                    : btnPrimary(dark),
                )}
              >
                {inList ? (
                  <>
                    <span className="inline-flex items-center gap-1.5 group-hover:hidden">
                      <Check size={15} /> {tr('stock.onWatchlist')}
                    </span>
                    <span className="hidden items-center gap-1.5 group-hover:inline-flex">
                      <X size={15} /> {tr('stock.removeWatch')}
                    </span>
                  </>
                ) : (
                  <>
                    <Plus size={15} /> {tr('stock.addWatch')}
                  </>
                )}
              </button>
            ) : (
              <Link
                href="/login"
                className={cx(
                  'inline-flex items-center justify-center gap-1.5 rounded-full px-4 py-2 text-[13px] font-semibold',
                  btnPrimary(dark),
                )}
              >
                <Plus size={15} /> {tr('stock.addWatch')}
              </Link>
            )}
            {/* propagation organ: save a branded snapshot card (needs a price) */}
            {quote && (
              <div className="flex sm:justify-end">
                <ShareCardButton card={shareCard} />
              </div>
            )}
            {bars.length >= 2 && (
              <Sparkline
                values={bars}
                up={bars[bars.length - 1] >= bars[0]}
                w={300}
                h={56}
              />
            )}
          </div>
        </div>
      </div>

      {/* brand-new ticker: data is being collected on first add */}
      {collecting && !quote && (
        <div
          className={cx(
            'mb-6 flex items-center gap-3 rounded-2xl border p-4',
            t.border,
            dark ? 'bg-amber-500/5' : 'bg-amber-50/70',
          )}
        >
          <span
            className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl"
            style={{background: dark ? 'rgba(245,158,11,.14)' : '#FEF3C7'}}
          >
            <Loader2
              size={18}
              className={cx('animate-spin', dark ? 'text-amber-300' : 'text-amber-600')}
            />
          </span>
          <div className="min-w-0">
            <p className={cx('text-[13.5px] font-semibold', t.text)}>{tr('stock.collecting')}</p>
            <p className={cx('text-[12px]', t.sub)}>{tr('stock.collectingSub')}</p>
          </div>
        </div>
      )}

      {/* pulse: Reddit buzz + news sentiment (renders nothing when empty) */}
      {/* id anchors below let research-report citations deep-link to each card */}
      <div id="signals" className="scroll-mt-20">
        <PulseBar signals={signals} />
      </div>

      {/* next earnings date (Finnhub calendar; hides when none upcoming) */}
      <EarningsChip ticker={norm} />

      {/* FINRA short pressure (squeeze radar; hides when the symbol has no row) */}
      <div id="short" className="scroll-mt-20">
        <ShortChip ticker={norm} />
      </div>

      {/* Congress trades in this ticker (House Clerk PTRs; hides when none) —
          each member links to their /congress/member/{slug} detail page */}
      <div id="congress" className="scroll-mt-20">
        <CongressChip ticker={norm} />
      </div>

      {/* Which famous 13F funds hold this ticker (reverse whale lookup; hides
          when none) — each fund links to its /fund/{slug} page */}
      <div id="whales" className="scroll-mt-20">
        <WhalesChip ticker={norm} />
      </div>

      {/* Fundamentals + AI digest, each full-width, above the chart. (They were
          briefly a 2-col grid, but the AI digest's variable length left the
          fundamentals card with a tall empty gap beside it — owner 2026-06-12.) */}
      {/* fundamentals: market cap / P/E / revenue / net income (SEC XBRL; hides for non-US) */}
      <div id="fundamentals" className="scroll-mt-20">
        <FundamentalsCard ticker={norm} />
      </div>
      {/* AI digest: daily-cached bullets from news+social (hides when LLM off/empty) */}
      <AISummaryCard ticker={norm} />

      {/* K-line candlestick chart + indicators — the price-and-indicators anchor,
          with the options panel directly below it */}
      <div className="mb-6">
        <KLineChart ticker={norm} quote={quote} />
      </div>

      {/* Computed per-stock indicators (latest values, grouped by domain) — the
          readout companion to the chart's client-side overlays + FundamentalsCard;
          hides entirely when no indicators are computable */}
      <div id="indicators" className="scroll-mt-20">
        <IndicatorsPanel ticker={norm} />
      </div>

      {/* Options overview: delayed Cboe P/C, max pain, OI leaders (hides for non-US/no options) */}
      <div id="options" className="scroll-mt-20">
        <OptionsCard ticker={norm} />
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
              {tr('stock.loginAdd').replace('{t}', norm)}
            </p>
            <p className={cx('text-[12px]', t.sub)}>{tr('stock.loginAddSub')}</p>
          </div>
          <Link
            href="/login"
            className={cx(
              'shrink-0 rounded-full px-3.5 py-1.5 text-[12.5px] font-semibold',
              btnPrimary(dark),
            )}
          >
            {tr('nav.login')}
          </Link>
        </div>
      )}

      {/* tabs */}
      <div className="mb-4">
        <div
          role="group"
          aria-label="Stock sections"
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
              aria-pressed={tab === tb}
              className={cx(
                'whitespace-nowrap rounded-lg px-3 py-1.5 text-[13px] font-medium transition',
                tab === tb
                  ? dark
                    ? 'bg-slate-700 text-white'
                    : 'bg-white text-slate-900 shadow-sm'
                  : t.sub,
              )}
            >
              {tr(TAB_LABELS[tb] ?? tb)}
            </button>
          ))}
        </div>
      </div>

      <div className="min-h-[280px]">
        {/* Research is lazy: it mounts (and fires its LLM-backed fetch) only when
            the tab is opened, so the heavy call never runs on page load. */}
        {tab === 'Research' && <ResearchReport ticker={norm} />}
        {tab === 'News' && (
          <FeedList
            feed={news}
            onRetry={loadNews}
            empty={{
              label: tr('mod.noNews'),
              sub: tr('stock.noNewsSub'),
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
              label: tr('mod.noChatter'),
              sub: tr('stock.noChatterSub'),
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
              label: tr('stock.noFilings'),
              sub: tr('stock.noFilingsSub'),
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
        {tab === 'Comments' && <CommentsPanel ticker={norm} />}
        {tab === 'Notes' && isAuthed && <NotesPanel ticker={norm} />}
        {tab === 'Alerts' && isAuthed && <AlertsPanel ticker={norm} />}
        {tab === 'Holdings' && isAuthed && <HoldingsPanel ticker={norm} quote={quote} cur={cur} />}
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
                placeholder={tr('stock.clipPlaceholder')}
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
                {tr('stock.save')}
              </button>
            </form>
            <FeedList
              feed={clips}
              onRetry={loadClips}
              empty={{
                label: tr('stock.noClips'),
                sub: tr('stock.noClipsSub'),
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
