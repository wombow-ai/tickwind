'use client';

import {ChevronDown, LogOut, Moon, Search, Settings, Star, StickyNote, Sun} from 'lucide-react';
import Link from 'next/link';
import {usePathname, useRouter} from 'next/navigation';
import {useEffect, useRef, useState} from 'react';
import {useAuth} from '@/lib/auth';
import {useLang, useT} from '@/lib/i18n';
import {useTheme} from '@/lib/theme';
import {btnPrimary, cx, tok} from '@/lib/ui';
import {Logo} from '@/components/ui/atoms';
import {SearchBox} from '@/components/SearchBox';

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
  const tr = useT();
  const {lang, toggle: toggleLang} = useLang();
  const router = useRouter();
  const pathname = usePathname();
  const [menu, setMenu] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);

  // Escape closes the account menu and the mobile search.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        setMenu(false);
        setSearchOpen(false);
      }
    }
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, []);

  const go = (ticker: string) => router.push(`/stock/${encodeURIComponent(ticker)}`);

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

        <SearchBox onSelect={go} placeholder={tr('nav.search')} className="ml-1 hidden w-56 sm:block" />

        <nav className="hidden items-center gap-1 md:flex">
          <Link
            href="/opportunities"
            aria-current={pathname === '/opportunities' ? 'page' : undefined}
            className={cx(
              'rounded-full px-3 py-1.5 text-[13px] font-medium hover:opacity-80',
              pathname === '/opportunities' ? t.accentText : t.sub,
            )}
          >
            {tr('nav.opportunities')}
          </Link>
          <Link
            href="/"
            aria-current={pathname === '/' ? 'page' : undefined}
            className={cx(
              'rounded-full px-3 py-1.5 text-[13px] font-medium hover:opacity-80',
              pathname === '/' ? t.accentText : t.sub,
            )}
          >
            {tr('nav.markets')}
          </Link>
          <Link
            href="/hot"
            aria-current={pathname === '/hot' ? 'page' : undefined}
            className={cx(
              'rounded-full px-3 py-1.5 text-[13px] font-medium hover:opacity-80',
              pathname === '/hot' ? t.accentText : t.sub,
            )}
          >
            {tr('nav.hot')}
          </Link>
          <Link
            href="/news"
            aria-current={pathname === '/news' ? 'page' : undefined}
            className={cx(
              'rounded-full px-3 py-1.5 text-[13px] font-medium hover:opacity-80',
              pathname === '/news' ? t.accentText : t.sub,
            )}
          >
            {tr('nav.news')}
          </Link>
          <MoreMenu pathname={pathname} authed={!!user} />
        </nav>

        <div className="ml-auto flex items-center gap-1.5 sm:gap-2">
          <button
            onClick={() => setSearchOpen(o => !o)}
            aria-label="Search a ticker"
            aria-expanded={searchOpen}
            className={cx(
              'inline-flex h-9 w-9 items-center justify-center rounded-full border sm:hidden',
              t.border,
              t.ghost,
            )}
          >
            <Search size={16} />
          </button>
          <Link
            href="/announcements"
            aria-current={pathname === '/announcements' ? 'page' : undefined}
            className={cx(
              'hidden rounded-full px-3 py-1.5 text-[13px] font-medium sm:inline-flex',
              pathname === '/announcements' ? t.accentText : t.sub,
              'hover:opacity-80',
            )}
          >
            {tr('nav.whatsnew')}
          </Link>

          <button
            onClick={toggleLang}
            aria-label="Switch language / 切换语言"
            className={cx(
              'inline-flex h-9 min-w-9 items-center justify-center rounded-full border px-2 text-[12px] font-semibold',
              t.border,
              t.ghost,
            )}
          >
            {lang === 'zh' ? 'EN' : '中'}
          </button>
          <button
            onClick={toggle}
            aria-label={dark ? 'Switch to light theme' : 'Switch to dark theme'}
            aria-pressed={dark}
            className={cx(
              'inline-flex h-9 w-9 items-center justify-center rounded-full border',
              t.border,
              t.ghost,
            )}
          >
            {dark ? <Sun size={15} /> : <Moon size={15} />}
          </button>

          {!user ? (
            <>
              <Link
                href="/login"
                className={cx(
                  'whitespace-nowrap rounded-full px-3 py-1.5 text-[13px] font-medium sm:px-3.5',
                  t.ghost,
                )}
              >
                {tr('nav.login')}
              </Link>
              <Link
                href="/signup"
                className={cx(
                  'whitespace-nowrap rounded-full px-3 py-1.5 text-[13px] font-semibold shadow-sm sm:px-3.5',
                  btnPrimary(dark),
                )}
              >
                {tr('nav.signup')}
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

      {searchOpen && (
        <div className={cx('border-t px-4 pb-3 pt-2 sm:hidden', t.border)}>
          <SearchBox
            onSelect={tk => {
              setSearchOpen(false);
              go(tk);
            }}
            autoFocus
            size="md"
            placeholder={tr('nav.search')}
          />
        </div>
      )}
    </div>
  );
}

/** Overflow nav dropdown for secondary public pages (keeps the bar uncluttered). */
function MoreMenu({pathname, authed}: {pathname: string; authed: boolean}) {
  const {dark} = useTheme();
  const t = tok(dark);
  const tr = useT();
  const [open, setOpen] = useState(false);
  const items = [
    {href: '/events', label: tr('nav.events')},
    {href: '/community', label: tr('nav.community')},
    ...(authed
      ? [
          {href: '/watchlist', label: tr('nav.watchlist')},
          {href: '/notes', label: tr('nav.notes')},
        ]
      : []),
  ];
  const active = items.some(i => i.href === pathname);
  return (
    <div className="relative">
      <button
        onClick={() => setOpen(o => !o)}
        aria-haspopup="menu"
        aria-expanded={open}
        className={cx(
          'inline-flex items-center gap-0.5 rounded-full px-3 py-1.5 text-[13px] font-medium hover:opacity-80',
          active ? t.accentText : t.sub,
        )}
      >
        {tr('nav.more')}
        <ChevronDown size={13} />
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-30" onClick={() => setOpen(false)} />
          <div
            className={cx(
              'absolute left-0 z-40 mt-2 w-44 rounded-2xl border p-1.5',
              t.card,
              t.border,
              t.soft,
            )}
          >
            {items.map(i => (
              <Link
                key={i.href}
                href={i.href}
                onClick={() => setOpen(false)}
                aria-current={pathname === i.href ? 'page' : undefined}
                className={cx(
                  'block rounded-xl px-2.5 py-2 text-[13px]',
                  pathname === i.href ? t.accentText : t.text,
                  t.ghost,
                )}
              >
                {i.label}
              </Link>
            ))}
          </div>
        </>
      )}
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
  const tr = useT();
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
        aria-haspopup="menu"
        aria-expanded={open}
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
              href="/watchlist"
              onClick={() => setOpen(false)}
              className={cx(
                'flex w-full items-center gap-2.5 rounded-xl px-2.5 py-2 text-[13px]',
                t.text,
                t.ghost,
              )}
            >
              <Star size={15} className={t.sub} /> {tr('nav.myWatchlist')}
            </Link>
            <Link
              href="/notes"
              onClick={() => setOpen(false)}
              className={cx(
                'flex w-full items-center gap-2.5 rounded-xl px-2.5 py-2 text-[13px]',
                t.text,
                t.ghost,
              )}
            >
              <StickyNote size={15} className={t.sub} /> {tr('nav.notes')}
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
              <Settings size={15} className={t.sub} /> {tr('nav.settings')}
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
              <LogOut size={15} /> {tr('nav.signout')}
            </button>
          </div>
        </>
      )}
    </div>
  );
}
