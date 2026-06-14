'use client';

import {useEffect, useState} from 'react';
import {getMacro, type Macro} from '@/lib/api';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

/** Refresh cadence — the Treasury curve updates once per business day, so hourly
 * polling is plenty (the backend cache itself refreshes ~12h). */
const POLL_MS = 60 * 60_000;

/** Finds a tenor's rate in the curve, or null when the Treasury didn't publish
 * it that day (never fabricated). */
function rateOf(yields: Macro['yields'], tenor: string): number | null {
  const y = yields.find(v => v.tenor === tenor);
  return y ? y.rate : null;
}

/**
 * Compact one-line U.S. Treasury yield-curve strip for the home hub (sits with
 * the indices / sentiment strips). Shows the key 2Y / 10Y par yields and the
 * 2s10s spread (10Y − 2Y) with an "倒挂" (inverted) chip in rose when negative —
 * the classic recession-watch signal — vs "正常" (normal) in emerald otherwise,
 * a date stamp, and a "美国国债收益率 · 美国财政部" source label. Server-driven
 * (`GET /v1/macro`); real Treasury data only — self-hides when unavailable.
 */
export function MacroStrip() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const [data, setData] = useState<Macro | null>(null);
  const [hidden, setHidden] = useState(false);

  useEffect(() => {
    const c = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const load = () => {
      getMacro(c.signal).then(
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
  // Self-hide until the server cache holds a real curve (no fake data).
  const yields = data.yields ?? [];
  if (!data.available || yields.length === 0) return null;

  const two = rateOf(yields, '2Y');
  const ten = rateOf(yields, '10Y');
  const spread = data.spread_2s10s; // null when a leg is absent
  const hasSpread = spread !== null && spread !== undefined;
  const inverted = data.inverted;

  // Inverted = rose (recession watch), normal = emerald.
  const chipCol = inverted
    ? dark
      ? 'bg-rose-500/15 text-rose-300'
      : 'bg-rose-50 text-rose-600'
    : dark
      ? 'bg-emerald-500/15 text-emerald-300'
      : 'bg-emerald-50 text-emerald-700';
  const spreadCol = inverted
    ? dark
      ? 'text-rose-300'
      : 'text-rose-600'
    : dark
      ? 'text-emerald-300'
      : 'text-emerald-600';

  // Single-language data value (the official source name) defaults to English;
  // the Chinese chrome label leads on the zh UI, per the project i18n rule.
  const sourceName = lang === 'zh' ? data.source_zh || data.source : data.source;
  const sourceLabel = `${tr('macro.title')} · ${sourceName}`;

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
      {/* key tenors: 2Y / 10Y */}
      <Tenor label="2Y" rate={two} t={t} />
      <Tenor label="10Y" rate={ten} t={t} />

      {/* 2s10s spread + inverted/normal chip */}
      {hasSpread && (
        <div className="flex items-center gap-2">
          <span className={cx('text-[11px] font-medium', t.faint)}>{tr('macro.spread')}</span>
          <span className={cx('text-[14px] font-bold tabular-nums', spreadCol)}>
            {spread! >= 0 ? '+' : '−'}
            {Math.abs(spread!).toFixed(2)}%
          </span>
          <span
            title={inverted ? tr('macro.invertedHint') : tr('macro.normalHint')}
            className={cx(
              'rounded-full px-2 py-0.5 text-[11px] font-semibold',
              chipCol,
            )}
          >
            {inverted ? tr('macro.inverted') : tr('macro.normal')}
          </span>
        </div>
      )}

      {/* date stamp + source label (pushed right on wide rows) */}
      <div className={cx('ml-auto flex items-center gap-2 text-[10.5px]', t.faint)}>
        {data.as_of && (
          <span className="tabular-nums">{tr('macro.updated').replace('{t}', data.as_of)}</span>
        )}
        <span className="hidden sm:inline">·</span>
        <span className="hidden truncate sm:inline">{sourceLabel}</span>
      </div>
    </div>
  );
}

/** One tenor cell: the maturity label + its par yield, or "—" when the Treasury
 * didn't publish that tenor for the day (never invented). */
function Tenor({
  label,
  rate,
  t,
}: {
  label: string;
  rate: number | null;
  t: ReturnType<typeof tok>;
}) {
  return (
    <div className="flex items-baseline gap-1.5">
      <span className={cx('text-[11px] font-semibold', t.sub)}>{label}</span>
      {rate !== null ? (
        <span className={cx('text-[14px] font-bold tabular-nums', t.text)}>{rate.toFixed(2)}%</span>
      ) : (
        <span className={cx('text-[14px] font-semibold', t.faint)}>—</span>
      )}
    </div>
  );
}
