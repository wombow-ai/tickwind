/**
 * Light-theme session badge for the Horizon landing page.
 *
 * A standalone, hairline-bordered pill that names the trading session a quote
 * was observed in. It is intentionally separate from the app's dark-theme
 * `SessionBadge` so this marketing surface can own its calm, monochrome palette.
 */

import type {Session} from '@/lib/api';

interface SessionBadgeProps {
  session: Session;
}

/** Human-facing label and Tailwind classes per known session. */
const SESSION_META: Record<string, {label: string; dot: string; text: string}> =
  {
    pre: {label: 'Pre-market', dot: 'bg-amber-500', text: 'text-amber-700'},
    regular: {label: 'Regular', dot: 'bg-emerald-500', text: 'text-emerald-700'},
    post: {label: 'After-hours', dot: 'bg-orange-500', text: 'text-orange-700'},
    overnight: {label: 'Overnight', dot: 'bg-indigo-500', text: 'text-indigo-700'},
    closed: {label: 'Closed', dot: 'bg-zinc-400', text: 'text-zinc-500'},
  };

const FALLBACK = {label: 'Live', dot: 'bg-zinc-400', text: 'text-zinc-600'};

/**
 * Renders a session (e.g. `overnight`) as a calm pill: a small colored status
 * dot plus a label, inside a hairline border. Unknown sessions fall back to a
 * neutral "Live" treatment.
 */
export function SessionBadge({session}: SessionBadgeProps) {
  const meta = SESSION_META[session.toLowerCase()] ?? FALLBACK;
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full border border-zinc-200 bg-white px-2.5 py-1 text-xs font-medium">
      <span
        aria-hidden
        className={`h-1.5 w-1.5 rounded-full ${meta.dot}`}
      />
      <span className={meta.text}>{meta.label}</span>
    </span>
  );
}
