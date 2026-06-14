'use client';

import {
  ArrowUpDown,
  Flame,
  MessageSquare,
  Mic,
  Newspaper,
  Sparkles,
  TrendingDown,
  TrendingUp,
} from 'lucide-react';
import Link from '@/components/LocalLink';
import {useEffect, useMemo, useState} from 'react';
import {
  getBarsBatch,
  getGurus,
  getHot,
  getNewsBatch,
  getOpportunities,
  getSocialBatch,
  getStock,
  type GuruItem,
  type HotStock,
  type NewsItem,
  type OpportunityStock,
  type Post,
  type Security,
} from '@/lib/api';
import {POPULAR_TICKERS} from '@/lib/config';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, timeAgo, tok} from '@/lib/ui';
import {useQuotes} from '@/lib/useQuotes';
import {BriefingCard} from '@/components/BriefingCard';
import {CryptoStrip} from '@/components/CryptoStrip';
import {IndicesStrip} from '@/components/IndicesStrip';
import {MacroStrip} from '@/components/MacroStrip';
import {RateCutOdds} from '@/components/RateCutOdds';
import {SentimentChip} from '@/components/SentimentChip';
import {
  changePct,
  sortSecurities,
  SortPills,
  type SortKey,
} from '@/components/SortControl';
import {StockCard} from '@/components/StockCard';
import {TopicsStrip} from '@/components/TopicsStrip';

type Tokens = ReturnType<typeof tok>;

function guessMarket(ticker: string): string {
  if (ticker.endsWith('.HK')) return 'HK';
  if (ticker.endsWith('.KS') || ticker.endsWith('.KQ')) return 'KR';
  if (ticker.endsWith('.TW') || ticker.endsWith('.TWO')) return 'TW';
  return 'US';
}
function placeholder(ticker: string): Security {
  return {ticker, name: ticker, market: guessMarket(ticker)};
}
function money(v: number): string {
  if (v >= 1e9) return `$${(v / 1e9).toFixed(1)}B`;
  if (v >= 1e6) return `$${(v / 1e6).toFixed(1)}M`;
  if (v >= 1e3) return `$${(v / 1e3).toFixed(0)}K`;
  return `$${v.toFixed(0)}`;
}

/**
 * The home as an information-source hub: a live Markets strip (hero), then a
 * 3-up row of module cards — Hot stocks (榜单), News (消息) and Discussion (评论) —
 * each previewing a few real items and linking to its full page. Every card
 * carries live content so the page has standalone value (and SEO equity), per
 * the IA research; it is never just a menu.
 */
export function HomeHub() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const tickers = useMemo(() => [...POPULAR_TICKERS], []);
  const tickerKey = tickers.join(',');

  const [securities, setSecurities] = useState<Record<string, Security>>({});
  const [bars, setBars] = useState<Record<string, number[]>>({});
  const [hot, setHot] = useState<HotStock[]>([]);
  const [opps, setOpps] = useState<OpportunityStock[]>([]);
  const [gurus, setGurus] = useState<GuruItem[]>([]);
  const [news, setNews] = useState<NewsItem[]>([]);
  const [posts, setPosts] = useState<Post[]>([]);
  // Per-module loading flags so each card shows a skeleton while fetching, and
  // its empty state only after the fetch settles (no "No data" flash on load).
  const [loading, setLoading] = useState({hot: true, opps: true, gurus: true, news: true, posts: true});
  const quotes = useQuotes(tickers);
  const [sortKey, setSortKey] = useState<SortKey>('default');
  // Guru-watch collapses to a couple of items so its card matches the height of
  // the Hot/Opportunity leaderboards beside it; "Show more" reveals the rest.
  const [guruExpanded, setGuruExpanded] = useState(false);

  useEffect(() => {
    const c = new AbortController();
    for (const tk of tickers) {
      getStock(tk, c.signal).then(
        s => setSecurities(p => ({...p, [tk]: s})),
        () => setSecurities(p => (p[tk] ? p : {...p, [tk]: placeholder(tk)})),
      );
    }
    getBarsBatch(tickers, c.signal).then(r => setBars(r.bars), () => setBars({}));
    const settle = (k: 'hot' | 'opps' | 'gurus' | 'news' | 'posts') =>
      setLoading(p => ({...p, [k]: false}));
    getHot('hot', 5, c.signal)
      .then(r => setHot(r.stocks ?? []), () => setHot([]))
      .finally(() => settle('hot'));
    getOpportunities(5, c.signal)
      .then(r => setOpps(r.stocks ?? []), () => setOpps([]))
      .finally(() => settle('opps'));
    getGurus(6, c.signal)
      .then(r => setGurus(r.items ?? []), () => setGurus([]))
      .finally(() => settle('gurus'));
    getNewsBatch(tickers, 6, c.signal)
      .then(r => setNews((r.news ?? []).slice(0, 3)), () => setNews([]))
      .finally(() => settle('news'));
    getSocialBatch(tickers, 6, c.signal)
      .then(r => setPosts((r.posts ?? []).slice(0, 2)), () => setPosts([]))
      .finally(() => settle('posts'));
    return () => c.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tickerKey]);

  const cards = tickers.map(tk => securities[tk] ?? placeholder(tk));
  const sortedCards = sortSecurities(cards, sortKey, tk =>
    changePct(quotes.get(tk), bars[tk]),
  );

  return (
    <div>
      <header className="mb-5">
        <h1 className={cx('text-[26px] font-bold tracking-tight', t.text)}>
          {tr('home.title')}
        </h1>
        <p className={cx('mt-1 text-[13.5px]', t.sub)}>
          {tr('home.subtitle')}
        </p>
      </header>

      <TopicsStrip />

      <IndicesStrip />

      {/* Fear & Greed index (own composite; self-hides until available) */}
      <SentimentChip />

      {/* U.S. Treasury yield curve — 2Y/10Y + the 2s10s recession signal
          (server-driven; self-hides until the Treasury curve is available) */}
      <MacroStrip />

      {/* Fed rate-cut odds (prediction markets) — grouped here with the Treasury
          strip so the macro/rates signals sit together; self-hides until live */}
      <RateCutOdds />

      {/* Crypto market mood — crypto Fear & Greed + best-effort BTC/ETH (context
          for COIN/MSTR/RIOT/MARA; server-driven; self-hides until available) */}
      <CryptoStrip />

      {/* Markets strip (hero) */}
      <div className="mb-8">
        <div className="mb-2.5 flex items-center justify-between gap-1.5">
          <Link
            href="/screen"
            className={cx('text-[12.5px] font-semibold', t.accentText, 'hover:underline')}
          >
            {tr('mkt.allStocks')} →
          </Link>
          <div className="flex items-center gap-1.5">
            <ArrowUpDown size={13} className={t.faint} />
            <SortPills
              value={sortKey}
              onChange={setSortKey}
              defaultLabel={tr('board.sortDefault')}
              changeLabel={tr('board.sortChange')}
              alphaLabel={tr('board.sortAlpha')}
            />
          </div>
        </div>
        <div className="flex gap-4 overflow-x-auto pb-2">
          {sortedCards.map(sec => (
            <div key={sec.ticker} className="w-[270px] shrink-0">
              <StockCard
                security={sec}
                quote={quotes.get(sec.ticker)}
                closes={bars[sec.ticker]}
              />
            </div>
          ))}
        </div>
      </div>

      {/* Daily AI briefing (folded in from the former /briefing page; self-hides
          until generated) */}
      <BriefingCard />

      {/* Boards & signals (榜单 · 机会 · 大V). Collapsed, Guru-watch shows only a
          couple of items so all three cards match height (items-stretch — the
          Guru "Show more" button sits at the card's bottom, filling the gap).
          Expanding Guru switches to items-start so the Hot/Opportunity cards keep
          their natural height instead of stretching to the now-tall Guru card. */}
      <div
        className={cx(
          'grid gap-5 sm:grid-cols-2 lg:grid-cols-3',
          guruExpanded ? 'items-start' : 'items-stretch',
        )}
      >
        <ModuleCard
          t={t}
          title={tr('mod.hotStocks')}
          href="/hot"
          seeAll={tr('mod.fullBoard')}
          icon={<Flame size={15} className={dark ? 'text-amber-300' : 'text-amber-500'} />}
        >
          {loading.hot ? (
            <CardSkeleton t={t} />
          ) : hot.length === 0 ? (
            <CardEmpty t={t} label={tr('mod.noData')} />
          ) : (
            hot.map((s, i) => (
              <Link
                key={s.ticker}
                href={`/stock/${encodeURIComponent(s.ticker)}`}
                className={cx(
                  'flex items-center gap-2 py-1.5',
                  i < hot.length - 1 && cx('border-b', t.hair),
                )}
              >
                <span
                  className={cx(
                    'w-4 text-[12px] font-bold tabular-nums',
                    s.rank <= 3 ? (dark ? 'text-amber-300' : 'text-amber-500') : t.faint,
                  )}
                >
                  {s.rank}
                </span>
                <span className={cx('flex-1 truncate text-[13px] font-bold', t.text)}>
                  {s.ticker}
                </span>
                <ChangeChip change={s.change} dark={dark} />
              </Link>
            ))
          )}
        </ModuleCard>

        <ModuleCard
          t={t}
          title={tr('mod.opportunity')}
          href="/opportunities"
          seeAll={tr('mod.fullBoard')}
          icon={<Sparkles size={15} className={dark ? 'text-sky-300' : 'text-sky-600'} />}
        >
          {loading.opps ? (
            <CardSkeleton t={t} />
          ) : opps.length === 0 ? (
            <CardEmpty t={t} label={tr('mod.noSignals')} />
          ) : (
            opps.map((s, i) => (
              <Link
                key={s.ticker}
                href={`/stock/${encodeURIComponent(s.ticker)}`}
                className={cx(
                  'flex items-center gap-2 py-1.5',
                  i < opps.length - 1 && cx('border-b', t.hair),
                )}
              >
                <span className={cx('w-4 text-[12px] font-bold tabular-nums', t.faint)}>
                  {s.rank}
                </span>
                <span className={cx('flex-1 truncate text-[13px] font-bold', t.text)}>
                  {s.ticker}
                </span>
                <span
                  className={cx(
                    'shrink-0 text-[11px] font-semibold tabular-nums',
                    t.faint,
                  )}
                >
                  {s.buyers} ins · {money(s.buy_value)}
                </span>
              </Link>
            ))
          )}
        </ModuleCard>

        <ModuleCard
          t={t}
          title={tr('mod.guru')}
          href="/opportunities"
          seeAll={tr('mod.seeAll')}
          icon={<Mic size={15} className={dark ? 'text-violet-300' : 'text-violet-600'} />}
        >
          {loading.gurus ? (
            <CardSkeleton t={t} rows={3} />
          ) : gurus.length === 0 ? (
            <CardEmpty t={t} label={tr('mod.noPosts')} />
          ) : (
            <div className="flex h-full flex-col">
              {(guruExpanded ? gurus : gurus.slice(0, 2)).map((g, i, arr) => (
                <a
                  key={g.url}
                  href={g.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className={cx('block py-2', i < arr.length - 1 && cx('border-b', t.hair))}
                >
                  <div className="flex items-center gap-1.5">
                    <span className={cx('truncate text-[12px] font-semibold', t.text)}>
                      {g.author}
                    </span>
                    {g.tickers?.[0] && (
                      <span
                        className={cx('shrink-0 text-[11px] font-semibold', t.accentText)}
                      >
                        ${g.tickers[0]}
                      </span>
                    )}
                  </div>
                  <p className={cx('mt-0.5 line-clamp-2 text-[12.5px]', t.sub)}>{g.title}</p>
                </a>
              ))}
              {gurus.length > 2 && (
                <button
                  type="button"
                  onClick={() => setGuruExpanded(v => !v)}
                  className={cx(
                    'mt-auto pt-2.5 text-left text-[12px] font-semibold hover:opacity-80',
                    t.accentText,
                  )}
                >
                  {guruExpanded
                    ? tr('mod.showLess')
                    : tr('mod.showMore').replace('{n}', String(gurus.length - 2))}
                </button>
              )}
            </div>
          )}
        </ModuleCard>
      </div>

      {/* Feeds (消息 · 评论) */}
      <div className="mt-5 grid gap-5 sm:grid-cols-2">
        <ModuleCard
          t={t}
          title={tr('mod.news')}
          href="/news"
          seeAll={tr('mod.moreNews')}
          icon={<Newspaper size={15} className={dark ? 'text-teal-300' : 'text-teal-600'} />}
        >
          {loading.news ? (
            <CardSkeleton t={t} rows={3} />
          ) : news.length === 0 ? (
            <CardEmpty t={t} label={tr('mod.noNews')} />
          ) : (
            news.map((n, i) => (
              <a
                key={`${n.ticker}:${n.id}`}
                href={n.url}
                target="_blank"
                rel="noopener noreferrer"
                className={cx(
                  'block py-2',
                  i < news.length - 1 && cx('border-b', t.hair),
                )}
              >
                <p className={cx('line-clamp-2 text-[13px] font-medium', t.text)}>
                  {n.headline}
                </p>
                <p className={cx('mt-0.5 text-[11px]', t.faint)}>
                  {n.source} · {timeAgo(n.published)} ago
                </p>
              </a>
            ))
          )}
        </ModuleCard>

        <ModuleCard
          t={t}
          title={tr('mod.discussion')}
          href="/discussion"
          seeAll={tr('mod.joinIn')}
          icon={
            <MessageSquare size={15} className={dark ? 'text-teal-300' : 'text-teal-600'} />
          }
        >
          {loading.posts ? (
            <CardSkeleton t={t} rows={2} />
          ) : posts.length === 0 ? (
            <CardEmpty t={t} label={tr('mod.noChatter')} />
          ) : (
            posts.map((p, i) => (
              <div
                key={`${p.ticker}:${p.id}`}
                className={cx('py-2', i < posts.length - 1 && cx('border-b', t.hair))}
              >
                <div className="flex items-center gap-1.5">
                  <span className={cx('truncate text-[12px] font-semibold', t.text)}>
                    {p.author}
                  </span>
                  <Link
                    href={`/stock/${encodeURIComponent(p.ticker)}`}
                    className={cx('shrink-0 text-[11px] font-semibold', t.accentText)}
                  >
                    ${p.ticker}
                  </Link>
                </div>
                <p className={cx('mt-0.5 line-clamp-2 text-[12.5px]', t.sub)}>{p.body}</p>
              </div>
            ))
          )}
        </ModuleCard>
      </div>
    </div>
  );
}

function ModuleCard({
  t,
  title,
  href,
  seeAll,
  icon,
  children,
}: {
  t: Tokens;
  title: string;
  href: string;
  seeAll: string;
  icon: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <section className={cx('flex flex-col rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-2 flex items-center justify-between">
        <h2 className={cx('flex items-center gap-1.5 text-[14px] font-bold', t.text)}>
          {icon}
          {title}
        </h2>
        <Link
          href={href}
          className={cx('shrink-0 text-[12px] font-semibold hover:opacity-80', t.accentText)}
        >
          {seeAll} →
        </Link>
      </div>
      <div className="flex-1">{children}</div>
    </section>
  );
}

function CardEmpty({t, label}: {t: Tokens; label: string}) {
  return <p className={cx('py-6 text-center text-[12px]', t.faint)}>{label}</p>;
}

/** Shimmer rows shown while a module's data is still loading. */
function CardSkeleton({t, rows = 4}: {t: Tokens; rows?: number}) {
  return (
    <div className="space-y-2.5 py-2" aria-hidden>
      {Array.from({length: rows}).map((_, i) => (
        <div key={i} className={cx('h-3.5 rounded', t.skel)} style={{width: `${88 - i * 9}%`}} />
      ))}
    </div>
  );
}

function ChangeChip({change, dark}: {change: number; dark: boolean}) {
  const up = change >= 0;
  const pct = Math.abs(change * 100);
  const color = up
    ? dark
      ? 'text-emerald-400'
      : 'text-emerald-600'
    : dark
      ? 'text-rose-400'
      : 'text-rose-500';
  return (
    <span
      className={cx(
        'inline-flex shrink-0 items-center gap-0.5 text-[12px] font-semibold tabular-nums',
        color,
      )}
    >
      {up ? <TrendingUp size={12} /> : <TrendingDown size={12} />}
      {pct.toFixed(0)}%
    </span>
  );
}
