'use client';

import {FileText, Sparkles, Users} from 'lucide-react';
import Link from 'next/link';
import {useCallback, useEffect, useState} from 'react';
import {getOpportunities, type OpportunityStock} from '@/lib/api';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';
import {EmptyState, ErrorState, FeedSkeleton} from '@/components/ui/states';

type Tokens = ReturnType<typeof tok>;
type Status = 'loading' | 'ready' | 'error';

function money(v: number): string {
  if (v >= 1e9) return `$${(v / 1e9).toFixed(1)}B`;
  if (v >= 1e6) return `$${(v / 1e6).toFixed(1)}M`;
  if (v >= 1e3) return `$${(v / 1e3).toFixed(0)}K`;
  return `$${v.toFixed(0)}`;
}

/**
 * The Opportunity board: small-cap US stocks where insiders are buying on the
 * open market (SEC Form 4). Presented as observed, sourced facts — every card
 * leads with the evidence ("3 insiders bought $1.2M") and links to the filing,
 * never a recommendation. Deliberately muted (no green-hero upside).
 */
export function OpportunityBoard() {
  const dark = useDark();
  const t = tok(dark);
  const [status, setStatus] = useState<Status>('loading');
  const [stocks, setStocks] = useState<OpportunityStock[]>([]);

  const load = useCallback(() => {
    setStatus('loading');
    getOpportunities(40).then(
      r => {
        setStocks(r.stocks ?? []);
        setStatus('ready');
      },
      () => setStatus('error'),
    );
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <div className="w-full">
      <header className="mb-4">
        <h1
          className={cx(
            'flex items-center gap-2 text-[22px] font-bold tracking-tight',
            t.text,
          )}
        >
          <Sparkles size={20} className={dark ? 'text-sky-300' : 'text-sky-600'} />
          Opportunity board
        </h1>
        <p className={cx('mt-1 text-[13.5px]', t.sub)}>
          Small-cap US stocks where company insiders are buying on the open market
          — surfaced from SEC Form 4 filings.
        </p>
      </header>

      <div
        className={cx(
          'mb-5 rounded-xl border p-3 text-[12px]',
          t.border,
          dark ? 'bg-slate-900' : 'bg-slate-50',
          t.sub,
        )}
      >
        Rows are surfaced by data signals (insider open-market buying), not
        recommendations. Inclusion is not a rating or a suggestion to buy.
        Small-caps can be volatile and illiquid. Not investment advice.
      </div>

      {status === 'loading' && <FeedSkeleton />}
      {status === 'error' && <ErrorState onRetry={load} />}
      {status === 'ready' && stocks.length === 0 && (
        <EmptyState
          label="No insider-buy signals yet"
          sub="The board fills in as recent SEC Form 4 open-market buys are filed."
          icon={Users}
        />
      )}
      {status === 'ready' && stocks.length > 0 && (
        <div className="tw-fade space-y-3">
          {stocks.map(s => (
            <OppCard key={s.ticker} s={s} dark={dark} t={t} />
          ))}
        </div>
      )}

      <p className={cx('mt-4 text-center text-[11px]', t.faint)}>
        Insider data from public SEC EDGAR filings. Not investment advice.
      </p>
    </div>
  );
}

function OppCard({s, dark, t}: {s: OpportunityStock; dark: boolean; t: Tokens}) {
  return (
    <section className={cx('rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="flex items-start gap-3">
        <span
          className={cx(
            'mt-0.5 w-6 shrink-0 text-center text-[13px] font-bold tabular-nums',
            t.faint,
          )}
        >
          {s.rank}
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
            <Link
              href={`/stock/${encodeURIComponent(s.ticker)}`}
              className={cx('text-[15px] font-bold hover:opacity-80', t.text)}
            >
              {s.ticker}
            </Link>
            {s.company && (
              <span className={cx('truncate text-[12.5px]', t.sub)}>{s.company}</span>
            )}
            <span
              className={cx(
                'rounded-full px-2 py-0.5 text-[10.5px] font-semibold',
                dark ? 'bg-slate-800 text-slate-300' : 'bg-slate-100 text-slate-500',
              )}
            >
              Small cap · {money(s.market_cap)}
            </span>
          </div>

          {/* hero = the sourced fact, not an upside number */}
          <p className={cx('mt-2 text-[13.5px] font-medium', t.text)}>{s.explainer}</p>

          <div className="mt-2 flex flex-wrap items-center gap-2 text-[12px]">
            <span
              className={cx(
                'inline-flex items-center gap-1 rounded-full border px-2 py-0.5',
                t.border,
                t.sub,
              )}
            >
              <Users size={12} /> {s.buyers} insider{s.buyers !== 1 ? 's' : ''}
            </span>
            <span
              className={cx(
                'inline-flex items-center rounded-full border px-2 py-0.5 font-semibold tabular-nums',
                t.border,
                t.sub,
              )}
            >
              {money(s.buy_value)} bought
            </span>
            {s.price > 0 && (
              <span className={cx('tabular-nums', t.faint)}>· ${s.price.toFixed(2)}</span>
            )}
          </div>

          {s.top_buyers && s.top_buyers.length > 0 && (
            <div className={cx('mt-2 space-y-0.5 text-[12px]', t.sub)}>
              {s.top_buyers.slice(0, 3).map((b, i) => (
                <div key={i} className="truncate">
                  <span className="font-medium">{b.title || 'Insider'}</span> {b.name} ·{' '}
                  <span className="tabular-nums">{money(b.value)}</span>
                </div>
              ))}
            </div>
          )}

          {s.filing_url && (
            <a
              href={s.filing_url}
              target="_blank"
              rel="noopener noreferrer"
              className={cx(
                'mt-2 inline-flex items-center gap-1 text-[12px] font-semibold',
                t.accentText,
              )}
            >
              <FileText size={12} /> View SEC filing
            </a>
          )}
        </div>
      </div>
    </section>
  );
}
