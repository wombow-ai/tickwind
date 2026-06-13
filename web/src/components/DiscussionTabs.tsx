'use client';

import {usePathname, useRouter, useSearchParams} from 'next/navigation';
import {Suspense, useCallback} from 'react';
import {CommentsPanel} from '@/components/CommentsPanel';
import {FeedPage} from '@/components/FeedPage';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

type Tab = 'discussion' | 'community';
const TABS: {id: Tab; key: string}[] = [
  {id: 'discussion', key: 'disc.discussion'},
  {id: 'community', key: 'disc.community'},
];

function isTab(v: string | null): v is Tab {
  return v === 'discussion' || v === 'community';
}

/**
 * The `/discussion` shell: a tab row over the aggregated social feed (Discussion)
 * and the global community comments board (Community). `?tab=` deep-links each;
 * defaults to the social feed (the SEO-indexed view).
 */
function DiscussionTabsInner() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const tab: Tab = isTab(params.get('tab')) ? (params.get('tab') as Tab) : 'discussion';

  const select = useCallback(
    (next: Tab) => {
      const q = new URLSearchParams(params.toString());
      if (next === 'discussion') q.delete('tab');
      else q.set('tab', next);
      const qs = q.toString();
      router.replace(qs ? `${pathname}?${qs}` : pathname, {scroll: false});
    },
    [params, pathname, router],
  );

  return (
    <div>
      <nav className="mx-auto mb-5 flex max-w-2xl items-center gap-1.5">
        {TABS.map(({id, key}) => {
          const active = tab === id;
          return (
            <button
              key={id}
              onClick={() => select(id)}
              aria-current={active ? 'page' : undefined}
              className={cx(
                'rounded-full border px-3.5 py-1.5 text-[13px] font-semibold transition',
                active
                  ? dark
                    ? 'border-teal-400/40 bg-teal-500/15 text-teal-200'
                    : 'border-teal-200 bg-teal-50 text-teal-700'
                  : cx(t.border, t.sub, t.ghost),
              )}
            >
              {tr(key)}
            </button>
          );
        })}
      </nav>

      {tab === 'discussion' ? (
        <FeedPage kind="discussion" />
      ) : (
        <div className="mx-auto max-w-2xl">
          <CommentsPanel />
        </div>
      )}
    </div>
  );
}

export function DiscussionTabs() {
  return (
    <Suspense fallback={null}>
      <DiscussionTabsInner />
    </Suspense>
  );
}
