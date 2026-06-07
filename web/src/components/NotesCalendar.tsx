'use client';

import {CalendarClock, ChevronLeft, ChevronRight, Trash2} from 'lucide-react';
import {useCallback, useEffect, useMemo, useState} from 'react';
import {createNote, deleteNote, getEvents, getNotes, type EventItem, type Note} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, tok} from '@/lib/ui';

/** Local YYYY-MM-DD (no timezone shift — matches the API's date strings). */
function ymd(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

/**
 * Month-grid calendar view of the user's dated notes (the "日历" surface). Fetches
 * the visible month via GET /v1/notes?from=&to= (no backend change); clicking a
 * day shows + adds notes for that date. Undated notes don't appear here (they
 * live in the List view).
 */
export function NotesCalendar() {
  const {getToken} = useAuth();
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [cursor, setCursor] = useState(() => {
    const n = new Date();
    return new Date(n.getFullYear(), n.getMonth(), 1);
  });
  const [notes, setNotes] = useState<Note[]>([]);
  const [events, setEvents] = useState<EventItem[]>([]);
  const [selected, setSelected] = useState('');
  const [draft, setDraft] = useState('');

  const monthStart = ymd(new Date(cursor.getFullYear(), cursor.getMonth(), 1));
  const monthEnd = ymd(new Date(cursor.getFullYear(), cursor.getMonth() + 1, 0));

  const load = useCallback(() => {
    getToken().then(token =>
      getNotes(token, {from: monthStart, to: monthEnd}).then(
        r => setNotes(r.notes ?? []),
        () => setNotes([]),
      ),
    );
  }, [getToken, monthStart, monthEnd]);
  useEffect(() => {
    load();
  }, [load]);

  // Major events (reused from the Events timeline) shown on the calendar by default.
  useEffect(() => {
    getEvents().then(
      r => setEvents(r.events ?? []),
      () => setEvents([]),
    );
  }, []);

  const byDay = useMemo(() => {
    const m = new Map<string, Note[]>();
    for (const n of notes) {
      if (!n.note_date) continue;
      const arr = m.get(n.note_date);
      if (arr) arr.push(n);
      else m.set(n.note_date, [n]);
    }
    return m;
  }, [notes]);

  const eventsByDay = useMemo(() => {
    const m = new Map<string, EventItem[]>();
    for (const e of events) {
      const d = e.start.slice(0, 10);
      const arr = m.get(d);
      if (arr) arr.push(e);
      else m.set(d, [e]);
    }
    return m;
  }, [events]);

  const weekdays = useMemo(() => {
    const fmt = new Intl.DateTimeFormat(undefined, {weekday: 'short'});
    return Array.from({length: 7}, (_, i) => fmt.format(new Date(2023, 0, 1 + i))); // 2023-01-01 = Sun
  }, []);

  // Leading blanks for the first weekday + the month's days.
  const firstDow = new Date(cursor.getFullYear(), cursor.getMonth(), 1).getDay();
  const daysInMonth = new Date(cursor.getFullYear(), cursor.getMonth() + 1, 0).getDate();
  const cells: (string | null)[] = [];
  for (let i = 0; i < firstDow; i++) cells.push(null);
  for (let d = 1; d <= daysInMonth; d++) {
    cells.push(ymd(new Date(cursor.getFullYear(), cursor.getMonth(), d)));
  }

  async function add() {
    const body = draft.trim();
    if (!body || !selected) return;
    try {
      const token = await getToken();
      const n = await createNote(token, {body, note_date: selected});
      setNotes(prev => [n, ...prev]);
      setDraft('');
    } catch {
      // keep the draft to retry
    }
  }

  async function remove(n: Note) {
    setNotes(prev => prev.filter(x => x.id !== n.id));
    try {
      const token = await getToken();
      await deleteNote(token, n.id);
    } catch {
      load();
    }
  }

  const today = ymd(new Date());
  const selectedNotes = selected ? byDay.get(selected) ?? [] : [];
  const selectedEvents = selected ? eventsByDay.get(selected) ?? [] : [];

  return (
    <div className="tw-fade">
      <div className="mb-3 flex items-center justify-between">
        <button
          onClick={() => setCursor(c => new Date(c.getFullYear(), c.getMonth() - 1, 1))}
          aria-label="Previous month"
          className={cx('inline-flex h-8 w-8 items-center justify-center rounded-full border', t.border, t.ghost)}
        >
          <ChevronLeft size={16} />
        </button>
        <span className={cx('text-[14px] font-bold', t.text)}>
          {cursor.toLocaleDateString(undefined, {year: 'numeric', month: 'long'})}
        </span>
        <button
          onClick={() => setCursor(c => new Date(c.getFullYear(), c.getMonth() + 1, 1))}
          aria-label="Next month"
          className={cx('inline-flex h-8 w-8 items-center justify-center rounded-full border', t.border, t.ghost)}
        >
          <ChevronRight size={16} />
        </button>
      </div>

      <div className="grid grid-cols-7 gap-1">
        {weekdays.map(w => (
          <div key={w} className={cx('pb-1 text-center text-[10px] font-semibold uppercase', t.faint)}>
            {w}
          </div>
        ))}
        {cells.map((day, i) =>
          day === null ? (
            <div key={`b${i}`} />
          ) : (
            <button
              key={day}
              onClick={() => setSelected(day)}
              className={cx(
                'flex aspect-square flex-col items-center justify-start rounded-lg border p-1 text-[11px] transition',
                day === selected
                  ? dark
                    ? 'border-teal-400/50 bg-teal-500/10'
                    : 'border-teal-300 bg-teal-50'
                  : cx(t.border, t.ghost),
                day === today && t.accentText,
              )}
            >
              <span className="flex w-full items-center justify-between">
                <span className={cx('tabular-nums', t.text)}>{Number(day.slice(8))}</span>
                {eventsByDay.has(day) && (
                  <span
                    className="h-1.5 w-1.5 shrink-0 rounded-full"
                    style={{background: dark ? '#fbbf24' : '#f59e0b'}}
                    title="event"
                  />
                )}
              </span>
              {byDay.has(day) && (
                <span
                  className={cx(
                    'mt-0.5 rounded-full px-1 text-[9px] font-bold',
                    dark ? 'bg-teal-500/20 text-teal-200' : 'bg-teal-100 text-teal-700',
                  )}
                >
                  {byDay.get(day)!.length}
                </span>
              )}
            </button>
          ),
        )}
      </div>

      {selected && (
        <div className="mt-4">
          <div className={cx('mb-2 text-[13px] font-semibold', t.text)}>{selected}</div>
          {selectedEvents.length > 0 && (
            <div className="mb-3 space-y-1">
              {selectedEvents.map(e => (
                <div
                  key={e.id}
                  className={cx(
                    'flex items-center gap-2 rounded-xl border px-3 py-1.5 text-[12px]',
                    t.border,
                    dark ? 'bg-amber-500/5' : 'bg-amber-50/60',
                  )}
                >
                  <CalendarClock size={12} className={dark ? 'text-amber-300' : 'text-amber-600'} />
                  <span className={cx('min-w-0 flex-1 truncate font-medium', t.text)}>{e.title}</span>
                  {e.importance === 'high' && (
                    <span
                      className={cx(
                        'shrink-0 rounded-full px-1.5 py-0.5 text-[9px] font-bold uppercase',
                        dark ? 'bg-amber-500/15 text-amber-300' : 'bg-amber-100 text-amber-700',
                      )}
                    >
                      {tr('events.high')}
                    </span>
                  )}
                </div>
              ))}
            </div>
          )}
          <div className={cx('mb-3 rounded-2xl border p-3', t.card, t.border, t.soft)}>
            <textarea
              value={draft}
              onChange={e => setDraft(e.target.value)}
              onKeyDown={e => {
                if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') add();
              }}
              placeholder={tr('notes.dayNote')}
              rows={2}
              className={cx(
                'w-full resize-none bg-transparent text-[13.5px] outline-none',
                dark ? 'text-slate-100 placeholder:text-slate-500' : 'text-slate-900 placeholder:text-slate-400',
              )}
            />
            <div className="mt-1 flex justify-end">
              <button
                onClick={add}
                disabled={!draft.trim()}
                className={cx(
                  'rounded-lg px-3.5 py-1.5 text-[12.5px] font-semibold transition disabled:opacity-50',
                  btnPrimary(dark),
                )}
              >
                {tr('notes.add')}
              </button>
            </div>
          </div>
          {selectedNotes.length === 0 ? (
            <p className={cx('text-[12.5px]', t.faint)}>{tr('notes.dayEmpty')}</p>
          ) : (
            <div className="space-y-2">
              {selectedNotes.map(n => (
                <div
                  key={n.id}
                  className={cx('group flex items-start gap-2 rounded-2xl border p-3', t.card, t.border, t.soft)}
                >
                  {n.ticker && (
                    <span className={cx('shrink-0 rounded-md px-1.5 py-0.5 text-[10.5px] font-bold', t.chip, t.accentText)}>
                      {n.ticker}
                    </span>
                  )}
                  <p className={cx('min-w-0 flex-1 whitespace-pre-wrap break-words text-[13.5px]', t.text)}>
                    {n.body}
                  </p>
                  <button
                    onClick={() => remove(n)}
                    aria-label="Delete note"
                    className={cx('shrink-0 opacity-0 transition group-hover:opacity-100', dark ? 'text-rose-400' : 'text-rose-500')}
                  >
                    <Trash2 size={13} />
                  </button>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
