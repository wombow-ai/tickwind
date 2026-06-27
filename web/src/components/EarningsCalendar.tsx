'use client';

import {CalendarClock} from 'lucide-react';
import Link from '@/components/LocalLink';
import {useEffect, useMemo, useState} from 'react';
import {getEarnings, type Earning} from '@/lib/api';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';
import {EmptyState, ErrorState, FeedSkeleton} from '@/components/ui/states';

type Status = 'loading' | 'ready' | 'error';
type Tokens = ReturnType<typeof tok>;

/**
 * A compact, NEUTRAL "typical earnings move" chip for one calendar row: the stock's Go-computed
 * historical ±avg-magnitude move around past earnings, with the up-rate + sample count in the
 * tooltip. The data rides in the calendar payload (the backend joins a cached aggregate for tracked
 * tickers — no per-row fetch), so it renders straight from the row. It is a disclosed HISTORICAL
 * statistic, never a forecast — and the magnitude carries NO green/red cue (direction isn't implied).
 * Renders nothing for rows without a reaction (untracked / too little history → insufficient).
 */
function ReactionBadge({reaction, t, tr}: {reaction: Earning['reaction']; t: Tokens; tr: (k: string) => string}) {
  if (!reaction || reaction.samples <= 0) return null;
  const tip = tr('er.calTip')
    .replace('{move}', `±${reaction.avg_abs_move.toFixed(1)}%`)
    .replace('{up}', `${Math.round(reaction.up_rate * 100)}%`)
    .replace('{n}', String(reaction.samples));
  return (
    <span
      title={tip}
      aria-label={tip}
      className={cx('rounded-md px-1.5 py-0.5 text-[10.5px] font-semibold tabular-nums', t.chip, t.chipText)}
    >
      ±{reaction.avg_abs_move.toFixed(1)}%
    </span>
  );
}

// Finnhub reporting-time codes → i18n keys (reused from the per-stock chip).
const HOUR_KEY: Record<string, string> = {
  bmo: 'earnings.bmo',
  amc: 'earnings.amc',
  dmh: 'earnings.dmh',
};

/** Groups earnings by their YYYY-MM-DD date, ascending; tickers A→Z within a day. */
function groupByDay(earnings: Earning[]): {day: string; rows: Earning[]}[] {
  const byDay = new Map<string, Earning[]>();
  for (const e of earnings) {
    const day = (e.date ?? '').slice(0, 10);
    if (!day) continue;
    (byDay.get(day) ?? byDay.set(day, []).get(day)!).push(e);
  }
  return [...byDay.entries()]
    .sort((a, b) => a[0].localeCompare(b[0]))
    .map(([day, rows]) => ({
      day,
      rows: rows.sort((a, b) => a.ticker.localeCompare(b.ticker)),
    }));
}

/**
 * Market-wide earnings calendar: upcoming company reports (Finnhub) grouped by
 * day, each row linking to the stock page with its pre/after-market slot and
 * consensus EPS. Public page; data from GET /v1/earnings (today .. +30d).
 */
export function EarningsCalendar() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const [status, setStatus] = useState<Status>('loading');
  const [earnings, setEarnings] = useState<Earning[]>([]);
  const [reload, setReload] = useState(0);
  // "Key companies" = rows the backend enriched with an earnings-reaction aggregate
  // (the tracked universe — notable, analyst-followed names), cutting the firehose of
  // ~1,000 micro-cap rows. Default on; "All" reveals the full market-wide calendar.
  const [keyOnly, setKeyOnly] = useState(true);

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    getEarnings(undefined, undefined, c.signal).then(
      r => {
        const today = new Date();
        today.setHours(0, 0, 0, 0);
        const upcoming = (r.earnings ?? []).filter(e => {
          const ts = Date.parse(e.date);
          return !Number.isNaN(ts) && ts >= today.getTime();
        });
        setEarnings(upcoming);
        setStatus('ready');
      },
      () => setStatus('error'),
    );
    return () => c.abort();
  }, [reload]);

  const isKey = (e: Earning) => (e.reaction?.samples ?? 0) > 0;
  const keyCount = useMemo(() => earnings.filter(isKey).length, [earnings]);
  const hasKey = keyCount > 0;
  const groups = useMemo(
    () => groupByDay(keyOnly ? earnings.filter(isKey) : earnings),
    [earnings, keyOnly],
  );

  const locale = lang === 'zh' ? 'zh-CN' : 'en-US';
  const fmtDay = (day: string) =>
    new Date(day + 'T00:00:00').toLocaleDateString(locale, {
      weekday: 'short',
      month: 'short',
      day: 'numeric',
    });

  return (
    <div className="mx-auto max-w-3xl">
      <header className="mb-5">
        <h1 className={cx('flex items-center gap-2 text-[22px] font-bold tracking-tight', t.text)}>
          <CalendarClock size={20} className={dark ? 'text-teal-300' : 'text-teal-600'} />
          {tr('earnings.calTitle')}
        </h1>
        <p className={cx('mt-1 text-[13px]', t.sub)}>{tr('earnings.calSubtitle')}</p>
        {status === 'ready' && (
          <div className={cx('mt-3 inline-flex gap-1 rounded-lg border p-0.5', t.card, t.border)}>
            {([true, false] as const).map(v => {
              const on = v === keyOnly;
              return (
                <button
                  key={String(v)}
                  type="button"
                  onClick={() => setKeyOnly(v)}
                  aria-pressed={on}
                  className={cx(
                    'rounded-md px-2.5 py-1 text-[12px] font-semibold transition',
                    on ? cx(t.chip, t.chipText) : t.faint,
                  )}
                >
                  {tr(v ? 'earnings.calKeyCompanies' : 'earnings.calAll')}
                  <span className="ml-1 opacity-60 tabular-nums">({v ? keyCount : earnings.length})</span>
                </button>
              );
            })}
          </div>
        )}
      </header>

      {status === 'loading' && <FeedSkeleton />}
      {status === 'error' && <ErrorState onRetry={() => setReload(n => n + 1)} />}
      {status === 'ready' && groups.length === 0 && (
        keyOnly && hasKey === false && earnings.length > 0 ? (
          <EmptyState label={tr('earnings.calKeyEmpty')} sub={tr('earnings.calKeyEmptySub')} />
        ) : (
          <EmptyState label={tr('earnings.calEmpty')} sub={tr('earnings.calEmptySub')} />
        )
      )}

      {status === 'ready' && groups.length > 0 && (
        <div className="space-y-5">
          {groups.map(({day, rows}) => (
            <section key={day}>
              <h2 className={cx('mb-1.5 text-[12.5px] font-bold uppercase tracking-wide', t.faint)}>
                {fmtDay(day)}
              </h2>
              <div className={cx('overflow-hidden rounded-2xl border', t.card, t.border, t.soft)}>
                {rows.map((e, i) => {
                  const hourLabel = e.hour && HOUR_KEY[e.hour] ? tr(HOUR_KEY[e.hour]) : '';
                  return (
                    <Link
                      key={`${e.ticker}-${i}`}
                      href={`/stock/${encodeURIComponent(e.ticker)}`}
                      className={cx(
                        'flex items-center gap-3 px-4 py-2.5 transition',
                        i > 0 && cx('border-t', t.border),
                        dark ? 'hover:bg-slate-800/40' : 'hover:bg-slate-50',
                      )}
                    >
                      <span className={cx('w-20 shrink-0 text-[14px] font-bold tabular-nums', t.text)}>
                        {e.ticker}
                      </span>
                      {hourLabel && (
                        <span className={cx('rounded-md px-1.5 py-0.5 text-[10.5px] font-semibold', t.chip, t.chipText)}>
                          {hourLabel}
                        </span>
                      )}
                      <span className="ml-auto flex items-center gap-3 text-[12px] tabular-nums">
                        <ReactionBadge reaction={e.reaction} t={t} tr={tr} />
                        {typeof e.eps_estimate === 'number' && (
                          <span className={t.faint}>
                            {tr('earnings.est')} <span className={cx('font-semibold', t.sub)}>${e.eps_estimate.toFixed(2)}</span>
                          </span>
                        )}
                        {typeof e.eps_actual === 'number' && (
                          <span className={t.faint}>
                            {tr('earnings.act')}{' '}
                            <span
                              className={cx(
                                'font-bold',
                                typeof e.eps_estimate === 'number'
                                  ? e.eps_actual >= e.eps_estimate
                                    ? dark ? 'text-emerald-400' : 'text-emerald-600'
                                    : dark ? 'text-rose-400' : 'text-rose-500'
                                  : t.text,
                              )}
                            >
                              ${e.eps_actual.toFixed(2)}
                            </span>
                          </span>
                        )}
                      </span>
                    </Link>
                  );
                })}
              </div>
            </section>
          ))}
        </div>
      )}
    </div>
  );
}
