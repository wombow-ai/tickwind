'use client';

import {ChevronDown, Search} from 'lucide-react';
import {useMemo, useState} from 'react';
import type {Indicator, IndicatorFacets} from '@/lib/api';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

const DOMAINS = ['technical', 'fundamental', 'sentiment'] as const;
const PRIORITIES = ['P0', 'P1', 'P2'] as const;

/** Display order of the domain groups (technical → fundamental → sentiment). */
const DOMAIN_ORDER: Record<string, number> = {technical: 0, fundamental: 1, sentiment: 2};

/** Priority badge palette — P0 (core) is the most prominent. */
function priorityClasses(priority: string, dark: boolean): string {
  switch (priority) {
    case 'P0':
      return dark ? 'bg-teal-500/15 text-teal-300' : 'bg-teal-50 text-teal-700';
    case 'P1':
      return dark ? 'bg-sky-500/15 text-sky-300' : 'bg-sky-50 text-sky-700';
    default:
      return dark ? 'bg-slate-700/50 text-slate-300' : 'bg-slate-100 text-slate-500';
  }
}

/** Renders one indicator card. */
function IndicatorCard({ind}: {ind: Indicator}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const core = ind.priority === 'P0';

  // The non-English display name leads in the Chinese UI; otherwise English leads.
  const primary = lang === 'zh' && ind.name_zh ? ind.name_zh : ind.name_en;
  const secondary = lang === 'zh' && ind.name_zh ? ind.name_en : ind.name_zh;

  return (
    <article
      id={ind.id}
      className={cx(
        'rounded-2xl border p-4',
        t.card,
        core ? (dark ? 'border-teal-500/40' : 'border-teal-300') : t.border,
      )}
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <h3 className={cx('text-[14.5px] font-semibold leading-tight', t.text)}>
            {primary}
            {ind.abbr && (
              <span className={cx('ml-2 text-[12px] font-medium', t.sub)}>{ind.abbr}</span>
            )}
          </h3>
          {secondary && secondary !== primary && (
            <p className={cx('mt-0.5 text-[12px]', t.faint)}>{secondary}</p>
          )}
        </div>
        <span
          className={cx(
            'shrink-0 rounded-full px-2 py-0.5 text-[10.5px] font-semibold',
            priorityClasses(ind.priority, dark),
          )}
        >
          {tr(`ind.priority.${ind.priority}`)}
        </span>
      </div>

      {ind.definition && (
        <p className={cx('mt-2 text-[12.5px] leading-relaxed', t.sub)}>{ind.definition}</p>
      )}

      {ind.formula && (
        <div className="mt-3">
          <div className={cx('mb-1 text-[10.5px] font-semibold uppercase tracking-wide', t.faint)}>
            {tr('ind.formula')}
          </div>
          <pre
            className={cx(
              'overflow-x-auto whitespace-pre-wrap break-words rounded-lg px-2.5 py-2 font-mono text-[11.5px] leading-relaxed',
              t.surf2,
              t.text,
            )}
          >
            {ind.formula}
          </pre>
        </div>
      )}

      {ind.interpretation && (
        <div className="mt-3">
          <div className={cx('mb-1 text-[10.5px] font-semibold uppercase tracking-wide', t.faint)}>
            {tr('ind.interpretation')}
          </div>
          <p className={cx('text-[12.5px] leading-relaxed', t.sub)}>{ind.interpretation}</p>
        </div>
      )}

      {/* Metadata chips: output shape, inputs, library hint, default params. */}
      <div className="mt-3 flex flex-wrap gap-1.5">
        {ind.output_type && <MetaChip label={tr('ind.output')} value={ind.output_type} />}
        {ind.inputs && ind.inputs.length > 0 && (
          <MetaChip label={tr('ind.inputs')} value={ind.inputs.join(', ')} />
        )}
        {ind.talib_or_lib && <MetaChip label={tr('ind.lib')} value={ind.talib_or_lib} />}
        {ind.default_params != null && (
          <MetaChip label={tr('ind.params')} value={JSON.stringify(ind.default_params)} mono />
        )}
      </div>
    </article>
  );
}

/** A small key:value metadata chip. */
function MetaChip({label, value, mono}: {label: string; value: string; mono?: boolean}) {
  const dark = useDark();
  const t = tok(dark);
  return (
    <span
      className={cx('inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[10.5px]', t.surf2)}
    >
      <span className={t.faint}>{label}</span>
      <span className={cx(mono ? 'font-mono' : '', t.sub)}>{value}</span>
    </span>
  );
}

/** A collapsible domain section, grouped internally by subcategory. */
function DomainSection({
  domain,
  items,
  defaultOpen,
}: {
  domain: string;
  items: Indicator[];
  defaultOpen: boolean;
}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [open, setOpen] = useState(defaultOpen);

  // Group within the domain by subcategory, preserving first-seen order.
  const groups = useMemo(() => {
    const map = new Map<string, Indicator[]>();
    for (const ind of items) {
      const list = map.get(ind.subcategory) ?? [];
      list.push(ind);
      map.set(ind.subcategory, list);
    }
    return [...map.entries()];
  }, [items]);

  const domainName = items[0]?.domain_name ?? domain;

  return (
    <section className="mb-5">
      <button
        onClick={() => setOpen(o => !o)}
        aria-expanded={open}
        className={cx(
          'flex w-full items-center justify-between rounded-xl border px-4 py-2.5 text-left',
          t.card,
          t.border,
        )}
      >
        <span className="flex items-center gap-2">
          <span className={cx('text-[15px] font-bold', t.text)}>
            {tr(`ind.domain.${domain}`)}
          </span>
          <span className={cx('text-[12px]', t.faint)}>{domainName}</span>
          <span className={cx('rounded-full px-2 py-0.5 text-[11px] font-medium', t.chip, t.chipText)}>
            {items.length}
          </span>
        </span>
        <ChevronDown
          size={18}
          className={cx('transition-transform', t.sub, open ? '' : '-rotate-90')}
        />
      </button>

      {open && (
        <div className="mt-3 space-y-5">
          {groups.map(([subcat, list]) => (
            <div key={subcat}>
              <h4 className={cx('mb-2 px-1 text-[12px] font-semibold uppercase tracking-wide', t.faint)}>
                {subcat} · {list.length}
              </h4>
              <div className="grid gap-3 md:grid-cols-2">
                {list.map(ind => (
                  <IndicatorCard key={ind.id} ind={ind} />
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

/**
 * The browsable indicator library: client-side search + domain/priority filters
 * over the embedded stock-applicable catalog, grouped by domain → subcategory,
 * with P0 (core) indicators highlighted. The catalog is small enough (282) to
 * filter entirely on the client, so filters are instant with no extra requests.
 */
export function IndicatorLibrary({
  initial,
  facets,
  total,
}: {
  initial: Indicator[];
  facets: IndicatorFacets;
  total: number;
}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();

  const [query, setQuery] = useState('');
  const [domain, setDomain] = useState('');
  const [priority, setPriority] = useState('');

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return initial.filter(ind => {
      if (domain && ind.domain !== domain) return false;
      if (priority && ind.priority !== priority) return false;
      if (q) {
        const hay = `${ind.name_en} ${ind.name_zh} ${ind.abbr} ${ind.definition}`.toLowerCase();
        if (!hay.includes(q)) return false;
      }
      return true;
    });
  }, [initial, query, domain, priority]);

  // Group the filtered set by domain, in canonical order; P0-first within domain
  // surfaces the core indicators at the top of each group.
  const byDomain = useMemo(() => {
    const map = new Map<string, Indicator[]>();
    for (const ind of filtered) {
      const list = map.get(ind.domain) ?? [];
      list.push(ind);
      map.set(ind.domain, list);
    }
    for (const list of map.values()) {
      list.sort((a, b) => {
        const pa = PRIORITIES.indexOf(a.priority as (typeof PRIORITIES)[number]);
        const pb = PRIORITIES.indexOf(b.priority as (typeof PRIORITIES)[number]);
        if (pa !== pb) return pa - pb;
        return a.name_en.localeCompare(b.name_en);
      });
    }
    return [...map.entries()].sort((a, b) => (DOMAIN_ORDER[a[0]] ?? 9) - (DOMAIN_ORDER[b[0]] ?? 9));
  }, [filtered]);

  const facetCount = (kind: 'domains' | 'priorities', value: string): number =>
    facets[kind]?.find(f => f.value === value)?.count ?? 0;

  const hasFilters = !!(query || domain || priority);
  // When the user has narrowed down (search or a single domain), open all
  // sections; otherwise open only the first so the long list stays scannable.
  const expandAll = !!query || byDomain.length === 1;

  const chip = (active: boolean) =>
    cx(
      'rounded-full px-3 py-1.5 text-[12.5px] font-medium transition',
      active
        ? dark
          ? 'bg-teal-500/20 text-teal-200'
          : 'bg-teal-600 text-white'
        : cx(t.surf2, t.sub, 'hover:opacity-80'),
    );

  return (
    <div className="w-full">
      {/* Search box */}
      <div className={cx('mb-3 flex items-center gap-2 rounded-xl border px-3 py-2.5', t.card, t.border)}>
        <Search size={16} className={t.faint} />
        <input
          value={query}
          onChange={e => setQuery(e.target.value)}
          placeholder={tr('ind.search')}
          className={cx('w-full bg-transparent text-[13.5px] outline-none', t.text)}
          aria-label={tr('ind.search')}
        />
      </div>

      {/* Domain + priority filter chips */}
      <div className="mb-2 flex flex-wrap items-center gap-1.5">
        <button onClick={() => setDomain('')} className={chip(!domain)}>
          {tr('ind.allDomains')} · {total}
        </button>
        {DOMAINS.map(d => (
          <button key={d} onClick={() => setDomain(domain === d ? '' : d)} className={chip(domain === d)}>
            {tr(`ind.domain.${d}`)} · {facetCount('domains', d)}
          </button>
        ))}
      </div>
      <div className="mb-4 flex flex-wrap items-center gap-1.5">
        <button onClick={() => setPriority('')} className={chip(!priority)}>
          {tr('ind.allPriorities')}
        </button>
        {PRIORITIES.map(p => (
          <button
            key={p}
            onClick={() => setPriority(priority === p ? '' : p)}
            className={chip(priority === p)}
          >
            {tr(`ind.priority.${p}`)} · {facetCount('priorities', p)}
          </button>
        ))}
      </div>

      <div className="mb-4 flex items-center justify-between">
        <p className={cx('text-[12px]', t.faint)}>
          {tr('ind.matches').replace('{n}', String(filtered.length)).replace('{total}', String(total))}
        </p>
        {hasFilters && (
          <button
            onClick={() => {
              setQuery('');
              setDomain('');
              setPriority('');
            }}
            className={cx('text-[12px] font-medium', t.accentText)}
          >
            {tr('ind.clear')}
          </button>
        )}
      </div>

      {filtered.length === 0 ? (
        <div className={cx('rounded-2xl border px-6 py-12 text-center', t.card, t.border)}>
          <p className={cx('text-[14px] font-semibold', t.text)}>{tr('ind.empty')}</p>
          <p className={cx('mt-1 text-[12.5px]', t.sub)}>{tr('ind.emptySub')}</p>
        </div>
      ) : (
        byDomain.map(([d, items], i) => (
          <DomainSection key={d} domain={d} items={items} defaultOpen={expandAll || i === 0} />
        ))
      )}

      <p className={cx('mt-6 text-[11px]', t.faint)}>{tr('ind.footer')}</p>
    </div>
  );
}
