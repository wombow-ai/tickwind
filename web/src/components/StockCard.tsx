'use client';

import {X} from 'lucide-react';
import Link from '@/components/LocalLink';
import type {Quote, Security} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, marketCurrency, tok} from '@/lib/ui';
import {
  ChangeLine,
  MarketBadge,
  PriceTag,
  SessionBadge,
  Sparkline,
} from '@/components/ui/atoms';

/**
 * A watchlist/board tile: ticker, name, live price and session. Links to the
 * stock's detail page. When `onRemove` is set, a hover affordance removes it.
 */
export function StockCard({
  security,
  quote,
  closes,
  onRemove,
}: {
  security: Security;
  quote?: Quote;
  /** Recent daily closes for a trend sparkline; omitted/empty hides it. */
  closes?: number[];
  onRemove?: () => void;
}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const cur = marketCurrency(security.market);

  // Regular vs extended-hours split (mirrors the detail header): the main line
  // is the regular-session price + its day-change vs the prior regular close; a
  // small line carries the pre/post delta. In extended hours quote.prev_close
  // can be anchored to regClose (thin-name overlay guard), so the day-change
  // measures against the daily bars' prior close instead — never a phantom zero.
  const regClose =
    quote?.regular_close && quote.regular_close > 0 ? quote.regular_close : quote?.price ?? 0;
  const regularPrice = quote?.session === 'regular' ? quote.price : regClose;
  const isExt =
    !!quote &&
    (quote.session === 'pre' || quote.session === 'post' || quote.session === 'overnight') &&
    regClose > 0 &&
    Math.abs(quote.price - regClose) > 1e-9;
  const priorClose =
    isExt && closes && closes.length >= 2 ? closes[closes.length - 2] : quote?.prev_close ?? 0;

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
          <div>
            {quote ? (
              <>
                <PriceTag value={regularPrice} cur={cur} size="md" />
                {priorClose > 0 ? (
                  <div className="mt-0.5">
                    <ChangeLine
                      chg={regularPrice - priorClose}
                      pct={((regularPrice - priorClose) / priorClose) * 100}
                      cur={cur}
                    />
                  </div>
                ) : null}
              </>
            ) : (
              <span className={cx('text-2xl font-semibold tabular-nums', t.faint)}>
                {cur}—
              </span>
            )}
          </div>
          {closes && closes.length >= 2 && (
            <div className="-mb-1 shrink-0 opacity-90">
              <Sparkline
                values={closes}
                up={closes[closes.length - 1] >= closes[0]}
                w={84}
                h={36}
              />
            </div>
          )}
        </div>

        {/* extended-hours mini line: pre/post price + its delta vs the regular
            close (the regular figure above stays the "official" last-session number) */}
        {isExt && quote && (
          <div className="mt-1.5 flex flex-wrap items-center gap-x-1.5 gap-y-0.5 text-[11px]">
            <span className={t.faint}>{tr(`session.${quote.session}`)}</span>
            <span className={cx('font-semibold tabular-nums', t.sub)}>
              {cur}
              {quote.price.toFixed(2)}
            </span>
            <ChangeLine
              chg={quote.price - regClose}
              pct={((quote.price - regClose) / regClose) * 100}
              cur={cur}
            />
          </div>
        )}

        <div className="mt-3">
          {quote ? (
            <SessionBadge session={quote.session} sm />
          ) : (
            <span className={cx('text-[11px]', t.faint)}>{tr('stock.waiting')}</span>
          )}
        </div>
      </Link>

      {onRemove && (
        <button
          onClick={onRemove}
          className={cx(
            'absolute right-2.5 top-2.5 z-10 inline-flex h-7 w-7 items-center justify-center rounded-full opacity-70 transition hover:text-rose-500 hover:opacity-100 group-hover:opacity-100',
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
