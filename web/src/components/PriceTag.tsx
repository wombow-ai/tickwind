/** Displays a live price with its trading-session badge. */

import type {Quote} from '@/lib/api';
import {formatPrice} from '@/lib/format';
import {SessionBadge} from '@/components/SessionBadge';

interface PriceTagProps {
  /** Latest quote, or `undefined` when none is available yet. */
  quote?: Quote;
  /**
   * Visual scale. `sm` suits dense contexts like watchlist cards; `lg` is for
   * the stock detail header. Defaults to `sm`.
   */
  size?: 'sm' | 'lg';
}

/** Em dash shown when no price is available. */
const PLACEHOLDER = '—';

const PRICE_SIZE: Record<'sm' | 'lg', string> = {
  sm: 'text-lg',
  lg: 'text-3xl sm:text-4xl',
};

/**
 * Renders the price prominently with a session badge beside it. With no quote,
 * it shows a muted em dash and omits the badge, so the layout stays stable
 * before the first price arrives.
 */
export function PriceTag({quote, size = 'sm'}: PriceTagProps) {
  if (!quote) {
    return (
      <span
        className={`font-mono font-semibold tabular-nums text-zinc-600 ${PRICE_SIZE[size]}`}
      >
        {PLACEHOLDER}
      </span>
    );
  }

  return (
    <span className="inline-flex items-baseline gap-2">
      <span
        className={`font-mono font-semibold tabular-nums text-zinc-50 ${PRICE_SIZE[size]}`}
      >
        {formatPrice(quote.price)}
      </span>
      <SessionBadge session={quote.session} />
    </span>
  );
}
