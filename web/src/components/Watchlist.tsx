'use client';

import {getStock} from '@/lib/api';
import {useAsync} from '@/lib/useAsync';
import {
  StockCard,
  StockCardError,
  StockCardSkeleton,
} from '@/components/StockCard';

interface WatchlistProps {
  tickers: readonly string[];
}

/**
 * Renders the watchlist board. Each ticker is fetched independently so one
 * slow or failing symbol never blocks the rest of the grid.
 */
export function Watchlist({tickers}: WatchlistProps) {
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {tickers.map(ticker => (
        <WatchlistItem key={ticker} ticker={ticker} />
      ))}
    </div>
  );
}

/** A single self-fetching watchlist tile. */
function WatchlistItem({ticker}: {ticker: string}) {
  const state = useAsync(signal => getStock(ticker, signal), ticker);

  switch (state.status) {
    case 'loading':
      return <StockCardSkeleton ticker={ticker} />;
    case 'error':
      return <StockCardError ticker={ticker} message={state.error} />;
    case 'success':
      return <StockCard security={state.data} />;
  }
}
