'use client';

import type {Quote, Security} from '@/lib/api';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, tok} from '@/lib/ui';

/**
 * How a stock strip is ordered. `default` keeps the natural order (a watchlist's
 * add-order, or the curated popular set); `change` ranks today's gainers first;
 * `alpha` is by ticker. Shared by the home Markets strip and the watchlist board.
 */
export type SortKey = 'default' | 'change' | 'alpha';

/**
 * Regular-session day-change %, mirroring what {@link StockCard} displays (the
 * regular price vs the prior regular close, with the extended-hours daily-bar
 * guard) so a "Change" sort matches the figure shown on each card. Returns null
 * when there's no usable quote — those names sort last.
 */
export function changePct(q: Quote | undefined, closes: number[] | undefined): number | null {
  if (!q) return null;
  const regClose = q.regular_close && q.regular_close > 0 ? q.regular_close : q.price;
  const regularPrice = q.session === 'regular' ? q.price : regClose;
  const isExt =
    (q.session === 'pre' || q.session === 'post' || q.session === 'overnight') &&
    regClose > 0 &&
    Math.abs(q.price - regClose) > 1e-9;
  const priorClose =
    isExt && closes && closes.length >= 2 ? closes[closes.length - 2] : q.prev_close ?? 0;
  if (!(priorClose > 0)) return null;
  return ((regularPrice - priorClose) / priorClose) * 100;
}

/**
 * Returns a new array of `cards` ordered by `sortKey`. `pctOf` supplies each
 * ticker's day-change % (null when unknown → sorted last). `default` returns the
 * input order unchanged (a stable identity, so callers can compare cheaply).
 */
export function sortSecurities<T extends Security>(
  cards: T[],
  sortKey: SortKey,
  pctOf: (ticker: string) => number | null,
): T[] {
  if (sortKey === 'default') return cards;
  const arr = [...cards];
  if (sortKey === 'alpha') {
    arr.sort((a, b) => a.ticker.localeCompare(b.ticker));
    return arr;
  }
  arr.sort((a, b) => {
    const ca = pctOf(a.ticker);
    const cb = pctOf(b.ticker);
    if (ca === null && cb === null) return 0;
    if (ca === null) return 1;
    if (cb === null) return -1;
    return cb - ca;
  });
  return arr;
}

/** A compact segmented control for choosing a stock-strip ordering. */
export function SortPills({
  value,
  onChange,
  defaultLabel,
  changeLabel,
  alphaLabel,
}: {
  value: SortKey;
  onChange: (k: SortKey) => void;
  defaultLabel: string;
  changeLabel: string;
  alphaLabel: string;
}) {
  const dark = useDark();
  const t = tok(dark);
  const opts: Array<{k: SortKey; label: string}> = [
    {k: 'default', label: defaultLabel},
    {k: 'change', label: changeLabel},
    {k: 'alpha', label: alphaLabel},
  ];
  return (
    <div className={cx('inline-flex items-center rounded-full border p-0.5', t.border)}>
      {opts.map(o => {
        const active = value === o.k;
        return (
          <button
            key={o.k}
            onClick={() => onChange(o.k)}
            aria-pressed={active}
            className={cx(
              'rounded-full px-2.5 py-1 text-[12px] font-semibold transition',
              active ? btnPrimary(dark) : cx('hover:opacity-80', t.sub),
            )}
          >
            {o.label}
          </button>
        );
      })}
    </div>
  );
}
