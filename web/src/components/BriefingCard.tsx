'use client';

import {Sparkles} from 'lucide-react';
import {useEffect, useState} from 'react';
import {getBriefing, type Briefing} from '@/lib/api';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, timeAgo, tok} from '@/lib/ui';
import {Markdown} from '@/components/Markdown';

/**
 * The daily AI pre-market briefing, folded into the home hub (replacing the
 * former standalone /briefing page): one summary of indices, movers, today's
 * earnings and smart-money filings, generated once a day server-side. Renders
 * nothing until a briefing exists, so the hub never shows an empty slot
 * (404 before generation / LLM off → hidden). Always AI-labeled, never advice.
 */
export function BriefingCard() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const [brief, setBrief] = useState<Briefing | null>(null);

  useEffect(() => {
    const c = new AbortController();
    getBriefing(lang, c.signal).then(
      b => setBrief(b),
      () => {}, // 404 (not generated) / LLM off → stay hidden
    );
    return () => c.abort();
  }, [lang]);

  if (!brief) return null;

  return (
    <section className={cx('mb-8 rounded-2xl border p-5', t.card, t.border, t.soft)}>
      <div className="mb-2 flex flex-wrap items-center gap-2">
        <h2 className={cx('flex items-center gap-1.5 text-[15px] font-bold', t.text)}>
          <Sparkles size={16} className={dark ? 'text-violet-300' : 'text-violet-600'} />
          {tr('brief.title')}
        </h2>
        <span
          className={cx(
            'rounded-md px-1.5 py-0.5 text-[10px] font-bold',
            dark ? 'bg-violet-500/15 text-violet-300' : 'bg-violet-50 text-violet-600',
          )}
        >
          {tr('ai.badge')}
        </span>
        <span className={cx('text-[12px]', t.faint)}>
          {brief.date} · {timeAgo(brief.generated_at)} {tr('common.ago')}
        </span>
      </div>
      <Markdown>{brief.text}</Markdown>
      <p className={cx('mt-4 border-t pt-3 text-[10.5px]', t.hair, t.faint)}>{tr('ai.disclaimer')}</p>
    </section>
  );
}
