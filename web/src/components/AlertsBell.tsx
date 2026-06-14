'use client';

import {Bell} from 'lucide-react';
import Link from '@/components/LocalLink';
import {usePathname} from 'next/navigation';
import {useEffect, useState} from 'react';
import {getAlerts} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useT} from '@/lib/i18n';
import {stripLocale} from '@/lib/locale';
import {cx, tok} from '@/lib/ui';

const POLL_MS = 60_000;

/**
 * Top-nav bell linking to /alerts, with a red badge counting triggered alerts —
 * so a fired alert is visible from anywhere, not just the stock it's on. Polls
 * GET /v1/alerts while signed in; renders nothing when logged out. Lightweight:
 * one request a minute, no websocket.
 */
export function AlertsBell({dark}: {dark: boolean}) {
  const {user, getToken} = useAuth();
  const t = tok(dark);
  const tr = useT();
  const pathname = stripLocale(usePathname()).rest;
  const [count, setCount] = useState(0);

  useEffect(() => {
    if (!user) {
      setCount(0);
      return;
    }
    const c = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const tick = () => {
      getToken().then(token =>
        getAlerts(token, c.signal).then(
          r => {
            setCount((r.alerts ?? []).filter(a => a.triggered_at).length);
            timer = setTimeout(tick, POLL_MS);
          },
          () => {
            timer = setTimeout(tick, POLL_MS);
          },
        ),
      );
    };
    tick();
    return () => {
      c.abort();
      if (timer !== undefined) clearTimeout(timer);
    };
    // pathname dep: refresh the badge right after navigating (e.g. away from /me).
  }, [user, getToken, pathname]);

  if (!user) return null;

  return (
    <Link
      href="/me?tab=alerts"
      aria-label={tr('nav.alerts')}
      className={cx(
        'relative inline-flex h-9 w-9 items-center justify-center rounded-full border',
        t.border,
        t.ghost,
        pathname === '/me' && t.accentText,
      )}
    >
      <Bell size={16} />
      {count > 0 && (
        <span
          className="absolute -right-0.5 -top-0.5 inline-flex min-w-[16px] items-center justify-center rounded-full px-1 text-[9.5px] font-bold leading-none text-white"
          style={{height: 16, background: '#f43f5e'}}
        >
          {count > 9 ? '9+' : count}
        </span>
      )}
    </Link>
  );
}
