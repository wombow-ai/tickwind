'use client';

import {
  Flame,
  MessageSquare,
  Mic,
  Newspaper,
  Sparkles,
  TrendingDown,
  TrendingUp,
} from 'lucide-react';
import Link from 'next/link';
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
import {useDark} from '@/lib/theme';
import {cx, timeAgo, tok} from '@/lib/ui';
import {useQuotes} from '@/lib/useQuotes';
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
  const tickers = useMemo(() => [...POPULAR_TICKERS], []);
  const tickerKey = tickers.join(',');

  const [securities, setSecurities] = useState<Record<string, Security>>({});
  const [bars, setBars] = useState<Record<string, number[]>>({});
  const [hot, setHot] = useState<HotStock[]>([]);
  const [opps, setOpps] = useState<OpportunityStock[]>([]);
  const [gurus, setGurus] = useState<GuruItem[]>([]);
  const [news, setNews] = useState<NewsItem[]>([]);
  const [posts, setPosts] = useState<Post[]>([]);
  const quotes = useQuotes(tickers);

  useEffect(() => {
    const c = new AbortController();
    for (const tk of tickers) {
      getStock(tk, c.signal).then(
        s => setSecurities(p => ({...p, [tk]: s})),
        () => setSecurities(p => (p[tk] ? p : {...p, [tk]: placeholder(tk)})),
      );
    }
    getBarsBatch(tickers, c.signal).then(r => setBars(r.bars), () => setBars({}));
    getHot('hot', 5, c.signal).then(r => setHot(r.stocks ?? []), () => setHot([]));
    getOpportunities(5, c.signal).then(
      r => setOpps(r.stocks ?? []),
      () => setOpps([]),
    );
    getGurus(3, c.signal).then(r => setGurus(r.items ?? []), () => setGurus([]));
    getNewsBatch(tickers, 6, c.signal).then(
      r => setNews((r.news ?? []).slice(0, 3)),
      () => setNews([]),
    );
    getSocialBatch(tickers, 6, c.signal).then(
      r => setPosts((r.posts ?? []).slice(0, 2)),
      () => setPosts([]),
    );
    return () => c.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tickerKey]);

  const cards = tickers.map(tk => securities[tk] ?? placeholder(tk));

  return (
    <div>
      <header className="mb-5">
        <h1 className={cx('text-[26px] font-bold tracking-tight', t.text)}>
          Markets today
        </h1>
        <p className={cx('mt-1 text-[13.5px]', t.sub)}>
          Live prices, then the trends, news and chatter across the market — all
          in one place.
        </p>
      </header>

      <TopicsStrip />

      {/* Markets strip (hero) */}
      <div className="mb-8 flex gap-4 overflow-x-auto pb-2">
        {cards.map(sec => (
          <div key={sec.ticker} className="w-[270px] shrink-0">
            <StockCard
              security={sec}
              quote={quotes.get(sec.ticker)}
              closes={bars[sec.ticker]}
            />
          </div>
        ))}
      </div>

      {/* Boards & signals (榜单 · 机会 · 大V) */}
      <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
        <ModuleCard
          t={t}
          title="Hot stocks"
          href="/hot"
          seeAll="Full board"
          icon={<Flame size={15} className={dark ? 'text-amber-300' : 'text-amber-500'} />}
        >
          {hot.length === 0 ? (
            <CardEmpty t={t} label="No data yet" />
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
          title="Opportunity"
          href="/opportunities"
          seeAll="Full board"
          icon={<Sparkles size={15} className={dark ? 'text-sky-300' : 'text-sky-600'} />}
        >
          {opps.length === 0 ? (
            <CardEmpty t={t} label="No signals yet" />
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
          title="Guru-watch"
          href="/opportunities"
          seeAll="See all"
          icon={<Mic size={15} className={dark ? 'text-violet-300' : 'text-violet-600'} />}
        >
          {gurus.length === 0 ? (
            <CardEmpty t={t} label="No posts yet" />
          ) : (
            gurus.map((g, i) => (
              <a
                key={g.url}
                href={g.url}
                target="_blank"
                rel="noopener noreferrer"
                className={cx(
                  'block py-2',
                  i < gurus.length - 1 && cx('border-b', t.hair),
                )}
              >
                <div className="flex items-center gap-1.5">
                  <span className={cx('truncate text-[12px] font-semibold', t.text)}>
                    {g.author}
                  </span>
                  {g.tickers[0] && (
                    <span
                      className={cx('shrink-0 text-[11px] font-semibold', t.accentText)}
                    >
                      ${g.tickers[0]}
                    </span>
                  )}
                </div>
                <p className={cx('mt-0.5 line-clamp-2 text-[12.5px]', t.sub)}>{g.title}</p>
              </a>
            ))
          )}
        </ModuleCard>
      </div>

      {/* Feeds (消息 · 评论) */}
      <div className="mt-5 grid gap-5 sm:grid-cols-2">
        <ModuleCard
          t={t}
          title="News"
          href="/news"
          seeAll="More news"
          icon={<Newspaper size={15} className={dark ? 'text-teal-300' : 'text-teal-600'} />}
        >
          {news.length === 0 ? (
            <CardEmpty t={t} label="No news yet" />
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
          title="Discussion"
          href="/discussion"
          seeAll="Join in"
          icon={
            <MessageSquare size={15} className={dark ? 'text-teal-300' : 'text-teal-600'} />
          }
        >
          {posts.length === 0 ? (
            <CardEmpty t={t} label="No chatter yet" />
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
