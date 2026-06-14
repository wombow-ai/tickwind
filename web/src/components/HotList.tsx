'use client';

import {Flame, TrendingDown, TrendingUp} from 'lucide-react';
import Link from '@/components/LocalLink';
import {useCallback, useEffect, useState} from 'react';
import {
  getHot,
  getShortVolume,
  type HotStock,
  type ShortVolumeStock,
} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';
import {EmptyState, ErrorState, FeedSkeleton} from '@/components/ui/states';

type Tokens = ReturnType<typeof tok>;
type Status = 'loading' | 'ready' | 'error';

// Buzz boards (HotStock shape) plus the short-volume board (its own shape,
// fetched separately) — all surfaced as sibling tabs so the squeeze crowd has
// one home alongside the discussion leaderboards.
const BOARDS = [
  {key: 'hot', labelKey: 'nav.hot', blurbKey: 'hot.blurbHot'},
  {key: 'surging', labelKey: 'hot.surging', blurbKey: 'hot.blurbSurging'},
  {key: 'shortvol', labelKey: 'shortvol.tab', blurbKey: 'shortvol.blurb'},
] as const;

/**
 * The trending leaderboards: the most-discussed US stocks (Hot), the biggest
 * 24h attention risers (Surging), and today's highest short-volume names
 * (Short volume). Buzz from ApeWisdom (Reddit mentions); short volume from
 * FINRA. Each row links through to the full stock page.
 */
export function HotList({initialBoard = 'hot'}: {initialBoard?: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [board, setBoard] = useState<string>(initialBoard);
  const [status, setStatus] = useState<Status>('loading');
  const [stocks, setStocks] = useState<HotStock[]>([]);
  const [shortVol, setShortVol] = useState<ShortVolumeStock[]>([]);
  const [shortAsOf, setShortAsOf] = useState<string>('');

  const isShort = board === 'shortvol';

  const load = useCallback((b: string) => {
    setStatus('loading');
    if (b === 'shortvol') {
      getShortVolume(50).then(
        r => {
          setShortVol(r.stocks ?? []);
          setShortAsOf(r.as_of ?? '');
          setStatus('ready');
        },
        () => setStatus('error'),
      );
      return;
    }
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
  const empty = isShort
    ? {label: tr('shortvol.empty'), sub: tr('shortvol.emptySub')}
    : {label: tr('mod.noData'), sub: tr('hot.emptySub')};
  const list = isShort ? shortVol : stocks;

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
          {tr(isShort ? 'shortvol.title' : 'mod.hotStocks')}
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
      {status === 'ready' && list.length === 0 && (
        <EmptyState label={empty.label} sub={empty.sub} icon={Flame} />
      )}
      {status === 'ready' && list.length > 0 && (
        <div
          className={cx(
            'tw-fade overflow-hidden rounded-2xl border',
            t.card,
            t.border,
            t.soft,
          )}
        >
          {isShort
            ? shortVol.map((s, i) => (
                <ShortVolRow
                  key={s.symbol}
                  s={s}
                  rank={i + 1}
                  dark={dark}
                  t={t}
                  last={i === shortVol.length - 1}
                />
              ))
            : stocks.map((s, i) => (
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

      <p className={cx('mt-4 text-center text-[11px]', t.faint)}>
        {tr(isShort ? 'shortvol.footer' : 'hot.footer')}
      </p>
    </div>
  );
}

/** Compact volume: 1_234_567 → "1.2M"; 12_345 → "12K". */
function fmtVol(n: number): string {
  if (n >= 1e9) return `${(n / 1e9).toFixed(2)}B`;
  if (n >= 1e6) return `${(n / 1e6).toFixed(1)}M`;
  if (n >= 1e3) return `${(n / 1e3).toFixed(0)}K`;
  return String(n);
}

function ShortVolRow({
  s,
  rank,
  dark,
  t,
  last,
}: {
  s: ShortVolumeStock;
  rank: number;
  dark: boolean;
  t: Tokens;
  last: boolean;
}) {
  const tr = useT();
  // Heavily-shorted share reads hot; bar fills to short_pct.
  const high = s.short_pct >= 50;
  const pctColor = high
    ? dark
      ? 'text-rose-300'
      : 'text-rose-600'
    : dark
      ? 'text-amber-300'
      : 'text-amber-600';
  const barColor = high ? (dark ? 'bg-rose-400/70' : 'bg-rose-400') : dark ? 'bg-amber-400/70' : 'bg-amber-400';
  const pct = Math.max(0, Math.min(100, s.short_pct));
  return (
    <Link
      href={`/stock/${encodeURIComponent(s.symbol)}`}
      className={cx(
        'flex items-center gap-3 px-4 py-3 transition',
        dark ? 'hover:bg-white/5' : 'hover:bg-slate-50',
        !last && cx('border-b', t.hair),
      )}
    >
      <span
        className={cx(
          'w-6 shrink-0 text-center text-[13px] font-bold tabular-nums',
          rank <= 3 ? (dark ? 'text-amber-300' : 'text-amber-500') : t.faint,
        )}
      >
        {rank}
      </span>

      <span className={cx('w-16 shrink-0 text-[14px] font-bold', t.text)}>{s.symbol}</span>

      {/* short-% bar fills the middle */}
      <div className="min-w-0 flex-1">
        <div className={cx('h-2 w-full overflow-hidden rounded-full', dark ? 'bg-slate-800' : 'bg-slate-100')}>
          <div className={cx('h-full rounded-full', barColor)} style={{width: `${pct}%`}} />
        </div>
      </div>

      <div className="hidden shrink-0 text-right sm:block">
        <div className={cx('text-[12px] font-medium tabular-nums', t.faint)}>
          {fmtVol(s.short_volume)} / {fmtVol(s.total_volume)}
        </div>
        <div className={cx('text-[10.5px]', t.faint)}>{tr('shortvol.colVolume')}</div>
      </div>

      <span className={cx('w-[58px] shrink-0 text-right text-[14px] font-bold tabular-nums', pctColor)}>
        {s.short_pct.toFixed(1)}%
      </span>
    </Link>
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

      {typeof s.price === 'number' && (
        <div className="hidden shrink-0 text-right sm:block">
          <div className={cx('text-[13px] font-semibold tabular-nums', t.text)}>
            ${s.price.toFixed(2)}
          </div>
          {typeof s.change_pct === 'number' && (
            <div
              className={cx(
                'text-[11px] font-semibold tabular-nums',
                s.change_pct >= 0
                  ? dark
                    ? 'text-emerald-400'
                    : 'text-emerald-600'
                  : dark
                    ? 'text-rose-400'
                    : 'text-rose-500',
              )}
            >
              {s.change_pct >= 0 ? '+' : ''}
              {s.change_pct.toFixed(2)}%
            </div>
          )}
        </div>
      )}

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
