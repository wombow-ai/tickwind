'use client';

import {Flame, TrendingDown, TrendingUp} from 'lucide-react';
import Link from 'next/link';
import {useCallback, useEffect, useState} from 'react';
import {getHot, type HotStock} from '@/lib/api';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

/**
 * The "WSB Trending" mini-board: the tickers r/wallstreetbets is talking about
 * most right now (mention volume + 24h momentum), from ApeWisdom. It is
 * discussion buzz — not sentiment, and not a recommendation. Hidden until it has
 * data so it never shows an empty shell.
 */
export function WsbBoard() {
  const dark = useDark();
  const t = tok(dark);
  const [stocks, setStocks] = useState<HotStock[]>([]);

  const load = useCallback(() => {
    getHot('wsb', 8).then(
      r => setStocks(r.stocks ?? []),
      () => setStocks([]),
    );
  }, []);
  useEffect(() => {
    load();
  }, [load]);

  if (stocks.length === 0) return null;

  return (
    <section className={cx('rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-2 flex items-center justify-between">
        <h2 className={cx('flex items-center gap-1.5 text-[14px] font-bold', t.text)}>
          <Flame size={15} className={dark ? 'text-orange-300' : 'text-orange-500'} />
          WSB Trending
        </h2>
        <Link
          href="/hot"
          className={cx('shrink-0 text-[12px] font-semibold hover:opacity-80', t.accentText)}
        >
          Hot list →
        </Link>
      </div>
      <div>
        {stocks.map((s, i) => (
          <Link
            key={s.ticker}
            href={`/stock/${encodeURIComponent(s.ticker)}`}
            className={cx(
              'flex items-center gap-2 py-1.5',
              i < stocks.length - 1 && cx('border-b', t.hair),
            )}
          >
            <span
              className={cx(
                'w-4 text-[12px] font-bold tabular-nums',
                s.rank <= 3 ? (dark ? 'text-orange-300' : 'text-orange-500') : t.faint,
              )}
            >
              {s.rank}
            </span>
            <span className={cx('flex-1 truncate text-[13px] font-bold', t.text)}>
              {s.ticker}
            </span>
            <span className={cx('shrink-0 text-[11px] tabular-nums', t.faint)}>
              {s.mentions} mentions
            </span>
            <Arrow change={s.change} dark={dark} />
          </Link>
        ))}
      </div>
      <p className={cx('mt-2 text-[10.5px]', t.faint)}>
        Source: ApeWisdom · discussion volume, not advice.
      </p>
    </section>
  );
}

function Arrow({change, dark}: {change: number; dark: boolean}) {
  if (!change) return <span className="w-12 shrink-0" />;
  const up = change >= 0;
  const color = up
    ? dark
      ? 'text-emerald-400'
      : 'text-emerald-600'
    : dark
      ? 'text-rose-400'
      : 'text-rose-500';
  return (
    <span
      className={cx(
        'inline-flex w-12 shrink-0 items-center justify-end gap-0.5 text-[11px] font-semibold tabular-nums',
        color,
      )}
    >
      {up ? <TrendingUp size={11} /> : <TrendingDown size={11} />}
      {Math.abs(change * 100).toFixed(0)}%
    </span>
  );
}
