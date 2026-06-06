'use client';

import {Sparkles} from 'lucide-react';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

interface Update {
  date: string;
  title: string;
  body: string;
}

const UPDATES: Update[] = [
  {
    date: 'Jun 2026',
    title: 'A calmer, data-first Tickwind',
    body: 'You now land straight on the board — popular stocks with live, all-session prices. Sign in to make it your own watchlist.',
  },
  {
    date: 'Jun 2026',
    title: 'Saved links, per stock',
    body: 'Found a great thread or teardown elsewhere? Paste the link on any stock to clip it onto your private feed.',
  },
  {
    date: 'May 2026',
    title: 'All-session prices + filings, news and chatter',
    body: 'Pre-market, regular, after-hours and overnight prices, alongside SEC filings, news and StockTwits/Reddit discussion — one page per stock.',
  },
];

/** Product changelog / what's-new. */
export default function AnnouncementsPage() {
  const dark = useDark();
  const t = tok(dark);
  return (
    <div className="mx-auto max-w-2xl">
      <div className="mb-6 flex items-center gap-2">
        <Sparkles size={18} className={dark ? 'text-teal-300' : 'text-teal-600'} />
        <h1 className={cx('text-[26px] font-bold tracking-tight', t.text)}>
          What&apos;s new
        </h1>
      </div>
      <div className="space-y-4">
        {UPDATES.map((u, i) => (
          <article
            key={i}
            className={cx('rounded-3xl border p-6', t.card, t.border, t.soft)}
          >
            <p className={cx('text-[12px] font-semibold uppercase tracking-wide', t.faint)}>
              {u.date}
            </p>
            <h2 className={cx('mt-1 text-[16px] font-semibold', t.text)}>{u.title}</h2>
            <p className={cx('mt-1.5 text-[13.5px] leading-relaxed', t.sub)}>{u.body}</p>
          </article>
        ))}
      </div>
    </div>
  );
}
