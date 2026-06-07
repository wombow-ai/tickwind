'use client';

import {StickyNote} from 'lucide-react';
import Link from 'next/link';
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

  return (
    <div className="mx-auto max-w-2xl">
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
        <NotesPanel />
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
