'use client';

import ReactMarkdown from 'react-markdown';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

/**
 * Renders a safe Markdown subset for user-authored notes/comments. react-markdown
 * does NOT render raw HTML by default (no `rehype-raw`), so this is XSS-safe;
 * images are stripped (avoid remote-content/tracking) and links open in a new tab
 * with rel="noopener noreferrer". Typography is set by the `.tw-md` class in
 * globals.css to keep blocks compact inside the existing panels.
 */
export function Markdown({children, className}: {children: string; className?: string}) {
  const dark = useDark();
  const t = tok(dark);
  const linkCls = dark ? 'text-teal-300 underline' : 'text-teal-600 underline';
  return (
    <div className={cx('tw-md break-words text-[13.5px] leading-relaxed', t.text, className)}>
      <ReactMarkdown
        disallowedElements={['img']}
        unwrapDisallowed
        components={{
          a(props) {
            return (
              <a
                href={props.href}
                target="_blank"
                rel="noopener noreferrer"
                className={linkCls}
              >
                {props.children}
              </a>
            );
          },
        }}
      >
        {children}
      </ReactMarkdown>
    </div>
  );
}
