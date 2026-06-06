'use client';

/**
 * Live demo card for the Horizon landing page.
 *
 * Fetches a real quote, security name, and recent news for a single ticker from
 * the public Tickwind API and presents them on a calm, hairline-bordered card.
 * It models the three meaningful outcomes explicitly — loading, error, and
 * success — and treats a missing quote (a 404 from `/quote`, common outside
 * market hours or before a price key is configured) as a soft, non-error state
 * rather than a failure, so the marketing surface always reads as composed.
 */

import Link from 'next/link';
import {
  ApiError,
  getNews,
  getQuote,
  getStock,
  type NewsItem,
  type Quote,
  type Security,
} from '@/lib/api';
import {formatPrice, formatPublishedDate, toDateTimeAttrFull} from '@/lib/format';
import {useAsync} from '@/lib/useAsync';
import {SessionBadge} from './SessionBadge';
import {ArrowUpRightIcon} from './icons';

/** Ticker featured in the live demo. */
const DEMO_TICKER = 'AAPL';

/** Recent headlines to surface on the card. */
const NEWS_LIMIT = 3;

/** What a single combined demo fetch resolves to. */
interface DemoData {
  security: Security | null;
  /** `null` when the API has no current price (e.g. `/quote` returned 404). */
  quote: Quote | null;
  news: NewsItem[];
}

/**
 * Loads the security, quote, and news for {@link DEMO_TICKER} in parallel.
 *
 * Security and news are best-effort: if either sub-request fails the card still
 * renders with whatever resolved, so one flaky upstream never blanks the demo.
 * The quote is likewise tolerant of a 404 (no price yet). A hard failure is
 * reserved for the case where everything fails — surfaced via {@link useAsync}.
 */
async function loadDemo(signal: AbortSignal): Promise<DemoData> {
  const [security, quote, news] = await Promise.all([
    getStock(DEMO_TICKER, signal).catch(() => null),
    getQuote(DEMO_TICKER, signal).catch((error: unknown) => {
      // A 404 means "no price right now", which is expected and fine. Any other
      // failure is real and should bubble so the card can show its error state.
      if (error instanceof ApiError && error.status === 404) {
        return null;
      }
      throw error;
    }),
    getNews(DEMO_TICKER, NEWS_LIMIT, signal)
      .then(res => res.news)
      .catch(() => [] as NewsItem[]),
  ]);
  return {security, quote, news};
}

/** The live demo card with loading, error, and success states. */
export function LiveDemo() {
  // `loadDemo` is a stable module-level function (no captured props/state), so
  // it can be passed straight to `useAsync`, which re-runs keyed on the ticker.
  const state = useAsync(loadDemo, DEMO_TICKER);

  return (
    <div
      id="live-example"
      className="scroll-mt-24 rounded-2xl border border-zinc-200 bg-white"
    >
      <DemoHeader />
      <div className="px-5 pb-5 sm:px-6 sm:pb-6">
        {state.status === 'loading' ? <DemoSkeleton /> : null}
        {state.status === 'error' ? <DemoError message={state.error} /> : null}
        {state.status === 'success' ? <DemoBody data={state.data} /> : null}
      </div>
    </div>
  );
}

/** Card chrome: a window-style label so the demo reads as a product preview. */
function DemoHeader() {
  return (
    <div className="flex items-center justify-between border-b border-zinc-100 px-5 py-3 sm:px-6">
      <span className="text-xs font-medium uppercase tracking-widest text-zinc-400">
        Live example
      </span>
      <Link
        href={`/stock?ticker=${DEMO_TICKER}`}
        className="group inline-flex items-center gap-1 text-xs font-medium text-zinc-500 transition-colors hover:text-indigo-700"
      >
        Open {DEMO_TICKER}
        <ArrowUpRightIcon className="h-3.5 w-3.5 transition-colors group-hover:text-indigo-700" />
      </Link>
    </div>
  );
}

/** Successful render: price block plus a short list of recent headlines. */
function DemoBody({data}: {data: DemoData}) {
  const {security, quote, news} = data;
  return (
    <div className="pt-5">
      <div className="flex items-start justify-between gap-4">
        <div>
          <div className="flex items-baseline gap-2">
            <span className="font-mono text-2xl font-semibold tracking-tight text-zinc-900">
              {DEMO_TICKER}
            </span>
            {security ? (
              <span className="truncate text-sm text-zinc-500">
                {security.name}
              </span>
            ) : null}
          </div>
          <PriceLine quote={quote} />
        </div>
        <SessionBadge session={quote?.session ?? 'closed'} />
      </div>

      <hr className="my-5 border-zinc-100" />

      <p className="mb-3 text-xs font-medium uppercase tracking-widest text-zinc-400">
        Latest news
      </p>
      {news.length > 0 ? (
        <ul className="space-y-3">
          {news.map((item, index) => (
            <NewsRow key={`${item.id || item.url}-${index}`} item={item} />
          ))}
        </ul>
      ) : (
        <p className="text-sm text-zinc-400">
          No recent headlines for {DEMO_TICKER} right now.
        </p>
      )}
    </div>
  );
}

/** The price figure, or a calm dash when no quote is available. */
function PriceLine({quote}: {quote: Quote | null}) {
  if (!quote) {
    return (
      <p className="mt-1 text-sm text-zinc-400">
        Price unavailable right now — it returns the moment a tick prints.
      </p>
    );
  }
  return (
    <p className="mt-1">
      <span className="font-mono text-4xl font-semibold tracking-tight text-zinc-900 tabular-nums sm:text-5xl">
        {formatPrice(quote.price)}
      </span>
      <span className="ml-2 align-top text-xs font-medium text-zinc-400">
        USD
      </span>
    </p>
  );
}

/** A single headline row linking out to the source article. */
function NewsRow({item}: {item: NewsItem}) {
  const dateTime = toDateTimeAttrFull(item.published);
  return (
    <li>
      <a
        href={item.url}
        target="_blank"
        rel="noopener noreferrer"
        className="group block rounded-lg border border-transparent px-3 py-2 transition-colors hover:border-zinc-200 hover:bg-zinc-50 focus:outline-none focus-visible:border-indigo-300"
      >
        <div className="flex items-center gap-2 text-xs text-zinc-400">
          {item.source ? (
            <span className="font-medium text-zinc-500">{item.source}</span>
          ) : null}
          {item.source ? <span aria-hidden>·</span> : null}
          <time dateTime={dateTime}>{formatPublishedDate(item.published)}</time>
        </div>
        <p className="mt-0.5 line-clamp-2 text-sm font-medium text-zinc-800 group-hover:text-indigo-700">
          {item.headline}
        </p>
      </a>
    </li>
  );
}

/** Loading placeholder mirroring the success layout's rhythm. */
function DemoSkeleton() {
  return (
    <div className="pt-5" aria-hidden>
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-3">
          <div className="h-5 w-40 animate-pulse rounded bg-zinc-100" />
          <div className="h-10 w-48 animate-pulse rounded bg-zinc-100" />
        </div>
        <div className="h-6 w-24 animate-pulse rounded-full bg-zinc-100" />
      </div>
      <hr className="my-5 border-zinc-100" />
      <div className="space-y-3">
        <div className="h-12 animate-pulse rounded-lg bg-zinc-100" />
        <div className="h-12 animate-pulse rounded-lg bg-zinc-100" />
        <div className="h-12 animate-pulse rounded-lg bg-zinc-100" />
      </div>
      <span className="sr-only">Loading live example…</span>
    </div>
  );
}

/** Error state shown when the demo cannot reach the API at all. */
function DemoError({message}: {message: string}) {
  return (
    <div className="pt-5">
      <p className="text-sm font-medium text-zinc-800">
        Couldn&rsquo;t load the live example.
      </p>
      <p className="mt-1 text-sm text-zinc-500">
        {message}. The public API may be briefly unavailable — the full app
        retries automatically.
      </p>
    </div>
  );
}
