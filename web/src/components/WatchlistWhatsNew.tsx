'use client';

import {Sparkles, X} from 'lucide-react';
import {useEffect, useState} from 'react';
import {type NewsItem} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, timeAgo, tok} from '@/lib/ui';

const WATERMARK_KEY = 'tw-last-visit';

/**
 * "Since your last visit" banner for the watchlist: counts the watchlist headlines newer than the
 * stored last-visit watermark and nudges the returning user toward what changed, then bumps the
 * watermark to now so the next visit diffs from this one. A self-serviceable retention hook.
 *
 * SSR-safe: the watermark is read POST-mount (localStorage is client-only), so the initial render
 * matches the server's "no banner" and there is no hydration mismatch. Per-device (localStorage),
 * dismissible. `ready` gates it to a fully-loaded, signed-in watchlist (not a focused/anon view).
 */
export function WatchlistWhatsNew({news, ready}: {news: NewsItem[]; ready: boolean}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [watermark, setWatermark] = useState<number | null>(null);
  const [dismissed, setDismissed] = useState(false);
  const [bumped, setBumped] = useState(false);

  // Read the PRIOR watermark once, post-mount (client-only → SSR render stays null).
  useEffect(() => {
    const raw = localStorage.getItem(WATERMARK_KEY);
    const n = raw ? parseInt(raw, 10) : NaN;
    setWatermark(Number.isFinite(n) ? n : null);
  }, []);

  // Once the feed has loaded, bump the watermark to now (next visit diffs from here). The
  // `watermark` STATE stays at the prior value, so this session's count is unaffected.
  useEffect(() => {
    if (ready && !bumped) {
      localStorage.setItem(WATERMARK_KEY, String(Date.now()));
      setBumped(true);
    }
  }, [ready, bumped]);

  if (!ready || dismissed || watermark == null) return null;
  const count = news.filter(n => {
    const ts = Date.parse(n.published);
    return Number.isFinite(ts) && ts > watermark;
  }).length;
  if (count === 0) return null;

  return (
    <div
      className={cx(
        'mb-4 flex items-center gap-2 rounded-xl border px-3.5 py-2.5 text-[13px]',
        dark ? 'border-amber-400/30 bg-amber-500/10' : 'border-amber-200 bg-amber-50',
      )}
    >
      <Sparkles size={15} className={cx('shrink-0', dark ? 'text-amber-300' : 'text-amber-500')} aria-hidden />
      <span className={t.text}>
        {tr('whatsnew.text')
          .replace('{n}', String(count))
          .replace('{ago}', timeAgo(new Date(watermark).toISOString()))}
      </span>
      <button
        type="button"
        onClick={() => setDismissed(true)}
        aria-label={tr('whatsnew.dismiss')}
        className={cx('ml-auto shrink-0 rounded p-0.5', t.ghost)}
      >
        <X size={14} className={t.sub} />
      </button>
    </div>
  );
}
