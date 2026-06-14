'use client';

import {ArrowDown, ArrowUp, RotateCcw, Search, X} from 'lucide-react';
import {useEffect, useMemo, useRef, useState} from 'react';
import {type StockIndicator} from '@/lib/api';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, fmtPrice, tok} from '@/lib/ui';

type Tokens = ReturnType<typeof tok>;

const DOMAINS = ['technical', 'fundamental', 'sentiment'] as const;

/** Display order of the domain groups (technical → fundamental → sentiment). */
const DOMAIN_ORDER: Record<string, number> = {technical: 0, fundamental: 1, sentiment: 2};

/**
 * Formats an indicator's headline value by its unit, mirroring the panel's
 * formatter (a row preview, so the user can prefer ids that actually compute).
 */
function fmtValue(value: number, unit?: string): string {
  switch (unit) {
    case '%':
      return `${trim(value)}%`;
    case 'x':
      return `${trim(value)}x`;
    case 'price':
      return fmtPrice('$', value);
    default:
      return trim(value);
  }
}

/** Trims a number to at most two decimals without trailing zeros. */
function trim(value: number): string {
  const abs = Math.abs(value);
  const decimals = abs >= 100 ? 1 : 2;
  return Number(value.toFixed(decimals)).toLocaleString('en-US', {
    maximumFractionDigits: decimals,
  });
}

/**
 * The indicator picker modal: add/remove, reorder (up/down, no drag dependency),
 * search (name_en / name_zh / abbr, case-insensitive), a domain filter chip row,
 * and "Reset to default". Lists ALL available indicators from the payload —
 * grouped by domain — except `unsupported` (hidden so users prefer computable
 * ids). Closes on Escape / backdrop click; focuses the search on open. The
 * selection is owned by the parent panel; every change flows back via
 * `onChange`, and the parent debounces persistence.
 */
export function IndicatorPicker({
  indicators,
  selected,
  onChange,
  onReset,
  onClose,
}: {
  /** All indicators the panel received (the full available set for this stock). */
  indicators: StockIndicator[];
  /** Currently selected ids, in display order. */
  selected: string[];
  /** Called with the next ordered id list on any add/remove/reorder. */
  onChange: (ids: string[]) => void;
  /** Called when the user resets to the P0 default (clears the saved selection). */
  onReset: () => void;
  /** Closes the modal. */
  onClose: () => void;
}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const searchRef = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState('');
  const [domain, setDomain] = useState('');

  // Escape closes; focus the search on open (accessibility).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', onKey);
    searchRef.current?.focus();
    return () => document.removeEventListener('keydown', onKey);
  }, [onClose]);

  const selectedSet = useMemo(() => new Set(selected), [selected]);

  // Index by id so the selected list can render in its saved order.
  const byId = useMemo(() => {
    const m = new Map<string, StockIndicator>();
    for (const ind of indicators) m.set(ind.id, ind);
    return m;
  }, [indicators]);

  // The available menu: every non-`unsupported` indicator, grouped by domain in
  // canonical order, filtered by the search + domain chip. Within a domain the
  // payload order is preserved.
  const groups = useMemo(() => {
    const q = query.trim().toLowerCase();
    const map = new Map<string, StockIndicator[]>();
    for (const ind of indicators) {
      if (ind.status === 'unsupported') continue;
      if (domain && ind.domain !== domain) continue;
      if (q) {
        const hay = `${ind.name_en} ${ind.name_zh} ${ind.abbr}`.toLowerCase();
        if (!hay.includes(q)) continue;
      }
      const list = map.get(ind.domain) ?? [];
      list.push(ind);
      map.set(ind.domain, list);
    }
    return [...map.entries()].sort(
      (a, b) => (DOMAIN_ORDER[a[0]] ?? 9) - (DOMAIN_ORDER[b[0]] ?? 9),
    );
  }, [indicators, query, domain]);

  // The selected indicators that still exist in the payload, in saved order.
  const selectedItems = useMemo(
    () => selected.map(id => byId.get(id)).filter((x): x is StockIndicator => !!x),
    [selected, byId],
  );

  const toggle = (id: string) => {
    if (selectedSet.has(id)) {
      onChange(selected.filter(x => x !== id));
    } else {
      onChange([...selected, id]);
    }
  };

  const move = (index: number, dir: -1 | 1) => {
    const next = [...selected];
    const target = index + dir;
    if (target < 0 || target >= next.length) return;
    [next[index], next[target]] = [next[target], next[index]];
    onChange(next);
  };

  const matchCount = groups.reduce((n, [, list]) => n + list.length, 0);

  const chip = (active: boolean) =>
    cx(
      'rounded-full px-3 py-1.5 text-[12px] font-medium transition',
      active
        ? dark
          ? 'bg-teal-500/20 text-teal-200'
          : 'bg-teal-600 text-white'
        : cx(t.surf2, t.sub, 'hover:opacity-80'),
    );

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/40 p-4 sm:items-center"
      role="dialog"
      aria-modal="true"
      aria-label={tr('ind2.picker.title')}
    >
      {/* Backdrop click closes. */}
      <button
        className="absolute inset-0 -z-10 cursor-default"
        aria-label={tr('ind2.picker.close')}
        onClick={onClose}
        tabIndex={-1}
      />
      <div
        className={cx(
          'my-4 w-full max-w-lg rounded-2xl border shadow-xl',
          t.card,
          t.border,
        )}
      >
        {/* Header */}
        <div className={cx('flex items-center gap-2 border-b px-4 py-3', t.border)}>
          <h2 className={cx('text-[14px] font-bold', t.text)}>{tr('ind2.picker.title')}</h2>
          <span className={cx('text-[11px]', t.faint)}>
            {tr('ind2.picker.count').replace('{n}', String(selected.length))}
          </span>
          <button
            onClick={onClose}
            aria-label={tr('ind2.picker.close')}
            className={cx('ml-auto rounded-lg p-1', t.ghost)}
          >
            <X size={16} />
          </button>
        </div>

        <div className="max-h-[70vh] overflow-y-auto px-4 py-3">
          {/* Selected list (reorder + remove) */}
          <div className="mb-4">
            <h3 className={cx('mb-2 text-[11px] font-semibold uppercase tracking-wide', t.faint)}>
              {tr('ind2.picker.selected')}
            </h3>
            {selectedItems.length === 0 ? (
              <p className={cx('rounded-lg px-3 py-3 text-[12px]', t.surf2, t.sub)}>
                {tr('ind2.picker.empty')}
              </p>
            ) : (
              <ul className={cx('divide-y rounded-xl border', t.hair, t.border)}>
                {selectedItems.map((ind, i) => (
                  <SelectedRow
                    key={ind.id}
                    ind={ind}
                    isFirst={i === 0}
                    isLast={i === selectedItems.length - 1}
                    onUp={() => move(i, -1)}
                    onDown={() => move(i, 1)}
                    onRemove={() => toggle(ind.id)}
                    dark={dark}
                    t={t}
                    tr={tr}
                  />
                ))}
              </ul>
            )}
          </div>

          {/* Search */}
          <div className={cx('mb-2 flex items-center gap-2 rounded-xl border px-3 py-2', t.card, t.border)}>
            <Search size={15} className={t.faint} />
            <input
              ref={searchRef}
              value={query}
              onChange={e => setQuery(e.target.value)}
              placeholder={tr('ind2.picker.search')}
              className={cx('w-full bg-transparent text-[13px] outline-none', t.text)}
              aria-label={tr('ind2.picker.search')}
            />
          </div>

          {/* Domain filter chips */}
          <div className="mb-3 flex flex-wrap items-center gap-1.5">
            <button onClick={() => setDomain('')} className={chip(!domain)}>
              {tr('ind2.picker.allDomains')}
            </button>
            {DOMAINS.map(d => (
              <button key={d} onClick={() => setDomain(domain === d ? '' : d)} className={chip(domain === d)}>
                {tr(`ind2.domain.${d}`)}
              </button>
            ))}
          </div>

          {/* Available menu, grouped by domain */}
          {matchCount === 0 ? (
            <p className={cx('rounded-lg px-3 py-4 text-center text-[12px]', t.surf2, t.sub)}>
              {tr('ind2.picker.noMatch')}
            </p>
          ) : (
            <div className="space-y-3">
              {groups.map(([d, items]) => (
                <div key={d}>
                  <h3 className={cx('mb-1.5 text-[11px] font-semibold uppercase tracking-wide', t.faint)}>
                    {tr(`ind2.domain.${d}`)}
                  </h3>
                  <ul className={cx('divide-y rounded-xl border', t.hair, t.border)}>
                    {items.map(ind => (
                      <AvailableRow
                        key={ind.id}
                        ind={ind}
                        checked={selectedSet.has(ind.id)}
                        onToggle={() => toggle(ind.id)}
                        dark={dark}
                        t={t}
                      />
                    ))}
                  </ul>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className={cx('flex items-center justify-between border-t px-4 py-3', t.border)}>
          <button
            onClick={onReset}
            className={cx('inline-flex items-center gap-1.5 text-[12px] font-medium', t.accentText)}
          >
            <RotateCcw size={13} />
            {tr('ind2.picker.reset')}
          </button>
          <span className={cx('text-[11px]', t.faint)}>
            {tr('ind2.picker.count').replace('{n}', String(selected.length))}
          </span>
        </div>
      </div>
    </div>
  );
}

/** One row in the selected list: name + up/down reorder + remove. */
function SelectedRow({
  ind,
  isFirst,
  isLast,
  onUp,
  onDown,
  onRemove,
  dark,
  t,
  tr,
}: {
  ind: StockIndicator;
  isFirst: boolean;
  isLast: boolean;
  onUp: () => void;
  onDown: () => void;
  onRemove: () => void;
  dark: boolean;
  t: Tokens;
  tr: (key: string) => string;
}) {
  const {lang} = useLang();
  const name = lang === 'zh' && ind.name_zh ? ind.name_zh : ind.name_en;
  return (
    <li className="flex items-center gap-2 px-3 py-2">
      <span className="flex flex-col">
        <button
          onClick={onUp}
          disabled={isFirst}
          aria-label={tr('ind2.picker.moveUp')}
          className={cx('rounded p-0.5 disabled:opacity-30', t.ghost)}
        >
          <ArrowUp size={13} />
        </button>
        <button
          onClick={onDown}
          disabled={isLast}
          aria-label={tr('ind2.picker.moveDown')}
          className={cx('rounded p-0.5 disabled:opacity-30', t.ghost)}
        >
          <ArrowDown size={13} />
        </button>
      </span>
      <span className={cx('min-w-0 flex-1 truncate text-[13px] font-medium', t.text)}>
        {name}
        {ind.abbr && <span className={cx('ml-1.5 text-[11px]', t.sub)}>{ind.abbr}</span>}
      </span>
      <StatusHint ind={ind} dark={dark} t={t} />
      <button
        onClick={onRemove}
        aria-label={tr('ind2.picker.remove')}
        className={cx('rounded-lg p-1', t.ghost)}
      >
        <X size={14} />
      </button>
    </li>
  );
}

/** One row in the available menu: a checkbox toggle + name + status hint. */
function AvailableRow({
  ind,
  checked,
  onToggle,
  dark,
  t,
}: {
  ind: StockIndicator;
  checked: boolean;
  onToggle: () => void;
  dark: boolean;
  t: Tokens;
}) {
  const {lang} = useLang();
  const name = lang === 'zh' && ind.name_zh ? ind.name_zh : ind.name_en;
  return (
    <li>
      <label className="flex cursor-pointer items-center gap-2.5 px-3 py-2">
        <input
          type="checkbox"
          checked={checked}
          onChange={onToggle}
          className="size-4 shrink-0 accent-teal-500"
        />
        <span className={cx('min-w-0 flex-1 truncate text-[13px]', t.text)}>
          {name}
          {ind.abbr && <span className={cx('ml-1.5 text-[11px]', t.sub)}>{ind.abbr}</span>}
        </span>
        <StatusHint ind={ind} dark={dark} t={t} />
      </label>
    </li>
  );
}

/** A small per-row status hint: the ok value preview, or a muted "—". */
function StatusHint({ind, dark, t}: {ind: StockIndicator; dark: boolean; t: Tokens}) {
  if (ind.status === 'ok' && ind.value != null) {
    return (
      <span className={cx('shrink-0 text-[12px] font-semibold tabular-nums', t.sub)}>
        {fmtValue(ind.value, ind.unit)}
      </span>
    );
  }
  return (
    <span
      className={cx(
        'shrink-0 text-[12px] font-semibold tabular-nums',
        dark ? 'text-slate-600' : 'text-slate-300',
      )}
    >
      —
    </span>
  );
}
