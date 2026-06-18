'use client';

import {LayoutGrid, Rows3, X} from 'lucide-react';
import {useCallback, useEffect, useState} from 'react';
import Link from '@/components/LocalLink';
import type {Quote, Security} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, marketCurrency, tok} from '@/lib/ui';
import {
  ChangeLine,
  MarketBadge,
  PriceTag,
  SessionBadge,
  Sparkline,
} from '@/components/ui/atoms';

/**
 * A single full-width stock ROW — the list-mode counterpart to StockCard (a
 * Futu-style dense list). Ticker + name on the left, a compact trend sparkline and
 * session in the middle, live price + day-change on the right; an optional hover ×
 * removes it (watchlist). Links to the detail page. Mirrors StockCard's regular /
 * extended-hours price logic EXACTLY so both views show identical numbers.
 */
export function StockRow({
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

  // Same regular/extended-hours split as StockCard so the row shows the same price
  // + day-change the card does (the day-change measures vs the prior regular close,
  // anchored to the daily bars in extended hours to avoid a phantom zero).
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
          'flex items-center gap-3 rounded-xl border px-3.5 py-2.5 transition',
          t.card,
          t.border,
          t.cardI,
        )}
      >
        {/* Ticker + name */}
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5">
            <span className={cx('text-[14px] font-bold tracking-tight', t.text)}>
              {security.ticker}
            </span>
            <MarketBadge mkt={security.market} />
          </div>
          <p className={cx('truncate text-[11.5px]', t.sub)}>{security.name}</p>
        </div>

        {/* Trend sparkline — hidden on the narrowest widths to keep the row clean */}
        {closes && closes.length >= 2 && (
          <div className="hidden shrink-0 opacity-90 sm:block">
            <Sparkline
              values={closes}
              up={closes[closes.length - 1] >= closes[0]}
              w={72}
              h={28}
            />
          </div>
        )}

        {/* Session badge — desktop only */}
        {quote && (
          <div className="hidden shrink-0 md:block">
            <SessionBadge session={quote.session} sm />
          </div>
        )}

        {/* Live price + day change (right-aligned) */}
        <div className="shrink-0 text-right">
          {quote ? (
            <>
              <PriceTag value={regularPrice} cur={cur} size="sm" />
              {priorClose > 0 ? (
                <div className="mt-0.5 flex justify-end">
                  <ChangeLine
                    chg={regularPrice - priorClose}
                    pct={((regularPrice - priorClose) / priorClose) * 100}
                    cur={cur}
                    size="sm"
                  />
                </div>
              ) : null}
            </>
          ) : (
            <span className={cx('text-[15px] font-semibold tabular-nums', t.faint)}>
              {cur}—
            </span>
          )}
        </div>

        {/* Reserve room for the hover remove button (watchlist) so the price never
            sits under it. */}
        {onRemove && <span className="w-6 shrink-0" aria-hidden />}
      </Link>

      {onRemove && (
        <button
          onClick={onRemove}
          className={cx(
            'absolute right-2 top-1/2 z-10 inline-flex h-7 w-7 -translate-y-1/2 items-center justify-center rounded-full opacity-0 transition hover:text-rose-500 group-hover:opacity-100',
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

/** Stock-strip view mode: card tiles (default) or a Futu-style one-row-per-stock list. */
export type StockListView = 'cards' | 'list';

/**
 * Shared cards/list view state for the stock strips (home Markets + watchlist),
 * persisted in localStorage so the choice is consistent across the app. SSR-safe:
 * defaults to 'cards', reads the saved value on mount.
 */
export function useStockListView(): [StockListView, (v: StockListView) => void] {
  const [view, setView] = useState<StockListView>('cards');
  useEffect(() => {
    try {
      const v = localStorage.getItem('tw-board-view');
      if (v === 'list' || v === 'cards') setView(v);
    } catch {
      /* localStorage unavailable (private mode) — keep the default */
    }
  }, []);
  const set = useCallback((v: StockListView) => {
    setView(v);
    try {
      localStorage.setItem('tw-board-view', v);
    } catch {
      /* ignore persistence failure */
    }
  }, []);
  return [view, set];
}

/** A small segmented Cards ⇄ List toggle for a stock strip. Self-contained (reads
 *  theme + language from context), so call sites just pass the view + onChange. */
export function StockListToggle({
  view,
  onChange,
}: {
  view: StockListView;
  onChange: (v: StockListView) => void;
}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const opts = [
    {key: 'cards' as const, icon: LayoutGrid, label: tr('board.viewCards')},
    {key: 'list' as const, icon: Rows3, label: tr('board.viewList')},
  ];
  return (
    <div className={cx('inline-flex items-center rounded-full border p-0.5', t.border, t.card)}>
      {opts.map(o => {
        const active = view === o.key;
        const Icon = o.icon;
        return (
          <button
            key={o.key}
            onClick={() => onChange(o.key)}
            aria-pressed={active}
            aria-label={o.label}
            title={o.label}
            className={cx(
              'inline-flex h-7 w-7 items-center justify-center rounded-full transition',
              active ? btnPrimary(dark) : cx('hover:opacity-80', t.sub),
            )}
          >
            <Icon size={14} />
          </button>
        );
      })}
    </div>
  );
}
