'use client';

import {FileText, Landmark} from 'lucide-react';
import Link from '@/components/LocalLink';
import {useCallback, useEffect, useState} from 'react';
import {getInstitutional, type InstitutionalFiling} from '@/lib/api';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';
import {EmptyState, ErrorState, FeedSkeleton} from '@/components/ui/states';

type Tokens = ReturnType<typeof tok>;
type Status = 'loading' | 'ready' | 'error';
type Filter = '' | '13d' | '13g';

/** "20260608" → localized date; leaves anything unexpected untouched. */
function fmtFiledDate(yyyymmdd: string, locale: string): string {
  if (!/^\d{8}$/.test(yyyymmdd)) return yyyymmdd;
  const dt = new Date(
    Date.UTC(+yyyymmdd.slice(0, 4), +yyyymmdd.slice(4, 6) - 1, +yyyymmdd.slice(6, 8)),
  );
  if (Number.isNaN(dt.getTime())) return yyyymmdd;
  return dt.toLocaleDateString(locale, {year: 'numeric', month: 'short', day: 'numeric'});
}

/**
 * The institutional / activist board: recent SEC Schedule 13D/13G beneficial-
 * ownership filings — who took a >5% stake in what. 13D = active/activist
 * (higher signal); 13G = passive (e.g. the index giants). Sourced facts only,
 * each row links to the official SEC filing; not a recommendation.
 */
export function InstitutionalBoard() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const [filter, setFilter] = useState<Filter>('');
  const [status, setStatus] = useState<Status>('loading');
  const [filings, setFilings] = useState<InstitutionalFiling[]>([]);

  const load = useCallback((f: Filter) => {
    setStatus('loading');
    getInstitutional({type: f || undefined, limit: 100}).then(
      r => {
        setFilings(r.filings ?? []);
        setStatus('ready');
      },
      () => setStatus('error'),
    );
  }, []);

  useEffect(() => {
    load(filter);
  }, [load, filter]);

  const locale = lang === 'zh' ? 'zh-CN' : 'en-US';
  const tabs: {k: Filter; label: string}[] = [
    {k: '', label: tr('inst.all')},
    {k: '13d', label: tr('inst.activist')},
    {k: '13g', label: tr('inst.passive')},
  ];

  return (
    <div className="w-full">
      <header className="mb-4">
        <h1 className={cx('flex items-center gap-2 text-[22px] font-bold tracking-tight', t.text)}>
          <Landmark size={20} className={dark ? 'text-sky-300' : 'text-sky-600'} />
          {tr('inst.title')}
        </h1>
        <p className={cx('mt-1 text-[13.5px]', t.sub)}>{tr('inst.subtitle')}</p>
      </header>

      <div className={cx('mb-4 rounded-xl border p-3 text-[12px]', t.border, dark ? 'bg-slate-900' : 'bg-slate-50', t.sub)}>
        {tr('inst.disclaimer')}
      </div>

      <div className={cx('mb-4 inline-flex items-center gap-1 rounded-xl border p-1', t.border, t.surf2)}>
        {tabs.map(tb => (
          <button
            key={tb.k || 'all'}
            onClick={() => setFilter(tb.k)}
            aria-pressed={filter === tb.k}
            className={cx(
              'rounded-lg px-3 py-1.5 text-[13px] font-medium transition',
              filter === tb.k
                ? dark
                  ? 'bg-slate-700 text-white'
                  : 'bg-white text-slate-900 shadow-sm'
                : t.sub,
            )}
          >
            {tb.label}
          </button>
        ))}
      </div>

      {status === 'loading' && <FeedSkeleton />}
      {status === 'error' && <ErrorState onRetry={() => load(filter)} />}
      {status === 'ready' && filings.length === 0 && (
        <EmptyState label={tr('inst.empty')} sub={tr('inst.emptySub')} icon={Landmark} />
      )}
      {status === 'ready' && filings.length > 0 && (
        <div className="tw-fade space-y-2.5">
          {filings.map(f => (
            <Row key={f.accession} f={f} t={t} dark={dark} tr={tr} locale={locale} />
          ))}
        </div>
      )}

      <p className={cx('mt-4 text-center text-[11px]', t.faint)}>{tr('inst.footer')}</p>
    </div>
  );
}

function Row({
  f,
  t,
  dark,
  tr,
  locale,
}: {
  f: InstitutionalFiling;
  t: Tokens;
  dark: boolean;
  tr: (k: string) => string;
  locale: string;
}) {
  const url = `https://www.sec.gov/Archives/edgar/data/${f.cik}/${f.accession.replace(/-/g, '')}/`;
  const badge = f.activist
    ? dark
      ? 'bg-amber-500/15 text-amber-300'
      : 'bg-amber-100 text-amber-700'
    : dark
      ? 'bg-slate-800 text-slate-300'
      : 'bg-slate-100 text-slate-500';
  return (
    <section className={cx('rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1.5">
        {f.filer ? (
          <span className={cx('text-[14.5px] font-bold', t.text)}>{f.filer}</span>
        ) : (
          <span className={cx('text-[13px] italic', t.faint)}>{tr('inst.unknownFiler')}</span>
        )}
        <span className={cx('text-[12px]', t.faint)}>→</span>
        {f.ticker ? (
          <Link
            href={`/stock/${encodeURIComponent(f.ticker)}`}
            className={cx('text-[13.5px] font-semibold hover:underline', t.accentText)}
          >
            {f.company}
          </Link>
        ) : (
          <span className={cx('text-[13.5px] font-semibold', t.sub)}>{f.company}</span>
        )}
        <span className={cx('rounded-full px-2 py-0.5 text-[10.5px] font-semibold', badge)}>
          {f.activist ? tr('inst.activist') : tr('inst.passive')} · {f.form_type}
        </span>
        <span className="ml-auto flex items-center gap-2.5">
          <span className={cx('text-[12px] tabular-nums', t.faint)}>
            {tr('inst.filed')} {fmtFiledDate(f.filed_date, locale)}
          </span>
          <a
            href={url}
            target="_blank"
            rel="noopener noreferrer"
            className={cx('inline-flex items-center gap-1 text-[12px] font-semibold', t.accentText)}
          >
            <FileText size={12} /> {tr('inst.viewFiling')}
          </a>
        </span>
      </div>
    </section>
  );
}
