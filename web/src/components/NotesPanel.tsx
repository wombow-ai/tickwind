'use client';

import {Pin, StickyNote, Trash2} from 'lucide-react';
import Link from 'next/link';
import {useCallback, useEffect, useState} from 'react';
import {createNote, deleteNote, getNotes, updateNote, type Note} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, timeAgo, tok} from '@/lib/ui';

type Tokens = ReturnType<typeof tok>;

/** Pinned first, then newest-created. */
function sortNotes(ns: Note[]): Note[] {
  return [...ns].sort((a, b) => {
    if (a.pinned !== b.pinned) return a.pinned ? -1 : 1;
    return b.created_at.localeCompare(a.created_at);
  });
}

/**
 * Private notes surface, reused by the stock detail "Notes" tab (pass `ticker`
 * → scopes the list + tags new notes to that stock) and the standalone /notes
 * page (no `ticker` → all the user's notes, ticker chips deep-link to the stock).
 * Auth is assumed (callers gate on a signed-in user).
 */
export function NotesPanel({ticker}: {ticker?: string}) {
  const {getToken} = useAuth();
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [notes, setNotes] = useState<Note[]>([]);
  const [draft, setDraft] = useState('');
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    getToken().then(token =>
      getNotes(token, ticker ? {ticker} : {}).then(
        r => setNotes(sortNotes(r.notes ?? [])),
        () => setNotes([]),
      ),
    );
  }, [getToken, ticker]);
  useEffect(() => {
    load();
  }, [load]);

  async function add() {
    const body = draft.trim();
    if (!body || busy) return;
    setBusy(true);
    try {
      const token = await getToken();
      const n = await createNote(token, ticker ? {body, ticker} : {body});
      setNotes(prev => sortNotes([n, ...prev]));
      setDraft('');
    } catch {
      // keep the draft so the user can retry
    } finally {
      setBusy(false);
    }
  }

  async function togglePin(n: Note) {
    setNotes(prev => sortNotes(prev.map(x => (x.id === n.id ? {...x, pinned: !x.pinned} : x))));
    try {
      const token = await getToken();
      const updated = await updateNote(token, n.id, {pinned: !n.pinned});
      setNotes(prev => sortNotes(prev.map(x => (x.id === n.id ? updated : x))));
    } catch {
      load();
    }
  }

  async function remove(n: Note) {
    setNotes(prev => prev.filter(x => x.id !== n.id)); // optimistic
    try {
      const token = await getToken();
      await deleteNote(token, n.id);
    } catch {
      load();
    }
  }

  return (
    <div className="tw-fade">
      <div className={cx('mb-4 rounded-2xl border p-3', t.card, t.border, t.soft)}>
        <textarea
          value={draft}
          onChange={e => setDraft(e.target.value)}
          onKeyDown={e => {
            if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') add();
          }}
          placeholder={ticker ? tr('stock.notesPlaceholder') : tr('notes.composePlaceholder')}
          rows={3}
          className={cx(
            'w-full resize-none bg-transparent text-[13.5px] outline-none',
            dark ? 'text-slate-100 placeholder:text-slate-500' : 'text-slate-900 placeholder:text-slate-400',
          )}
        />
        <div className="mt-1 flex justify-end">
          <button
            onClick={add}
            disabled={!draft.trim() || busy}
            className={cx(
              'rounded-lg px-3.5 py-1.5 text-[12.5px] font-semibold transition disabled:opacity-50',
              btnPrimary(dark),
            )}
          >
            {ticker ? tr('stock.save') : tr('notes.add')}
          </button>
        </div>
      </div>

      {notes.length === 0 ? (
        <div className={cx('rounded-2xl border p-8 text-center', t.card, t.border, t.soft)}>
          <StickyNote className={cx('mx-auto mb-2', dark ? 'text-teal-300' : 'text-teal-600')} size={22} />
          <p className={cx('text-[14px] font-semibold', t.text)}>
            {ticker ? tr('stock.noNotes') : tr('notes.empty')}
          </p>
          <p className={cx('mt-1 text-[12.5px]', t.sub)}>
            {ticker ? tr('stock.noNotesSub') : tr('notes.emptySub')}
          </p>
        </div>
      ) : (
        <div className="space-y-2.5">
          {notes.map(n => (
            <NoteCard
              key={n.id}
              n={n}
              dark={dark}
              t={t}
              tr={tr}
              showTicker={!ticker}
              onPin={() => togglePin(n)}
              onDelete={() => remove(n)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function NoteCard({
  n,
  dark,
  t,
  tr,
  showTicker,
  onPin,
  onDelete,
}: {
  n: Note;
  dark: boolean;
  t: Tokens;
  tr: (k: string) => string;
  showTicker: boolean;
  onPin: () => void;
  onDelete: () => void;
}) {
  const edited = n.updated_at !== n.created_at;
  return (
    <div
      className={cx(
        'group rounded-2xl border p-3.5',
        t.card,
        t.soft,
        n.pinned ? (dark ? 'border-amber-400/40' : 'border-amber-200') : t.border,
      )}
    >
      <div className="mb-1.5 flex flex-wrap items-center gap-2">
        {n.pinned && <Pin size={12} className={dark ? 'text-amber-300' : 'text-amber-500'} />}
        {showTicker && n.ticker && (
          <Link
            href={`/stock/${encodeURIComponent(n.ticker)}`}
            className={cx('rounded-md px-1.5 py-0.5 text-[10.5px] font-bold', t.chip, t.accentText)}
          >
            {n.ticker}
          </Link>
        )}
        {n.note_date && (
          <span className={cx('text-[11px] font-medium tabular-nums', t.faint)}>{n.note_date}</span>
        )}
        <span className={cx('ml-auto text-[11px]', t.faint)}>
          {edited ? tr('notes.edited') + ' · ' : ''}
          {timeAgo(n.updated_at)} {tr('common.ago')}
        </span>
      </div>
      <p className={cx('whitespace-pre-wrap break-words text-[13.5px]', t.text)}>{n.body}</p>
      <div className="mt-2 flex items-center gap-3 opacity-0 transition group-hover:opacity-100">
        <button
          onClick={onPin}
          className={cx('inline-flex items-center gap-1 text-[11.5px] font-medium hover:opacity-80', t.sub)}
        >
          <Pin size={12} /> {n.pinned ? tr('notes.unpin') : tr('notes.pin')}
        </button>
        <button
          onClick={onDelete}
          className={cx(
            'inline-flex items-center gap-1 text-[11.5px] font-medium hover:opacity-80',
            dark ? 'text-rose-400' : 'text-rose-500',
          )}
        >
          <Trash2 size={12} /> {tr('notes.delete')}
        </button>
      </div>
    </div>
  );
}
