'use client';

import {ArrowRight, Loader2, Sparkles} from 'lucide-react';
import {useEffect, useState} from 'react';
import Link from '@/components/LocalLink';
import {getSummary} from '@/lib/api';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, timeAgo, tok} from '@/lib/ui';
import {Markdown} from '@/components/Markdown';

type Status = 'loading' | 'ready' | 'hidden';

/** Re-fetch cadence while a background digest generation is in flight (~4s). */
const POLL_INTERVAL_MS = 4000;
/** Safety cap on automatic polls (~25 × 4s ≈ 100s) before giving up (best-effort). */
const MAX_POLLS = 25;

/**
 * The per-stock AI digest card: 3-5 bullets distilled from recent news +
 * community posts, in the user's UI language (zh/en), generated at most once
 * per day per (ticker, language) — the backend caches; the first visitor pays
 * the LLM call. Hides entirely when the LLM is off (503), the budget is spent
 * (429), there's no material yet, or the fetch fails — never a broken card.
 * Always labeled AI-generated, never advice.
 *
 * ASYNC: `/summary` returns instantly. While `prose_status === 'generating'` the
 * digest is still being composed in the background → keep the loading skeleton
 * and poll every {@link POLL_INTERVAL_MS} (cap {@link MAX_POLLS}; after the cap,
 * stop and hide — the digest is best-effort). A non-empty `summary` → show; any
 * other terminal status (or 503/429/error) → hide. BACKWARD-COMPATIBLE: when
 * `prose_status` is absent (older synchronous backend) this is exactly today's
 * behavior (non-empty → show, empty → hide, no polling).
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
    // One AbortController + one timer span this effect run. `active` guards every
    // setState so a late resolve after unmount / ticker / lang change is dropped;
    // cleanup aborts the in-flight fetch AND clears the pending poll timer.
    const c = new AbortController();
    let active = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let polls = 0;
    setStatus('loading');

    const tick = () => {
      getSummary(ticker, lang, c.signal).then(
        r => {
          if (!active) return;
          // A non-empty digest is final regardless of status → show it.
          if (r.summary.trim()) {
            setSummary(r.summary);
            setAt(r.generated_at);
            setStatus('ready');
            return;
          }
          // Empty + still generating: keep the skeleton and poll (cap-guarded).
          // BACKWARD-COMPAT: absent prose_status ⇒ this branch is never taken
          // for an empty summary (we fall straight through to 'hidden' below),
          // so the synchronous backend behaves exactly as before (no polling).
          if (r.prose_status === 'generating' && polls < MAX_POLLS) {
            polls += 1;
            timer = setTimeout(tick, POLL_INTERVAL_MS);
            return;
          }
          // Empty + terminal status (or poll cap reached) → no material → hide.
          setStatus('hidden');
        },
        () => {
          if (!active) return;
          if (c.signal.aborted) return;
          setStatus('hidden'); // 503 (LLM off) / 429 (budget) / error → hide
        },
      );
    };

    tick();

    return () => {
      active = false;
      c.abort(); // cancel any in-flight fetch
      if (timer) clearTimeout(timer); // clear the pending poll timer
    };
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
          <DeepEntry ticker={ticker} dark={dark} tr={tr} />
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
        <div className="ml-auto flex items-center gap-2">
          {at && (
            <span className={cx('text-[10.5px]', t.faint)}>
              {timeAgo(at)} {tr('common.ago')}
            </span>
          )}
          <DeepEntry ticker={ticker} dark={dark} tr={tr} />
        </div>
      </div>
      <Markdown>{summary}</Markdown>
      <p className={cx('mt-2 text-[10.5px]', t.faint)}>{tr('ai.disclaimer')}</p>
    </section>
  );
}

/**
 * The entry button to the dedicated AI Deep Research report, placed at the AI
 * Digest module's top-right (owner spec). Subtle, Aurora-styled, bilingual; a
 * locale-aware link to `/stock/{ticker}/research`. The deep report is gated
 * (login + 1/day quota) — that UX lives on the target route, so this is just the
 * navigation affordance.
 */
function DeepEntry({
  ticker,
  dark,
  tr,
  className,
}: {
  ticker: string;
  dark: boolean;
  tr: (key: string) => string;
  className?: string;
}) {
  return (
    <Link
      href={`/stock/${encodeURIComponent(ticker)}/research`}
      className={cx(
        'inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-[11px] font-semibold transition',
        dark
          ? 'border-violet-500/30 text-violet-300 hover:border-violet-400/50 hover:bg-violet-500/10'
          : 'border-violet-200 text-violet-600 hover:border-violet-300 hover:bg-violet-50',
        className,
      )}
    >
      {tr('deep.entry')}
      <ArrowRight size={12} />
    </Link>
  );
}
