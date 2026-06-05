'use client';

import {useEffect, useState} from 'react';
import {API_BASE, getQuote, type Quote} from '@/lib/api';

/**
 * Subscribes to live prices for a set of tickers.
 *
 * On mount (and whenever the ticker set changes) it fetches an initial quote
 * per ticker via {@link getQuote}, then opens a single
 * `GET /v1/stream` {@link EventSource} and applies every `quote` event to a
 * `Map<ticker, Quote>`. Tickers without a quote yet (e.g. the backend returned
 * 404 because no price key is configured) are simply absent from the map, so
 * callers can render a placeholder.
 *
 * The map identity changes on each update, so consumers re-render. Initial
 * fetches are aborted and the stream is closed on unmount.
 *
 * @param tickers Symbols to track. Order is irrelevant; duplicates are ignored.
 * @returns A map from uppercased ticker to its latest {@link Quote}.
 */
export function useQuotes(
  tickers: readonly string[],
): ReadonlyMap<string, Quote> {
  const [quotes, setQuotes] = useState<ReadonlyMap<string, Quote>>(
    () => new Map(),
  );

  // Canonical, de-duplicated symbols. The sorted join is the effect key, so the
  // subscription only resets when the *set* of tickers actually changes — not
  // on every render that passes a fresh array literal.
  const symbols = Array.from(
    new Set(tickers.map(t => t.trim().toUpperCase()).filter(Boolean)),
  );
  const key = [...symbols].sort().join(',');

  // Reset the map synchronously when the ticker set changes, following React's
  // "adjusting state when a prop changes" pattern (state-held previous key, no
  // effect setState). Stale symbols must not linger between subscriptions.
  const [activeKey, setActiveKey] = useState(key);
  if (key !== activeKey) {
    setActiveKey(key);
    setQuotes(new Map());
  }

  useEffect(() => {
    if (symbols.length === 0) {
      return;
    }

    const wanted = new Set(symbols);

    /** Merges one quote into state if it belongs to the tracked set. */
    function apply(quote: Quote): void {
      const ticker = quote.ticker.toUpperCase();
      if (!wanted.has(ticker)) {
        return;
      }
      setQuotes(prev => {
        const next = new Map(prev);
        next.set(ticker, quote);
        return next;
      });
    }

    const controller = new AbortController();
    for (const ticker of symbols) {
      getQuote(ticker, controller.signal).then(apply, () => {
        // No quote yet (404) or a transient fetch error: leave it unset and let
        // the stream fill it in when a price arrives.
      });
    }

    const source = new EventSource(`${API_BASE}/v1/stream`);
    source.addEventListener('quote', event => {
      let quote: Quote;
      try {
        quote = JSON.parse((event as MessageEvent<string>).data) as Quote;
      } catch {
        return;
      }
      apply(quote);
    });

    return () => {
      controller.abort();
      source.close();
    };
    // `symbols`/`wanted` are derived from `key`; re-subscribing on `key` keeps
    // the effect from tearing down on unrelated re-renders.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);

  return quotes;
}
