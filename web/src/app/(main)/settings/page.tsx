'use client';

import {LogOut, Moon, Sun} from 'lucide-react';
import Link from 'next/link';
import {useAuth} from '@/lib/auth';
import {useTheme} from '@/lib/theme';
import {btnPrimary, cx, tok} from '@/lib/ui';

/** Account + appearance settings (requires sign-in). */
export default function SettingsPage() {
  const {user, loading, signOut} = useAuth();
  const {dark, toggle} = useTheme();
  const t = tok(dark);

  if (loading) {
    return <div className={cx('h-40 rounded-3xl border', t.card, t.border, t.skel)} />;
  }

  if (!user) {
    return (
      <div
        className={cx(
          'mx-auto max-w-md rounded-3xl border p-8 text-center',
          t.card,
          t.border,
          t.soft,
        )}
      >
        <h1 className={cx('text-[18px] font-bold', t.text)}>Sign in to continue</h1>
        <p className={cx('mt-1.5 text-[13.5px]', t.sub)}>
          Your settings live with your account.
        </p>
        <Link
          href="/login"
          className={cx(
            'mt-5 inline-flex rounded-full px-4 py-2 text-[13px] font-semibold',
            btnPrimary(dark),
          )}
        >
          Log in
        </Link>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-xl">
      <h1 className={cx('mb-6 text-[26px] font-bold tracking-tight', t.text)}>
        Settings
      </h1>

      <section className={cx('rounded-3xl border p-6', t.card, t.border, t.soft)}>
        <h2 className={cx('text-[13px] font-semibold uppercase tracking-wide', t.faint)}>
          Account
        </h2>
        <div className="mt-3 flex items-center gap-3">
          <span
            className="flex h-11 w-11 items-center justify-center rounded-full text-[14px] font-bold text-white"
            style={{background: 'linear-gradient(135deg,#2DD4BF,#0EA5E9)'}}
          >
            {(user.email ?? 'TW').slice(0, 2).toUpperCase()}
          </span>
          <div className="min-w-0">
            <p className={cx('truncate text-[14px] font-semibold', t.text)}>
              {user.email}
            </p>
            <p className={cx('text-[12px]', t.faint)}>Signed in</p>
          </div>
        </div>
      </section>

      <section className={cx('mt-5 rounded-3xl border p-6', t.card, t.border, t.soft)}>
        <h2 className={cx('text-[13px] font-semibold uppercase tracking-wide', t.faint)}>
          Appearance
        </h2>
        <div className="mt-3 flex items-center justify-between">
          <div>
            <p className={cx('text-[14px] font-semibold', t.text)}>Theme</p>
            <p className={cx('text-[12.5px]', t.sub)}>
              {dark ? 'Dark' : 'Light'} — switch any time.
            </p>
          </div>
          <button
            onClick={toggle}
            className={cx(
              'inline-flex items-center gap-2 rounded-full border px-3.5 py-2 text-[13px] font-medium',
              t.border,
              t.ghost,
            )}
          >
            {dark ? <Sun size={15} /> : <Moon size={15} />}
            {dark ? 'Light' : 'Dark'}
          </button>
        </div>
      </section>

      <button
        onClick={signOut}
        className={cx(
          'mt-5 inline-flex items-center gap-2 rounded-full border px-4 py-2 text-[13px] font-semibold',
          t.border,
          dark ? 'text-rose-400 hover:bg-slate-800' : 'text-rose-500 hover:bg-rose-50',
        )}
      >
        <LogOut size={15} /> Sign out
      </button>
    </div>
  );
}
