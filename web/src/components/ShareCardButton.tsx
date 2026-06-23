'use client';

import {ImageDown, Share2} from 'lucide-react';
import type {OgParams} from '@/lib/og';
import {ogImage} from '@/lib/og';
import {useToast} from '@/components/ui/Toast';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx} from '@/lib/ui';

/**
 * Tickwind's propagation organ — two complementary share actions:
 *
 *  - **Share** (link): the native share sheet on mobile (WeChat / 小红书 / etc.), else copy
 *    the page URL to the clipboard with a toast. Every shared link is a backlink + a new
 *    visitor — the cheapest compounding growth loop for a young domain.
 *  - **Save image**: opens the dynamic branded card (a 1200×630 PNG from `/api/og/[kind]`)
 *    to save and paste into image-first apps (小红书 / 微信群), where links don't spread.
 *
 * `card` is the same param object `ogImage` takes; the disclaimer/source rides on its subtitle.
 */
export function ShareCardButton({card, label}: {card: OgParams; label?: string}) {
  const tr = useT();
  const {lang} = useLang();
  const dark = useDark();
  const {toast} = useToast();

  const pill = cx(
    'inline-flex items-center gap-1.5 whitespace-nowrap rounded-full border px-3 py-1.5 text-[12.5px] font-semibold transition',
    dark
      ? 'border-teal-500/40 text-teal-300 hover:border-teal-400 hover:bg-teal-500/10'
      : 'border-teal-600/30 text-teal-700 hover:border-teal-600 hover:bg-teal-50',
  );

  async function shareLink() {
    if (typeof window === 'undefined') return;
    const url = window.location.href;
    const title = card.title || document.title || 'Tickwind';
    // Mobile / supporting browsers → the native share sheet. A user-cancel (AbortError) or
    // any failure is a no-op (we must NOT silently copy after the user cancelled).
    if (navigator.share) {
      navigator.share({title, url}).catch(() => {});
      return;
    }
    // Desktop fallback → copy the link + confirm.
    try {
      await navigator.clipboard.writeText(url);
      toast(tr('share.copied'));
    } catch {
      // Clipboard unavailable (insecure context / denied) → nothing to do.
    }
  }

  return (
    <span className="inline-flex items-center gap-2">
      <button
        type="button"
        onClick={shareLink}
        aria-label={tr('share.linkAria')}
        title={tr('share.linkAria')}
        className={pill}
      >
        <Share2 size={15} />
        {tr('share.link')}
      </button>
      <button
        type="button"
        onClick={() =>
          window.open(ogImage({...card, lang: card.lang ?? lang}), '_blank', 'noopener,noreferrer')
        }
        aria-label={tr('share.aria')}
        title={tr('share.aria')}
        className={pill}
      >
        <ImageDown size={15} />
        {label ?? tr('share.save')}
      </button>
    </span>
  );
}
