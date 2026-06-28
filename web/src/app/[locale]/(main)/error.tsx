'use client';

import {CloudOff, Home, RefreshCw} from 'lucide-react';
import Link from 'next/link';
import {useEffect} from 'react';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

/**
 * Route-level error boundary for the whole main app. Rendered in place of the
 * page content when any Client Component below the `(main)` layout throws — the
 * TopNav + Footer chrome (owned by the parent layout) stay put, so the user
 * keeps navigation instead of a white screen. `reset()` re-renders the segment,
 * matching the existing retry ethos. Lives inside the locale providers (theme /
 * i18n), so the hooks below resolve.
 */
export default function MainError({
  error,
  reset,
}: {
  error: Error & {digest?: string};
  reset: () => void;
}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();

  useEffect(() => {
    if (process.env.NODE_ENV !== 'production') {
      // eslint-disable-next-line no-console
      console.error('[route error]', error);
    }
  }, [error]);

  return (
    <div className="flex min-h-[55vh] flex-col items-center justify-center px-6 py-16 text-center">
      <div
        className="mb-4 flex items-center justify-center rounded-2xl"
        style={{
          width: 64,
          height: 64,
          background: dark ? 'rgba(244,63,94,.12)' : '#FFF1F2',
        }}
      >
        <CloudOff className={dark ? 'text-rose-400' : 'text-rose-500'} size={26} />
      </div>
      <p className={cx('text-[15px] font-semibold', t.text)}>{tr('states.errorTitle')}</p>
      <p className={cx('mt-1 max-w-[320px] text-[13px]', t.sub)}>{tr('boundary.routeSub')}</p>
      <div className="mt-5 flex items-center gap-2">
        <button
          onClick={() => reset()}
          className={cx(
            'inline-flex items-center gap-1.5 rounded-full border px-4 py-2 text-[13px] font-medium',
            t.border,
            t.ghost,
          )}
        >
          <RefreshCw size={14} /> {tr('states.retry')}
        </button>
        <Link
          href="/"
          className={cx(
            'inline-flex items-center gap-1.5 rounded-full border px-4 py-2 text-[13px] font-medium',
            t.border,
            t.ghost,
          )}
        >
          <Home size={14} /> {tr('boundary.home')}
        </Link>
      </div>
    </div>
  );
}
