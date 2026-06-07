'use client';

import {StickyNote} from 'lucide-react';
import Link from 'next/link';
import {useState} from 'react';
import {NotesCalendar} from '@/components/NotesCalendar';
import {NotesPanel} from '@/components/NotesPanel';
import {useAuth} from '@/lib/auth';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, tok} from '@/lib/ui';

/** The signed-in user's private notes across all stocks and dates. */
export default function NotesPage() {
  const {user, loading} = useAuth();
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [view, setView] = useState<'list' | 'calendar'>('list');

  return (
    <div className={cx('mx-auto', user && view === 'calendar' ? 'max-w-4xl' : 'max-w-2xl')}>
      <header className="mb-5">
        <h1
          className={cx('flex items-center gap-2 text-[22px] font-bold tracking-tight', t.text)}
        >
          <StickyNote size={20} className={dark ? 'text-teal-300' : 'text-teal-600'} />
          {tr('notes.title')}
        </h1>
        <p className={cx('mt-1 text-[13.5px]', t.sub)}>{tr('notes.subtitle')}</p>
      </header>

      {loading ? (
        <div className={cx('h-40 rounded-3xl border', t.card, t.border, t.skel)} />
      ) : user ? (
        <>
          <div className={cx('mb-4 inline-flex items-center gap-1 rounded-xl border p-1', t.border, t.surf2)}>
            {(['list', 'calendar'] as const).map(v => (
              <button
                key={v}
                onClick={() => setView(v)}
                aria-pressed={view === v}
                className={cx(
                  'rounded-lg px-3.5 py-1.5 text-[13px] font-medium transition',
                  view === v
                    ? dark
                      ? 'bg-slate-700 text-white'
                      : 'bg-white text-slate-900 shadow-sm'
                    : t.sub,
                )}
              >
                {tr(v === 'list' ? 'notes.list' : 'notes.calendar')}
              </button>
            ))}
          </div>
          {view === 'list' ? <NotesPanel /> : <NotesCalendar />}
        </>
      ) : (
        <div className={cx('rounded-3xl border p-8 text-center', t.card, t.border, t.soft)}>
          <p className={cx('text-[14px] font-semibold', t.text)}>{tr('settings.signInTitle')}</p>
          <p className={cx('mt-1 text-[13.5px]', t.sub)}>{tr('settings.signInSub')}</p>
          <Link
            href="/login"
            className={cx(
              'mt-4 inline-flex rounded-full px-4 py-2 text-[13px] font-semibold',
              btnPrimary(dark),
            )}
          >
            {tr('nav.login')}
          </Link>
        </div>
      )}
    </div>
  );
}
