import type {Security} from '@/lib/api';
import {MarketBadge} from '@/components/MarketBadge';

interface StockHeaderProps {
  security: Security;
}

/** Detail-page header: company name, ticker, market badge, and CIK. */
export function StockHeader({security}: StockHeaderProps) {
  return (
    <div className="flex flex-col gap-3 border-b border-white/10 pb-6">
      <div className="flex flex-wrap items-center gap-3">
        <h1 className="text-2xl font-bold tracking-tight text-zinc-50 sm:text-3xl">
          {security.name}
        </h1>
        <MarketBadge market={security.market} />
      </div>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-zinc-500">
        <span className="font-mono font-semibold text-zinc-300">
          {security.ticker}
        </span>
        {security.cik ? (
          <span>
            CIK <span className="font-mono text-zinc-400">{security.cik}</span>
          </span>
        ) : null}
      </div>
    </div>
  );
}
