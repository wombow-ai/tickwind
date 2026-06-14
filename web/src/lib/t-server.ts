/**
 * Server-side translator. Mirrors the client {@link useT} fallback chain
 * (`dict[locale][key] ?? dict.en[key] ?? key`) so Server Components and
 * `generateMetadata` can translate chrome strings without the React context.
 * Plain data import only — safe outside the client boundary.
 */

import {dict} from '@/lib/dict';
import type {Lang} from '@/lib/i18n';

/** Returns a translator `t(key)` for `locale`, falling back to en then the key. */
export function getT(locale: Lang): (key: string) => string {
  return (key: string) => dict[locale][key] ?? dict.en[key] ?? key;
}
