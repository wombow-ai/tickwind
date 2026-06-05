'use client';

import {getStock, type Quote} from '@/lib/api';
import {useAsync} from '@/lib/useAsync';
import {useQuotes} from '@/lib/useQuotes';
import {
  StockCard,
  StockCardError,
  StockCardSkeleton,
} from '@/components/StockCard';

interface WatchlistProps {
  tickers: readonly string[];
}

/**
 * Renders the watchlist board. Each ticker's security is fetched independently
 * so one slow or failing symbol never blocks the rest of the grid, while live
 * prices stream in over a single shared {@link useQuotes} subscription.
 */
export function Watchlist({tickers}: WatchlistProps) {
  const quotes = useQuotes(tickers);
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {tickers.map(ticker => (
        <WatchlistItem
          key={ticker}
          ticker={ticker}
          quote={quotes.get(ticker.toUpperCase())}
        />
      ))}
    </div>
  );
}

/** A single self-fetching watchlist tile. */
function WatchlistItem({ticker, quote}: {ticker: string; quote?: Quote}) {
  const state = useAsync(signal => getStock(ticker, signal), ticker);

  switch (state.status) {
    case 'loading':
      return <StockCardSkeleton ticker={ticker} />;
    case 'error':
      return <StockCardError ticker={ticker} message={state.error} />;
    case 'success':
      return <StockCard security={state.data} quote={quote} />;
  }
}
