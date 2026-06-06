/** Static app configuration. */

/**
 * Popular US tickers shown on the public board to anonymous visitors, so the
 * entry page is data-first (no marketing page). Keep this in sync with the
 * backend's default `WATCHLIST` env so every tile has live data to show.
 */
export const POPULAR_TICKERS: readonly string[] = [
  'AAPL',
  'NVDA',
  'TSLA',
  'MSFT',
  'AMZN',
  'GOOGL',
  'META',
  'AMD',
  'NFLX',
  'AVGO',
];

/** Suggested tickers offered on the empty watchlist state. */
export const SUGGESTED_TICKERS: readonly string[] = ['AAPL', 'NVDA', 'TSLA'];

/**
 * Canonical public origin, used for metadata, robots and the sitemap. Override
 * with `NEXT_PUBLIC_SITE_URL` (e.g. a Vercel preview URL); defaults to prod.
 */
export const SITE_URL: string = (
  process.env.NEXT_PUBLIC_SITE_URL ?? 'https://tickwind.com'
).replace(/\/+$/, '');

/** Product name, used in the header and document title. */
export const APP_NAME = 'Tickwind';

/** Short tagline shown in the header. */
export const APP_TAGLINE = "Read every tick. See where the market's blowing.";
