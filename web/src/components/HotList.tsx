'use client';

import {Flame, TrendingDown, TrendingUp} from 'lucide-react';
import Link from 'next/link';
import {useCallback, useEffect, useState} from 'react';
import {getHot, type HotStock} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';
import {EmptyState, ErrorState, FeedSkeleton} from '@/components/ui/states';

type Tokens = ReturnType<typeof tok>;
type Status = 'loading' | 'ready' | 'error';

const BOARDS = [
  {key: 'hot', labelKey: 'nav.hot', blurbKey: 'hot.blurbHot'},
  {key: 'surging', labelKey: 'hot.surging', blurbKey: 'hot.blurbSurging'},
] as const;

/**
 * The trending leaderboards: the most-discussed US stocks (Hot) and the biggest
 * 24h attention risers (Surging), market-wide. Buzz data from ApeWisdom (Reddit
 * mentions); each row links through to the full stock page.
 */
export function HotList({initialBoard = 'hot'}: {initialBoard?: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [board, setBoard] = useState<string>(initialBoard);
  const [status, setStatus] = useState<Status>('loading');
  const [stocks, setStocks] = useState<HotStock[]>([]);

  const load = useCallback((b: string) => {
    setStatus('loading');
    getHot(b, 40).then(
      r => {
        setStocks(r.stocks ?? []);
        setStatus('ready');
      },
      () => setStatus('error'),
    );
  }, []);

  useEffect(() => {
    load(board);
  }, [board, load]);

  const blurb = tr(BOARDS.find(b => b.key === board)?.blurbKey ?? 'hot.blurbHot');

  return (
    <div className="mx-auto max-w-3xl">
      <header className="mb-5">
        <h1
          className={cx(
            'flex items-center gap-2 text-[22px] font-bold tracking-tight',
            t.text,
          )}
        >
          <Flame size={22} className={dark ? 'text-amber-300' : 'text-amber-500'} />
          {tr('mod.hotStocks')}
        </h1>
        <p className={cx('mt-1 text-[13.5px]', t.sub)}>{blurb}</p>
      </header>

      <div className="mb-4">
        <div
          role="tablist"
          aria-label="Leaderboards"
          className={cx('inline-flex items-center gap-1 rounded-xl border p-1', t.border, t.surf2)}
        >
          {BOARDS.map(b => (
            <button
              key={b.key}
              role="tab"
              aria-selected={board === b.key}
              onClick={() => setBoard(b.key)}
              className={cx(
                'rounded-lg px-3.5 py-1.5 text-[13px] font-medium transition',
                board === b.key
                  ? dark
                    ? 'bg-slate-700 text-white'
                    : 'bg-white text-slate-900 shadow-sm'
                  : t.sub,
              )}
            >
              {tr(b.labelKey)}
            </button>
          ))}
        </div>
      </div>

      {status === 'loading' && <FeedSkeleton />}
      {status === 'error' && <ErrorState onRetry={() => load(board)} />}
      {status === 'ready' && stocks.length === 0 && (
        <EmptyState label={tr('mod.noData')} sub={tr('hot.emptySub')} icon={Flame} />
      )}
      {status === 'ready' && stocks.length > 0 && (
        <div
          className={cx(
            'tw-fade overflow-hidden rounded-2xl border',
            t.card,
            t.border,
            t.soft,
          )}
        >
          {stocks.map((s, i) => (
            <HotRow
              key={s.ticker}
              s={s}
              dark={dark}
              t={t}
              last={i === stocks.length - 1}
            />
          ))}
        </div>
      )}

      <p className={cx('mt-4 text-center text-[11px]', t.faint)}>{tr('hot.footer')}</p>
    </div>
  );
}

function HotRow({
  s,
  dark,
  t,
  last,
}: {
  s: HotStock;
  dark: boolean;
  t: Tokens;
  last: boolean;
}) {
  const tr = useT();
  const up = s.change >= 0;
  const pct = Math.abs(s.change * 100);
  const surging = s.change >= 0.5; // a notable riser
  const changeColor = up
    ? dark
      ? 'text-emerald-400'
      : 'text-emerald-600'
    : dark
      ? 'text-rose-400'
      : 'text-rose-500';
  return (
    <Link
      href={`/stock/${encodeURIComponent(s.ticker)}`}
      className={cx(
        'flex items-center gap-3 px-4 py-3 transition',
        dark ? 'hover:bg-white/5' : 'hover:bg-slate-50',
        !last && cx('border-b', t.hair),
      )}
    >
      <span
        className={cx(
          'w-6 shrink-0 text-center text-[13px] font-bold tabular-nums',
          s.rank <= 3 ? (dark ? 'text-amber-300' : 'text-amber-500') : t.faint,
        )}
      >
        {s.rank}
      </span>

      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5">
          <span className={cx('text-[14px] font-bold', t.text)}>{s.ticker}</span>
          {surging && (
            <Flame size={12} className={dark ? 'text-amber-300' : 'text-amber-500'} />
          )}
        </div>
        {s.name && <p className={cx('truncate text-[12px]', t.sub)}>{s.name}</p>}
      </div>

      <div className="shrink-0 text-right">
        <div className={cx('text-[13px] font-semibold tabular-nums', t.text)}>
          {s.mentions.toLocaleString()}
        </div>
        <div className={cx('text-[11px]', t.faint)}>{tr('wsb.mentions')}</div>
      </div>

      <div
        className={cx(
          'flex w-[68px] shrink-0 items-center justify-end gap-0.5 text-[12.5px] font-semibold tabular-nums',
          changeColor,
        )}
      >
        {up ? <TrendingUp size={13} /> : <TrendingDown size={13} />}
        {pct.toFixed(0)}%
      </div>
    </Link>
  );
}
