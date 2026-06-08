'use client';

import Link from 'next/link';
import {useEffect, useState} from 'react';
import {searchSymbols, type SymbolMatch} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, tok} from '@/lib/ui';

type State =
  | {status: 'idle'}
  | {status: 'loading'}
  | {status: 'ready'; results: SymbolMatch[]};

/**
 * Full search-results landing page (reached by pressing Enter in the nav search
 * box). Lists ticker/company matches; on zero matches it still offers a direct
 * "go to {TICKER}" link, so any valid symbol — including pink-sheet/OTC tickers
 * we don't index — stays reachable.
 */
export function SearchResults({q}: {q: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [state, setState] = useState<State>({status: 'idle'});

  const query = q.trim();
  useEffect(() => {
    if (!query) {
      setState({status: 'idle'});
      return;
    }
    const ctrl = new AbortController();
    setState({status: 'loading'});
    searchSymbols(query, 25, ctrl.signal).then(
      r => setState({status: 'ready', results: r.results ?? []}),
      () => {}, // ignore aborts / transient errors
    );
    return () => ctrl.abort();
  }, [query]);

  const upper = query.toUpperCase();

  return (
    <div className="mx-auto max-w-2xl">
      <h1 className={cx('mb-1 text-[22px] font-bold tracking-tight', t.text)}>{tr('search.title')}</h1>
      <p className={cx('mb-5 text-[13.5px]', t.sub)}>
        {query ? tr('search.for').replace('{q}', query) : tr('search.hint')}
      </p>

      {state.status === 'loading' && (
        <div className={cx('text-[13px]', t.faint)}>{tr('search.searching')}</div>
      )}

      {state.status === 'ready' && state.results.length === 0 && (
        <div className={cx('rounded-2xl border p-6 text-center', t.card, t.border, t.soft)}>
          <p className={cx('text-[14px] font-medium', t.text)}>
            {tr('search.empty').replace('{q}', query)}
          </p>
          <p className={cx('mx-auto mt-1 max-w-sm text-[12.5px]', t.sub)}>{tr('search.emptyHint')}</p>
          <Link
            href={`/stock/${encodeURIComponent(upper)}`}
            className={cx(
              'mt-4 inline-flex items-center gap-1.5 rounded-full px-4 py-2 text-[13px] font-semibold',
              btnPrimary(dark),
            )}
          >
            {tr('search.gotoAnyway').replace('{q}', upper)} →
          </Link>
        </div>
      )}

      {state.status === 'ready' && state.results.length > 0 && (
        <ul className={cx('overflow-hidden rounded-2xl border', t.card, t.border, t.soft)}>
          {state.results.map((r, i) => (
            <li key={`${r.ticker}:${r.country}`}>
              <Link
                href={`/stock/${encodeURIComponent(r.ticker)}`}
                className={cx(
                  'flex items-center gap-3 px-4 py-3 transition',
                  i > 0 && cx('border-t', t.border),
                  dark ? 'hover:bg-slate-800/60' : 'hover:bg-slate-50',
                )}
              >
                <span className={cx('w-20 shrink-0 text-[14px] font-bold', t.text)}>{r.ticker}</span>
                <span className={cx('flex-1 truncate text-[13px]', t.sub)}>{r.name}</span>
                <span className={cx('shrink-0 text-[11px]', t.faint)}>{r.exchange}</span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
