/**
 * Aurora design tokens and small presentation helpers.
 *
 * The product uses an explicit light/dark token map (`tok(dark)`) threaded from
 * the theme rather than Tailwind `dark:` variants, which keeps the two palettes
 * legible side by side and matches the source design system exactly.
 */

/** Joins truthy class fragments into a single className string. */
export function cx(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(' ');
}

/** Semantic class tokens for a given theme. */
export interface Tokens {
  text: string;
  sub: string;
  faint: string;
  card: string;
  border: string;
  hair: string;
  chip: string;
  chipText: string;
  input: string;
  ghost: string;
  surf2: string;
  accentText: string;
  /** Soft elevation utility (defined in globals.css). */
  soft: string;
  /** Interactive card elevation utility (hover lift). */
  cardI: string;
  /** Skeleton shimmer utility. */
  skel: string;
}

/** Returns the semantic token set for the active theme. */
export function tok(dark: boolean): Tokens {
  return dark
    ? {
        text: 'text-slate-100',
        sub: 'text-slate-400',
        faint: 'text-slate-500',
        card: 'bg-slate-900',
        border: 'border-slate-800',
        hair: 'border-slate-800',
        chip: 'bg-slate-800',
        chipText: 'text-slate-300',
        input: 'bg-slate-900 border-slate-700 text-slate-100 placeholder-slate-500',
        ghost: 'text-slate-300 hover:bg-slate-800',
        surf2: 'bg-slate-800/50',
        accentText: 'text-teal-300',
        soft: 'tw-soft-d',
        cardI: 'tw-card-d',
        skel: 'tw-skel-d',
      }
    : {
        text: 'text-slate-900',
        sub: 'text-slate-500',
        faint: 'text-slate-400',
        card: 'bg-white',
        border: 'border-slate-200',
        hair: 'border-slate-200',
        chip: 'bg-slate-100',
        chipText: 'text-slate-600',
        input: 'bg-white border-slate-200 text-slate-900 placeholder-slate-400',
        ghost: 'text-slate-600 hover:bg-slate-100',
        surf2: 'bg-slate-50',
        accentText: 'text-teal-700',
        soft: 'tw-soft-l',
        cardI: 'tw-card-l',
        skel: 'tw-skel-l',
      };
}

/** Primary (teal) button classes for the active theme. */
export function btnPrimary(dark: boolean): string {
  return dark
    ? 'bg-teal-500 hover:bg-teal-400 text-slate-950'
    : 'bg-teal-600 hover:bg-teal-700 text-white';
}

/** Per-session color metadata, keyed by the API's {@link Session} values. */
interface SessionStyle {
  label: string;
  /** Light-theme colors. */
  L: {bg: string; fg: string; dot: string};
  /** Dark-theme colors. */
  D: {bg: string; fg: string; dot: string};
  /** Whether to pulse the indicator dot (regular trading only). */
  live?: boolean;
}

/**
 * Trading-session badge palette. Keys match the backend `Quote.session`
 * values: `pre`, `regular`, `post`, `overnight`, `closed`.
 */
export const SESSIONS: Record<string, SessionStyle> = {
  pre: {
    label: 'Pre-market',
    L: {bg: '#FEF3C7', fg: '#B45309', dot: '#F59E0B'},
    D: {bg: 'rgba(245,158,11,.16)', fg: '#FCD34D', dot: '#F59E0B'},
  },
  regular: {
    label: 'Regular',
    L: {bg: '#D1FAE5', fg: '#047857', dot: '#10B981'},
    D: {bg: 'rgba(16,185,129,.16)', fg: '#6EE7B7', dot: '#10B981'},
    live: true,
  },
  post: {
    label: 'After-hours',
    L: {bg: '#EDE9FE', fg: '#6D28D9', dot: '#8B5CF6'},
    D: {bg: 'rgba(139,92,246,.18)', fg: '#C4B5FD', dot: '#8B5CF6'},
  },
  overnight: {
    label: 'Overnight',
    L: {bg: '#DBEAFE', fg: '#1D4ED8', dot: '#3B82F6'},
    D: {bg: 'rgba(59,130,246,.18)', fg: '#93C5FD', dot: '#3B82F6'},
  },
  closed: {
    label: 'Closed',
    L: {bg: '#F1F5F9', fg: '#475569', dot: '#94A3B8'},
    D: {bg: 'rgba(148,163,184,.16)', fg: '#CBD5E1', dot: '#94A3B8'},
  },
};

/** Returns the session style for a value, falling back to `closed`. */
export function sessionStyle(session: string): SessionStyle {
  return SESSIONS[session] ?? SESSIONS.closed;
}

/** Currency symbol for a listing market (`US`, `HK`, `KR`). */
export function marketCurrency(market: string): string {
  switch (market.toUpperCase()) {
    case 'HK':
      return 'HK$';
    case 'KR':
      return '₩';
    case 'TW':
      return 'NT$';
    default:
      return '$';
  }
}

/** Formats a price with its currency symbol (KRW has no decimals). */
export function fmtPrice(cur: string, v: number): string {
  if (cur === '₩') {
    return '₩' + Math.round(v).toLocaleString('en-US');
  }
  return (
    cur +
    v.toLocaleString('en-US', {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    })
  );
}

/** Formats an absolute delta magnitude (no sign, no currency symbol). */
export function fmtDelta(cur: string, v: number): string {
  const a = Math.abs(v);
  return cur === '₩'
    ? Math.round(a).toLocaleString('en-US')
    : a.toLocaleString('en-US', {
        minimumFractionDigits: 2,
        maximumFractionDigits: 2,
      });
}

/** Compact USD for large figures: `$4.51T` / `$416.2B` / `$1.2M` / `-$3.85B`. */
export function fmtCompactUSD(v: number): string {
  const a = Math.abs(v);
  const sign = v < 0 ? '-' : '';
  if (a >= 1e12) return `${sign}$${(a / 1e12).toFixed(2)}T`;
  if (a >= 1e9) return `${sign}$${(a / 1e9).toFixed(2)}B`;
  if (a >= 1e6) return `${sign}$${(a / 1e6).toFixed(2)}M`;
  if (a >= 1e3) return `${sign}$${(a / 1e3).toFixed(1)}K`;
  return `${sign}$${a.toFixed(0)}`;
}

/** Compact relative time (e.g. `18m`, `3h`, `2d`) from an RFC 3339 timestamp. */
export function timeAgo(iso: string): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) {
    return '';
  }
  const secs = Math.max(0, (Date.now() - then) / 1000);
  if (secs < 60) return 'now';
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `${mins}m`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h`;
  const days = Math.floor(hrs / 24);
  if (days < 30) return `${days}d`;
  const months = Math.floor(days / 30);
  if (months < 12) return `${months}mo`;
  return `${Math.floor(months / 12)}y`;
}
