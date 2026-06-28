/** Static app configuration. */

/**
 * Evergreen DEMO deep reports — mega-caps whose deep research report is fully unlocked to
 * EVERYONE (incl. anon prospects), so the FE fetches it even when logged out. Must mirror the
 * backend's demoReportTickers (internal/api/api.go). A mismatch only degrades gracefully (an
 * anon fetch of a non-demo ticker just 401s → the anon login gate). Exactly one by design.
 */
const DEMO_REPORT_TICKERS = new Set(['AAPL']);
export function isDemoReportTicker(ticker: string): boolean {
  return DEMO_REPORT_TICKERS.has((ticker || '').toUpperCase().trim());
}
/** The canonical demo report URL (path only; prefix with the locale where needed). */
export const DEMO_REPORT_PATH = '/stock/AAPL/research';

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

/**
 * Whether to show the "Continue with Google" button. The Google provider is now
 * configured in Supabase, so it's ON by default; set `NEXT_PUBLIC_GOOGLE_OAUTH=0`
 * to hide it again (e.g. if the provider is temporarily disabled).
 */
export const GOOGLE_OAUTH_ENABLED = process.env.NEXT_PUBLIC_GOOGLE_OAUTH !== '0';

/** Product name, used in the header and document title. */
export const APP_NAME = 'Tickwind';

/** Short tagline shown in the header. */
export const APP_TAGLINE = "Read every tick. See where the market's blowing.";

/**
 * Path-based bilingual hreflang alternates for an un-prefixed page `path` (e.g.
 * `/smart-money`). The site does URL-level i18n via `/en` and `/zh` route
 * segments, so each language gets its own indexable URL. `x-default` points at
 * the English variant. `canonical` is the current locale's URL when the page
 * knows its locale (pass `currentLocale`), else it defaults to English. Assign
 * to a page's `metadata.alternates`.
 *
 * @param path the path WITHOUT a locale prefix, starting with `/`.
 * @param currentLocale the page's own locale, used for the canonical URL.
 */
export function langAlternates(
  path: string,
  currentLocale: 'en' | 'zh' = 'en',
): {
  canonical: string;
  languages: Record<string, string>;
} {
  const en = `${SITE_URL}/en${path}`;
  const zh = `${SITE_URL}/zh${path}`;
  return {
    canonical: currentLocale === 'zh' ? zh : en,
    languages: {
      en,
      'zh-CN': zh,
      'x-default': en,
    },
  };
}
