'use client';

import {
  ArrowUpRight,
  FileText,
  Link2,
  MessageSquare,
  Newspaper,
} from 'lucide-react';
import Link from 'next/link';
import type {Clip, Filing, NewsItem, Post} from '@/lib/api';
import {useDark} from '@/lib/theme';
import {cx, timeAgo, tok} from '@/lib/ui';

/** A single feed entry, discriminated by source kind. */
export type FeedEntry =
  | {kind: 'news'; item: NewsItem}
  | {kind: 'disc'; item: Post}
  | {kind: 'clip'; item: Clip}
  | {kind: 'filing'; item: Filing};

/** A short, friendly source label for a saved link, from its URL host. */
export function clipSource(url: string): string {
  const u = url.toLowerCase();
  if (/x\.com|twitter\.com/.test(u)) return 'X';
  if (/tiktok\.com/.test(u)) return 'TikTok';
  if (/xiaohongshu|xhslink/.test(u)) return 'Xiaohongshu';
  if (/youtube\.com|youtu\.be/.test(u)) return 'YouTube';
  try {
    return new URL(url).hostname.replace(/^www\./, '');
  } catch {
    return 'Link';
  }
}

const ICONS = {
  news: Newspaper,
  disc: MessageSquare,
  clip: Link2,
  filing: FileText,
} as const;

/** Renders one timeline node with a leading dot + connector and a card. */
export function TimelineItem({
  entry,
  last,
  showTicker,
}: {
  entry: FeedEntry;
  last?: boolean;
  /** Show a ticker tag (for cross-stock aggregated feeds like the home page). */
  showTicker?: boolean;
}) {
  const dark = useDark();
  const t = tok(dark);
  const accent = dark ? '#2DD4BF' : '#0D9488';
  const Icon = ICONS[entry.kind];

  const formColor = (f: string) => {
    if (f.startsWith('8-K')) {
      return {
        bg: dark ? 'rgba(244,63,94,.16)' : '#FFE4E6',
        fg: dark ? '#FDA4AF' : '#BE123C',
      };
    }
    if (f.startsWith('10-Q') || f.startsWith('10-K')) {
      return {
        bg: dark ? 'rgba(59,130,246,.16)' : '#DBEAFE',
        fg: dark ? '#93C5FD' : '#1D4ED8',
      };
    }
    return {
      bg: dark ? 'rgba(139,92,246,.16)' : '#EDE9FE',
      fg: dark ? '#C4B5FD' : '#6D28D9',
    };
  };

  const when =
    entry.kind === 'news'
      ? timeAgo(entry.item.published)
      : entry.kind === 'disc'
        ? timeAgo(entry.item.created_at)
        : entry.kind === 'clip'
          ? timeAgo(entry.item.created_at)
          : timeAgo(entry.item.filed_at);
  const showAgo = entry.kind === 'news' || entry.kind === 'filing';

  return (
    <div className={cx('flex gap-3', !last && 'pb-1')}>
      <div className="relative flex flex-col items-center" style={{width: 22}}>
        <span
          className="rounded-full"
          style={{
            width: 9,
            height: 9,
            background: accent,
            boxShadow: `0 0 0 4px ${dark ? 'rgba(20,184,166,.10)' : 'rgba(13,148,136,.10)'}`,
          }}
        />
        <span className={cx('mt-1 w-px flex-1', dark ? 'bg-slate-800' : 'bg-slate-200')} />
      </div>

      <div
        className={cx(
          'mb-3 flex-1 rounded-2xl border p-3.5 transition-colors',
          t.card,
          t.border,
          t.soft,
        )}
      >
        <div className="mb-1.5 flex items-center gap-2">
          <span
            className={cx(
              'inline-flex h-5 w-5 items-center justify-center rounded-md',
              t.surf2,
            )}
          >
            <Icon className={t.sub} size={12} />
          </span>
          {entry.kind === 'filing' ? (
            <span
              className="rounded-md px-1.5 py-0.5 text-[10.5px] font-bold tabular-nums"
              style={{
                background: formColor(entry.item.form).bg,
                color: formColor(entry.item.form).fg,
              }}
            >
              {entry.item.form}
            </span>
          ) : entry.kind === 'disc' ? (
            <span className={cx('text-[13px] font-semibold', t.text)}>
              @{entry.item.author}
            </span>
          ) : entry.kind === 'clip' ? (
            <span
              className={cx(
                'text-[11.5px] font-semibold uppercase tracking-wide',
                t.accentText,
              )}
            >
              Clip · {clipSource(entry.item.url)}
            </span>
          ) : (
            <span className={cx('text-[12px] font-semibold', t.sub)}>
              {entry.item.source}
            </span>
          )}
          {showTicker && (
            <Link
              href={`/stock/${encodeURIComponent(entry.item.ticker)}`}
              className={cx(
                'rounded-md px-1.5 py-0.5 text-[10.5px] font-bold tracking-wide',
                t.chip,
                t.accentText,
              )}
            >
              {entry.item.ticker}
            </Link>
          )}
          <span className={cx('ml-auto text-[11px] tabular-nums', t.faint)}>
            {when}
            {showAgo ? ' ago' : ''}
          </span>
        </div>

        {entry.kind === 'news' && (
          <a
            href={entry.item.url}
            target="_blank"
            rel="noopener noreferrer"
            className="group block"
          >
            <p className={cx('text-[14px] font-semibold leading-snug', t.text)}>
              {entry.item.headline}
              <ArrowUpRight
                className="ml-1 -mt-0.5 inline opacity-50 transition group-hover:opacity-100"
                size={14}
              />
            </p>
            {entry.item.summary && (
              <p className={cx('mt-1 text-[12.5px] leading-relaxed', t.sub)}>
                {entry.item.summary}
              </p>
            )}
          </a>
        )}

        {entry.kind === 'disc' && (
          <a
            href={entry.item.url}
            target="_blank"
            rel="noopener noreferrer"
            className="block"
          >
            <p className={cx('text-[13px] leading-relaxed', t.text)}>
              {entry.item.body}
            </p>
            <p className={cx('mt-1 text-[11px]', t.faint)}>via {entry.item.source}</p>
          </a>
        )}

        {entry.kind === 'clip' && (
          <a
            href={entry.item.url}
            target="_blank"
            rel="noopener noreferrer"
            className="group block"
          >
            <p className={cx('text-[13.5px] font-medium leading-snug', t.text)}>
              {entry.item.title}
            </p>
            <p
              className={cx(
                'mt-1 truncate text-[12px] tabular-nums',
                t.accentText,
              )}
            >
              {entry.item.url.replace(/^https?:\/\//, '')}
            </p>
          </a>
        )}

        {entry.kind === 'filing' && (
          <a
            href={entry.item.url}
            target="_blank"
            rel="noopener noreferrer"
            className="group block"
          >
            <p className={cx('text-[13.5px] font-medium leading-snug', t.text)}>
              {entry.item.title}
              <ArrowUpRight
                className="ml-1 -mt-0.5 inline opacity-50 transition group-hover:opacity-100"
                size={14}
              />
            </p>
          </a>
        )}
      </div>
    </div>
  );
}
