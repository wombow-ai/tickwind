'use client';

import {useId} from 'react';
import {Info, LineChart, TrendingDown, TrendingUp} from 'lucide-react';
import type {Backtest} from '@/lib/api';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {ShareCardButton} from '@/components/ShareCardButton';

/** Signed percentage, e.g. "+26.71%" / "−4.20%" (uses a real minus glyph). */
function fmtPct(v: number): string {
  const s = v >= 0 ? '+' : '−';
  return `${s}${Math.abs(v).toFixed(2)}%`;
}

/**
 * Follow-trade SIMULATION section for a member page: a historical replay of
 * equal-weight virtual buys at each disclosed purchase's disclosure-date close
 * vs. an equal-dollar SPY buy-and-hold baseline. Loud about being a simulation,
 * not realized returns and not advice; methodology is shown inline.
 *
 * `bt` is fetched server-side (see the member page); this component only renders
 * + colors it. When `bt.insufficient` it shows a "not enough data" notice.
 */
export function FollowTradeSim({bt, memberName}: {bt: Backtest; memberName: string}) {
  const tr = useT();
  const {lang} = useLang();

  if (bt.insufficient) {
    return (
      <section className="mb-8">
        <SectionHeader tr={tr} />
        <div className="rounded-2xl border border-slate-200 px-6 py-8 text-center dark:border-slate-800">
          <p className="text-[14px] font-semibold text-slate-900 dark:text-slate-100">
            {tr('sim.insufficient')}
          </p>
          <p className="mx-auto mt-1 max-w-md text-[12.5px] text-slate-500 dark:text-slate-400">
            {tr('sim.insufficientSub')}
          </p>
        </div>
      </section>
    );
  }

  const beat = bt.member_return_pct >= bt.spy_return_pct;
  const months = Math.max(1, Math.round(bt.window_days / 30));
  const memberPct = fmtPct(bt.member_return_pct);
  const spyPct = fmtPct(bt.spy_return_pct);

  // Share card: a 跟单 result card for 小红书 / 微信. Chrome + copy follow the UI
  // language (ShareCardButton injects `lang`); the disclaimer rides on subtitle.
  const tone = (beat ? 'up' : 'down') as 'up' | 'down';
  const shareCard =
    lang === 'en'
      ? {
          eyebrow: 'Congress trades · follow-along sim',
          title: `Following ${memberName}: ${memberPct}`,
          subtitle: `vs SPY ${spyPct} · last ${months} mo · simulated backtest, not advice`,
          stat: memberPct,
          tone,
        }
      : {
          eyebrow: '国会交易 · 跟单模拟',
          title: `跟着 ${memberName} 买 ${memberPct}`,
          subtitle: `vs 标普 SPY ${spyPct} · 近 ${months} 个月 · 模拟复盘非投资建议`,
          stat: memberPct,
          tone,
        };

  return (
    <section className="mb-8">
      <SectionHeader tr={tr} />

      <div className="rounded-2xl border border-slate-200 p-4 dark:border-slate-800 sm:p-5">
        {/* Headline stat: follow vs SPY, colored by who won. */}
        <div className="flex flex-wrap items-end justify-between gap-x-6 gap-y-3">
          <div>
            <div className="flex items-baseline gap-2">
              <span
                className={`text-[34px] font-extrabold leading-none tracking-tight tabular-nums ${
                  beat
                    ? 'text-emerald-600 dark:text-emerald-400'
                    : 'text-rose-600 dark:text-rose-400'
                }`}
              >
                {memberPct}
              </span>
              <span className="flex items-center gap-1 text-[13px] font-semibold text-slate-400 dark:text-slate-500">
                {beat ? <TrendingUp size={15} /> : <TrendingDown size={15} />}
                {tr('sim.vs')} {spyPct}
              </span>
            </div>
            <p className="mt-1.5 text-[12px] font-medium uppercase tracking-wide text-slate-400 dark:text-slate-500">
              {tr('sim.statSuffix')}
            </p>
          </div>
          {/* propagation organ: save a branded follow-result card */}
          <ShareCardButton card={shareCard} />
        </div>

        {/* Net-value comparison curve (member vs SPY), both indexed to 0%. */}
        <DualLineChart curve={bt.curve ?? []} beat={beat} tr={tr} />

        {/* Window + coverage. */}
        <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-[12px] text-slate-500 dark:text-slate-400">
          <span>
            {tr('sim.windowNote')
              .replace('{start}', bt.window_start)
              .replace('{months}', String(months))}
          </span>
          <span className="text-slate-300 dark:text-slate-600">·</span>
          <span>
            {tr('sim.coverageBody')
              .replace('{used}', String(bt.trades_used))
              .replace('{skipped}', String(bt.trades_skipped))}
          </span>
        </div>

        {/* Methodology (always visible — transparency requirement). */}
        <div className="mt-4 rounded-xl bg-slate-50 p-3 text-[12px] leading-relaxed text-slate-500 dark:bg-slate-900 dark:text-slate-400">
          <p className="mb-1 flex items-center gap-1.5 font-semibold text-slate-600 dark:text-slate-300">
            <Info size={13} />
            {tr('sim.method')}
          </p>
          <p>{tr('sim.methodBody')}</p>
        </div>

        {/* Loud disclaimer: simulation, not realized returns, not advice. */}
        <p className="mt-3 rounded-xl border border-amber-300/60 bg-amber-50 px-3 py-2 text-[12px] font-semibold text-amber-700 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300">
          {tr('sim.notReal')}
        </p>
      </div>
    </section>
  );
}

/** Section title + one-line description (shared by the empty + populated states). */
function SectionHeader({tr}: {tr: (k: string) => string}) {
  return (
    <header className="mb-3">
      <h2 className="flex items-center gap-2 text-[15px] font-bold text-slate-900 dark:text-slate-100">
        <LineChart size={16} className="text-teal-600 dark:text-teal-300" />
        {tr('sim.title')}
      </h2>
      <p className="mt-1 text-[12.5px] text-slate-500 dark:text-slate-400">{tr('sim.subtitle')}</p>
    </header>
  );
}

/**
 * A two-line SVG net-value chart: the follow-trade curve (teal when ahead, rose
 * when behind) over the SPY baseline (slate). Both share one min/max scale so
 * the gap is visually honest. Renders nothing useful with <2 points.
 */
function DualLineChart({
  curve,
  beat,
  tr,
}: {
  curve: readonly {date: string; member_pct: number; spy_pct: number}[];
  beat: boolean;
  tr: (k: string) => string;
}) {
  const dark = useDark();
  const gid = useId();
  const w = 560;
  const h = 150;
  const pad = 6;

  if (curve.length < 2) {
    return <div className="mt-3 h-[150px]" aria-hidden />;
  }

  const all = curve.flatMap((p) => [p.member_pct, p.spy_pct]);
  const mx = Math.max(...all, 0);
  const mn = Math.min(...all, 0);
  const span = mx - mn || 1;
  const n = curve.length;
  const step = (w - pad * 2) / (n - 1);
  const y = (v: number) => pad + (1 - (v - mn) / span) * (h - pad * 2);
  const x = (i: number) => pad + i * step;

  const path = (key: 'member_pct' | 'spy_pct') =>
    curve.map((p, i) => `${i === 0 ? 'M' : 'L'} ${x(i).toFixed(1)} ${y(p[key]).toFixed(1)}`).join(' ');

  const memberCol = beat ? '#0d9488' : '#e11d48';
  const spyCol = dark ? '#64748b' : '#94a3b8';
  const zeroY = y(0);

  return (
    <figure className="mt-4">
      <svg
        viewBox={`0 0 ${w} ${h}`}
        className="h-auto w-full"
        role="img"
        aria-label={`${tr('sim.member')} vs ${tr('sim.spy')}`}
      >
        <defs>
          <linearGradient id={gid} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" stopColor={memberCol} stopOpacity={dark ? '.22' : '.16'} />
            <stop offset="1" stopColor={memberCol} stopOpacity="0" />
          </linearGradient>
        </defs>
        {/* 0% reference line. */}
        <line
          x1={pad}
          x2={w - pad}
          y1={zeroY}
          y2={zeroY}
          stroke={dark ? '#334155' : '#e2e8f0'}
          strokeWidth="1"
          strokeDasharray="3 3"
        />
        {/* Member area + line. */}
        <path d={`${path('member_pct')} L ${x(n - 1).toFixed(1)} ${h - pad} L ${pad} ${h - pad} Z`} fill={`url(#${gid})`} />
        <path d={path('spy_pct')} fill="none" stroke={spyCol} strokeWidth="1.75" strokeDasharray="4 3" strokeLinejoin="round" />
        <path d={path('member_pct')} fill="none" stroke={memberCol} strokeWidth="2.25" strokeLinejoin="round" strokeLinecap="round" />
      </svg>
      <figcaption className="mt-2 flex items-center gap-4 text-[11.5px] text-slate-500 dark:text-slate-400">
        <span className="flex items-center gap-1.5">
          <span className="inline-block h-[2px] w-4 rounded" style={{backgroundColor: memberCol}} />
          {tr('sim.member')}
        </span>
        <span className="flex items-center gap-1.5">
          <span
            className="inline-block h-[2px] w-4 rounded"
            style={{backgroundColor: spyCol, backgroundImage: 'none'}}
          />
          {tr('sim.spy')}
        </span>
      </figcaption>
    </figure>
  );
}
