'use client';

import {ImageDown} from 'lucide-react';
import type {OgParams} from '@/lib/og';
import {ogImage} from '@/lib/og';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx} from '@/lib/ui';

/**
 * "Save image" share button — Tickwind's propagation organ. Opens the dynamic
 * branded card (a 1200×630 PNG rendered by `/api/og/[kind]`) in a new tab so the
 * user can save it (desktop right-click → Save, mobile long-press → Save) and
 * drop it into 小红书 / 微信群. This matches the actual sharing flow on those
 * apps, which spread images rather than links.
 *
 * `card` is the same param object `ogImage` takes; pass page-specific data
 * (eyebrow/title/subtitle/stat/tone). The disclaimer/source lives on the card's
 * own subtitle, so callers must encode it there.
 */
export function ShareCardButton({card, label}: {card: OgParams; label?: string}) {
  const tr = useT();
  const dark = useDark();

  return (
    <button
      type="button"
      onClick={() => window.open(ogImage(card), '_blank', 'noopener,noreferrer')}
      aria-label={tr('share.aria')}
      title={tr('share.aria')}
      className={cx(
        'inline-flex items-center gap-1.5 whitespace-nowrap rounded-full border px-3 py-1.5 text-[12.5px] font-semibold transition',
        dark
          ? 'border-teal-500/40 text-teal-300 hover:border-teal-400 hover:bg-teal-500/10'
          : 'border-teal-600/30 text-teal-700 hover:border-teal-600 hover:bg-teal-50',
      )}
    >
      <ImageDown size={15} />
      {label ?? tr('share.save')}
    </button>
  );
}
