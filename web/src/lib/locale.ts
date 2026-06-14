/**
 * Path-based i18n primitives — the single source of truth for the locale set and
 * the URL ⇄ locale plumbing. Deliberately NON-'use client' (plain functions, no
 * React) so Server Components, `generateMetadata`, the sitemap, robots and the
 * `proxy` middleware can all import it.
 */

import type {Lang} from '@/lib/i18n';

/** The supported locales, in priority order (the first is the default). */
export const LOCALES = ['en', 'zh'] as const;

/** Default / x-default locale, served when nothing else matches. */
export const DEFAULT_LOCALE: Lang = 'en';

/** Narrows an arbitrary value to a supported {@link Lang}. */
export function isLocale(x: unknown): x is Lang {
  return x === 'en' || x === 'zh';
}

/**
 * Prefixes a root-relative app path with `/${locale}`. External URLs (http…,
 * protocol-relative), in-page anchors (`#…`), `mailto:`/`tel:` and paths already
 * carrying a locale segment are returned untouched. The query string and hash
 * are preserved.
 */
export function localizedPath(locale: Lang, path: string): string {
  if (!path) return `/${locale}`;
  // Leave non-navigational / external targets alone.
  if (
    path.startsWith('http://') ||
    path.startsWith('https://') ||
    path.startsWith('//') ||
    path.startsWith('#') ||
    path.startsWith('mailto:') ||
    path.startsWith('tel:')
  ) {
    return path;
  }
  // Only root-relative paths get localized.
  if (!path.startsWith('/')) return path;
  // Already locale-prefixed (`/en`, `/zh`, `/en/…`, `/zh?…`)? Leave it.
  const seg = path.slice(1).split(/[/?#]/, 1)[0];
  if (isLocale(seg)) return path;
  return `/${locale}${path}`;
}

/**
 * Splits a pathname into its leading locale segment (if any) and the rest. A
 * bare path with no locale prefix yields `{locale: null, rest: <pathname>}`; the
 * `rest` always starts with `/` (or is `/` for a bare locale root).
 */
export function stripLocale(pathname: string): {locale: Lang | null; rest: string} {
  const seg = pathname.slice(1).split('/', 1)[0];
  if (isLocale(seg)) {
    const rest = pathname.slice(1 + seg.length);
    return {locale: seg, rest: rest === '' ? '/' : rest};
  }
  return {locale: null, rest: pathname || '/'};
}

/**
 * Replaces (or inserts) the leading locale segment of `pathname` with
 * `toLocale`, preserving the rest of the path. Used by the language toggle to
 * jump to the same page in the other language.
 */
export function swapLocaleInPath(pathname: string, toLocale: Lang): string {
  const {rest} = stripLocale(pathname);
  return rest === '/' ? `/${toLocale}` : `/${toLocale}${rest}`;
}
