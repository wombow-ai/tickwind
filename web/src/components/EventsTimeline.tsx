'use client';

import {CalendarClock, Globe, Landmark} from 'lucide-react';
import {useCallback, useEffect, useState} from 'react';
import {getEvents, type EventItem} from '@/lib/api';
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

/**
 * The "Major events timeline / 大事件时间线": upcoming market-moving events — Fed
 * (FOMC) decisions, key US releases (CPI, jobs report) and notable world events —
 * from GET /v1/events. Informational (what to watch), never advice.
 */
export function EventsTimeline() {
  const dark = useDark();
  const t = tok(dark);
  const [status, setStatus] = useState<Status>('loading');
  const [events, setEvents] = useState<EventItem[]>([]);

  const load = useCallback(() => {
    setStatus('loading');
    getEvents(40).then(
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
          Events timeline
        </h1>
        <p className={cx('mt-1 text-[13.5px]', t.sub)}>
          Upcoming market-moving events — Fed decisions, key US data (CPI, jobs), and
          notable world events. For context, not advice.
        </p>
      </header>

      {status === 'loading' && <FeedSkeleton />}
      {status === 'error' && <ErrorState onRetry={load} />}
      {status === 'ready' && events.length === 0 && (
        <EmptyState label="No upcoming events" sub="Check back soon." icon={CalendarClock} />
      )}
      {status === 'ready' && events.length > 0 && (
        <ol className="tw-fade space-y-2">
          {events.map((e, i) => (
            <Row key={e.id || i} e={e} dark={dark} t={t} />
          ))}
        </ol>
      )}

      <p className={cx('mt-4 text-center text-[11px]', t.faint)}>
        Sources: BLS, Federal Reserve + curated. Times in your local zone. Not investment
        advice.
      </p>
    </div>
  );
}

function Row({e, dark, t}: {e: EventItem; dark: boolean; t: Tokens}) {
  const d = new Date(e.start);
  const high = e.importance === 'high';
  const Icon = e.category === 'world' ? Globe : Landmark;
  const accent = high ? (dark ? 'text-amber-300' : 'text-amber-600') : t.sub;
  return (
    <li className={cx('flex items-center gap-3 rounded-2xl border p-3', t.card, t.border, t.soft)}>
      <div
        className={cx(
          'flex w-12 shrink-0 flex-col items-center justify-center rounded-xl py-1',
          dark ? 'bg-slate-800' : 'bg-slate-100',
        )}
      >
        <span className={cx('text-[10px] font-semibold uppercase', t.faint)}>
          {MONTHS[d.getMonth()]}
        </span>
        <span className={cx('text-[16px] font-bold leading-none', t.text)}>{d.getDate()}</span>
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5">
          <Icon size={13} className={cx('shrink-0', accent)} />
          <span className={cx('truncate text-[14px] font-semibold', t.text)}>{e.title}</span>
          {high && (
            <span
              className={cx(
                'shrink-0 rounded-full px-1.5 py-0.5 text-[9.5px] font-bold uppercase',
                dark ? 'bg-amber-500/15 text-amber-300' : 'bg-amber-50 text-amber-700',
              )}
            >
              High
            </span>
          )}
        </div>
        <div className={cx('mt-0.5 flex flex-wrap items-center gap-x-2 text-[11.5px]', t.faint)}>
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
