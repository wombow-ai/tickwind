'use client';

import {CloudOff, Inbox, RefreshCw} from 'lucide-react';
import type {LucideIcon} from 'lucide-react';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';
import {Skeleton} from './atoms';

/** A timeline node dot + connector, shared by the feed list and skeleton. */
function TimelineDot({color}: {color: string}) {
  const dark = useDark();
  return (
    <div className="relative flex flex-col items-center" style={{width: 22}}>
      <span
        className="rounded-full"
        style={{
          width: 9,
          height: 9,
          background: color,
          boxShadow: `0 0 0 4px ${dark ? 'rgba(20,184,166,.10)' : 'rgba(13,148,136,.10)'}`,
        }}
      />
      <span className={cx('mt-1 w-px flex-1', dark ? 'bg-slate-800' : 'bg-slate-200')} />
    </div>
  );
}

/** Three shimmering placeholder rows while a feed loads. */
export function FeedSkeleton() {
  const dark = useDark();
  const t = tok(dark);
  return (
    <div>
      {[0, 1, 2].map(i => (
        <div key={i} className="flex gap-3 pb-3">
          <TimelineDot color={dark ? '#334155' : '#CBD5E1'} />
          <div className={cx('flex-1 rounded-2xl border p-3.5', t.card, t.border)}>
            <Skeleton className="mb-3 h-3 w-24" />
            <Skeleton className="mb-2 h-3.5 w-full" />
            <Skeleton className="h-3.5 w-2/3" />
          </div>
        </div>
      ))}
    </div>
  );
}

/** A calm empty state with a small breeze flourish. */
export function EmptyState({
  label,
  sub,
  icon: Icon = Inbox,
}: {
  label: string;
  sub: string;
  icon?: LucideIcon;
}) {
  const dark = useDark();
  const t = tok(dark);
  return (
    <div className="flex flex-col items-center px-6 py-12 text-center">
      <div className="relative mb-4">
        <div
          className="flex items-center justify-center rounded-2xl"
          style={{
            width: 64,
            height: 64,
            background: dark ? 'rgba(20,184,166,.10)' : 'rgba(13,148,136,.08)',
          }}
        >
          <Icon className={dark ? 'text-teal-300' : 'text-teal-600'} size={26} />
        </div>
        <svg
          width="120"
          height="30"
          viewBox="0 0 120 30"
          className="absolute -right-12 top-3 opacity-60"
          aria-hidden
        >
          <path
            d="M0 18 q30 -14 60 -2"
            fill="none"
            stroke={dark ? '#1f3b3a' : '#cdeae7'}
            strokeWidth="2"
            strokeLinecap="round"
            className="tw-breeze"
          />
          <path
            d="M6 24 q26 -8 50 0"
            fill="none"
            stroke={dark ? '#1a2e3a' : '#d6eef6'}
            strokeWidth="2"
            strokeLinecap="round"
            className="tw-breeze"
            style={{animationDelay: '.6s'}}
          />
        </svg>
      </div>
      <p className={cx('text-[14px] font-semibold', t.text)}>{label}</p>
      <p className={cx('mt-1 max-w-[260px] text-[12.5px]', t.sub)}>{sub}</p>
    </div>
  );
}

/** An error state with a retry affordance. */
export function ErrorState({onRetry}: {onRetry: () => void}) {
  const dark = useDark();
  const t = tok(dark);
  return (
    <div className="flex flex-col items-center px-6 py-12 text-center">
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
      <p className={cx('text-[14px] font-semibold', t.text)}>
        The wind dropped for a moment
      </p>
      <p className={cx('mt-1 max-w-[260px] text-[12.5px]', t.sub)}>
        We couldn&apos;t load this feed. Check your connection and try again.
      </p>
      <button
        onClick={onRetry}
        className={cx(
          'mt-4 inline-flex items-center gap-1.5 rounded-full border px-3.5 py-1.5 text-[12.5px] font-medium',
          t.border,
          t.ghost,
        )}
      >
        <RefreshCw size={13} /> Retry
      </button>
    </div>
  );
}
