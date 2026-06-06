'use client';

import Link from 'next/link';
import {useRouter} from 'next/navigation';
import {useState} from 'react';
import {useAuth} from '@/lib/auth';
import {GOOGLE_OAUTH_ENABLED} from '@/lib/config';
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

  async function google() {
    setError(null);
    setBusy(true);
    const {error} = await supabase.auth.signInWithOAuth({
      provider: 'google',
      options: {redirectTo: `${window.location.origin}/auth/callback`},
    });
    if (error) {
      setError(error.message);
      setBusy(false);
    }
    // On success the browser is redirected to Google; no further work here.
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

      {GOOGLE_OAUTH_ENABLED && (
        <div className="mt-6">
          <button
            type="button"
            onClick={google}
            disabled={busy}
            className={cx(
              'flex w-full items-center justify-center gap-2.5 rounded-xl border py-2.5 text-[14px] font-semibold transition disabled:opacity-60',
              t.border,
              t.ghost,
              t.text,
            )}
          >
            <GoogleIcon /> Continue with Google
          </button>
          <div className="mt-5 flex items-center gap-3">
            <span className={cx('h-px flex-1', dark ? 'bg-slate-800' : 'bg-slate-200')} />
            <span className={cx('text-[11.5px] font-medium', t.faint)}>or</span>
            <span className={cx('h-px flex-1', dark ? 'bg-slate-800' : 'bg-slate-200')} />
          </div>
        </div>
      )}

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

/** Official multicolor Google "G" mark. */
function GoogleIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 48 48" aria-hidden>
      <path
        fill="#FFC107"
        d="M43.611 20.083H42V20H24v8h11.303c-1.649 4.657-6.08 8-11.303 8-6.627 0-12-5.373-12-12s5.373-12 12-12c3.059 0 5.842 1.154 7.961 3.039l5.657-5.657C34.046 6.053 29.268 4 24 4 12.955 4 4 12.955 4 24s8.955 20 20 20 20-8.955 20-20c0-1.341-.138-2.65-.389-3.917z"
      />
      <path
        fill="#FF3D00"
        d="M6.306 14.691l6.571 4.819C14.655 15.108 18.961 12 24 12c3.059 0 5.842 1.154 7.961 3.039l5.657-5.657C34.046 6.053 29.268 4 24 4 16.318 4 9.656 8.337 6.306 14.691z"
      />
      <path
        fill="#4CAF50"
        d="M24 44c5.166 0 9.86-1.977 13.409-5.192l-6.19-5.238C29.211 35.091 26.715 36 24 36c-5.202 0-9.619-3.317-11.283-7.946l-6.522 5.025C9.505 39.556 16.227 44 24 44z"
      />
      <path
        fill="#1976D2"
        d="M43.611 20.083H42V20H24v8h11.303c-.792 2.237-2.231 4.166-4.087 5.571l6.19 5.238C36.971 39.205 44 34 44 24c0-1.341-.138-2.65-.389-3.917z"
      />
    </svg>
  );
}
