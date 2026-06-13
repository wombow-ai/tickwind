'use client';

import {LayoutGrid} from 'lucide-react';
import type {Indicator, IndicatorFacets} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';
import {IndicatorLibrary} from '@/components/IndicatorLibrary';

/**
 * Client shell for the indicator-library page: the localized header (needs the
 * client-side language hook) plus the {@link IndicatorLibrary} browser. The page
 * fetches the catalog server-side and passes it in, so the content is in the SSR
 * HTML and filtering is instant on the client.
 */
export function IndicatorLibraryShell({
  indicators,
  facets,
  total,
}: {
  indicators: Indicator[];
  facets: IndicatorFacets;
  total: number;
}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();

  return (
    <div className="w-full">
      <header className="mb-5">
        <h1 className={cx('flex items-center gap-2 text-[22px] font-bold tracking-tight', t.text)}>
          <LayoutGrid size={20} className={dark ? 'text-teal-300' : 'text-teal-600'} />
          {tr('ind.title')}
        </h1>
        <p className={cx('mt-1 max-w-2xl text-[13.5px]', t.sub)}>
          {tr('ind.subtitle').replace('{n}', String(total || 282))}
        </p>
      </header>
      <IndicatorLibrary initial={indicators} facets={facets} total={total} />
    </div>
  );
}
