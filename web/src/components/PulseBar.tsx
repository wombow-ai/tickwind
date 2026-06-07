'use client';

import {Activity, Flame, TrendingDown, TrendingUp} from 'lucide-react';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';
import type {Signal} from '@/lib/api';

type Tokens = ReturnType<typeof tok>;

/**
 * Compact "pulse" bar shown on the stock detail page: a Reddit mention-momentum
 * (buzz) chip and a news-sentiment chip, side by side. Each chip renders only
 * when its source has data; the whole bar renders nothing when neither does, so
 * it never shows empty scaffolding.
 */
export function PulseBar({signals}: {signals: Signal[]}) {
  const dark = useDark();
  const t = tok(dark);
  const buzz = signals.find(s => s.kind === 'buzz');
  const sentiment = signals.find(s => s.kind === 'sentiment');
  if (!buzz && !sentiment) return null;

  return (
    <div className="tw-fade mb-6 flex flex-col gap-3 sm:flex-row">
      {buzz && <BuzzChip s={buzz} dark={dark} t={t} />}
      {sentiment && <SentimentChip s={sentiment} dark={dark} t={t} />}
    </div>
  );
}

function BuzzChip({s, dark, t}: {s: Signal; dark: boolean; t: Tokens}) {
  const tr = useT();
  const mentions = s.mentions ?? 0;
  const prev = s.mentions_prev ?? 0;
  const delta = prev > 0 ? ((mentions - prev) / prev) * 100 : 0;
  const up = mentions >= prev;
  const deltaColor = up
    ? dark
      ? 'text-emerald-400'
      : 'text-emerald-600'
    : dark
      ? 'text-rose-400'
      : 'text-rose-500';
  return (
    <div className={cx('flex-1 rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-2 flex items-center gap-2">
        <span
          className="flex items-center justify-center rounded-lg"
          style={{
            width: 30,
            height: 30,
            background: dark ? 'rgba(245,158,11,.14)' : 'rgba(245,158,11,.12)',
          }}
        >
          <Flame size={16} className={dark ? 'text-amber-300' : 'text-amber-500'} />
        </span>
        <span className={cx('text-[12px] font-semibold uppercase tracking-wide', t.sub)}>
          {tr('pulse.buzz')}
        </span>
      </div>
      <div className="flex items-baseline gap-2">
        <span className={cx('text-2xl font-bold tabular-nums', t.text)}>
          {mentions.toLocaleString()}
        </span>
        <span className={cx('text-[12px]', t.faint)}>{tr('pulse.mentions24h')}</span>
      </div>
      <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-[12px]">
        {s.rank ? (
          <span className={cx('font-medium', t.sub)}>
            {tr('pulse.rank').replace('{n}', String(s.rank))}
          </span>
        ) : null}
        {prev > 0 && (
          <span className={cx('inline-flex items-center gap-1 font-semibold', deltaColor)}>
            {up ? <TrendingUp size={13} /> : <TrendingDown size={13} />}
            {Math.abs(delta).toFixed(0)}% {tr('pulse.vs24h')}
          </span>
        )}
        {s.upvotes ? (
          <span className={t.faint}>
            {s.upvotes.toLocaleString()} {tr('pulse.upvotes')}
          </span>
        ) : null}
      </div>
    </div>
  );
}

function SentimentChip({s, dark, t}: {s: Signal; dark: boolean; t: Tokens}) {
  const tr = useT();
  const score = s.score ?? 0;
  const tone = score >= 0.15 ? 'pos' : score <= -0.15 ? 'neg' : 'neu';
  const color =
    tone === 'pos'
      ? dark
        ? 'text-emerald-400'
        : 'text-emerald-600'
      : tone === 'neg'
        ? dark
          ? 'text-rose-400'
          : 'text-rose-500'
        : dark
          ? 'text-slate-300'
          : 'text-slate-600';
  const bg =
    tone === 'pos'
      ? dark
        ? 'rgba(16,185,129,.14)'
        : 'rgba(16,185,129,.12)'
      : tone === 'neg'
        ? dark
          ? 'rgba(244,63,94,.14)'
          : 'rgba(244,63,94,.1)'
        : dark
          ? 'rgba(148,163,184,.16)'
          : 'rgba(148,163,184,.14)';
  const label = (s.label ?? '').replace(/-/g, ' ') || '—';
  return (
    <div className={cx('flex-1 rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-2 flex items-center gap-2">
        <span
          className="flex items-center justify-center rounded-lg"
          style={{width: 30, height: 30, background: bg}}
        >
          <Activity size={16} className={color} />
        </span>
        <span className={cx('text-[12px] font-semibold uppercase tracking-wide', t.sub)}>
          {tr('pulse.sentiment')}
        </span>
      </div>
      <div className="flex items-baseline gap-2">
        <span className={cx('text-2xl font-bold', color)}>{label}</span>
        <span className={cx('text-[13px] font-semibold tabular-nums', color)}>
          {score >= 0 ? '+' : ''}
          {score.toFixed(2)}
        </span>
      </div>
      <div className="mt-1.5 text-[12px]">
        <span className={t.faint}>
          {tr('pulse.across').replace('{n}', (s.sample_size ?? 0).toLocaleString())}
        </span>
      </div>
    </div>
  );
}
