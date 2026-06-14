'use client';

import {FileText, Landmark, Users} from 'lucide-react';
import Link from '@/components/LocalLink';
import {useCallback, useEffect, useState} from 'react';
import {congressSlug, getCongress, type CongressFiling} from '@/lib/api';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';
import {EmptyState, ErrorState, FeedSkeleton} from '@/components/ui/states';

type Tokens = ReturnType<typeof tok>;
type Status = 'loading' | 'ready' | 'error';

/**
 * The Congress trading board: the latest U.S. House Periodic Transaction Reports
 * (PTRs) — members' disclosed stock trades — from the official, public-domain
 * House Clerk dataset. Presented as sourced facts: each row is a filing with a
 * link to the official PDF, never a recommendation. Ticker-level detail lives in
 * the PDF (parsing deferred), so v1 surfaces who filed + when + the official link.
 */
export function CongressBoard() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const [status, setStatus] = useState<Status>('loading');
  const [filings, setFilings] = useState<CongressFiling[]>([]);

  const load = useCallback(() => {
    setStatus('loading');
    getCongress(80).then(
      r => {
        setFilings(r.filings ?? []);
        setStatus('ready');
      },
      () => setStatus('error'),
    );
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const locale = lang === 'zh' ? 'zh-CN' : 'en-US';

  return (
    <div className="w-full">
      <header className="mb-4">
        <h1
          className={cx(
            'flex items-center gap-2 text-[22px] font-bold tracking-tight',
            t.text,
          )}
        >
          <Landmark size={20} className={dark ? 'text-sky-300' : 'text-sky-600'} />
          {tr('congress.title')}
        </h1>
        <p className={cx('mt-1 text-[13.5px]', t.sub)}>{tr('congress.subtitle')}</p>
      </header>

      <div
        className={cx(
          'mb-5 rounded-xl border p-3 text-[12px]',
          t.border,
          dark ? 'bg-slate-900' : 'bg-slate-50',
          t.sub,
        )}
      >
        {tr('congress.disclaimer')}
      </div>

      {status === 'loading' && <FeedSkeleton />}
      {status === 'error' && <ErrorState onRetry={load} />}
      {status === 'ready' && filings.length === 0 && (
        <EmptyState
          label={tr('congress.empty')}
          sub={tr('congress.emptySub')}
          icon={Users}
        />
      )}
      {status === 'ready' && filings.length > 0 && (
        <div className="tw-fade space-y-2.5">
          {filings.map(f => (
            <CongressCard key={f.doc_id} f={f} dark={dark} t={t} locale={locale} />
          ))}
        </div>
      )}

      <p className={cx('mt-4 text-center text-[11px]', t.faint)}>
        {tr('congress.footer')}
      </p>
    </div>
  );
}

function CongressCard({
  f,
  dark,
  t,
  locale,
}: {
  f: CongressFiling;
  dark: boolean;
  t: Tokens;
  locale: string;
}) {
  const tr = useT();
  const place = f.district ? `${f.state}-${f.district}` : f.state;
  let filed = f.filed_date;
  const d = new Date(f.filed_date);
  if (!Number.isNaN(d.getTime())) {
    filed = d.toLocaleDateString(locale, {year: 'numeric', month: 'short', day: 'numeric'});
  }
  return (
    <section className={cx('rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
        <Link
          href={`/congress/member/${congressSlug(f.name)}`}
          className={cx('text-[15px] font-bold hover:underline', t.text)}
        >
          {f.name}
        </Link>
        {place && (
          <span
            className={cx(
              'rounded-full px-2 py-0.5 text-[10.5px] font-semibold',
              dark ? 'bg-slate-800 text-slate-300' : 'bg-slate-100 text-slate-500',
            )}
          >
            {place}
          </span>
        )}
        <span className={cx('text-[12.5px] tabular-nums', t.faint)}>
          {tr('congress.filed')} {filed}
        </span>
        {f.pdf_url && (
          <a
            href={f.pdf_url}
            target="_blank"
            rel="noopener noreferrer"
            className={cx(
              'ml-auto inline-flex items-center gap-1 text-[12px] font-semibold',
              t.accentText,
            )}
          >
            <FileText size={12} /> {tr('congress.viewFiling')}
          </a>
        )}
      </div>
    </section>
  );
}
