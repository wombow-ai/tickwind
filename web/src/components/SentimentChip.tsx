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
  const history = data.history ?? [];
  const hasTrend = history.length >= 2;
  // The chip expands to reveal the trend line and/or the scored components.
  const canExpand = components.length > 0 || hasTrend;
  const score = Math.round(data.score);

  // Share card: today's market mood for 小红书 / 微信. Chrome + copy follow the UI
  // language (ShareCardButton injects `lang`); tone tilts green/red around neutral.
  const tone = (score >= 55 ? 'up' : score < 45 ? 'down' : undefined) as
    | 'up'
    | 'down'
    | undefined;
  const shareCard =
    lang === 'en'
      ? {
          eyebrow: 'Tickwind Fear & Greed',
          title: `US market sentiment: ${score} · ${data.label}`,
          subtitle: 'Composite of VIX / put-call / short interest · for reference',
          tone,
        }
      : {
          eyebrow: '潮汐恐贪指数',
          title: `今日美股情绪 ${score} · ${data.label_zh || data.label}`,
          subtitle: 'VIX/看跌看涨/做空 等合成 · 仅供参考',
          tone,
        };

  return (
    <div className={cx('mb-5 rounded-2xl border', t.card, t.border, t.soft)}>
      <div className="flex items-center">
        <button
          type="button"
          onClick={() => canExpand && setOpen(o => !o)}
          aria-expanded={canExpand ? open : undefined}
          className={cx(
            'flex flex-1 items-center gap-3 px-4 py-3 text-left',
            canExpand && 'cursor-pointer',
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
        {canExpand && (
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

      {open && canExpand && (
        <div className={cx('tw-fade border-t px-4 py-3', t.hair)}>
          {/* Fear & Greed trend over the recorded daily history (chart #2). */}
          {hasTrend && (
            <div className="mb-3">
              <div className="mb-1.5 flex items-baseline justify-between">
                <p className={cx('text-[11px] font-semibold uppercase tracking-wide', t.faint)}>
                  {tr('sentiment.trend')}
                </p>
                <span className={cx('text-[10.5px] tabular-nums', t.faint)}>
                  {tr('sentiment.trendDays').replace('{n}', String(history.length))}
                </span>
              </div>
              <FearGreedTrend points={history} dark={dark} />
            </div>
          )}
          {components.length > 0 && (
            <>
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
            </>
          )}
          <p className={cx('mt-3 text-[10.5px]', t.faint)}>{tr('sentiment.footer')}</p>
        </div>
      )}
    </div>
  );
}

/**
 * A compact Fear & Greed history line chart (0–100, greed at top). Renders the
 * recorded daily score series; the line is coloured by the latest band (greed
 * green / fear red / neutral sky). A dashed 50 line marks neutral. Stretches to
 * its container width; the stroke stays crisp via vector-effect.
 */
function FearGreedTrend({
  points,
  dark,
}: {
  points: {date: string; score: number}[];
  dark: boolean;
}) {
  const W = 100;
  const H = 34;
  const n = points.length;
  const x = (i: number) => (n <= 1 ? W / 2 : (i / (n - 1)) * W);
  const y = (s: number) => H - (Math.max(0, Math.min(100, s)) / 100) * H;
  const d = points
    .map((p, i) => `${i === 0 ? 'M' : 'L'}${x(i).toFixed(2)},${y(p.score).toFixed(2)}`)
    .join(' ');
  const last = points[n - 1].score;
  const stroke = last >= 55 ? '#10b981' : last < 45 ? '#ef4444' : '#0ea5e9';
  const scores = points.map(p => p.score);
  const hi = Math.max(...scores);
  const lo = Math.min(...scores);
  return (
    <div>
      <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className="h-12 w-full">
        <line
          x1="0"
          y1={y(50)}
          x2={W}
          y2={y(50)}
          stroke={dark ? '#334155' : '#e2e8f0'}
          strokeWidth="1"
          strokeDasharray="2 2"
          vectorEffect="non-scaling-stroke"
        />
        <path d={d} fill="none" stroke={stroke} strokeWidth="1.6" vectorEffect="non-scaling-stroke" strokeLinejoin="round" strokeLinecap="round" />
      </svg>
      <div className={cx('mt-1 flex justify-between text-[10px] tabular-nums', dark ? 'text-slate-500' : 'text-slate-400')}>
        <span>{points[0].date}</span>
        <span>
          {lo}–{hi}
        </span>
        <span>{points[n - 1].date}</span>
      </div>
    </div>
  );
}
