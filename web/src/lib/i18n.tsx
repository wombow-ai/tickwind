'use client';

import {createContext, useCallback, useContext, useEffect, useMemo} from 'react';
import {usePathname, useRouter} from 'next/navigation';
import {dict} from '@/lib/dict';
import {DEFAULT_LOCALE, swapLocaleInPath} from '@/lib/locale';

/**
 * Path-based i18n (English / 简体中文). The active locale comes from the URL
 * `[locale]` route segment, fed into a React {@link LangProvider} Context that is
 * populated server-side (from `params`) so SSR renders the correct language — no
 * more English-only first paint. UI chrome strings are translated via
 * {@link useT}; data (prices, headlines, company names) is shown as-sourced.
 *
 * The hook API ({@link useLang} / {@link useT}) is intentionally unchanged from
 * the previous `<html lang>` + useSyncExternalStore implementation, so the ~592
 * `tr('key')` call sites keep working without modification.
 */
export type Lang = 'en' | 'zh';

/** Cookie the middleware reads to remember the visitor's locale preference. */
const COOKIE_KEY = 'tw-lang';

/**
 * @deprecated Superseded by path-based routing — the locale now comes from the
 * URL `[locale]` segment (resolved server-side), not a pre-paint script. Kept
 * exported for back-compat; no longer referenced.
 */
export const langNoFlashScript = `(function(){})();`;

/** The dictionary, re-exported from the non-client {@link dict} module. */
export {dict};

/** Context holding the active locale (from the route param via LangProvider). */
const LangContext = createContext<Lang>(DEFAULT_LOCALE);

/**
 * Provides the active locale to the client hook tree. Rendered by the
 * `[locale]` layout with `locale` resolved from the route params, so the value
 * is correct during SSR. On the client it mirrors the locale onto
 * `document.documentElement.lang` (so the `[data-i18n]` CSS dual-render rule
 * keeps working) and persists the `tw-lang` cookie (so the middleware remembers
 * the preference on the next bare-path visit).
 */
export function LangProvider({lang, children}: {lang: Lang; children: React.ReactNode}) {
  useEffect(() => {
    document.documentElement.lang = lang;
    try {
      document.cookie = `${COOKIE_KEY}=${lang};path=/;max-age=31536000;samesite=lax`;
    } catch {
      // Cookies disabled: locale still applies via the URL for this session.
    }
  }, [lang]);
  return <LangContext.Provider value={lang}>{children}</LangContext.Provider>;
}

/**
 * Current language + setters. `lang` is read from the {@link LangProvider}
 * context (correct during SSR). `setLang`/`toggle` navigate to the same page in
 * the target locale (swapping the leading path segment) and persist the cookie.
 */
export function useLang(): {lang: Lang; setLang: (l: Lang) => void; toggle: () => void} {
  const lang = useContext(LangContext);
  const router = useRouter();
  const pathname = usePathname();
  const setLang = useCallback(
    (next: Lang) => {
      try {
        document.cookie = `${COOKIE_KEY}=${next};path=/;max-age=31536000;samesite=lax`;
      } catch {
        // Cookies disabled: the URL still carries the locale.
      }
      // `usePathname()` excludes the query string and hash, so re-append them
      // from `window.location` (this runs only on a user click, client-side) to
      // preserve e.g. `?tab=alerts` / `?q=AAPL` / `#anchor` across the toggle.
      router.push(
        swapLocaleInPath(pathname, next) +
          window.location.search +
          window.location.hash,
      );
    },
    [router, pathname],
  );
  const toggle = useCallback(() => setLang(lang === 'zh' ? 'en' : 'zh'), [lang, setLang]);
  return {lang, setLang, toggle};
}

/**
 * Returns a translator `t(key)` for the current language, falling back to the
 * English string, then the key itself, so a missing translation never blanks
 * the UI.
 */
export function useT(): (key: string) => string {
  const lang = useContext(LangContext);
  return useMemo(() => (key: string) => dict[lang][key] ?? dict.en[key] ?? key, [lang]);
}
