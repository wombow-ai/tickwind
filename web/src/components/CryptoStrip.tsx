'use client';

import {useEffect, useState} from 'react';
import {getCrypto, type Crypto, type CryptoPrice} from '@/lib/api';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

/** Refresh cadence — the crypto F&G index updates ~daily and the backend cache
 * refreshes hourly, so hourly client polling is plenty. */
const POLL_MS = 60 * 60_000;

/**
 * Maps a 0–100 crypto fear/greed score to a band: fear leans rose, greed leans
 * emerald, the middle is neutral slate. Returns the i18n label key + a
 * theme-aware text color and the gauge fill (mirroring the stock SentimentChip's
 * band so the two read consistently).
 */
function band(score: number, dark: boolean): {labelKey: string; text: string; fill: string} {
  if (score < 25)
    return {
      labelKey: 'crypto.extremeFear',
      text: dark ? 'text-rose-300' : 'text-rose-600',
      fill: '#F43F5E',
    };
  if (score < 45)
    return {
      labelKey: 'crypto.fear',
      text: dark ? 'text-orange-300' : 'text-orange-600',
      fill: '#FB923C',
    };
  if (score < 55)
    return {
      labelKey: 'crypto.neutral',
      text: dark ? 'text-slate-300' : 'text-slate-500',
      fill: dark ? '#94A3B8' : '#64748B',
    };
  if (score < 75)
    return {
      labelKey: 'crypto.greed',
      text: dark ? 'text-emerald-300' : 'text-emerald-600',
      fill: '#34D399',
    };
  return {
    labelKey: 'crypto.extremeGreed',
    text: dark ? 'text-emerald-300' : 'text-emerald-700',
    fill: '#10B981',
  };
}

/** Compact USD price: no decimals at $1k+ (BTC/ETH range), two at sub-$1. */
function fmtUSD(v: number): string {
  return v >= 1000
    ? `$${Math.round(v).toLocaleString('en-US')}`
    : `$${v.toLocaleString('en-US', {maximumFractionDigits: 2})}`;
}

/**
 * Compact one-line crypto market-mood strip for the home hub (sits under the
 * Treasury MacroStrip). Shows the crypto Fear & Greed score on a thin colored
 * gauge band (fear rose / greed emerald / neutral slate — mirroring the stock
 * SentimentChip) with its classification label, plus BTC/ETH spot price + 24h%
 * chips when present, and a "加密市场情绪 · alternative.me" source label. This is
 * crypto context for the crypto-linked equities COIN/MSTR/RIOT/MARA the audience
 * follows. Server-driven (`GET /v1/crypto`); real data only — self-hides when
 * unavailable, and BTC/ETH chips simply don't render when the price source was
 * unavailable (the score alone is the feature; prices are never fabricated).
 */
export function CryptoStrip() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const [data, setData] = useState<Crypto | null>(null);
  const [hidden, setHidden] = useState(false);

  useEffect(() => {
    const c = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const load = () => {
      getCrypto(c.signal).then(
        r => {
          setData(r);
          timer = setTimeout(load, POLL_MS);
        },
        () => setHidden(true),
      );
    };
    load();
    return () => {
      c.abort();
      if (timer !== undefined) clearTimeout(timer);
    };
  }, []);

  if (hidden) return null;
  if (!data) {
    return <div className={cx('mb-5 h-[52px] rounded-2xl', t.skel)} />;
  }
  // Self-hide until the server cache holds a real index (no fake data).
  if (!data.available) return null;

  const b = band(data.score, dark);
  // Single-language value (the classification) defaults to English; the Chinese
  // label leads on the zh UI, per the project i18n rule.
  const label = lang === 'zh' ? data.label_zh || data.label : data.label || tr(b.labelKey);
  const pos = Math.max(0, Math.min(100, data.score));
  const sourceLabel = `${tr('crypto.title')} · ${data.source}`;

  return (
    <div
      aria-label={sourceLabel}
      className={cx(
        'mb-5 flex flex-wrap items-center gap-x-4 gap-y-2 rounded-2xl border px-4 py-3',
        t.card,
        t.border,
        t.soft,
      )}
    >
      {/* crypto F&G score + band label on a thin colored gauge */}
      <div className="flex min-w-0 items-center gap-2.5">
        <span className={cx('text-[11px] font-semibold', t.sub)}>{tr('crypto.title')}</span>
        <span className={cx('text-[18px] font-bold tabular-nums', b.text)}>{data.score}</span>
        <span className={cx('text-[12.5px] font-semibold', b.text)}>{label}</span>
        <div
          className={cx('relative h-1.5 w-20 shrink-0 rounded-full', dark ? 'bg-slate-800' : 'bg-slate-100')}
        >
          <div className="h-full rounded-full" style={{width: `${pos}%`, background: b.fill}} />
          <span
            className="absolute top-1/2 h-2.5 w-2.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white shadow"
            style={{left: `${pos}%`, background: b.fill}}
          />
        </div>
      </div>

      {/* best-effort BTC / ETH price + 24h% chips (omitted when absent) */}
      <CoinChip sym="BTC" p={data.btc} t={t} dark={dark} />
      <CoinChip sym="ETH" p={data.eth} t={t} dark={dark} />

      {/* date stamp + source label (pushed right on wide rows) */}
      <div className={cx('ml-auto flex items-center gap-2 text-[10.5px]', t.faint)}>
        {data.as_of && (
          <span className="tabular-nums">{tr('crypto.updated').replace('{t}', data.as_of)}</span>
        )}
        <span className="hidden sm:inline">·</span>
        <span className="hidden truncate sm:inline">{sourceLabel}</span>
      </div>
    </div>
  );
}

/** One coin chip: symbol + spot price + signed 24h% (green up / rose down).
 * Renders nothing when the price was unavailable (never a fabricated 0). */
function CoinChip({
  sym,
  p,
  t,
  dark,
}: {
  sym: string;
  p: CryptoPrice | null;
  t: ReturnType<typeof tok>;
  dark: boolean;
}) {
  if (!p) return null;
  const up = p.change_24h >= 0;
  const chgCol = up
    ? dark
      ? 'text-emerald-300'
      : 'text-emerald-600'
    : dark
      ? 'text-rose-300'
      : 'text-rose-600';
  return (
    <div className="flex items-baseline gap-1.5">
      <span className={cx('text-[11px] font-semibold', t.sub)}>{sym}</span>
      <span className={cx('text-[13px] font-bold tabular-nums', t.text)}>{fmtUSD(p.price)}</span>
      <span className={cx('text-[11.5px] font-semibold tabular-nums', chgCol)}>
        {up ? '+' : '−'}
        {Math.abs(p.change_24h).toFixed(1)}%
      </span>
    </div>
  );
}
