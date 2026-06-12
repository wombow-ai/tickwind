'use client';

import Link from 'next/link';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';
import {Logo} from '@/components/ui/atoms';

// Each column = [headingKey, [[labelKey, href], …]]; headings and labels are
// i18n dict keys, resolved via the translator at render time.
const COLUMNS: Array<[string, Array<[string, string]>]> = [
  [
    'footer.product',
    [
      ['nav.markets', '/'],
      ['mod.hotStocks', '/hot'],
      ['nav.opportunities', '/opportunities'],
      ['nav.news', '/news'],
      ['mod.discussion', '/discussion'],
      ['nav.watchlist', '/watchlist'],
      ['nav.whatsnew', '/announcements'],
    ],
  ],
  [
    'footer.account',
    [
      ['nav.settings', '/settings'],
      ['nav.login', '/login'],
      ['nav.signup', '/signup'],
    ],
  ],
];

/** The site footer with brand, link columns, and a disclaimer. */
export function Footer() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  return (
    <footer className={cx('mt-8 border-t', t.border, dark ? 'bg-slate-950' : 'bg-slate-50/60')}>
      <div className="mx-auto max-w-6xl px-4 py-12 sm:px-6">
        <div className="grid gap-8 sm:grid-cols-4">
          <div className="sm:col-span-2">
            <Logo size={28} />
            <p className={cx('mt-3 max-w-[220px] text-[13px]', t.sub)}>
              {tr('footer.blurb')}
            </p>
          </div>
          {/* Desktop: clean labeled text columns. */}
          {COLUMNS.map(([heading, items]) => (
            <div key={heading} className="hidden sm:block">
              <p className={cx('mb-3 text-[12px] font-semibold', t.text)}>{tr(heading)}</p>
              <ul className="space-y-2">
                {items.map(([label, href]) => (
                  <li key={label}>
                    <Link href={href} className={cx('text-[13px] hover:opacity-80', t.sub)}>
                      {tr(label)}
                    </Link>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>

        {/* Mobile: a 2-column grid of tappable chip-blocks (a tall plain-text
            list reads poorly on phones), matching the home directory cards. */}
        <nav className="mt-6 grid grid-cols-2 gap-2 sm:hidden" aria-label="Footer">
          {COLUMNS.flatMap(([, items]) => items).map(([label, href]) => (
            <Link
              key={label}
              href={href}
              className={cx(
                'rounded-xl border px-3 py-2.5 text-[13px] font-medium transition active:opacity-70',
                t.border,
                t.surf2,
                t.sub,
              )}
            >
              {tr(label)}
            </Link>
          ))}
        </nav>
        <div
          className={cx(
            'mt-10 flex flex-wrap items-center justify-between gap-2 border-t pt-5',
            t.hair,
          )}
        >
          <p className={cx('text-[12px]', t.faint)}>{tr('footer.copyright')}</p>
          <p className={cx('text-[12px]', t.faint)}>{tr('footer.tagline')}</p>
        </div>
      </div>
    </footer>
  );
}
