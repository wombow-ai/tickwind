'use client';

import {ExternalLink, Mic} from 'lucide-react';
import Link from 'next/link';
import {useCallback, useEffect, useState} from 'react';
import {getGurus, type GuruItem} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';
import {EmptyState, ErrorState, FeedSkeleton} from '@/components/ui/states';

type Tokens = ReturnType<typeof tok>;
type Status = 'loading' | 'ready' | 'error';

function ago(iso: string): string {
  const then = new Date(iso).getTime();
  if (!Number.isFinite(then) || then <= 0) return '';
  const mins = Math.max(0, Math.round((Date.now() - then) / 60000));
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.round(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  return `${Math.round(hrs / 24)}d ago`;
}

/**
 * The Guru-watch rail: recent posts from curated finance writers (KOLs, e.g.
 * Serenity) with the tickers each mentions. Opinions for context — every row is
 * attributed and links to the source; never a recommendation.
 */
export function GuruRail() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [status, setStatus] = useState<Status>('loading');
  const [items, setItems] = useState<GuruItem[]>([]);

  const load = useCallback(() => {
    setStatus('loading');
    getGurus(30).then(
      r => {
        setItems(r.items ?? []);
        setStatus('ready');
      },
      () => setStatus('error'),
    );
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <div className="w-full">
      <header className="mb-4">
        <h2
          className={cx(
            'flex items-center gap-2 text-[22px] font-bold tracking-tight',
            t.text,
          )}
        >
          <Mic size={20} className={dark ? 'text-violet-300' : 'text-violet-600'} />
          {tr('guru.title')}
        </h2>
        <p className={cx('mt-1 text-[13.5px]', t.sub)}>{tr('guru.subtitle')}</p>
      </header>

      {status === 'loading' && <FeedSkeleton />}
      {status === 'error' && <ErrorState onRetry={load} />}
      {status === 'ready' && items.length === 0 && (
        <EmptyState
          label={tr('guru.empty')}
          sub={tr('guru.emptySub')}
          icon={Mic}
        />
      )}
      {status === 'ready' && items.length > 0 && (
        <div className="tw-fade space-y-3">
          {items.map((g, i) => (
            <GuruCard key={g.url || i} g={g} dark={dark} t={t} />
          ))}
        </div>
      )}

      <p className={cx('mt-4 text-center text-[11px]', t.faint)}>
        {tr('guru.footer')}
      </p>
    </div>
  );
}

function GuruCard({g, dark, t}: {g: GuruItem; dark: boolean; t: Tokens}) {
  const tr = useT();
  const when = ago(g.published);
  return (
    <section className={cx('rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="flex flex-wrap items-center gap-2">
        <span
          className={cx(
            'inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-semibold',
            dark ? 'bg-violet-500/15 text-violet-200' : 'bg-violet-50 text-violet-700',
          )}
        >
          {g.author}
        </span>
        {when && <span className={cx('text-[11.5px]', t.faint)}>{when}</span>}
      </div>

      <a
        href={g.url}
        target="_blank"
        rel="noopener noreferrer"
        className={cx(
          'mt-2 block text-[15px] font-bold leading-snug hover:opacity-80',
          t.text,
        )}
      >
        {g.title}
      </a>

      {g.teaser && <p className={cx('mt-1 line-clamp-2 text-[13px]', t.sub)}>{g.teaser}</p>}

      <div className="mt-2.5 flex flex-wrap items-center gap-1.5">
        {g.tickers.slice(0, 8).map(tk => (
          <Link
            key={tk}
            href={`/stock/${encodeURIComponent(tk)}`}
            className={cx(
              'rounded-md border px-1.5 py-0.5 text-[11.5px] font-semibold tabular-nums hover:opacity-80',
              t.border,
              t.accentText,
            )}
          >
            ${tk}
          </Link>
        ))}
        <a
          href={g.url}
          target="_blank"
          rel="noopener noreferrer"
          className={cx(
            'ml-auto inline-flex items-center gap-1 text-[11.5px] font-semibold',
            t.faint,
          )}
        >
          <ExternalLink size={11} /> {tr('guru.source')}
        </a>
      </div>
    </section>
  );
}
