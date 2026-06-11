'use client';

import {Newspaper, Sparkles} from 'lucide-react';
import {useEffect, useState} from 'react';
import {getBriefing, type Briefing} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, timeAgo, tok} from '@/lib/ui';
import {Markdown} from '@/components/Markdown';

type Status = 'loading' | 'ready' | 'empty';

/**
 * The daily AI pre-market briefing page (/briefing): one Chinese summary of
 * indices, movers, today's earnings and smart-money filings, generated once a
 * day server-side and shared by everyone. 404 before generation → friendly
 * empty state. Always AI-labeled, never advice.
 */
export function BriefingView() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [brief, setBrief] = useState<Briefing | null>(null);
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    getBriefing(c.signal).then(
      b => {
        setBrief(b);
        setStatus('ready');
      },
      () => setStatus('empty'), // 404 (not generated) / LLM off
    );
    return () => c.abort();
  }, []);

  return (
    <div className="mx-auto max-w-2xl">
      <header className="mb-5">
        <h1 className={cx('flex items-center gap-2 text-[22px] font-bold tracking-tight', t.text)}>
          <Newspaper size={20} className={dark ? 'text-teal-300' : 'text-teal-600'} />
          {tr('brief.title')}
          <span
            className={cx(
              'rounded-md px-1.5 py-0.5 text-[10px] font-bold',
              dark ? 'bg-violet-500/15 text-violet-300' : 'bg-violet-50 text-violet-600',
            )}
          >
            {tr('ai.badge')}
          </span>
        </h1>
        {brief && (
          <p className={cx('mt-1 text-[13px]', t.sub)}>
            {brief.date} ·{' '}
            <Sparkles size={12} className="inline" /> {timeAgo(brief.generated_at)}{' '}
            {tr('common.ago')}
          </p>
        )}
      </header>

      {status === 'loading' && <div className={cx('h-64 rounded-2xl', t.skel)} />}

      {status === 'empty' && (
        <p className={cx('rounded-2xl border p-8 text-center text-[13px]', t.card, t.border, t.soft, t.sub)}>
          {tr('brief.empty')}
        </p>
      )}

      {status === 'ready' && brief && (
        <article className={cx('rounded-2xl border p-5', t.card, t.border, t.soft)}>
          <Markdown>{brief.text}</Markdown>
          <p className={cx('mt-4 border-t pt-3 text-[10.5px]', t.hair, t.faint)}>
            {tr('ai.disclaimer')}
          </p>
        </article>
      )}
    </div>
  );
}
