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

/**
 * Whether to show the "Continue with Google" button. Hidden by default; set
 * `NEXT_PUBLIC_GOOGLE_OAUTH=1` once the Google provider is enabled in Supabase.
 */
export const GOOGLE_OAUTH_ENABLED = process.env.NEXT_PUBLIC_GOOGLE_OAUTH === '1';

/** Product name, used in the header and document title. */
export const APP_NAME = 'Tickwind';

/** Short tagline shown in the header. */
export const APP_TAGLINE = "Read every tick. See where the market's blowing.";

/**
 * Bilingual hreflang alternates for a page `path` (e.g. `/smart-money`). The
 * site does URL-level i18n via a `?lang=zh|en` param (resolved before paint by
 * the language no-flash script), so search engines can index both languages of
 * the same page. `canonical` is the clean path; `x-default` points there (it
 * defaults to English). Assign to a page's `metadata.alternates`.
 */
export function langAlternates(path: string): {
  canonical: string;
  languages: Record<string, string>;
} {
  const base = `${SITE_URL}${path}`;
  return {
    canonical: base,
    languages: {
      en: `${base}?lang=en`,
      'zh-CN': `${base}?lang=zh`,
      'x-default': base,
    },
  };
}
