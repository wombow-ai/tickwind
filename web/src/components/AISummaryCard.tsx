'use client';

import {Loader2, Sparkles} from 'lucide-react';
import {useEffect, useState} from 'react';
import {getSummary} from '@/lib/api';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, timeAgo, tok} from '@/lib/ui';
import {Markdown} from '@/components/Markdown';

type Status = 'loading' | 'ready' | 'hidden';

/**
 * The per-stock AI digest card: 3-5 bullets distilled from recent news +
 * community posts, in the user's UI language (zh/en), generated at most once
 * per day per (ticker, language) — the backend caches; the first visitor pays
 * the LLM call. Hides entirely when the LLM is off (503), the budget is spent
 * (429), there's no material yet, or the fetch fails — never a broken card.
 * Always labeled AI-generated, never advice.
 */
export function AISummaryCard({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const [summary, setSummary] = useState('');
  const [at, setAt] = useState<string | undefined>();
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    getSummary(ticker, lang, c.signal).then(
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
  }, [ticker, lang]);

  if (status === 'hidden') return null;
  if (status === 'loading') {
    // A labeled, animated placeholder (an LLM call can take a few seconds) —
    // less abrupt than a bare skeleton block.
    return (
      <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
        <div className="mb-3 flex flex-wrap items-center gap-2">
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
          <span className={cx('ml-auto inline-flex items-center gap-1.5 text-[11.5px]', t.sub)}>
            <Loader2 size={13} className="animate-spin" />
            {tr('ai.loading')}
          </span>
        </div>
        <div className="space-y-2" aria-hidden>
          <div className={cx('h-3 rounded', t.skel)} style={{width: '92%'}} />
          <div className={cx('h-3 rounded', t.skel)} style={{width: '78%'}} />
          <div className={cx('h-3 rounded', t.skel)} style={{width: '85%'}} />
        </div>
      </section>
    );
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
