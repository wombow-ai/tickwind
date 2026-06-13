'use client';

import {ChevronDown, Gauge} from 'lucide-react';
import {useEffect, useState} from 'react';
import {getSentiment, type Sentiment} from '@/lib/api';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, timeAgo, tok} from '@/lib/ui';
import {ShareCardButton} from '@/components/ShareCardButton';

/**
 * Maps a 0–100 fear/greed score to a band: fear leans red, greed leans green,
 * the middle is neutral grey. Returns the i18n label key + a theme-aware color
 * for the score text and the gauge fill.
 */
function band(score: number, dark: boolean): {labelKey: string; text: string; fill: string} {
  if (score < 25)
    return {
      labelKey: 'sentiment.extremeFear',
      text: dark ? 'text-rose-300' : 'text-rose-600',
      fill: '#F43F5E',
    };
  if (score < 45)
    return {
      labelKey: 'sentiment.fear',
      text: dark ? 'text-orange-300' : 'text-orange-600',
      fill: '#FB923C',
    };
  if (score < 55)
    return {
      labelKey: 'sentiment.neutral',
      text: dark ? 'text-slate-300' : 'text-slate-500',
      fill: dark ? '#94A3B8' : '#64748B',
    };
  if (score < 75)
    return {
      labelKey: 'sentiment.greed',
      text: dark ? 'text-emerald-300' : 'text-emerald-600',
      fill: '#34D399',
    };
  return {
    labelKey: 'sentiment.extremeGreed',
    text: dark ? 'text-emerald-300' : 'text-emerald-700',
    fill: '#10B981',
  };
}

/**
 * Compact Fear & Greed chip for the home hub (sits beside the indices strip).
 * Shows the composite score + its band label (color-coded: fear red, greed
 * green, neutral grey) on a thin gauge track; click to expand the component
 * breakdown. Self-hides when the index isn't available yet (no fake data),
 * mirroring the other self-hiding home modules.
 */
export function SentimentChip() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const [data, setData] = useState<Sentiment | null>(null);
  const [hidden, setHidden] = useState(false);
  const [open, setOpen] = useState(false);

  useEffect(() => {
    const c = new AbortController();
    getSentiment(c.signal).then(
      r => setData(r),
      () => setHidden(true),
    );
    return () => c.abort();
  }, []);

  if (hidden) return null;
  if (!data) {
    return <div className={cx('mb-5 h-[60px] rounded-2xl', t.skel)} />;
  }

  const b = band(data.score, dark);
  // Single-language values default to English; Chinese label follows useLang.
  const label = lang === 'zh' ? data.label_zh || data.label : data.label || tr(b.labelKey);
  const pos = Math.max(0, Math.min(100, data.score));
  const components = data.components ?? [];
  const score = Math.round(data.score);

  // Share card: today's market mood for 小红书 / 微信. Chinese label always (the
  // card is a Chinese propagation asset); tone tilts green/red around neutral.
  const shareCard = {
    eyebrow: '潮汐恐贪指数',
    title: `今日美股情绪 ${score} · ${data.label_zh || data.label}`,
    subtitle: 'VIX/看跌看涨/做空 等合成 · 仅供参考',
    tone: (score >= 55 ? 'up' : score < 45 ? 'down' : undefined) as 'up' | 'down' | undefined,
  };

  return (
    <div className={cx('mb-5 rounded-2xl border', t.card, t.border, t.soft)}>
      <div className="flex items-center">
        <button
          type="button"
          onClick={() => components.length > 0 && setOpen(o => !o)}
          aria-expanded={components.length > 0 ? open : undefined}
          className={cx(
            'flex flex-1 items-center gap-3 px-4 py-3 text-left',
            components.length > 0 && 'cursor-pointer',
          )}
        >
        <Gauge size={18} className={dark ? 'text-teal-300' : 'text-teal-600'} />
        <div className="min-w-0 flex-1">
          <div className="flex items-baseline gap-2">
            <span className={cx('text-[12px] font-semibold', t.sub)}>
              {tr('sentiment.title')}
            </span>
            <span className={cx('text-[18px] font-bold tabular-nums', b.text)}>
              {Math.round(data.score)}
            </span>
            <span className={cx('text-[12.5px] font-semibold', b.text)}>{label}</span>
            {data.updated_at && (
              <span className={cx('ml-auto hidden text-[10.5px] sm:inline', t.faint)}>
                {tr('sentiment.updated').replace('{t}', timeAgo(data.updated_at))}
              </span>
            )}
          </div>
          {/* thin gauge track with the score marker */}
          <div className={cx('relative mt-2 h-1.5 w-full rounded-full', dark ? 'bg-slate-800' : 'bg-slate-100')}>
            <div className="h-full rounded-full" style={{width: `${pos}%`, background: b.fill}} />
            <span
              className="absolute top-1/2 h-3 w-3 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white shadow"
              style={{left: `${pos}%`, background: b.fill}}
            />
          </div>
        </div>
        {components.length > 0 && (
          <ChevronDown
            size={16}
            className={cx('shrink-0 transition-transform', t.faint, open && 'rotate-180')}
          />
        )}
        </button>
        {/* propagation organ: save today's mood as a branded card */}
        <div className="shrink-0 pr-3">
          <ShareCardButton card={shareCard} />
        </div>
      </div>

      {open && components.length > 0 && (
        <div className={cx('tw-fade border-t px-4 py-3', t.hair)}>
          <p className={cx('mb-2 text-[11px] font-semibold uppercase tracking-wide', t.faint)}>
            {tr('sentiment.components')}
          </p>
          <div className="space-y-2">
            {components.map(comp => {
              const cb = band(comp.score, dark);
              const cp = Math.max(0, Math.min(100, comp.score));
              return (
                <div key={comp.name} className="flex items-center gap-2.5">
                  <span className={cx('w-24 shrink-0 truncate text-[12px] font-medium', t.text)} title={comp.note || comp.name}>
                    {comp.name}
                  </span>
                  <div className={cx('h-1.5 flex-1 overflow-hidden rounded-full', dark ? 'bg-slate-800' : 'bg-slate-100')}>
                    <div className="h-full rounded-full" style={{width: `${cp}%`, background: cb.fill}} />
                  </div>
                  <span className={cx('w-8 shrink-0 text-right text-[12px] font-semibold tabular-nums', cb.text)}>
                    {Math.round(comp.score)}
                  </span>
                </div>
              );
            })}
          </div>
          <p className={cx('mt-3 text-[10.5px]', t.faint)}>{tr('sentiment.footer')}</p>
        </div>
      )}
    </div>
  );
}
