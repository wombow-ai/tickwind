'use client';

import {Bell, ExternalLink, PieChart, Sparkles, Star, StickyNote, Sunrise, User} from 'lucide-react';
import type {LucideIcon} from 'lucide-react';
import Link from 'next/link';
import {usePathname, useRouter, useSearchParams} from 'next/navigation';
import {Suspense, useCallback, useEffect, useState} from 'react';
import {AlertsCenter} from '@/components/AlertsCenter';
import {Board} from '@/components/Board';
import {Markdown} from '@/components/Markdown';
import {NotesCalendar} from '@/components/NotesCalendar';
import {NotesPanel} from '@/components/NotesPanel';
import {PortfolioView} from '@/components/PortfolioView';
import {getMyDigest, type DigestStock, type MyDigest} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, tok} from '@/lib/ui';

type Tab = 'overview' | 'watchlist' | 'holdings' | 'notes' | 'alerts';
const TABS: {id: Tab; key: string; icon: LucideIcon}[] = [
  {id: 'overview', key: 'me.overview', icon: Sunrise},
  {id: 'watchlist', key: 'me.watchlist', icon: Star},
  {id: 'holdings', key: 'me.holdings', icon: PieChart},
  {id: 'notes', key: 'me.notes', icon: StickyNote},
  {id: 'alerts', key: 'me.alerts', icon: Bell},
];

function isTab(v: string | null): v is Tab {
  return (
    v === 'overview' ||
    v === 'watchlist' ||
    v === 'holdings' ||
    v === 'notes' ||
    v === 'alerts'
  );
}

/** One watchlist row in the overnight digest: change % + freshest headline + next event. */
function DigestRow({st, dark, last}: {st: DigestStock; dark: boolean; last: boolean}) {
  const t = tok(dark);
  const pos = st.change_pct != null && st.change_pct >= 0;
  const chgColor =
    st.change_pct == null
      ? t.faint
      : pos
        ? dark
          ? 'text-emerald-400'
          : 'text-emerald-600'
        : dark
          ? 'text-rose-400'
          : 'text-rose-500';
  return (
    <div className={cx('px-4 py-3', last ? '' : cx('border-b', t.border))}>
      <div className="flex items-center gap-3">
        <Link
          href={`/stock/${encodeURIComponent(st.ticker)}`}
          className={cx('font-bold transition hover:opacity-80', t.text)}
        >
          {st.ticker}
        </Link>
        {st.name && <span className={cx('truncate text-[12.5px]', t.sub)}>{st.name}</span>}
        <span className={cx('ml-auto shrink-0 font-semibold tabular-nums', chgColor)}>
          {st.change_pct == null
            ? '—'
            : `${pos ? '+' : ''}${st.change_pct.toFixed(2)}%`}
        </span>
      </div>
      {(st.headline || st.next_event) && (
        <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1">
          {st.headline &&
            (st.headline_url ? (
              <a
                href={st.headline_url}
                target="_blank"
                rel="noopener noreferrer"
                className={cx(
                  'inline-flex items-center gap-1 text-[12.5px] transition hover:opacity-80',
                  t.sub,
                )}
              >
                <span className="line-clamp-1">{st.headline}</span>
                <ExternalLink size={11} className="shrink-0" aria-hidden />
              </a>
            ) : (
              <span className={cx('line-clamp-1 text-[12.5px]', t.sub)}>{st.headline}</span>
            ))}
          {st.next_event && (
            <span
              className={cx(
                'shrink-0 rounded-md px-1.5 py-0.5 text-[10.5px] font-semibold',
                dark ? 'bg-amber-500/15 text-amber-300' : 'bg-amber-50 text-amber-700',
              )}
            >
              {st.next_event}
            </span>
          )}
        </div>
      )}
    </div>
  );
}

/**
 * The "Overview / 我的" tab: the personalized overnight report — an AI overview
 * (when the LLM is on) over the user's watchlist, then one row per tracked stock
 * (overnight change %, freshest headline → original, next earnings/event). Empty
 * watchlist nudges the user to add stocks; auth is gated by the parent.
 */
function OverviewTab() {
  const {getToken} = useAuth();
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const [digest, setDigest] = useState<MyDigest | null>(null);
  const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading');

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    (async () => {
      try {
        const token = await getToken();
        const d = await getMyDigest(token, lang, c.signal);
        setDigest(d);
        setStatus('ready');
      } catch {
        if (!c.signal.aborted) setStatus('error');
      }
    })();
    return () => c.abort();
  }, [getToken, lang]);

  if (status === 'loading') {
    return (
      <div className="space-y-3">
        <div className={cx('h-24 rounded-2xl border', t.card, t.border, t.skel)} />
        <div className={cx('h-40 rounded-2xl border', t.card, t.border, t.skel)} />
      </div>
    );
  }
  if (status === 'error') {
    return (
      <div className={cx('rounded-2xl border p-6 text-center text-[13.5px]', t.card, t.border, t.sub)}>
        {tr('digest.error')}
      </div>
    );
  }

  const stocks = digest?.stocks ?? [];
  if (stocks.length === 0) {
    return (
      <div className={cx('rounded-2xl border p-8 text-center', t.card, t.border, t.soft)}>
        <Sunrise size={22} className={cx('mx-auto mb-2', dark ? 'text-amber-300' : 'text-amber-500')} />
        <p className={cx('text-[14px] font-semibold', t.text)}>{tr('digest.emptyTitle')}</p>
        <p className={cx('mt-1 text-[13px]', t.sub)}>{tr('digest.emptySub')}</p>
        <Link
          href="/me?tab=watchlist"
          className={cx('mt-4 inline-flex rounded-full px-4 py-2 text-[13px] font-semibold', btnPrimary(dark))}
        >
          {tr('digest.emptyCta')}
        </Link>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {digest?.summary && (
        <section className={cx('rounded-2xl border p-4', t.card, t.border, t.soft)}>
          <div className="mb-2 flex flex-wrap items-center gap-2">
            <h2 className={cx('flex items-center gap-1.5 text-[14px] font-bold', t.text)}>
              <Sparkles size={15} className={dark ? 'text-violet-300' : 'text-violet-500'} />
              {tr('digest.aiTitle')}
            </h2>
            <span
              className={cx(
                'rounded-md px-1.5 py-0.5 text-[10px] font-bold',
                dark ? 'bg-violet-500/15 text-violet-300' : 'bg-violet-50 text-violet-600',
              )}
            >
              {tr('ai.badge')}
            </span>
          </div>
          <Markdown>{digest.summary}</Markdown>
          <p className={cx('mt-2 text-[10.5px]', t.faint)}>{tr('ai.disclaimer')}</p>
        </section>
      )}
      <section className={cx('overflow-hidden rounded-2xl border', t.card, t.border)}>
        <div className={cx('border-b px-4 py-2.5 text-[12.5px] font-semibold', t.border, t.sub)}>
          {tr('digest.stocksTitle')}
        </div>
        {stocks.map((st, i) => (
          <DigestRow key={st.ticker} st={st} dark={dark} last={i === stocks.length - 1} />
        ))}
      </section>
    </div>
  );
}

/** The signed-in user's personal hub: overview / watchlist / holdings / notes / alerts tabs. */
function MeHub() {
  const {user, loading} = useAuth();
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const tab: Tab = isTab(params.get('tab')) ? (params.get('tab') as Tab) : 'overview';
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

      {tab === 'overview' && <OverviewTab />}
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
