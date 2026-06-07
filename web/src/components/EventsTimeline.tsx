'use client';

import {CalendarClock, Globe, Landmark} from 'lucide-react';
import {useCallback, useEffect, useMemo, useState} from 'react';
import {getEvents, type EventItem} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';
import {EmptyState, ErrorState, FeedSkeleton} from '@/components/ui/states';

type Tokens = ReturnType<typeof tok>;
type Status = 'loading' | 'ready' | 'error';

const MONTHS = [
  'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec',
];

function relative(d: Date): string {
  const days = Math.round((d.getTime() - Date.now()) / 86_400_000);
  if (days === 0) return 'Today';
  if (days === 1) return 'Tomorrow';
  if (days === -1) return 'Yesterday';
  if (days > 1) return `in ${days} days`;
  return `${-days} days ago`;
}

/** Bucket key for the month selector, e.g. "2026-5". */
function monthKey(iso: string): string {
  const d = new Date(iso);
  return `${d.getFullYear()}-${d.getMonth()}`;
}

/** Timeline-node size + colour by importance (high = big/amber, med = sky, low = faint). */
function nodeStyle(importance: string, dark: boolean): {size: string; dot: string; ring: string} {
  switch (importance) {
    case 'high':
      return {
        size: 'h-3.5 w-3.5',
        dot: dark ? 'bg-amber-400' : 'bg-amber-500',
        ring: dark ? 'ring-4 ring-amber-400/15' : 'ring-4 ring-amber-500/15',
      };
    case 'med':
      return {size: 'h-2.5 w-2.5', dot: dark ? 'bg-sky-400' : 'bg-sky-500', ring: ''};
    default:
      return {size: 'h-2 w-2', dot: dark ? 'bg-slate-600' : 'bg-slate-300', ring: ''};
  }
}

/**
 * The "Major events timeline / 大事件时间线": upcoming market-moving events — Fed
 * (FOMC) decisions, key US releases (CPI, jobs report) and notable world events —
 * from GET /v1/events. A left node-rail sizes/colours each event by importance; a
 * month selector filters the list. Informational (what to watch), never advice.
 */
export function EventsTimeline() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [status, setStatus] = useState<Status>('loading');
  const [events, setEvents] = useState<EventItem[]>([]);
  const [month, setMonth] = useState<string>('all');

  const load = useCallback(() => {
    setStatus('loading');
    getEvents(60).then(
      r => {
        setEvents(r.events ?? []);
        setStatus('ready');
      },
      () => setStatus('error'),
    );
  }, []);
  useEffect(() => {
    load();
  }, [load]);

  // Distinct months present, in chronological order, for the selector.
  const months = useMemo(() => {
    const seen = new Map<string, Date>();
    for (const e of events) {
      const k = monthKey(e.start);
      if (!seen.has(k)) seen.set(k, new Date(e.start));
    }
    return [...seen.entries()].map(([key, d]) => ({key, label: MONTHS[d.getMonth()]}));
  }, [events]);

  const shown = useMemo(
    () => (month === 'all' ? events : events.filter(e => monthKey(e.start) === month)),
    [events, month],
  );

  return (
    <div className="mx-auto max-w-3xl">
      <header className="mb-4">
        <h1
          className={cx(
            'flex items-center gap-2 text-[22px] font-bold tracking-tight',
            t.text,
          )}
        >
          <CalendarClock size={20} className={dark ? 'text-sky-300' : 'text-sky-600'} />
          {tr('events.title')}
        </h1>
        <p className={cx('mt-1 text-[13.5px]', t.sub)}>{tr('events.subtitle')}</p>
      </header>

      {status === 'ready' && months.length > 1 && (
        <div className="mb-5 flex flex-wrap gap-1.5">
          <MonthChip active={month === 'all'} onClick={() => setMonth('all')} t={t} dark={dark}>
            {tr('events.all')}
          </MonthChip>
          {months.map(m => (
            <MonthChip
              key={m.key}
              active={month === m.key}
              onClick={() => setMonth(m.key)}
              t={t}
              dark={dark}
            >
              {m.label}
            </MonthChip>
          ))}
        </div>
      )}

      {status === 'loading' && <FeedSkeleton />}
      {status === 'error' && <ErrorState onRetry={load} />}
      {status === 'ready' && events.length === 0 && (
        <EmptyState label={tr('events.empty')} sub={tr('events.emptySub')} icon={CalendarClock} />
      )}
      {status === 'ready' && shown.length > 0 && (
        <ol className="tw-fade relative ml-1">
          {/* the connecting rail behind the nodes */}
          <span
            aria-hidden
            className={cx(
              'absolute left-[6px] top-2 bottom-3 w-0.5 rounded',
              dark ? 'bg-slate-700' : 'bg-slate-200',
            )}
          />
          {shown.map((e, i) => (
            <Row key={e.id || i} e={e} dark={dark} t={t} />
          ))}
        </ol>
      )}

      <p className={cx('mt-4 text-center text-[11px]', t.faint)}>{tr('events.footer')}</p>
    </div>
  );
}

function MonthChip({
  active,
  onClick,
  t,
  dark,
  children,
}: {
  active: boolean;
  onClick: () => void;
  t: Tokens;
  dark: boolean;
  children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      aria-pressed={active}
      className={cx(
        'rounded-full border px-3 py-1 text-[12px] font-semibold transition',
        active
          ? dark
            ? 'border-sky-400/40 bg-sky-500/15 text-sky-200'
            : 'border-sky-200 bg-sky-50 text-sky-700'
          : cx(t.border, t.sub, t.ghost),
      )}
    >
      {children}
    </button>
  );
}

function Row({e, dark, t}: {e: EventItem; dark: boolean; t: Tokens}) {
  const tr = useT();
  const d = new Date(e.start);
  const high = e.importance === 'high';
  const Icon = e.category === 'world' ? Globe : Landmark;
  const ns = nodeStyle(e.importance, dark);
  return (
    <li className="relative flex gap-4 pb-4 last:pb-0">
      {/* node on the rail */}
      <div className="relative z-10 flex w-3.5 shrink-0 justify-center pt-2">
        <span className={cx('rounded-full', ns.size, ns.dot, ns.ring)} />
      </div>
      {/* event card */}
      <div className={cx('min-w-0 flex-1 rounded-2xl border p-3', t.card, t.border, t.soft)}>
        <div className="flex items-center gap-2">
          <span className={cx('shrink-0 text-[11px] font-semibold uppercase tabular-nums', t.faint)}>
            {MONTHS[d.getMonth()]} {d.getDate()}
          </span>
          <Icon
            size={13}
            className={cx('shrink-0', high ? (dark ? 'text-amber-300' : 'text-amber-600') : t.sub)}
          />
          <span className={cx('truncate text-[14px] font-semibold', t.text)}>{e.title}</span>
          {high && (
            <span
              className={cx(
                'ml-auto shrink-0 rounded-full px-1.5 py-0.5 text-[9.5px] font-bold uppercase',
                dark ? 'bg-amber-500/15 text-amber-300' : 'bg-amber-50 text-amber-700',
              )}
            >
              {tr('events.high')}
            </span>
          )}
        </div>
        <div className={cx('mt-1 flex flex-wrap items-center gap-x-2 text-[11.5px]', t.faint)}>
          <span>{relative(d)}</span>
          <span>·</span>
          <span>{e.region}</span>
          {!e.all_day && (
            <>
              <span>·</span>
              <span>{d.toLocaleTimeString([], {hour: 'numeric', minute: '2-digit'})}</span>
            </>
          )}
          {e.source_url && (
            <>
              <span>·</span>
              <a
                href={e.source_url}
                target="_blank"
                rel="noopener noreferrer"
                className={cx('font-semibold hover:opacity-80', t.accentText)}
              >
                {e.source_name}
              </a>
            </>
          )}
        </div>
      </div>
    </li>
  );
}
