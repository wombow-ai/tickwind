'use client';

import {X} from 'lucide-react';
import Link from 'next/link';
import type {Quote, Security} from '@/lib/api';
import {useDark} from '@/lib/theme';
import {cx, marketCurrency, tok} from '@/lib/ui';
import {
  ChangeLine,
  MarketBadge,
  PriceTag,
  SessionBadge,
} from '@/components/ui/atoms';

/**
 * A watchlist/board tile: ticker, name, live price and session. Links to the
 * stock's detail page. When `onRemove` is set, a hover affordance removes it.
 */
export function StockCard({
  security,
  quote,
  onRemove,
}: {
  security: Security;
  quote?: Quote;
  onRemove?: () => void;
}) {
  const dark = useDark();
  const t = tok(dark);
  const cur = marketCurrency(security.market);

  return (
    <div className="group relative">
      <Link
        href={`/stock/${encodeURIComponent(security.ticker)}`}
        className={cx(
          'block overflow-hidden rounded-2xl border p-4',
          t.card,
          t.border,
          t.cardI,
        )}
      >
        <div className="mb-1 flex items-center gap-2">
          <span className={cx('text-[15px] font-bold tracking-tight', t.text)}>
            {security.ticker}
          </span>
          <MarketBadge mkt={security.market} />
        </div>
        <p className={cx('mb-3 truncate text-[12px]', t.sub)}>{security.name}</p>

        <div className="flex items-end justify-between gap-2">
          {quote ? (
            <div>
              <PriceTag value={quote.price} cur={cur} size="md" />
              {quote.prev_close ? (
                <div className="mt-0.5">
                  <ChangeLine
                    chg={quote.price - quote.prev_close}
                    pct={
                      ((quote.price - quote.prev_close) / quote.prev_close) * 100
                    }
                    cur={cur}
                  />
                </div>
              ) : null}
            </div>
          ) : (
            <span className={cx('text-2xl font-semibold tabular-nums', t.faint)}>
              {cur}—
            </span>
          )}
        </div>

        <div className="mt-3">
          {quote ? (
            <SessionBadge session={quote.session} sm />
          ) : (
            <span className={cx('text-[11px]', t.faint)}>Waiting for price…</span>
          )}
        </div>
      </Link>

      {onRemove && (
        <button
          onClick={onRemove}
          className={cx(
            'absolute right-2.5 top-2.5 z-10 inline-flex h-7 w-7 items-center justify-center rounded-full opacity-0 transition hover:text-rose-500 group-hover:opacity-100',
            t.surf2,
            t.sub,
          )}
          aria-label={`Remove ${security.ticker}`}
        >
          <X size={14} />
        </button>
      )}
    </div>
  );
}
