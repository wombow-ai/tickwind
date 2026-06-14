'use client';

import {SlidersHorizontal} from 'lucide-react';
import Link from '@/components/LocalLink';
import {useEffect, useState} from 'react';
import {getScreen, type ScreenParams, type ScreenResult} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, tok} from '@/lib/ui';
import {SessionBadge} from '@/components/ui/atoms';
import {EmptyState, ErrorState, FeedSkeleton} from '@/components/ui/states';

type Status = 'loading' | 'ready' | 'error';

const SORTS = ['change_desc', 'change_asc', 'price_desc', 'price_asc'] as const;
const SESSIONS = ['', 'regular', 'pre', 'post', 'overnight'] as const;

/**
 * The stock screener: filter the whole-US universe (delayed IEX quotes) by price,
 * daily %-change, and session, sorted. Apply-button driven (one request per run).
 */
export function Screener() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();

  const [minPrice, setMinPrice] = useState('');
  const [maxPrice, setMaxPrice] = useState('');
  const [minChange, setMinChange] = useState('');
  const [maxChange, setMaxChange] = useState('');
  const [session, setSession] = useState('');
  const [sort, setSort] = useState('change_desc');

  const [status, setStatus] = useState<Status>('loading');
  const [results, setResults] = useState<ScreenResult[]>([]);

  function fetchWith(params: ScreenParams, signal?: AbortSignal) {
    setStatus('loading');
    getScreen({...params, limit: 100}, signal).then(
      r => {
        setResults(r.results ?? []);
        setStatus('ready');
      },
      () => setStatus('error'),
    );
  }

  // Default screen (top gainers) on mount.
  useEffect(() => {
    const c = new AbortController();
    fetchWith({sort: 'change_desc'}, c.signal);
    return () => c.abort();
  }, []);

  function currentParams(): ScreenParams {
    const num = (s: string) => {
      const v = s.trim();
      if (!v) return undefined;
      const n = Number(v);
      return Number.isNaN(n) ? undefined : n;
    };
    return {
      minPrice: num(minPrice),
      maxPrice: num(maxPrice),
      minChange: num(minChange),
      maxChange: num(maxChange),
      session: session || undefined,
      sort,
    };
  }

  const inputCls = cx(
    'w-full rounded-lg border bg-transparent px-2.5 py-1.5 text-[13px] outline-none',
    t.border,
    dark ? 'text-slate-100 placeholder:text-slate-500' : 'text-slate-900 placeholder:text-slate-400',
  );
  const labelCls = cx('mb-1 block text-[11px] font-medium', t.faint);

  return (
    <div className="w-full">
      <header className="mb-4">
        <h1 className={cx('flex items-center gap-2 text-[22px] font-bold tracking-tight', t.text)}>
          <SlidersHorizontal size={20} className={dark ? 'text-sky-300' : 'text-sky-600'} />
          {tr('screen.title')}
        </h1>
        <p className={cx('mt-1 text-[13.5px]', t.sub)}>{tr('screen.subtitle')}</p>
      </header>

      <form
        onSubmit={e => {
          e.preventDefault();
          fetchWith(currentParams());
        }}
        className={cx('mb-5 rounded-2xl border p-4', t.card, t.border, t.soft)}
      >
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
          <div>
            <label className={labelCls}>{tr('screen.minPrice')}</label>
            <input type="number" inputMode="decimal" step="any" value={minPrice} onChange={e => setMinPrice(e.target.value)} placeholder="0" className={inputCls} />
          </div>
          <div>
            <label className={labelCls}>{tr('screen.maxPrice')}</label>
            <input type="number" inputMode="decimal" step="any" value={maxPrice} onChange={e => setMaxPrice(e.target.value)} placeholder="∞" className={inputCls} />
          </div>
          <div>
            <label className={labelCls}>{tr('screen.minChange')}</label>
            <input type="number" inputMode="decimal" step="any" value={minChange} onChange={e => setMinChange(e.target.value)} placeholder="%" className={inputCls} />
          </div>
          <div>
            <label className={labelCls}>{tr('screen.maxChange')}</label>
            <input type="number" inputMode="decimal" step="any" value={maxChange} onChange={e => setMaxChange(e.target.value)} placeholder="%" className={inputCls} />
          </div>
          <div>
            <label className={labelCls}>{tr('screen.session')}</label>
            <select value={session} onChange={e => setSession(e.target.value)} className={inputCls}>
              {SESSIONS.map(s => (
                <option key={s || 'all'} value={s}>
                  {s ? tr(`screen.session.${s}`) : tr('screen.sessionAll')}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className={labelCls}>{tr('screen.sort')}</label>
            <select value={sort} onChange={e => setSort(e.target.value)} className={inputCls}>
              {SORTS.map(s => (
                <option key={s} value={s}>
                  {tr(`screen.sort.${s}`)}
                </option>
              ))}
            </select>
          </div>
        </div>
        <div className="mt-3 flex items-center justify-between gap-3">
          <span className={cx('text-[11px]', t.faint)}>{tr('screen.delayed')}</span>
          <button type="submit" className={cx('rounded-full px-4 py-1.5 text-[13px] font-semibold', btnPrimary(dark))}>
            {tr('screen.apply')}
          </button>
        </div>
      </form>

      {status === 'loading' && <FeedSkeleton />}
      {status === 'error' && <ErrorState onRetry={() => fetchWith(currentParams())} />}
      {status === 'ready' && results.length === 0 && (
        <EmptyState label={tr('screen.empty')} sub={tr('screen.emptySub')} icon={SlidersHorizontal} />
      )}
      {status === 'ready' && results.length > 0 && (
        <div className={cx('tw-fade overflow-hidden rounded-2xl border', t.card, t.border, t.soft)}>
          {/* header row */}
          <div className={cx('flex items-center gap-3 border-b px-4 py-2 text-[11px] font-semibold uppercase tracking-wide', t.border, t.faint)}>
            <span className="w-24">{tr('screen.colTicker')}</span>
            <span className="flex-1 text-right tabular-nums">{tr('screen.colPrice')}</span>
            <span className="w-24 text-right tabular-nums">{tr('screen.colChange')}</span>
            <span className="hidden w-24 text-right sm:block">{tr('screen.colSession')}</span>
          </div>
          {results.map((r, i) => (
            <Row key={r.ticker} r={r} t={t} dark={dark} last={i === results.length - 1} />
          ))}
        </div>
      )}

      <p className={cx('mt-4 text-center text-[11px]', t.faint)}>{tr('screen.footer')}</p>
    </div>
  );
}

function Row({
  r,
  t,
  dark,
  last,
}: {
  r: ScreenResult;
  t: ReturnType<typeof tok>;
  dark: boolean;
  last: boolean;
}) {
  const pos = r.change_pct != null && r.change_pct >= 0;
  const chgColor =
    r.change_pct == null
      ? t.faint
      : pos
        ? dark
          ? 'text-emerald-400'
          : 'text-emerald-600'
        : dark
          ? 'text-rose-400'
          : 'text-rose-500';
  return (
    <Link
      href={`/stock/${encodeURIComponent(r.ticker)}`}
      className={cx(
        'flex items-center gap-3 px-4 py-2.5 text-[13.5px] transition hover:opacity-80',
        last ? '' : cx('border-b', t.border),
      )}
    >
      <span className={cx('w-24 font-bold', t.text)}>{r.ticker}</span>
      <span className={cx('flex-1 text-right font-semibold tabular-nums', t.text)}>
        ${r.price.toFixed(2)}
      </span>
      <span className={cx('w-24 text-right font-semibold tabular-nums', chgColor)}>
        {r.change_pct == null ? '—' : `${pos ? '+' : ''}${r.change_pct.toFixed(2)}%`}
      </span>
      <span className="hidden w-24 justify-end sm:flex">
        <SessionBadge session={r.session} sm />
      </span>
    </Link>
  );
}
