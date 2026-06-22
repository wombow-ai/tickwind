/**
 * Helpers for the `/compare/[pair]` side-by-side stock comparison pages.
 *
 * A pair is encoded in the URL as `a-vs-b` (lowercase, e.g. `aapl-vs-msft`). The curated
 * COMPARE_PAIRS are prerendered (popular rivalries); any other fetchable pair renders on-demand
 * via ISR. Every number on the page is Go-computed (fundamentals + quote) — anti-hallucination-safe,
 * and the page never declares a "winner" (just the figures, side by side).
 */

/** Curated, frequently-searched comparisons — prerendered + cross-linked from the hub. */
export const COMPARE_PAIRS: readonly (readonly [string, string])[] = [
  ['AAPL', 'MSFT'],
  ['NVDA', 'AMD'],
  ['GOOGL', 'META'],
  ['AMZN', 'GOOGL'],
  ['TSLA', 'NVDA'],
  ['MSFT', 'GOOGL'],
  ['AVGO', 'AMD'],
  ['META', 'NFLX'],
];

/** A ticker as it appears in a compare slug: letters/digits plus `.`/`-` (e.g. BRK.B). */
const TICKER_RE = /^[A-Z0-9][A-Z0-9.\-]{0,9}$/;

/** Builds the canonical lowercase slug for a pair, e.g. ("AAPL","MSFT") → "aapl-vs-msft". */
export function pairSlug(a: string, b: string): string {
  return `${a.toLowerCase()}-vs-${b.toLowerCase()}`;
}

/**
 * Parses a compare slug into a normalized [A, B] upper-case ticker pair, or null when it isn't a
 * well-formed two-ticker `a-vs-b` slug (so the page can 404/noindex rather than fetch garbage).
 * Rejects a pair of the same ticker (nothing to compare).
 */
export function parsePair(slug: string): [string, string] | null {
  const parts = slug.split('-vs-');
  if (parts.length !== 2) return null;
  const a = parts[0].trim().toUpperCase();
  const b = parts[1].trim().toUpperCase();
  if (!TICKER_RE.test(a) || !TICKER_RE.test(b) || a === b) return null;
  return [a, b];
}
