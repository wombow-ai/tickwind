'use client';

import Link from '@/components/LocalLink';
import ReactMarkdown from 'react-markdown';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

// Mirrors internal/cashtag (keep in sync): $ + 1-6 alphanumerics + optional
// venue suffix, not preceded by a word char or another $, not run into a longer
// word. Pure-digit tags without a suffix ("$100") are prices, not tickers.
const CASHTAG = /(^|[^A-Za-z0-9$])\$([A-Za-z0-9]{1,6}(?:\.[A-Za-z]{1,3})?)\b/g;

/** "$aapl 冲" → "[$AAPL](/stock/AAPL) 冲", leaving code spans/fences alone. */
export function linkifyCashtags(md: string): string {
  // Split out fenced blocks and inline code; only transform prose segments
  // (odd indices are the code captures from the splitting group).
  const parts = md.split(/(```[\s\S]*?```|`[^`\n]*`)/g);
  return parts
    .map((seg, i) => {
      if (i % 2 === 1) return seg;
      return seg.replace(CASHTAG, (full, pre: string, tag: string) => {
        if (!tag.includes('.') && /^\d+$/.test(tag)) return full; // "$100"
        const sym = tag.toUpperCase();
        return `${pre}[$${sym}](/stock/${encodeURIComponent(sym)})`;
      });
    })
    .join('');
}

/**
 * Renders a safe Markdown subset for user-authored notes/comments. react-markdown
 * does NOT render raw HTML by default (no `rehype-raw`), so this is XSS-safe;
 * images are stripped (avoid remote-content/tracking) and external links open in
 * a new tab with rel="noopener noreferrer". $TICKER cashtags become internal
 * links to the stock page (same-tab, next/link). Typography is set by the
 * `.tw-md` class in globals.css to keep blocks compact inside existing panels.
 */
export function Markdown({children, className}: {children: string; className?: string}) {
  const dark = useDark();
  const t = tok(dark);
  const linkCls = dark ? 'text-teal-300 underline' : 'text-teal-600 underline';
  const tagCls = cx('font-semibold no-underline', dark ? 'text-teal-300' : 'text-teal-600');
  return (
    <div className={cx('tw-md break-words text-[13.5px] leading-relaxed', t.text, className)}>
      <ReactMarkdown
        disallowedElements={['img']}
        unwrapDisallowed
        components={{
          a(props) {
            const href = props.href ?? '';
            if (href.startsWith('/')) {
              // Internal cashtag/stock link → client-side nav, same tab.
              return (
                <Link href={href} className={tagCls}>
                  {props.children}
                </Link>
              );
            }
            return (
              <a href={href} target="_blank" rel="noopener noreferrer" className={linkCls}>
                {props.children}
              </a>
            );
          },
        }}
      >
        {linkifyCashtags(children)}
      </ReactMarkdown>
    </div>
  );
}
