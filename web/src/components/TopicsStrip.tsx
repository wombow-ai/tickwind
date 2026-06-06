'use client';

import {Flame} from 'lucide-react';
import Link from 'next/link';
import {useEffect, useState} from 'react';
import {getTopics, type HotTopic} from '@/lib/api';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

/**
 * The Hot Topics strip: a horizontally-scrollable row of trending-theme chips
 * (each with an article count) that link to a topic-filtered news view. Renders
 * nothing until there is data, so it never shows empty scaffolding.
 */
export function TopicsStrip() {
  const dark = useDark();
  const t = tok(dark);
  const [topics, setTopics] = useState<HotTopic[]>([]);

  useEffect(() => {
    const c = new AbortController();
    getTopics(c.signal).then(
      r => setTopics(r.topics ?? []),
      () => setTopics([]),
    );
    return () => c.abort();
  }, []);

  if (topics.length === 0) return null;

  return (
    <div className="mb-6">
      <div className="mb-2 flex items-center gap-1.5">
        <Flame size={14} className={dark ? 'text-amber-300' : 'text-amber-500'} />
        <span className={cx('text-[12px] font-semibold uppercase tracking-wide', t.sub)}>
          Hot topics
        </span>
      </div>
      <div className="-mx-1 flex gap-2 overflow-x-auto px-1 pb-1">
        {topics.map((tp, i) => (
          <Link
            key={tp.key}
            href={`/news?topic=${encodeURIComponent(tp.key)}&label=${encodeURIComponent(tp.label)}`}
            className={cx(
              'inline-flex shrink-0 items-center gap-1.5 rounded-full border px-3 py-1.5 text-[12.5px] font-medium transition hover:opacity-80',
              t.border,
              dark ? 'bg-slate-900' : 'bg-white',
              t.text,
            )}
          >
            {i === 0 && (
              <Flame size={12} className={dark ? 'text-amber-300' : 'text-amber-500'} />
            )}
            {tp.label}
            <span
              className={cx(
                'rounded-full px-1.5 text-[11px] font-semibold tabular-nums',
                dark ? 'bg-slate-800 text-slate-300' : 'bg-slate-100 text-slate-500',
              )}
            >
              {tp.count}
            </span>
          </Link>
        ))}
      </div>
    </div>
  );
}
