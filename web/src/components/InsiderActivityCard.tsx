'use client';

import {ExternalLink, Users} from 'lucide-react';
import {useEffect, useState} from 'react';
import {
  getInsiderActivity,
  type InsiderActivityResponse,
  type InsiderTransaction,
} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, fmtCompactUSD, tok} from '@/lib/ui';

type Status = 'loading' | 'ready' | 'hidden';

/**
 * The insider-activity card: a company's recent Form 4 open-market transactions —
 * both buys (green) AND sells (red) — newest first, each with shares, price,
 * value, date, the insider's name + role, and a subtle "10b5-1 plan" tag on
 * affirmed planned sales (these carry far less signal than discretionary sells).
 *
 * Pure structured data, NO LLM: every number/fact is Go-owned, parsed straight
 * from the Form 4 XML server-side. Hides entirely (renders null) when the symbol
 * is unknown (404 → null) or the fetch fails. An existing company with zero
 * recent Form 4s shows a subtle empty line rather than vanishing, so the section
 * reads as "checked, nothing recent".
 */
export function InsiderActivityCard({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [data, setData] = useState<InsiderActivityResponse | null>(null);
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    getInsiderActivity(ticker, c.signal).then(
      r => {
        if (!r) {
          setStatus('hidden'); // unknown symbol (404) → hide
          return;
        }
        setData(r);
        setStatus('ready');
      },
      () => setStatus('hidden'), // error → hide
    );
    return () => c.abort();
  }, [ticker]);

  if (status === 'hidden') return null;

  const Header = (
    <div className="mb-3 flex flex-wrap items-center gap-2">
      <h2 className={cx('flex items-center gap-1.5 text-[14px] font-bold', t.text)}>
        <Users size={15} className={dark ? 'text-sky-300' : 'text-sky-500'} />
        {tr('insider.title')}
      </h2>
      <span className={cx('ml-auto text-[10.5px]', t.faint)}>
        {tr('insider.source')}
        {data?.generated_at ? ` · ${tr('insider.asOf')} ${data.generated_at.slice(0, 10)}` : ''}
      </span>
    </div>
  );

  if (status === 'loading') {
    return (
      <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
        {Header}
        <div className="space-y-2" aria-hidden>
          <div className={cx('h-3 rounded', t.skel)} style={{width: '68%'}} />
          <div className={cx('h-3 rounded', t.skel)} style={{width: '50%'}} />
        </div>
      </section>
    );
  }

  // status === 'ready' — data is non-null.
  const txns = data?.transactions ?? [];

  return (
    <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
      {Header}

      {txns.length === 0 ? (
        <p className={cx('text-[12.5px]', t.faint)}>{tr('insider.empty')}</p>
      ) : (
        <>
          {/* Lightweight aggregate strip: net buy/sell flow over the window. */}
          {data && (data.buy_count > 0 || data.sell_count > 0) && (
            <div className={cx('mb-2.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11.5px]', t.sub)}>
              <span>
                {data.buy_count} {tr('insider.buys')} · {data.sell_count} {tr('insider.sells')}
              </span>
              <span
                className={cx(
                  'font-semibold tabular-nums',
                  data.net_value >= 0
                    ? dark
                      ? 'text-emerald-300'
                      : 'text-emerald-600'
                    : dark
                      ? 'text-rose-300'
                      : 'text-rose-600',
                )}
              >
                {tr('insider.net')} {fmtCompactUSD(data.net_value)}
              </span>
            </div>
          )}

          <ul className="flex flex-col gap-2">
            {txns.map((tx, i) => (
              <TxnRow key={i} tx={tx} dark={dark} />
            ))}
          </ul>
        </>
      )}

      <p className={cx('mt-3 text-[10.5px]', t.faint)}>{tr('insider.delay')}</p>
    </section>
  );
}

/** One Form 4 transaction row: a BUY/SELL badge, the insider + role, the
 * Go-owned shares/price/value, the date, an optional 10b5-1 tag, and the SEC
 * source link. */
function TxnRow({tx, dark}: {tx: InsiderTransaction; dark: boolean}) {
  const t = tok(dark);
  const tr = useT();
  const isBuy = tx.type === 'buy';

  return (
    <li className={cx('rounded-xl border p-2.5', t.border)}>
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        {/* BUY (green) / SELL (red) badge — Go-owned classification. */}
        <span
          className={cx(
            'rounded-md px-1.5 py-0.5 text-[10px] font-bold',
            isBuy
              ? dark
                ? 'bg-emerald-500/15 text-emerald-300'
                : 'bg-emerald-50 text-emerald-600'
              : dark
                ? 'bg-rose-500/15 text-rose-300'
                : 'bg-rose-50 text-rose-600',
          )}
        >
          {isBuy ? tr('insider.buy') : tr('insider.sell')}
        </span>

        {/* Insider name + role (Go-owned facts). */}
        <span className={cx('text-[12.5px] font-semibold', t.text)}>{tx.owner}</span>
        {tx.role && <span className={cx('text-[11px]', t.sub)}>{tx.role}</span>}

        {/* 10b5-1 planned-sale tag — only on affirmed planned sells. */}
        {tx.planned_10b5_1 && (
          <span
            title={tr('insider.plannedTitle')}
            className={cx(
              'rounded-md px-1.5 py-0.5 text-[10px] font-semibold',
              t.chip,
              t.chipText,
            )}
          >
            {tr('insider.planned')}
          </span>
        )}

        {/* Date (Go-owned fact). */}
        <span className={cx('ml-auto text-[11px] tabular-nums', t.faint)}>{tx.date}</span>
      </div>

      <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[11.5px]">
        {/* shares @ price = value — all Go-owned. */}
        <span className={cx('tabular-nums', t.sub)}>
          {tx.shares.toLocaleString('en-US')} {tr('insider.shares')} {tr('insider.atPrice')}{' '}
          ${tx.price.toLocaleString('en-US', {minimumFractionDigits: 2, maximumFractionDigits: 2})}
        </span>
        <span
          className={cx(
            'font-semibold tabular-nums',
            isBuy
              ? dark
                ? 'text-emerald-300'
                : 'text-emerald-600'
              : dark
                ? 'text-rose-300'
                : 'text-rose-600',
          )}
        >
          {fmtCompactUSD(tx.value)}
        </span>
        {tx.accession_url && (
          <a
            href={tx.accession_url}
            target="_blank"
            rel="noopener noreferrer"
            className={cx(
              'ml-auto inline-flex items-center gap-1 text-[11px] transition-colors',
              t.sub,
              dark ? 'hover:text-sky-300' : 'hover:text-sky-600',
            )}
          >
            <ExternalLink size={12} />
            {tr('insider.viewSource')}
          </a>
        )}
      </div>
    </li>
  );
}
