'use client';

import Link from 'next/link';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';
import {Logo} from '@/components/ui/atoms';

const COLUMNS: Array<[string, Array<[string, string]>]> = [
  [
    'Product',
    [
      ['Markets', '/'],
      ['Your watchlist', '/watchlist'],
      ["What's new", '/announcements'],
    ],
  ],
  [
    'Account',
    [
      ['Settings', '/settings'],
      ['Log in', '/login'],
      ['Sign up', '/signup'],
    ],
  ],
];

/** The site footer with brand, link columns, and a disclaimer. */
export function Footer() {
  const dark = useDark();
  const t = tok(dark);
  return (
    <footer className={cx('mt-8 border-t', t.border, dark ? 'bg-slate-950' : 'bg-slate-50/60')}>
      <div className="mx-auto max-w-6xl px-4 py-12 sm:px-6">
        <div className="grid gap-8 sm:grid-cols-4">
          <div className="sm:col-span-2">
            <Logo size={28} />
            <p className={cx('mt-3 max-w-[220px] text-[13px]', t.sub)}>
              Read every tick. See where the market&apos;s blowing.
            </p>
          </div>
          {COLUMNS.map(([heading, items]) => (
            <div key={heading}>
              <p className={cx('mb-3 text-[12px] font-semibold', t.text)}>{heading}</p>
              <ul className="space-y-2">
                {items.map(([label, href]) => (
                  <li key={label}>
                    <Link href={href} className={cx('text-[13px] hover:opacity-80', t.sub)}>
                      {label}
                    </Link>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
        <div
          className={cx(
            'mt-10 flex flex-wrap items-center justify-between gap-2 border-t pt-5',
            t.hair,
          )}
        >
          <p className={cx('text-[12px]', t.faint)}>
            © 2026 Tickwind. Not investment advice.
          </p>
          <p className={cx('text-[12px]', t.faint)}>Made for the curious investor.</p>
        </div>
      </div>
    </footer>
  );
}
