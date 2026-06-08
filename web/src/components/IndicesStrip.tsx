'use client';

import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';
import {useQuotes} from '@/lib/useQuotes';

// US major indices shown as their liquid ETF proxies. The free price pipeline
// (Alpaca IEX) serves ETFs but NOT index symbols (^GSPC/^DJI/^IXIC), so we show
// the proxy's % change — which tracks the index move closely — as the headline,
// and attribute the ETF + its price honestly on a sub-line (so e.g. "SPY 745.16"
// is never misread as the S&P 500 level). QQQ tracks the Nasdaq-100, labelled so.
const INDICES = [
  {symbol: 'SPY', label: 'S&P 500'},
  {symbol: 'DIA', label: 'Dow Jones'},
  {symbol: 'QQQ', label: 'Nasdaq 100'},
] as const;

const SYMBOLS = INDICES.map(i => i.symbol);

export function IndicesStrip() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const quotes = useQuotes(SYMBOLS);

  return (
    <div
      aria-label={tr('home.indices')}
      className={cx(
        'mb-5 grid grid-cols-3 overflow-hidden rounded-2xl border',
        t.card,
        t.border,
        t.soft,
      )}
    >
      {INDICES.map((idx, i) => {
        const q = quotes.get(idx.symbol);
        const hasChg = !!q && !!q.prev_close && q.prev_close > 0;
        const pct = hasChg ? ((q!.price - q!.prev_close!) / q!.prev_close!) * 100 : 0;
        const up = pct >= 0;
        const col = up
          ? dark
            ? 'text-emerald-400'
            : 'text-emerald-600'
          : dark
            ? 'text-rose-400'
            : 'text-rose-500';
        return (
          <div
            key={idx.symbol}
            className={cx('px-3 py-2.5 sm:px-4', i > 0 && cx('border-l', t.border))}
          >
            <div className={cx('truncate text-[12px] font-semibold', t.text)}>{idx.label}</div>
            {hasChg ? (
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
            <div className={cx('text-[10.5px] tabular-nums', t.faint)}>
              {q ? `${idx.symbol} ${q.price.toFixed(2)}` : idx.symbol}
            </div>
          </div>
        );
      })}
    </div>
  );
}
