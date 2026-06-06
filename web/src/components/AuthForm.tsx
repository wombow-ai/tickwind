'use client';

import Link from 'next/link';
import {useRouter} from 'next/navigation';
import {useState} from 'react';
import {useAuth} from '@/lib/auth';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, tok} from '@/lib/ui';

/** Email/password sign-in or sign-up form, backed by Supabase Auth. */
export function AuthForm({mode}: {mode: 'login' | 'signup'}) {
  const {supabase} = useAuth();
  const router = useRouter();
  const dark = useDark();
  const t = tok(dark);

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const isSignup = mode === 'signup';

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setNotice(null);
    if (password.length < 6) {
      setError('Password must be at least 6 characters.');
      return;
    }
    setBusy(true);
    try {
      if (isSignup) {
        const {data, error} = await supabase.auth.signUp({email, password});
        if (error) throw error;
        if (data.session) {
          router.push('/');
          router.refresh();
        } else {
          setNotice('Check your email to confirm your account, then log in.');
        }
      } else {
        const {error} = await supabase.auth.signInWithPassword({email, password});
        if (error) throw error;
        router.push('/');
        router.refresh();
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong.');
    } finally {
      setBusy(false);
    }
  }

  const inputCls = cx(
    'w-full rounded-xl border px-3.5 py-2.5 text-[14px] outline-none transition focus:border-teal-400',
    t.input,
  );

  return (
    <div className={cx('w-full max-w-sm rounded-3xl border p-7', t.card, t.border, t.soft)}>
      <h1 className={cx('text-[20px] font-bold tracking-tight', t.text)}>
        {isSignup ? 'Create your account' : 'Welcome back'}
      </h1>
      <p className={cx('mt-1 text-[13.5px]', t.sub)}>
        {isSignup
          ? 'Track your own watchlist and clip links — free.'
          : 'Log in to your watchlist and saved links.'}
      </p>

      <form onSubmit={submit} className="mt-6 space-y-3.5">
        <div>
          <label className={cx('mb-1.5 block text-[12.5px] font-medium', t.sub)}>
            Email
          </label>
          <input
            type="email"
            required
            autoComplete="email"
            value={email}
            onChange={e => setEmail(e.target.value)}
            placeholder="you@example.com"
            className={inputCls}
          />
        </div>
        <div>
          <label className={cx('mb-1.5 block text-[12.5px] font-medium', t.sub)}>
            Password
          </label>
          <input
            type="password"
            required
            autoComplete={isSignup ? 'new-password' : 'current-password'}
            value={password}
            onChange={e => setPassword(e.target.value)}
            placeholder="••••••••"
            className={inputCls}
          />
        </div>

        {error && (
          <p className="rounded-xl bg-rose-500/10 px-3 py-2 text-[12.5px] font-medium text-rose-500">
            {error}
          </p>
        )}
        {notice && (
          <p className="rounded-xl bg-teal-500/10 px-3 py-2 text-[12.5px] font-medium text-teal-600">
            {notice}
          </p>
        )}

        <button
          type="submit"
          disabled={busy}
          className={cx(
            'w-full rounded-xl py-2.5 text-[14px] font-semibold shadow-sm transition disabled:opacity-60',
            btnPrimary(dark),
          )}
        >
          {busy ? 'One moment…' : isSignup ? 'Create account' : 'Log in'}
        </button>
      </form>

      <p className={cx('mt-5 text-center text-[13px]', t.sub)}>
        {isSignup ? 'Already have an account? ' : "Don't have an account? "}
        <Link
          href={isSignup ? '/login' : '/signup'}
          className={cx('font-semibold', t.accentText)}
        >
          {isSignup ? 'Log in' : 'Sign up'}
        </Link>
      </p>
    </div>
  );
}
