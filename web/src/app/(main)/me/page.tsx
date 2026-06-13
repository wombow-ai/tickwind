'use client';

import {Bell, PieChart, Star, StickyNote, User} from 'lucide-react';
import type {LucideIcon} from 'lucide-react';
import Link from 'next/link';
import {usePathname, useRouter, useSearchParams} from 'next/navigation';
import {Suspense, useCallback, useState} from 'react';
import {AlertsCenter} from '@/components/AlertsCenter';
import {Board} from '@/components/Board';
import {NotesCalendar} from '@/components/NotesCalendar';
import {NotesPanel} from '@/components/NotesPanel';
import {PortfolioView} from '@/components/PortfolioView';
import {useAuth} from '@/lib/auth';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, tok} from '@/lib/ui';

type Tab = 'watchlist' | 'holdings' | 'notes' | 'alerts';
const TABS: {id: Tab; key: string; icon: LucideIcon}[] = [
  {id: 'watchlist', key: 'me.watchlist', icon: Star},
  {id: 'holdings', key: 'me.holdings', icon: PieChart},
  {id: 'notes', key: 'me.notes', icon: StickyNote},
  {id: 'alerts', key: 'me.alerts', icon: Bell},
];

function isTab(v: string | null): v is Tab {
  return v === 'watchlist' || v === 'holdings' || v === 'notes' || v === 'alerts';
}

/** The signed-in user's personal hub: watchlist / holdings / notes / alerts tabs. */
function MeHub() {
  const {user, loading} = useAuth();
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const tab: Tab = isTab(params.get('tab')) ? (params.get('tab') as Tab) : 'watchlist';
  // Notes keeps its list/calendar sub-toggle (calendar view preserved).
  const [notesView, setNotesView] = useState<'list' | 'calendar'>('list');

  const select = useCallback(
    (next: Tab) => {
      const q = new URLSearchParams(params.toString());
      q.set('tab', next);
      router.replace(`${pathname}?${q.toString()}`, {scroll: false});
    },
    [params, pathname, router],
  );

  if (loading) {
    return (
      <div className="mx-auto max-w-3xl">
        <div className={cx('h-40 rounded-3xl border', t.card, t.border, t.skel)} />
      </div>
    );
  }
  if (!user) {
    return (
      <div className="mx-auto max-w-2xl">
        <div className={cx('rounded-3xl border p-8 text-center', t.card, t.border, t.soft)}>
          <p className={cx('text-[14px] font-semibold', t.text)}>{tr('settings.signInTitle')}</p>
          <p className={cx('mt-1 text-[13.5px]', t.sub)}>{tr('settings.signInSub')}</p>
          <Link
            href="/login"
            className={cx('mt-4 inline-flex rounded-full px-4 py-2 text-[13px] font-semibold', btnPrimary(dark))}
          >
            {tr('nav.login')}
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div className={cx('mx-auto', tab === 'notes' && notesView === 'calendar' ? 'max-w-4xl' : 'max-w-3xl')}>
      <header className="mb-5">
        <h1 className={cx('flex items-center gap-2 text-[22px] font-bold tracking-tight', t.text)}>
          <User size={20} className={dark ? 'text-teal-300' : 'text-teal-600'} />
          {tr('me.title')}
        </h1>
      </header>

      <nav className="mb-5 flex flex-wrap items-center gap-1.5">
        {TABS.map(({id, key, icon: Icon}) => {
          const active = tab === id;
          return (
            <button
              key={id}
              onClick={() => select(id)}
              aria-current={active ? 'page' : undefined}
              className={cx(
                'inline-flex items-center gap-1.5 rounded-full border px-3.5 py-1.5 text-[13px] font-semibold transition',
                active
                  ? dark
                    ? 'border-teal-400/40 bg-teal-500/15 text-teal-200'
                    : 'border-teal-200 bg-teal-50 text-teal-700'
                  : cx(t.border, t.sub, t.ghost),
              )}
            >
              <Icon size={14} aria-hidden />
              {tr(key)}
            </button>
          );
        })}
      </nav>

      {tab === 'watchlist' && <Board variant="watchlist" />}
      {tab === 'holdings' && <PortfolioView />}
      {tab === 'notes' && (
        <>
          <div className={cx('mb-4 inline-flex items-center gap-1 rounded-xl border p-1', t.border, t.surf2)}>
            {(['list', 'calendar'] as const).map(v => (
              <button
                key={v}
                onClick={() => setNotesView(v)}
                aria-pressed={notesView === v}
                className={cx(
                  'rounded-lg px-3.5 py-1.5 text-[13px] font-medium transition',
                  notesView === v
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
          {notesView === 'list' ? <NotesPanel /> : <NotesCalendar />}
        </>
      )}
      {tab === 'alerts' && <AlertsCenter />}
    </div>
  );
}

export default function MePage() {
  return (
    <Suspense fallback={null}>
      <MeHub />
    </Suspense>
  );
}
