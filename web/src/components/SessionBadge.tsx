/** A small colored badge denoting the trading session of a quote. */

import type {Session} from '@/lib/api';

interface SessionBadgeProps {
  session: Session;
}

/** Human-facing label and Tailwind style per known session. */
const SESSION_META: Record<string, {label: string; style: string}> = {
  pre: {
    label: 'Pre',
    style: 'bg-amber-500/15 text-amber-300 ring-amber-500/30',
  },
  regular: {
    label: 'Regular',
    style: 'bg-emerald-500/15 text-emerald-300 ring-emerald-500/30',
  },
  post: {
    label: 'Post',
    style: 'bg-orange-500/15 text-orange-300 ring-orange-500/30',
  },
  overnight: {
    label: 'Overnight',
    style: 'bg-indigo-500/15 text-indigo-300 ring-indigo-500/30',
  },
  closed: {
    label: 'Closed',
    style: 'bg-zinc-500/15 text-zinc-400 ring-zinc-500/30',
  },
};

const FALLBACK_STYLE = 'bg-zinc-500/15 text-zinc-300 ring-zinc-500/30';

/** Renders the session (e.g. `regular`) as a rounded, color-coded pill. */
export function SessionBadge({session}: SessionBadgeProps) {
  const meta = SESSION_META[session.toLowerCase()];
  const label = meta?.label ?? session;
  const style = meta?.style ?? FALLBACK_STYLE;
  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium uppercase tracking-wide ring-1 ring-inset ${style}`}
    >
      {label}
    </span>
  );
}
