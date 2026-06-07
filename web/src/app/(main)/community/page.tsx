'use client';

import {MessageSquare} from 'lucide-react';
import {CommentsPanel} from '@/components/CommentsPanel';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

/** The global community discussion board (comments not tied to one stock). */
export default function CommunityPage() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  return (
    <div className="mx-auto max-w-2xl">
      <header className="mb-5">
        <h1
          className={cx('flex items-center gap-2 text-[22px] font-bold tracking-tight', t.text)}
        >
          <MessageSquare size={20} className={dark ? 'text-teal-300' : 'text-teal-600'} />
          {tr('comments.title')}
        </h1>
        <p className={cx('mt-1 text-[13.5px]', t.sub)}>{tr('comments.subtitle')}</p>
      </header>
      <CommentsPanel />
    </div>
  );
}
