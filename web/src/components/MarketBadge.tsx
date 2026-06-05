/** A small colored badge denoting an instrument's listing market. */

interface MarketBadgeProps {
  market: string;
}

/** Tailwind classes per known market; unknown markets get a neutral style. */
const MARKET_STYLES: Record<string, string> = {
  US: 'bg-sky-500/15 text-sky-300 ring-sky-500/30',
  HK: 'bg-rose-500/15 text-rose-300 ring-rose-500/30',
  KR: 'bg-violet-500/15 text-violet-300 ring-violet-500/30',
};

const NEUTRAL_STYLE = 'bg-zinc-500/15 text-zinc-300 ring-zinc-500/30';

/** Renders the market code (e.g. `US`) as a rounded, color-coded pill. */
export function MarketBadge({market}: MarketBadgeProps) {
  const code = market.toUpperCase();
  const style = MARKET_STYLES[code] ?? NEUTRAL_STYLE;
  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium uppercase tracking-wide ring-1 ring-inset ${style}`}
    >
      {code}
    </span>
  );
}
