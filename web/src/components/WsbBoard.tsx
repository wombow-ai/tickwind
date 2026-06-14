'use client';

import {Flame, TrendingDown, TrendingUp} from 'lucide-react';
import Link from '@/components/LocalLink';
import {useCallback, useEffect, useState} from 'react';
import {getHot, type HotStock} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

type Tokens = ReturnType<typeof tok>;

const ROWS = 8;

/**
 * The "WSB Trending" mini-board: the tickers r/wallstreetbets is talking about
 * most right now, ranked by *rising* 24h mention momentum (from ApeWisdom) so it
 * surfaces buzz that's gaining traction rather than perennially-loud, cooling
 * names. It is discussion buzz — not sentiment, and not a recommendation.
 *
 * The card (header + skeleton rows) renders from first paint and fills in on
 * load, so it never "pops in" after the rest of the page; it collapses only if
 * the feed comes back genuinely empty.
 */
export function WsbBoard() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [stocks, setStocks] = useState<HotStock[]>([]);
  const [ready, setReady] = useState(false);

  const load = useCallback(() => {
    getHot('wsb', ROWS).then(
      r => {
        setStocks(r.stocks ?? []);
        setReady(true);
      },
      () => {
        setStocks([]);
        setReady(true);
      },
    );
  }, []);
  useEffect(() => {
    load();
  }, [load]);

  // Only hide once we know the feed is genuinely empty; while loading we keep the
  // card (with skeleton rows) so it reserves its space and never pops in.
  if (ready && stocks.length === 0) return null;

  return (
    <section className={cx('rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-2 flex items-center justify-between">
        <h2 className={cx('flex items-center gap-1.5 text-[14px] font-bold', t.text)}>
          <Flame size={15} className={dark ? 'text-orange-300' : 'text-orange-500'} />
          {tr('wsb.title')}
        </h2>
        <Link
          href="/hot"
          className={cx('shrink-0 text-[12px] font-semibold hover:opacity-80', t.accentText)}
        >
          {tr('wsb.hotList')} →
        </Link>
      </div>
      <div>
        {ready
          ? stocks.map((s, i) => (
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
                  {s.mentions} {tr('wsb.mentions')}
                </span>
                <Arrow change={s.change} dark={dark} />
              </Link>
            ))
          : Array.from({length: ROWS}).map((_, i) => (
              <SkeletonRow key={i} dark={dark} t={t} last={i === ROWS - 1} />
            ))}
      </div>
      <p className={cx('mt-2 text-[10.5px]', t.faint)}>{tr('wsb.footer')}</p>
    </section>
  );
}

/** A shimmer placeholder row matching a loaded row's height, so the card doesn't jump. */
function SkeletonRow({dark, t, last}: {dark: boolean; t: Tokens; last: boolean}) {
  const bar = dark ? 'bg-slate-700/60' : 'bg-slate-200';
  return (
    <div
      className={cx(
        'flex animate-pulse items-center gap-2 py-1.5',
        !last && cx('border-b', t.hair),
      )}
    >
      <span className={cx('h-3 w-3 rounded', bar)} />
      <span className={cx('h-3 w-14 rounded', bar)} />
      <span className={cx('ml-auto h-3 w-20 rounded', bar)} />
    </div>
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
