'use client';

import {useEffect, useState, type FormEvent} from 'react';
import {
  ApiError,
  addToWatchlist,
  getWatchlist,
  removeFromWatchlist,
} from '@/lib/api';
import {Watchlist} from '@/components/Watchlist';
import {ErrorState, LoadingState} from '@/components/states';

/**
 * The editable watchlist board: loads the tracked tickers from the API,
 * supports adding/removing them (the server is the source of truth — each
 * mutation returns the new list), and renders live prices via {@link Watchlist}.
 */
export function WatchlistBoard() {
  const [tickers, setTickers] = useState<readonly string[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    getWatchlist(controller.signal).then(
      res => setTickers(res.tickers),
      (err: unknown) => {
        if (!controller.signal.aborted) {
          setLoadError(
            err instanceof ApiError ? err.message : 'Failed to load watchlist',
          );
        }
      },
    );
    return () => controller.abort();
  }, []);

  if (loadError !== null) {
    return (
      <ErrorState title="Couldn't load your watchlist" message={loadError} />
    );
  }
  if (tickers === null) {
    return <LoadingState label="Loading watchlist…" />;
  }

  async function add(ticker: string) {
    const res = await addToWatchlist(ticker);
    setTickers(res.tickers);
  }
  async function remove(ticker: string) {
    const res = await removeFromWatchlist(ticker);
    setTickers(res.tickers);
  }

  return (
    <div className="space-y-5">
      <AddTicker onAdd={add} existing={tickers} />
      {tickers.length === 0 ? (
        <p className="rounded-xl border border-white/10 bg-white/[0.02] p-6 text-sm text-zinc-400">
          Your watchlist is empty. Add a ticker above to start tracking it.
        </p>
      ) : (
        <Watchlist tickers={tickers} onRemove={remove} />
      )}
    </div>
  );
}

/** Inline add-ticker form. */
function AddTicker({
  onAdd,
  existing,
}: {
  onAdd: (ticker: string) => Promise<void>;
  existing: readonly string[];
}) {
  const [value, setValue] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(event: FormEvent) {
    event.preventDefault();
    const ticker = value.trim().toUpperCase();
    if (ticker === '' || busy) {
      return;
    }
    if (existing.includes(ticker)) {
      setError(`${ticker} is already on your watchlist`);
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await onAdd(ticker);
      setValue('');
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to add ticker');
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="space-y-1.5">
      <div className="flex gap-2">
        <input
          value={value}
          onChange={event => setValue(event.target.value)}
          placeholder="Add a ticker — e.g. MSFT"
          className="w-56 rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-sm uppercase text-zinc-200 placeholder:normal-case placeholder:text-zinc-600 focus:border-sky-500/50 focus:outline-none"
        />
        <button
          type="submit"
          disabled={busy || value.trim() === ''}
          className="rounded-lg bg-sky-500/90 px-4 py-2 text-sm font-medium text-white transition hover:bg-sky-400 disabled:opacity-40"
        >
          {busy ? 'Adding…' : 'Add'}
        </button>
      </div>
      {error ? <p className="text-xs text-red-400">{error}</p> : null}
    </form>
  );
}
