'use client';

import Link from 'next/link';
import {useSearchParams} from 'next/navigation';
import {
  getFilings,
  getNews,
  getStock,
  type Filing,
  type NewsItem,
  type Security,
} from '@/lib/api';
import {useAsync} from '@/lib/useAsync';
import {useQuotes} from '@/lib/useQuotes';
import {StockHeader} from '@/components/StockHeader';
import {FilingsTimeline} from '@/components/FilingsTimeline';
import {NewsTimeline} from '@/components/NewsTimeline';
import {EmptyState, ErrorState, LoadingState} from '@/components/states';

/** Combined payload for the detail page. */
interface StockDetailData {
  security: Security;
  news: NewsItem[];
  filings: Filing[];
}

/** Number of filings to request for the timeline. */
const FILINGS_LIMIT = 25;

/** Number of news articles to request for the timeline. */
const NEWS_LIMIT = 25;

/**
 * Detail view for a single stock. Reads `ticker` from the URL query, fetches
 * the security and its filings, and renders a header plus a filings timeline.
 * Must be rendered inside a `<Suspense>` boundary because it calls
 * {@link useSearchParams} (required for static export).
 */
export function StockDetail() {
  const params = useSearchParams();
  const ticker = (params.get('ticker') ?? '').trim().toUpperCase();

  if (!ticker) {
    return (
      <ErrorState
        title="No ticker selected"
        message="Add a ticker to the URL, e.g. /stock?ticker=AAPL."
        action={<BackLink />}
      />
    );
  }

  return <StockDetailBody ticker={ticker} />;
}

/** Fetches and renders detail content for a known ticker. */
function StockDetailBody({ticker}: {ticker: string}) {
  const state = useAsync<StockDetailData>(
    async signal => {
      // Fetch the security first; if it 404s the catch surfaces a clear error
      // rather than a confusing empty news/filings list. The two timelines are
      // independent, so fetch them together once the ticker is known to exist.
      const security = await getStock(ticker, signal);
      const [newsRes, filingsRes] = await Promise.all([
        getNews(ticker, NEWS_LIMIT, signal),
        getFilings(ticker, FILINGS_LIMIT, signal),
      ]);
      return {security, news: newsRes.news, filings: filingsRes.filings};
    },
    ticker,
  );
  const quotes = useQuotes([ticker]);
  const quote = quotes.get(ticker);

  switch (state.status) {
    case 'loading':
      return <LoadingState label={`Loading ${ticker}…`} />;
    case 'error':
      return (
        <ErrorState
          title={`Couldn't load ${ticker}`}
          message={state.error}
          action={<BackLink />}
        />
      );
    case 'success':
      return (
        <div className="space-y-8">
          <StockHeader security={state.data.security} quote={quote} />
          <section className="space-y-4">
            <div className="flex items-baseline justify-between">
              <h2 className="text-lg font-semibold text-zinc-100">News</h2>
              <span className="text-xs text-zinc-500">
                {state.data.news.length} recent
              </span>
            </div>
            {state.data.news.length === 0 ? (
              <EmptyState
                title="No news yet"
                message="The backend hasn't ingested any news for this ticker."
              />
            ) : (
              <NewsTimeline news={state.data.news} />
            )}
          </section>
          <section className="space-y-4">
            <div className="flex items-baseline justify-between">
              <h2 className="text-lg font-semibold text-zinc-100">Filings</h2>
              <span className="text-xs text-zinc-500">
                {state.data.filings.length} recent
              </span>
            </div>
            {state.data.filings.length === 0 ? (
              <EmptyState
                title="No filings yet"
                message="The backend hasn't ingested any filings for this ticker."
              />
            ) : (
              <FilingsTimeline filings={state.data.filings} />
            )}
          </section>
        </div>
      );
  }
}

/** Link back to the watchlist board. */
function BackLink() {
  return (
    <Link
      href="/"
      className="inline-flex items-center gap-1 text-sm font-medium text-sky-400 hover:text-sky-300"
    >
      <span aria-hidden>←</span> Back to watchlist
    </Link>
  );
}
