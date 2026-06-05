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
  /** When provided, each tile shows a control to remove its ticker. */
  onRemove?: (ticker: string) => void;
}

/**
 * Renders the watchlist board. Each ticker's security is fetched independently
 * so one slow or failing symbol never blocks the rest of the grid, while live
 * prices stream in over a single shared {@link useQuotes} subscription.
 */
export function Watchlist({tickers, onRemove}: WatchlistProps) {
  const quotes = useQuotes(tickers);
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {tickers.map(ticker => (
        <Tile
          key={ticker}
          ticker={ticker}
          quote={quotes.get(ticker.toUpperCase())}
          onRemove={onRemove}
        />
      ))}
    </div>
  );
}

/** A tile: the card, plus an optional remove button overlaid in the corner. */
function Tile({
  ticker,
  quote,
  onRemove,
}: {
  ticker: string;
  quote?: Quote;
  onRemove?: (ticker: string) => void;
}) {
  if (!onRemove) {
    return <WatchlistItem ticker={ticker} quote={quote} />;
  }
  return (
    <div className="relative">
      <WatchlistItem ticker={ticker} quote={quote} />
      <button
        type="button"
        onClick={() => onRemove(ticker)}
        aria-label={`Remove ${ticker} from watchlist`}
        className="absolute right-2 top-2 z-10 flex h-6 w-6 items-center justify-center rounded-full border border-white/10 bg-zinc-900/80 text-zinc-400 opacity-70 transition hover:border-rose-500/40 hover:text-rose-300 hover:opacity-100"
      >
        ×
      </button>
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
