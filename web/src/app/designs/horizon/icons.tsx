/**
 * Hairline stroke icons for the Horizon landing page.
 *
 * Inline SVGs (no icon dependency) drawn on a 24px grid with a 1.5px stroke to
 * match the page's thin, editorial lines. Each is decorative and hidden from
 * assistive tech; the adjacent heading carries the meaning.
 */

import type {SVGProps} from 'react';

/** Shared SVG attributes: 24px grid, rounded 1.5px strokes, no fill. */
function iconProps(extra?: SVGProps<SVGSVGElement>): SVGProps<SVGSVGElement> {
  return {
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke: 'currentColor',
    strokeWidth: 1.5,
    strokeLinecap: 'round',
    strokeLinejoin: 'round',
    'aria-hidden': true,
    ...extra,
  };
}

/** A candlestick-style price line — all-session live price. */
export function PulseIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...iconProps(props)}>
      <path d="M3 12h3l2.5-6 4 13 3-9 1.5 2H21" />
    </svg>
  );
}

/** Stacked sheets — filings and news in one feed. */
export function FeedIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...iconProps(props)}>
      <path d="M7 3.5h7.5L19 8v12.5H7z" />
      <path d="M14 3.5V8h5" />
      <path d="M4.5 7v13.5H16" />
      <path d="M9.5 12.5h6M9.5 16h4" />
    </svg>
  );
}

/** Linked nodes — social chatter and your saved links. */
export function ChatterIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...iconProps(props)}>
      <path d="M4 6.5A2.5 2.5 0 0 1 6.5 4h7A2.5 2.5 0 0 1 16 6.5v3A2.5 2.5 0 0 1 13.5 12H9l-3.5 3v-3H6.5A2.5 2.5 0 0 1 4 9.5z" />
      <path d="M19 9.5h-1v3.5a2.5 2.5 0 0 1-2.5 2.5H11l1.5 1.5h4l3 2.5v-2.5h0a2.5 2.5 0 0 0 0-5z" />
    </svg>
  );
}

/** A small upper-right arrow used on outbound links. */
export function ArrowUpRightIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...iconProps(props)}>
      <path d="M8 16 16 8M9 8h7v7" />
    </svg>
  );
}
