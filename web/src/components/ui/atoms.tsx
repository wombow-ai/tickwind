'use client';

import {useEffect, useId, useRef, useState} from 'react';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, fmtDelta, fmtPrice, SESSIONS, sessionStyle, tok} from '@/lib/ui';

/**
 * The Tickwind "streams" mark — three rising streams (the wind) with the green leader ending
 * in a ringed price node (the tick). The two streams adapt to light/dark via currentColor
 * (deep navy on light, soft slate on dark); the green leader + node stay constant. Transparent
 * (no tile) so the SAME graphic reads on the main site and the warm chat surface.
 */
export function LogoMark({size = 30, accent = '#00C08B'}: {size?: number; accent?: string}) {
  const dark = useDark();
  return (
    <svg
      width={size}
      height={size}
      viewBox="12 13 76 76"
      fill="none"
      aria-hidden
      style={{color: dark ? '#E2E8F0' : '#0E1B2E'}}
    >
      <g stroke="currentColor" strokeWidth="6" strokeLinecap="round">
        <path d="M18 67 C42 67 54 60 67 46" />
        <path d="M18 76 C38 76 48 71 58 60" />
      </g>
      <path d="M18 58 C44 58 57 50 76 32" stroke={accent} strokeWidth="6" strokeLinecap="round" />
      <circle cx="76" cy="32" r="4.4" fill={accent} />
      <circle cx="76" cy="32" r="8.4" stroke={accent} strokeWidth="1.5" />
    </svg>
  );
}

/** The Tickwind wordmark + streams mark. `wordmarkClassName` lets a caller hide
 *  the wordmark responsively (e.g. glyph-only on a narrow nav) without affecting
 *  other Logo usages like the Footer. */
export function Logo({size = 30, wordmarkClassName}: {size?: number; wordmarkClassName?: string}) {
  const dark = useDark();
  return (
    <div className="flex select-none items-center gap-2">
      <LogoMark size={size} />
      <span
        className={cx(
          'text-[17px] font-semibold tracking-tight',
          dark ? 'text-slate-100' : 'text-slate-900',
          wordmarkClassName,
        )}
      >
        Tick<span className={dark ? 'text-teal-300' : 'text-teal-600'}>wind</span>
      </span>
    </div>
  );
}

/** Signature trading-session badge (pre / regular / post / overnight / closed). */
export function SessionBadge({session, sm}: {session: string; sm?: boolean}) {
  const dark = useDark();
  const tr = useT();
  const s = sessionStyle(session);
  const c = dark ? s.D : s.L;
  const key = SESSIONS[session] ? session : 'closed';
  return (
    <span
      className={cx(
        'inline-flex items-center gap-1.5 whitespace-nowrap rounded-full font-medium',
        sm ? 'px-2 py-0.5 text-[10.5px]' : 'px-2.5 py-1 text-[11px]',
      )}
      style={{background: c.bg, color: c.fg}}
    >
      <span
        className={cx('rounded-full', s.live && 'tw-livedot')}
        style={{width: 6, height: 6, background: c.dot}}
      />
      {tr(`session.${key}`)}
    </span>
  );
}

/** Listing-market tag (`US`, `HK`, `KR`). */
export function MarketBadge({mkt}: {mkt: string}) {
  const dark = useDark();
  const t = tok(dark);
  return (
    <span
      className={cx(
        'inline-flex items-center rounded-md px-1.5 py-0.5 text-[10px] font-semibold tracking-wider',
        t.chip,
        t.chipText,
      )}
    >
      {mkt}
    </span>
  );
}

/** A bordered pill, used for small status labels. */
export function Pill({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  const dark = useDark();
  const t = tok(dark);
  return (
    <span
      className={cx(
        'inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] font-medium',
        t.border,
        t.chipText,
        className,
      )}
    >
      {children}
    </span>
  );
}

/** A shimmering skeleton block. */
export function Skeleton({
  className,
  style,
}: {
  className?: string;
  style?: React.CSSProperties;
}) {
  const dark = useDark();
  return (
    <div className={cx(tok(dark).skel, 'rounded-md', className)} style={style} />
  );
}

type PriceSize = 'sm' | 'md' | 'lg';

/** A price that briefly flashes green/red when it ticks up/down. */
export function PriceTag({
  value,
  cur,
  size = 'md',
}: {
  value: number;
  cur: string;
  size?: PriceSize;
}) {
  const dark = useDark();
  const [flash, setFlash] = useState<'up' | 'down' | null>(null);
  const last = useRef(value);

  useEffect(() => {
    if (value > last.current) setFlash('up');
    else if (value < last.current) setFlash('down');
    last.current = value;
    const t = setTimeout(() => setFlash(null), 850);
    return () => clearTimeout(t);
  }, [value]);

  const sz =
    size === 'lg'
      ? 'text-4xl sm:text-5xl'
      : size === 'md'
        ? 'text-2xl'
        : 'text-lg';
  return (
    <span
      className={cx(
        '-mx-1 inline-block rounded-lg px-1 font-semibold tracking-tight tabular-nums',
        sz,
        dark ? 'text-slate-50' : 'text-slate-900',
        flash === 'up' && 'tw-flash-up',
        flash === 'down' && 'tw-flash-down',
      )}
    >
      {fmtPrice(cur, value)}
    </span>
  );
}

/** A signed change + percentage, colored green/red by direction. */
export function ChangeLine({
  chg,
  pct,
  cur,
  size = 'sm',
}: {
  chg: number;
  pct: number;
  cur: string;
  size?: 'sm' | 'md' | 'lg';
}) {
  const dark = useDark();
  const up = chg >= 0;
  const col = up
    ? dark
      ? 'text-emerald-400'
      : 'text-emerald-600'
    : dark
      ? 'text-rose-400'
      : 'text-rose-500';
  const sz =
    size === 'lg' ? 'text-base' : size === 'md' ? 'text-sm' : 'text-[12.5px]';
  return (
    <span
      className={cx(
        'inline-flex items-center gap-1 font-medium tabular-nums',
        col,
        sz,
      )}
    >
      <span style={{fontSize: size === 'lg' ? 12 : 10}}>{up ? '▲' : '▼'}</span>
      {fmtDelta(cur, chg)}
      <span className="opacity-70">
        ({up ? '+' : '−'}
        {Math.abs(pct).toFixed(2)}%)
      </span>
    </span>
  );
}

/**
 * A smoothed area sparkline over a series of values. Render only with real
 * price history (e.g. intraday bars); it intentionally takes data rather than
 * synthesizing it.
 */
export function Sparkline({
  values,
  up,
  w = 150,
  h = 40,
}: {
  values: readonly number[];
  up: boolean;
  w?: number;
  h?: number;
}) {
  const dark = useDark();
  const gid = useId();
  if (values.length < 2) {
    return <svg width={w} height={h} aria-hidden />;
  }
  const n = values.length;
  const mx = Math.max(...values);
  const mn = Math.min(...values);
  const span = mx - mn || 1;
  const step = w / (n - 1);
  const pts = values.map(
    (val, i) => [i * step, h - ((val - mn) / span) * (h - 8) - 4] as const,
  );
  let d = `M ${pts[0][0]} ${pts[0][1]}`;
  for (let i = 1; i < pts.length; i++) {
    const [x0, y0] = pts[i - 1];
    const [x1, y1] = pts[i];
    const xm = (x0 + x1) / 2;
    d += ` C ${xm} ${y0}, ${xm} ${y1}, ${x1} ${y1}`;
  }
  const col = up ? '#10B981' : '#F43F5E';
  return (
    <svg width={w} height={h} viewBox={`0 0 ${w} ${h}`} className="overflow-visible">
      <defs>
        <linearGradient id={gid} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor={col} stopOpacity={dark ? '.28' : '.18'} />
          <stop offset="1" stopColor={col} stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d={`${d} L ${w} ${h} L 0 ${h} Z`} fill={`url(#${gid})`} />
      <path d={d} fill="none" stroke={col} strokeWidth="2" strokeLinecap="round" />
    </svg>
  );
}
