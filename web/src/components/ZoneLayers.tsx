'use client';

import {Lock, Zap} from 'lucide-react';
import Link from '@/components/LocalLink';
import {type Quote} from '@/lib/api';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';
import {useQuotes} from '@/lib/useQuotes';
import {type Zone, type ZoneTicker, zoneTickers} from '@/lib/zones';

// Renders a zone's curated layers with LIVE prices. The structure (layer names,
// company names, rationales, chokepoint flags) is curated editorial content from
// zones.ts; every price / % change is fetched live from Go via the shared quote
// stream — no number is ever hardcoded (the anti-hallucination contract).

function pctOf(q: Quote | undefined): number | null {
  if (!q || !q.prev_close || q.prev_close <= 0 || !q.price) return null;
  return (q.price / q.prev_close - 1) * 100;
}

export function ZoneLayers({zone, zh}: {zone: Zone; zh: boolean}) {
  const dark = useDark();
  const t = tok(dark);
  const quotes = useQuotes(zoneTickers(zone));

  return (
    <div className="space-y-7">
      {zone.layers.map((layer, i) => (
        <section key={layer.key} className="tw-fade">
          <div className="mb-1.5 flex items-center gap-2">
            <span className={cx('flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-[11px] font-bold', dark ? 'bg-slate-800 text-slate-300' : 'bg-slate-100 text-slate-600')}>
              {i + 1}
            </span>
            <h2 className={cx('text-[16.5px] font-bold tracking-tight', t.text)}>
              {zh ? layer.titleZh : layer.titleEn}
            </h2>
            {layer.chokepoint && <ChokepointBadge zh={zh} dark={dark} />}
          </div>
          <p className={cx('mb-3 text-[13px] leading-relaxed', t.sub)}>
            {zh ? layer.blurbZh : layer.blurbEn}
          </p>
          <div className="grid gap-2 sm:grid-cols-2">
            {layer.tickers.map(tk => (
              <TickerCard key={tk.ticker} tk={tk} quote={quotes.get(tk.ticker)} zh={zh} dark={dark} t={t} />
            ))}
          </div>
          {layer.named && layer.named.length > 0 && (
            <div className={cx('mt-2 rounded-xl border border-dashed px-3 py-2 text-[12px]', t.border, t.faint)}>
              <span className="font-semibold">{zh ? '另:无美股代码 — ' : 'Also (no US ticker): '}</span>
              {layer.named.map((n, j) => (
                <span key={n.company}>
                  {j > 0 && ' · '}
                  <span className={cx('font-medium', t.sub)}>{n.company}</span> — {n.note}
                </span>
              ))}
            </div>
          )}
        </section>
      ))}
    </div>
  );
}

function ChokepointBadge({zh, dark}: {zh: boolean; dark: boolean}) {
  return (
    <span
      title={zh ? '供应链卡脖子环节' : 'Supply-chain chokepoint'}
      className={cx(
        'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10.5px] font-bold uppercase tracking-wide',
        dark ? 'bg-amber-500/15 text-amber-300' : 'bg-amber-100 text-amber-700',
      )}
    >
      <Lock size={10} /> {zh ? '卡脖子' : 'Chokepoint'}
    </span>
  );
}

function TickerCard({
  tk,
  quote,
  zh,
  dark,
  t,
}: {
  tk: ZoneTicker;
  quote: Quote | undefined;
  zh: boolean;
  dark: boolean;
  t: ReturnType<typeof tok>;
}) {
  const pct = pctOf(quote);
  const chg =
    pct == null
      ? 'text-slate-400 dark:text-slate-500'
      : pct >= 0
        ? 'text-emerald-600 dark:text-emerald-400'
        : 'text-rose-500 dark:text-rose-400';
  return (
    <Link
      href={`/stock/${encodeURIComponent(tk.ticker)}`}
      className={cx('group block rounded-xl border p-3 transition', t.card, t.border, dark ? 'hover:border-slate-700' : 'hover:border-slate-300')}
    >
      <div className="flex items-baseline justify-between gap-2">
        <div className="flex items-center gap-1.5 min-w-0">
          <span className={cx('text-[13.5px] font-bold', t.text)}>{tk.ticker}</span>
          {tk.chokepoint && <Zap size={12} className={dark ? 'text-amber-300' : 'text-amber-600'} />}
        </div>
        <div className="text-right">
          <span className={cx('text-[13px] font-semibold tabular-nums', t.text)}>
            {quote?.price ? `$${quote.price.toFixed(2)}` : '—'}
          </span>
          {pct != null && (
            <span className={cx('ml-1.5 text-[11.5px] font-semibold tabular-nums', chg)}>
              {pct >= 0 ? '+' : ''}
              {pct.toFixed(2)}%
            </span>
          )}
        </div>
      </div>
      <div className={cx('truncate text-[11.5px] font-medium', t.sub)}>{tk.company}</div>
      <p className={cx('mt-1 text-[11.5px] leading-snug', t.faint)}>{tk.rationale}</p>
    </Link>
  );
}
