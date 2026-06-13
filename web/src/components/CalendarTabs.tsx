'use client';

import {CalendarClock} from 'lucide-react';
import Link from 'next/link';
import {usePathname} from 'next/navigation';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

/**
 * Shared tab row for the unified `/calendar/*` shell. Each tab is a real link to
 * an independently-indexable subpath (Earnings · Macro · IPO), so SEO metadata
 * lives on the page and the crawler sees three distinct URLs. The active tab is
 * highlighted from the current pathname.
 */
const TABS: {href: string; key: string}[] = [
  {href: '/calendar/earnings', key: 'cal.earnings'},
  {href: '/calendar/macro', key: 'cal.macro'},
  {href: '/calendar/ipo', key: 'cal.ipo'},
];

export function CalendarTabs() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const pathname = usePathname();
  return (
    <nav className="mx-auto mb-5 flex max-w-3xl items-center gap-1.5">
      <CalendarClock
        size={18}
        className={cx('mr-1 shrink-0', dark ? 'text-teal-300' : 'text-teal-600')}
        aria-hidden
      />
      {TABS.map(tab => {
        const active = pathname === tab.href;
        return (
          <Link
            key={tab.href}
            href={tab.href}
            aria-current={active ? 'page' : undefined}
            className={cx(
              'rounded-full border px-3.5 py-1.5 text-[13px] font-semibold transition',
              active
                ? dark
                  ? 'border-teal-400/40 bg-teal-500/15 text-teal-200'
                  : 'border-teal-200 bg-teal-50 text-teal-700'
                : cx(t.border, t.sub, t.ghost),
            )}
          >
            {tr(tab.key)}
          </Link>
        );
      })}
    </nav>
  );
}
