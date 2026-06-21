import type {CSSProperties} from 'react';

/**
 * BrandLoader — the Tickwind "streams" loading mark (owner-designed): three wind streams
 * flowing toward a live data point with a sonar ping. All animation + theming lives in CSS
 * (`.tw-load` in globals.css), so this is a PURE component with no hooks or `'use client'`
 * — it renders as a server component (zero client JS) and works in route `loading.tsx`
 * fallbacks as well as inside client trees.
 *
 * - Ink strokes follow `currentColor` (navy on light, near-white on dark, via `html.dark`);
 *   pass `color` to pin it (e.g. a fixed-surface overlay).
 * - The accent (the green stream + ping) is the `--tw-load-accent` CSS var; pass `accent`
 *   to retheme it — e.g. the gold chat hub passes its `var(--accent)`.
 */
export function BrandLoader({
  size = 48,
  accent,
  color,
  className,
  label = 'Loading',
}: {
  size?: number;
  accent?: string;
  color?: string;
  className?: string;
  label?: string;
}) {
  const style: CSSProperties = {width: size, height: size};
  if (color) style.color = color;
  // Custom properties aren't in the CSSProperties type; assign through a record cast.
  if (accent) (style as Record<string, string | number>)['--tw-load-accent'] = accent;
  return (
    <svg
      className={className ? `tw-load ${className}` : 'tw-load'}
      style={style}
      viewBox="0 0 100 100"
      fill="none"
      role="img"
      aria-label={label}
    >
      {/* faint always-on tracks */}
      <path className="trk ink" d="M18 67 C42 67 54 60 67 46" />
      <path className="trk ink" d="M18 76 C38 76 48 71 58 60" />
      <path className="trk acc" d="M18 58 C44 58 57 50 76 32" />
      {/* flowing gusts */}
      <path className="flow ink d1" pathLength={100} d="M18 67 C42 67 54 60 67 46" />
      <path className="flow ink d2" pathLength={100} d="M18 76 C38 76 48 71 58 60" />
      <path className="flow acc" pathLength={100} d="M18 58 C44 58 57 50 76 32" />
      {/* live data point + sonar ping */}
      <circle className="ping" cx={76} cy={32} r={5} />
      <circle className="ping p2" cx={76} cy={32} r={5} />
      <circle className="dot" cx={76} cy={32} r={4.4} />
    </svg>
  );
}
