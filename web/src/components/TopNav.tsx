'use client';

import {ChevronDown, LogOut, Moon, Search, Settings, Star, Sun} from 'lucide-react';
import Link from 'next/link';
import {usePathname, useRouter} from 'next/navigation';
import {useRef, useState} from 'react';
import {useAuth} from '@/lib/auth';
import {useTheme} from '@/lib/theme';
import {btnPrimary, cx, tok} from '@/lib/ui';
import {Logo} from '@/components/ui/atoms';

/** Two-letter initials for an email/name, for the avatar chip. */
function initials(email: string | undefined): string {
  if (!email) return 'TW';
  const name = email.split('@')[0];
  const parts = name.split(/[._-]+/).filter(Boolean);
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase();
  return name.slice(0, 2).toUpperCase();
}

/** The sticky top navigation: brand, ticker search, theme, and account. */
export function TopNav() {
  const {dark, toggle} = useTheme();
  const {user, signOut} = useAuth();
  const t = tok(dark);
  const router = useRouter();
  const pathname = usePathname();
  const [query, setQuery] = useState('');
  const [menu, setMenu] = useState(false);

  function search(e: React.FormEvent) {
    e.preventDefault();
    const ticker = query.trim().toUpperCase();
    if (!ticker) return;
    setQuery('');
    router.push(`/stock/${encodeURIComponent(ticker)}`);
  }

  return (
    <div
      className={cx(
        'sticky top-0 z-30 border-b backdrop-blur',
        t.border,
        dark ? 'bg-slate-950/70' : 'bg-white/70',
      )}
    >
      <div className="mx-auto flex h-14 max-w-6xl items-center gap-3 px-4 sm:px-6">
        <Link href="/" aria-label="Tickwind home">
          <Logo size={28} />
        </Link>

        <form onSubmit={search} className="ml-1 hidden sm:flex">
          <div
            className={cx(
              'flex items-center gap-1.5 rounded-full border px-3 py-1.5',
              t.border,
              t.surf2,
            )}
          >
            <Search size={14} className={t.faint} />
            <input
              value={query}
              onChange={e => setQuery(e.target.value)}
              placeholder="Search a ticker…"
              className={cx(
                'w-36 bg-transparent text-[13px] uppercase tracking-wide outline-none',
                dark
                  ? 'text-slate-100 placeholder:text-slate-500'
                  : 'text-slate-900 placeholder:text-slate-400',
              )}
            />
          </div>
        </form>

        <div className="ml-auto flex items-center gap-2">
          <Link
            href="/announcements"
            className={cx(
              'hidden rounded-full px-3 py-1.5 text-[13px] font-medium sm:inline-flex',
              pathname === '/announcements' ? t.accentText : t.sub,
              'hover:opacity-80',
            )}
          >
            What&apos;s new
          </Link>

          <button
            onClick={toggle}
            aria-label="Toggle theme"
            className={cx('rounded-full border p-2', t.border, t.ghost)}
          >
            {dark ? <Sun size={15} /> : <Moon size={15} />}
          </button>

          {!user ? (
            <>
              <Link
                href="/login"
                className={cx(
                  'rounded-full px-3.5 py-1.5 text-[13px] font-medium',
                  t.ghost,
                )}
              >
                Log in
              </Link>
              <Link
                href="/signup"
                className={cx(
                  'rounded-full px-3.5 py-1.5 text-[13px] font-semibold shadow-sm',
                  btnPrimary(dark),
                )}
              >
                Sign up
              </Link>
            </>
          ) : (
            <AccountMenu
              open={menu}
              setOpen={setMenu}
              email={user.email}
              onSignOut={signOut}
            />
          )}
        </div>
      </div>
    </div>
  );
}

function AccountMenu({
  open,
  setOpen,
  email,
  onSignOut,
}: {
  open: boolean;
  setOpen: (v: boolean) => void;
  email: string | undefined;
  onSignOut: () => void;
}) {
  const {dark} = useTheme();
  const t = tok(dark);
  const ref = useRef<HTMLDivElement>(null);

  return (
    <div className="relative" ref={ref}>
      <button
        onClick={() => setOpen(!open)}
        className={cx(
          'flex items-center gap-2 rounded-full border py-1 pl-1 pr-2 transition',
          t.border,
          t.ghost,
        )}
        aria-label="Account menu"
      >
        <span
          className="flex h-7 w-7 items-center justify-center rounded-full text-[11px] font-bold text-white"
          style={{background: 'linear-gradient(135deg,#2DD4BF,#0EA5E9)'}}
        >
          {initials(email)}
        </span>
        <ChevronDown size={14} className={t.sub} />
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-30" onClick={() => setOpen(false)} />
          <div
            className={cx(
              'absolute right-0 z-40 mt-2 w-56 rounded-2xl border p-1.5',
              t.card,
              t.border,
              t.soft,
            )}
          >
            <div className={cx('mb-1 border-b px-2.5 py-2', t.hair)}>
              <p className={cx('truncate text-[13px] font-semibold', t.text)}>
                {email ?? 'Signed in'}
              </p>
            </div>
            <Link
              href="/"
              onClick={() => setOpen(false)}
              className={cx(
                'flex w-full items-center gap-2.5 rounded-xl px-2.5 py-2 text-[13px]',
                t.text,
                t.ghost,
              )}
            >
              <Star size={15} className={t.sub} /> My watchlist
            </Link>
            <Link
              href="/settings"
              onClick={() => setOpen(false)}
              className={cx(
                'flex w-full items-center gap-2.5 rounded-xl px-2.5 py-2 text-[13px]',
                t.text,
                t.ghost,
              )}
            >
              <Settings size={15} className={t.sub} /> Settings
            </Link>
            <button
              onClick={() => {
                setOpen(false);
                onSignOut();
              }}
              className={cx(
                'mt-0.5 flex w-full items-center gap-2.5 rounded-xl px-2.5 py-2 text-[13px]',
                dark
                  ? 'text-rose-400 hover:bg-slate-800'
                  : 'text-rose-500 hover:bg-rose-50',
              )}
            >
              <LogOut size={15} /> Sign out
            </button>
          </div>
        </>
      )}
    </div>
  );
}
