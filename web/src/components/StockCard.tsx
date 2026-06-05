import Link from 'next/link';
import type {Quote, Security} from '@/lib/api';
import {MarketBadge} from '@/components/MarketBadge';
import {PriceTag} from '@/components/PriceTag';

interface StockCardProps {
  security: Security;
  /** Latest live price, or `undefined` until one arrives. */
  quote?: Quote;
}

/**
 * A watchlist tile linking to the stock detail page. Built from a resolved
 * {@link Security} plus an optional live {@link Quote}; the parent decides how
 * to handle load/error per ticker and supplies prices as they stream in.
 */
export function StockCard({security, quote}: StockCardProps) {
  return (
    <Link
      href={{pathname: '/stock', query: {ticker: security.ticker}}}
      className="group relative flex flex-col gap-4 rounded-2xl border border-white/10 bg-white/[0.03] p-5 transition hover:border-sky-400/40 hover:bg-white/[0.06] focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/60"
    >
      <div className="flex items-start justify-between gap-3">
        <span className="font-mono text-lg font-bold tracking-tight text-zinc-100">
          {security.ticker}
        </span>
        <MarketBadge market={security.market} />
      </div>
      <PriceTag quote={quote} />
      <p className="line-clamp-2 text-sm text-zinc-400 group-hover:text-zinc-300">
        {security.name}
      </p>
      <span className="mt-auto inline-flex items-center gap-1 text-xs font-medium text-sky-400/80 group-hover:text-sky-300">
        View filings
        <span aria-hidden className="transition group-hover:translate-x-0.5">
          →
        </span>
      </span>
    </Link>
  );
}

/** Skeleton placeholder matching {@link StockCard}'s footprint. */
export function StockCardSkeleton({ticker}: {ticker: string}) {
  return (
    <div className="flex flex-col gap-4 rounded-2xl border border-white/10 bg-white/[0.03] p-5">
      <div className="flex items-start justify-between gap-3">
        <span className="font-mono text-lg font-bold tracking-tight text-zinc-300">
          {ticker}
        </span>
        <span className="h-5 w-9 animate-pulse rounded-full bg-white/10" />
      </div>
      <span className="h-4 w-3/4 animate-pulse rounded bg-white/10" />
      <span className="mt-auto h-4 w-1/3 animate-pulse rounded bg-white/5" />
    </div>
  );
}

/** Error tile shown when a single watchlist ticker fails to load. */
export function StockCardError({
  ticker,
  message,
}: {
  ticker: string;
  message: string;
}) {
  return (
    <Link
      href={{pathname: '/stock', query: {ticker}}}
      className="flex flex-col gap-3 rounded-2xl border border-rose-500/25 bg-rose-500/[0.04] p-5 transition hover:border-rose-500/40"
    >
      <span className="font-mono text-lg font-bold tracking-tight text-zinc-200">
        {ticker}
      </span>
      <p className="text-sm text-rose-300/80">{message}</p>
      <span className="mt-auto text-xs text-zinc-500">Open anyway →</span>
    </Link>
  );
}
