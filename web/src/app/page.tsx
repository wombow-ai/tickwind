import {WatchlistBoard} from '@/components/WatchlistBoard';

/** Home board: the editable watchlist of tracked tickers. */
export default function HomePage() {
  return (
    <div className="space-y-8">
      <section className="space-y-2">
        <h1 className="text-2xl font-bold tracking-tight text-zinc-50 sm:text-3xl">
          Watchlist
        </h1>
        <p className="max-w-2xl text-sm text-zinc-400">
          Your tracked stocks at a glance — add or remove tickers, and open any
          one for live price, news, discussion and filings.
        </p>
      </section>
      <WatchlistBoard />
    </div>
  );
}
