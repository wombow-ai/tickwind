'use client';

import {Sparkles} from 'lucide-react';
import {useEffect, useState} from 'react';
import {getSummary} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, timeAgo, tok} from '@/lib/ui';
import {Markdown} from '@/components/Markdown';

type Status = 'loading' | 'ready' | 'hidden';

/**
 * The per-stock AI digest card: 3-5 Chinese bullets distilled from recent news
 * + community posts, generated at most once per day per ticker (the backend
 * caches; the first visitor pays the LLM call). Hides entirely when the LLM is
 * off (503), the budget is spent (429), there's no material yet, or the fetch
 * fails — never a broken card. Always labeled AI-generated, never advice.
 */
export function AISummaryCard({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [summary, setSummary] = useState('');
  const [at, setAt] = useState<string | undefined>();
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    getSummary(ticker, c.signal).then(
      r => {
        if (!r.summary.trim()) {
          setStatus('hidden'); // no material yet — show nothing, not an empty shell
          return;
        }
        setSummary(r.summary);
        setAt(r.generated_at);
        setStatus('ready');
      },
      () => setStatus('hidden'), // 503 (LLM off) / 429 (budget) / error → hide
    );
    return () => c.abort();
  }, [ticker]);

  if (status === 'hidden') return null;
  if (status === 'loading') {
    return <div className={cx('mb-6 h-28 rounded-2xl', t.skel)} />;
  }

  return (
    <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-2 flex flex-wrap items-center gap-2">
        <h2 className={cx('flex items-center gap-1.5 text-[14px] font-bold', t.text)}>
          <Sparkles size={15} className={dark ? 'text-violet-300' : 'text-violet-500'} />
          {tr('ai.title')}
        </h2>
        <span
          className={cx(
            'rounded-md px-1.5 py-0.5 text-[10px] font-bold',
            dark ? 'bg-violet-500/15 text-violet-300' : 'bg-violet-50 text-violet-600',
          )}
        >
          {tr('ai.badge')}
        </span>
        {at && (
          <span className={cx('ml-auto text-[10.5px]', t.faint)}>
            {timeAgo(at)} {tr('common.ago')}
          </span>
        )}
      </div>
      <Markdown>{summary}</Markdown>
      <p className={cx('mt-2 text-[10.5px]', t.faint)}>{tr('ai.disclaimer')}</p>
    </section>
  );
}
