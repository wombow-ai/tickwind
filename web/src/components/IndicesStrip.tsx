'use client';

import {useEffect, useState} from 'react';
import {getIndices, type IndexQuote} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';
import {useQuotes} from '@/lib/useQuotes';

// Real index levels from `GET /v1/indices`. The backend index-level source was
// removed, so the endpoint currently returns empty and the strip falls back to
// the keyless ETF proxies below (Alpaca SPY/DIA/QQQ): it then shows the proxy's
// % change with the ETF attributed honestly. The /v1/indices path is kept for
// when a licensed index feed is wired (real index levels would take precedence).
const FALLBACK = [
  {symbol: 'SPY', label: 'S&P 500'},
  {symbol: 'DIA', label: 'Dow Jones'},
  {symbol: 'QQQ', label: 'Nasdaq 100'},
] as const;

const FALLBACK_SYMBOLS = FALLBACK.map(i => i.symbol);

/** Poll cadence for index levels; matches the backend's refresh. */
const POLL_MS = 60_000;

function Cell({
  label,
  pct,
  sub,
  title,
}: {
  label: string;
  pct: number | null;
  sub: string;
  title?: string;
}) {
  const dark = useDark();
  const t = tok(dark);
  const up = (pct ?? 0) >= 0;
  const col = up
    ? dark
      ? 'text-emerald-400'
      : 'text-emerald-600'
    : dark
      ? 'text-rose-400'
      : 'text-rose-500';
  return (
    <div title={title} className={cx('px-3 py-2.5 sm:px-4', t.card)}>
      <div className={cx('truncate text-[12px] font-semibold', t.text)}>{label}</div>
      {pct !== null ? (
        <div
          className={cx(
            'mt-0.5 inline-flex items-center gap-0.5 text-[13.5px] font-semibold tabular-nums',
            col,
          )}
        >
          <span style={{fontSize: 9}}>{up ? '▲' : '▼'}</span>
          {up ? '+' : '−'}
          {Math.abs(pct).toFixed(2)}%
        </div>
      ) : (
        <div className={cx('mt-0.5 text-[13.5px] font-semibold', t.faint)}>—</div>
      )}
      <div className={cx('text-[10.5px] tabular-nums', t.faint)}>{sub}</div>
    </div>
  );
}

export function IndicesStrip() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [indices, setIndices] = useState<IndexQuote[]>([]);
  // ETF fallback quotes — only subscribed while the real indices are absent.
  const quotes = useQuotes(indices.length > 0 ? [] : FALLBACK_SYMBOLS);

  useEffect(() => {
    const c = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const load = () => {
      getIndices(c.signal).then(
        r => {
          if (r.indices.length > 0) setIndices(r.indices);
          timer = setTimeout(load, POLL_MS);
        },
        () => {
          timer = setTimeout(load, POLL_MS);
        },
      );
    };
    load();
    return () => {
      c.abort();
      if (timer !== undefined) clearTimeout(timer);
    };
  }, []);

  // Column count tracks the cells actually rendered: 4 real indices (incl. the
  // Hang Seng) vs the 3-ETF fallback. The 4-cell set wraps to a 2×2 grid on
  // mobile so long labels ("Hang Seng") aren't truncated; the 3-ETF fallback
  // stays on one row (3 short labels fit even on narrow screens).
  const cellCount = indices.length > 0 ? indices.length : FALLBACK.length;
  const colsClass = cellCount >= 4 ? 'grid-cols-2 sm:grid-cols-4' : 'grid-cols-3';
  // Hairline dividers via a 1px grid gap over a divider-colored backdrop: each
  // cell paints its own card bg, so the backdrop shows through only in the gaps.
  // This renders correct seams for any layout (2×2 or 1×N) without per-cell
  // border math. Matches the t.border/t.hair slate hairline.
  const divider = dark ? 'bg-slate-800' : 'bg-slate-200';

  return (
    <div
      aria-label={tr('home.indices')}
      className={cx(
        'mb-5 grid gap-px overflow-hidden rounded-2xl border',
        colsClass,
        divider,
        t.border,
        t.soft,
      )}
    >
      {indices.length > 0
        ? indices.map(ix => {
            const hasChg = !!ix.prev_close && ix.prev_close > 0;
            return (
              <Cell
                key={ix.symbol}
                label={ix.name || ix.symbol}
                pct={hasChg ? ((ix.price - ix.prev_close!) / ix.prev_close!) * 100 : null}
                sub={ix.price.toLocaleString('en-US', {
                  minimumFractionDigits: 2,
                  maximumFractionDigits: 2,
                })}
                title={`${ix.symbol} · via ${ix.source}`}
              />
            );
          })
        : FALLBACK.map(idx => {
            const q = quotes.get(idx.symbol);
            const hasChg = !!q && !!q.prev_close && q.prev_close > 0;
            return (
              <Cell
                key={idx.symbol}
                label={idx.label}
                pct={hasChg ? ((q!.price - q!.prev_close!) / q!.prev_close!) * 100 : null}
                sub={q ? `${idx.symbol} ${q.price.toFixed(2)}` : idx.symbol}
                title={q ? `ETF proxy · via ${q.source}` : undefined}
              />
            );
          })}
    </div>
  );
}
