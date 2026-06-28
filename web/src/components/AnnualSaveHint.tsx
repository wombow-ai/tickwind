'use client';

import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

/**
 * A muted one-liner nudging annual billing, shown under Pro upgrade CTAs (/pro +
 * the paywall walls). The concrete numbers ($99/yr ≈ $57 less than 12×$12.99)
 * live in the `pro.saveHint` dict entry — keep it in sync with the real prices on
 * the /pro page if pricing ever changes (display copy only; never the source of
 * truth for a charge).
 */
export function AnnualSaveHint({className}: {className?: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  return <p className={cx('text-[11px] leading-snug', t.faint, className)}>{tr('pro.saveHint')}</p>;
}

export default AnnualSaveHint;
