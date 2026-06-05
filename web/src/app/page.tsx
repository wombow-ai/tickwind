import {Watchlist} from '@/components/Watchlist';
import {DEFAULT_WATCHLIST} from '@/lib/config';

/** Home board: the default watchlist of tracked tickers. */
export default function HomePage() {
  return (
    <div className="space-y-8">
      <section className="space-y-2">
        <h1 className="text-2xl font-bold tracking-tight text-zinc-50 sm:text-3xl">
          Watchlist
        </h1>
        <p className="max-w-2xl text-sm text-zinc-400">
          Your tracked stocks at a glance. Open any one for its latest SEC
          filings timeline.
        </p>
      </section>
      <Watchlist tickers={DEFAULT_WATCHLIST} />
    </div>
  );
}
