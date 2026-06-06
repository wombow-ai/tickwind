'use client';

import {MessageSquare, Newspaper} from 'lucide-react';
import Link from 'next/link';
import {useCallback, useEffect, useState} from 'react';
import {
  getNewsBatch,
  getSocialBatch,
  type NewsItem,
  type Post,
} from '@/lib/api';
import {POPULAR_TICKERS} from '@/lib/config';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';
import {TimelineItem} from '@/components/TimelineItem';
import {EmptyState, ErrorState, FeedSkeleton} from '@/components/ui/states';

type Status = 'loading' | 'ready' | 'error';

/**
 * A full-page aggregated market feed: News (消息) or Discussion (评论) across the
 * most-watched US stocks. These are the "see all" destinations the home-hub
 * module cards link to. Public + SEO-friendly (popular-ticker universe).
 */
export function FeedPage({
  kind,
  topic,
  topicLabel,
}: {
  kind: 'news' | 'discussion';
  topic?: string;
  topicLabel?: string;
}) {
  const dark = useDark();
  const t = tok(dark);
  const isNews = kind === 'news';
  const filtered = isNews && !!topic;
  const [status, setStatus] = useState<Status>('loading');
  const [news, setNews] = useState<NewsItem[]>([]);
  const [posts, setPosts] = useState<Post[]>([]);

  const load = useCallback(() => {
    setStatus('loading');
    if (isNews) {
      getNewsBatch([...POPULAR_TICKERS], topic ? 12 : 6, undefined, topic).then(
        r => {
          setNews(r.news ?? []);
          setStatus('ready');
        },
        () => setStatus('error'),
      );
    } else {
      getSocialBatch([...POPULAR_TICKERS]).then(
        r => {
          setPosts(r.posts ?? []);
          setStatus('ready');
        },
        () => setStatus('error'),
      );
    }
  }, [isNews, topic]);

  useEffect(() => {
    load();
  }, [load]);

  const Icon = isNews ? Newspaper : MessageSquare;
  const title = filtered
    ? topicLabel || 'Filtered news'
    : isNews
      ? 'Market news'
      : 'Discussion';
  const sub = filtered
    ? `Latest headlines about “${topicLabel || topic}”.`
    : isNews
      ? 'The latest headlines across the most-watched US stocks.'
      : 'What people are saying about the most-watched US stocks — StockTwits, Bluesky and more.';
  const empty = filtered
    ? {label: 'No recent news for this topic', sub: 'Try another topic or see all news.'}
    : isNews
      ? {label: 'No news yet', sub: 'Headlines will appear here as they break.'}
      : {label: 'No chatter yet', sub: 'Posts will show up here as they come in.'};
  const count = isNews ? news.length : posts.length;

  return (
    <div className="mx-auto max-w-2xl">
      <header className="mb-5">
        <h1
          className={cx(
            'flex items-center gap-2 text-[22px] font-bold tracking-tight',
            t.text,
          )}
        >
          <Icon size={20} className={dark ? 'text-teal-300' : 'text-teal-600'} />
          {title}
        </h1>
        <p className={cx('mt-1 text-[13.5px]', t.sub)}>{sub}</p>
        {filtered && (
          <Link
            href="/news"
            className={cx('mt-2 inline-block text-[12.5px] font-semibold', t.accentText)}
          >
            ← All news
          </Link>
        )}
      </header>

      {status === 'loading' ? (
        <FeedSkeleton />
      ) : status === 'error' ? (
        <ErrorState onRetry={load} />
      ) : count === 0 ? (
        <EmptyState label={empty.label} sub={empty.sub} icon={Icon} />
      ) : (
        <div className="tw-fade">
          {isNews
            ? news.map((n, i) => (
                <TimelineItem
                  key={`${n.ticker}:${n.id}`}
                  entry={{kind: 'news', item: n}}
                  showTicker
                  last={i === news.length - 1}
                />
              ))
            : posts.map((p, i) => (
                <TimelineItem
                  key={`${p.ticker}:${p.id}`}
                  entry={{kind: 'disc', item: p}}
                  showTicker
                  last={i === posts.length - 1}
                />
              ))}
        </div>
      )}
    </div>
  );
}
