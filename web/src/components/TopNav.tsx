'use client';

import {ChevronDown, LogOut, Menu, Moon, Search, Settings, Star, StickyNote, Sun, Wallet, X} from 'lucide-react';
import Link from 'next/link';
import {usePathname, useRouter} from 'next/navigation';
import {useEffect, useRef, useState} from 'react';
import {useAuth} from '@/lib/auth';
import {useLang, useT} from '@/lib/i18n';
import {useTheme} from '@/lib/theme';
import {btnPrimary, cx, tok} from '@/lib/ui';
import {Logo} from '@/components/ui/atoms';
import {AlertsBell} from '@/components/AlertsBell';
import {SearchBox} from '@/components/SearchBox';

type Tokens = ReturnType<typeof tok>;
type NavItem = {href: string; label: string};

/** Two-letter initials for an email/name, for the avatar chip. */
function initials(email: string | undefined): string {
  if (!email) return 'TW';
  const name = email.split('@')[0];
  const parts = name.split(/[._-]+/).filter(Boolean);
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase();
  return name.slice(0, 2).toUpperCase();
}

/**
 * Whether a nav item is the active route. Exact match, except the calendar group
 * — its pill (`/calendar/earnings`) stays highlighted across all `/calendar/*`
 * subpaths (Earnings · Macro · IPO).
 */
function navActive(href: string, pathname: string): boolean {
  if (href.startsWith('/calendar')) return pathname.startsWith('/calendar');
  return pathname === href;
}

/** A single desktop nav pill. */
function NavPill({item, pathname, t}: {item: NavItem; pathname: string; t: Tokens}) {
  const active = navActive(item.href, pathname);
  return (
    <Link
      href={item.href}
      aria-current={active ? 'page' : undefined}
      className={cx(
        'rounded-full px-3 py-1.5 text-[13px] font-medium hover:opacity-80',
        active ? t.accentText : t.sub,
      )}
    >
      {item.label}
    </Link>
  );
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
  const authed = !!user;
  const [menu, setMenu] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);

  // Escape closes every transient surface; route changes close the mobile menu.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        setMenu(false);
        setSearchOpen(false);
        setMobileOpen(false);
      }
    }
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, []);
  useEffect(() => {
    setMobileOpen(false);
    setSearchOpen(false);
  }, [pathname]);

  const go = (ticker: string) => router.push(`/stock/${encodeURIComponent(ticker)}`);
  const search = (q: string) => router.push(`/search?q=${encodeURIComponent(q)}`);

  // One source of truth for destinations, shared by desktop nav, the More
  // dropdown, and the mobile menu.
  const primary: NavItem[] = [
    {href: '/opportunities', label: tr('nav.opportunities')},
    {href: '/', label: tr('nav.markets')},
    {href: '/hot', label: tr('nav.hot')},
    {href: '/news', label: tr('nav.news')},
  ];
  const my: NavItem = {href: '/me', label: tr('nav.my')};
  const secondary: NavItem[] = [
    {href: '/screen', label: tr('nav.screen')},
    {href: '/indicators', label: tr('nav.indicators')},
    {href: '/smart-money', label: tr('nav.smartMoney')},
    {href: '/unusual', label: tr('nav.unusual')},
    {href: '/calendar/earnings', label: tr('nav.calendar')},
    {href: '/discussion', label: tr('nav.discussion')},
  ];
  const whatsnew: NavItem = {href: '/announcements', label: tr('nav.whatsnew')};
  // The full ordered list for the mobile sheet.
  const mobileItems: NavItem[] = [
    ...primary,
    ...(authed ? [my] : []),
    ...secondary,
    whatsnew,
  ];

  return (
    <div
      className={cx(
        'sticky top-0 z-30 border-b backdrop-blur',
        t.border,
        dark ? 'bg-slate-950/70' : 'bg-white/70',
      )}
    >
      <div className="mx-auto flex h-14 max-w-6xl items-center gap-2 px-4 sm:gap-3 sm:px-6">
        <button
          onClick={() => {
            setMobileOpen(o => !o);
            setSearchOpen(false);
          }}
          aria-label={tr('nav.menu')}
          aria-expanded={mobileOpen}
          className={cx(
            'inline-flex h-9 w-9 items-center justify-center rounded-full border md:hidden',
            t.border,
            t.ghost,
          )}
        >
          {mobileOpen ? <X size={17} /> : <Menu size={17} />}
        </button>

        <Link href="/" aria-label="Tickwind home">
          <Logo size={28} />
        </Link>

        <SearchBox
          onSelect={go}
          onSubmit={search}
          placeholder={tr('nav.search')}
          className="ml-1 hidden w-44 lg:block"
        />

        <nav className="hidden items-center gap-1 md:flex">
          {primary.map(item => (
            <NavPill key={item.href} item={item} pathname={pathname} t={t} />
          ))}
          {authed && <NavPill item={my} pathname={pathname} t={t} />}
          <MoreMenu pathname={pathname} items={secondary} />
        </nav>

        <div className="ml-auto flex items-center gap-1.5 sm:gap-2">
          <button
            onClick={() => {
              setSearchOpen(o => !o);
              setMobileOpen(false);
            }}
            aria-label="Search a ticker"
            aria-expanded={searchOpen}
            className={cx(
              'inline-flex h-9 w-9 items-center justify-center rounded-full border lg:hidden',
              t.border,
              t.ghost,
            )}
          >
            <Search size={16} />
          </button>
          <AlertsBell dark={dark} />
          <Link
            href="/announcements"
            aria-current={pathname === '/announcements' ? 'page' : undefined}
            className={cx(
              'hidden whitespace-nowrap rounded-full px-3 py-1.5 text-[13px] font-medium md:inline-flex',
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
        <div className={cx('border-t px-4 pb-3 pt-2 lg:hidden', t.border)}>
          <SearchBox
            onSelect={tk => {
              setSearchOpen(false);
              go(tk);
            }}
            onSubmit={q => {
              setSearchOpen(false);
              search(q);
            }}
            autoFocus
            size="md"
            placeholder={tr('nav.search')}
          />
        </div>
      )}

      {mobileOpen && (
        <>
          <div className="fixed inset-x-0 bottom-0 top-14 z-20 md:hidden" onClick={() => setMobileOpen(false)} />
          <nav className={cx('relative z-30 border-t px-3 py-2 md:hidden', t.border)}>
            {mobileItems.map(item => {
              const active = navActive(item.href, pathname);
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  onClick={() => setMobileOpen(false)}
                  aria-current={active ? 'page' : undefined}
                  className={cx(
                    'block rounded-xl px-3 py-2.5 text-[14px] font-medium',
                    active ? t.accentText : t.text,
                    t.ghost,
                  )}
                >
                  {item.label}
                </Link>
              );
            })}
          </nav>
        </>
      )}
    </div>
  );
}

/** Overflow nav dropdown for secondary public pages (keeps the bar uncluttered). */
function MoreMenu({pathname, items}: {pathname: string; items: NavItem[]}) {
  const {dark} = useTheme();
  const t = tok(dark);
  const tr = useT();
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const active = items.some(i => navActive(i.href, pathname));

  // Escape closes the dropdown + restores focus to its trigger. The global TopNav
  // Escape handler only covers the account/mobile menus (whose state it owns);
  // MoreMenu owns its own open state, so it must handle Escape itself.
  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        setOpen(false);
        triggerRef.current?.focus();
      }
    }
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open]);
  return (
    <div className="relative">
      <button
        ref={triggerRef}
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
            {items.map(i => {
              const active = navActive(i.href, pathname);
              return (
                <Link
                  key={i.href}
                  href={i.href}
                  onClick={() => setOpen(false)}
                  aria-current={active ? 'page' : undefined}
                  className={cx(
                    'block rounded-xl px-2.5 py-2 text-[13px]',
                    active ? t.accentText : t.text,
                    t.ghost,
                  )}
                >
                  {i.label}
                </Link>
              );
            })}
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
                {email ?? tr('nav.signedIn')}
              </p>
            </div>
            <Link
              href="/me?tab=watchlist"
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
              href="/me?tab=notes"
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
              href="/me?tab=holdings"
              onClick={() => setOpen(false)}
              className={cx(
                'flex w-full items-center gap-2.5 rounded-xl px-2.5 py-2 text-[13px]',
                t.text,
                t.ghost,
              )}
            >
              <Wallet size={15} className={t.sub} /> {tr('nav.portfolio')}
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
