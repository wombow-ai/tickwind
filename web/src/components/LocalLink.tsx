'use client';

import NextLink from 'next/link';
import {usePathname} from 'next/navigation';
import type {ComponentProps} from 'react';
import {DEFAULT_LOCALE, isLocale, localizedPath} from '@/lib/locale';
import type {Lang} from '@/lib/i18n';

type NextLinkProps = ComponentProps<typeof NextLink>;

/**
 * Drop-in replacement for `next/link` that keeps internal navigation inside the
 * current locale. It reads the active locale from the first pathname segment and
 * prefixes root-relative internal `href`s via {@link localizedPath} — so the
 * ~101 existing `<Link href="/…">` call sites stay locale-correct without being
 * touched. External (`http`/`//`), `#`, `mailto:`/`tel:` and already-prefixed
 * hrefs pass through unchanged.
 *
 * Only string hrefs are localized (the only form used in this codebase); a
 * `UrlObject` href is forwarded verbatim.
 */
export function LocalLink({href, ...rest}: NextLinkProps) {
  const pathname = usePathname();
  const seg = pathname.slice(1).split('/', 1)[0];
  const locale: Lang = isLocale(seg) ? seg : DEFAULT_LOCALE;
  const localized = typeof href === 'string' ? localizedPath(locale, href) : href;
  return <NextLink href={localized} {...rest} />;
}

export default LocalLink;
