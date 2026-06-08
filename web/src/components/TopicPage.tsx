'use client';

import {Flame} from 'lucide-react';
import {useEffect, useMemo, useState} from 'react';
import {
  getBarsBatch,
  getNewsBatch,
  getStock,
  getTopics,
  type HotTopic,
  type NewsItem,
  type Security,
} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';
import {useQuotes} from '@/lib/useQuotes';
import {StockCard} from '@/components/StockCard';
import {TimelineItem} from '@/components/TimelineItem';
import {EmptyState, FeedSkeleton} from '@/components/ui/states';

function placeholder(ticker: string): Security {
  return {ticker, name: ticker, market: 'US'};
}

/**
 * A trending-topic landing page: the stocks + news tied to one Hot Topic. Reuses
 * the live `/v1/topics` snapshot (related_tickers) + the batched news endpoint —
 * no new backend needed. Degrades gracefully when a topic has cooled off.
 */
export function TopicPage({topicKey, label}: {topicKey: string; label?: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [topic, setTopic] = useState<HotTopic | null>(null);
  const [resolved, setResolved] = useState(false);
  const [securities, setSecurities] = useState<Record<string, Security>>({});
  const [bars, setBars] = useState<Record<string, number[]>>({});
  const [news, setNews] = useState<NewsItem[]>([]);
  const [newsLoading, setNewsLoading] = useState(true);

  // Resolve the topic by key from the live topics snapshot.
  useEffect(() => {
    const c = new AbortController();
    getTopics(c.signal).then(
      r => {
        setTopic((r.topics ?? []).find(tp => tp.key === topicKey) ?? null);
        setResolved(true);
      },
      () => setResolved(true),
    );
    return () => c.abort();
  }, [topicKey]);

  const tickers = useMemo(() => topic?.related_tickers ?? [], [topic]);
  const tickerKey = tickers.join(',');
  const quotes = useQuotes(tickers);

  // Once tickers are known, fetch their securities + sparklines + topic news.
  useEffect(() => {
    if (!tickers.length) {
      setNewsLoading(false);
      return;
    }
    const c = new AbortController();
    setNewsLoading(true);
    for (const tk of tickers) {
      getStock(tk, c.signal).then(
        s => setSecurities(p => ({...p, [tk]: s})),
        () => setSecurities(p => (p[tk] ? p : {...p, [tk]: placeholder(tk)})),
      );
    }
    getBarsBatch(tickers, c.signal).then(
      r => setBars(r.bars),
      () => setBars({}),
    );
    getNewsBatch(tickers, 12, c.signal, topicKey)
      .then(
        r => setNews(r.news ?? []),
        () => setNews([]),
      )
      .finally(() => setNewsLoading(false));
    return () => c.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tickerKey, topicKey]);

  const heading = label || topic?.label || topicKey;

  return (
    <div className="mx-auto max-w-3xl">
      <header className="mb-5">
        <div className="flex items-center gap-2">
          <Flame size={20} className={dark ? 'text-amber-300' : 'text-amber-500'} />
          <h1 className={cx('text-[22px] font-bold tracking-tight', t.text)}>{heading}</h1>
          {topic && topic.momentum > 1 && (
            <span
              className={cx(
                'rounded-full px-2 py-0.5 text-[11px] font-semibold',
                dark ? 'bg-amber-500/15 text-amber-300' : 'bg-amber-50 text-amber-700',
              )}
            >
              {tr('topic.heating')}
            </span>
          )}
        </div>
        <p className={cx('mt-1 text-[13.5px]', t.sub)}>{tr('topic.sub').replace('{t}', heading)}</p>
      </header>

      {resolved && !topic && (
        <EmptyState label={tr('topic.empty')} sub={tr('topic.emptySub')} icon={Flame} />
      )}

      {topic && (
        <>
          {tickers.length > 0 && (
            <section className="mb-7">
              <h2 className={cx('mb-2 text-[12px] font-semibold uppercase tracking-wide', t.sub)}>
                {tr('topic.stocks')}
              </h2>
              <div className="flex gap-4 overflow-x-auto pb-2">
                {tickers.map(tk => (
                  <div key={tk} className="w-[270px] shrink-0">
                    <StockCard
                      security={securities[tk] ?? placeholder(tk)}
                      quote={quotes.get(tk)}
                      closes={bars[tk]}
                    />
                  </div>
                ))}
              </div>
            </section>
          )}

          <section>
            <h2 className={cx('mb-2 text-[12px] font-semibold uppercase tracking-wide', t.sub)}>
              {tr('topic.news')}
            </h2>
            {newsLoading ? (
              <FeedSkeleton />
            ) : news.length === 0 ? (
              <EmptyState label={tr('topic.noNews')} sub={tr('topic.noNewsSub')} icon={Flame} />
            ) : (
              <div className="tw-fade">
                {news.map((n, i) => (
                  <TimelineItem
                    key={`${n.ticker}:${n.id}`}
                    entry={{kind: 'news', item: n}}
                    showTicker
                    last={i === news.length - 1}
                  />
                ))}
              </div>
            )}
          </section>
        </>
      )}
    </div>
  );
}
