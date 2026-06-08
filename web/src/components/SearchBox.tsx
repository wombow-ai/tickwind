'use client';

import {Search} from 'lucide-react';
import {useCallback, useEffect, useId, useRef, useState} from 'react';
import {searchSymbols, type SymbolMatch} from '@/lib/api';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

interface Props {
  /** Called with the chosen ticker (uppercased). */
  onSelect: (ticker: string) => void;
  /**
   * Called with the raw trimmed query when Enter is pressed without a
   * highlighted match. When provided it replaces the legacy "treat the raw
   * entry as a ticker" behaviour — callers route to a search-results page.
   */
  onSubmit?: (query: string) => void;
  placeholder?: string;
  autoFocus?: boolean;
  size?: 'sm' | 'md';
  /** Extra classes for the outer wrapper (width, visibility). */
  className?: string;
}

/**
 * A ticker/company search combobox: debounced calls to GET /v1/search render a
 * dropdown of matches (ticker + name + exchange); choosing one (click, Enter, or
 * arrow-keys) fires onSelect. On Enter with no highlighted match it calls
 * onSubmit (route to a search-results page) when provided, else falls back to
 * treating the raw entry as a ticker.
 */
export function SearchBox({
  onSelect,
  onSubmit,
  placeholder = 'Search a stock…',
  autoFocus,
  size = 'sm',
  className,
}: Props) {
  const dark = useDark();
  const t = tok(dark);
  const [q, setQ] = useState('');
  const [results, setResults] = useState<SymbolMatch[]>([]);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const boxRef = useRef<HTMLDivElement>(null);
  const listboxId = useId();
  const listOpen = open && results.length > 0;

  // Debounced search; aborts the in-flight request when the query changes.
  useEffect(() => {
    const query = q.trim();
    if (!query) {
      setResults([]);
      setOpen(false);
      return;
    }
    const ctrl = new AbortController();
    const id = setTimeout(() => {
      searchSymbols(query, 8, ctrl.signal).then(
        r => {
          setResults(r.results ?? []);
          setActive(0);
          setOpen(true);
        },
        () => {}, // ignore aborts / transient errors — keep last results
      );
    }, 140);
    return () => {
      clearTimeout(id);
      ctrl.abort();
    };
  }, [q]);

  // Click outside closes the dropdown.
  useEffect(() => {
    function onDoc(e: MouseEvent) {
      if (boxRef.current && !boxRef.current.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, []);

  const choose = useCallback(
    (ticker: string) => {
      onSelect(ticker.toUpperCase());
      setQ('');
      setResults([]);
      setOpen(false);
    },
    [onSelect],
  );

  function onKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setOpen(true);
      setActive(a => Math.min(a + 1, results.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setActive(a => Math.max(a - 1, 0));
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const pick = results[active];
      if (pick) {
        choose(pick.ticker);
      } else if (q.trim()) {
        const raw = q.trim();
        setQ('');
        setResults([]);
        setOpen(false);
        if (onSubmit) onSubmit(raw);
        else onSelect(raw.toUpperCase()); // legacy raw fallback
      }
    } else if (e.key === 'Escape') {
      setOpen(false);
    }
  }

  const pad = size === 'md' ? 'px-3.5 py-2.5 text-[14px]' : 'px-3 py-1.5 text-[13px]';

  return (
    <div ref={boxRef} className={cx('relative', className)}>
      <div className={cx('flex items-center gap-1.5 rounded-full border', t.border, t.surf2, pad)}>
        <Search size={size === 'md' ? 16 : 14} className={t.faint} />
        <input
          autoFocus={autoFocus}
          value={q}
          onChange={e => setQ(e.target.value)}
          onKeyDown={onKeyDown}
          onFocus={() => {
            if (results.length) setOpen(true);
          }}
          placeholder={placeholder}
          role="combobox"
          aria-expanded={listOpen}
          aria-controls={listboxId}
          aria-activedescendant={listOpen ? `${listboxId}-opt-${active}` : undefined}
          aria-autocomplete="list"
          className={cx(
            'w-full flex-1 bg-transparent outline-none',
            dark
              ? 'text-slate-100 placeholder:text-slate-500'
              : 'text-slate-900 placeholder:text-slate-400',
          )}
        />
      </div>

      {listOpen && (
        <ul
          id={listboxId}
          role="listbox"
          className={cx(
            'absolute z-50 mt-1.5 max-h-80 w-full min-w-[300px] overflow-auto rounded-2xl border p-1 shadow-lg',
            t.card,
            t.border,
          )}
        >
          {results.map((r, i) => (
            <li key={`${r.ticker}:${r.country}`}>
              <button
                type="button"
                id={`${listboxId}-opt-${i}`}
                // mousedown (not click) fires before the input blurs, so the
                // dropdown is still open when we read the choice.
                onMouseDown={e => {
                  e.preventDefault();
                  choose(r.ticker);
                }}
                onMouseEnter={() => setActive(i)}
                role="option"
                aria-selected={i === active}
                className={cx(
                  'flex w-full items-center gap-2 rounded-xl px-2.5 py-2 text-left',
                  i === active && (dark ? 'bg-slate-800' : 'bg-slate-100'),
                )}
              >
                <span className={cx('w-16 shrink-0 text-[13px] font-bold', t.text)}>
                  {r.ticker}
                </span>
                <span className={cx('flex-1 truncate text-[12.5px]', t.sub)}>{r.name}</span>
                <span className={cx('shrink-0 text-[10.5px]', t.faint)}>{r.exchange}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
