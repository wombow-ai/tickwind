'use client';

import {Activity, FileText, Newspaper, TrendingDown, TrendingUp, UserCheck} from 'lucide-react';
import {useEffect, useState} from 'react';
import {getMovement, type MovementEvidence, type MovementResponse} from '@/lib/api';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, timeAgo, tok} from '@/lib/ui';
import {Markdown} from '@/components/Markdown';

type Status = 'loading' | 'ready' | 'hidden';

/** Re-fetch cadence while a background LLM upgrade is in flight (~4s). */
const POLL_INTERVAL_MS = 4000;
/** Safety cap on automatic polls (~25 × 4s ≈ 100s) before settling on the canned line. */
const MAX_POLLS = 25;

/** Source-type → lucide icon for an evidence chip. */
function EvidenceIcon({type, size = 12}: {type: MovementEvidence['type']; size?: number}) {
  if (type === 'filing') return <FileText size={size} />;
  if (type === 'insider') return <UserCheck size={size} />;
  return <Newspaper size={size} />;
}

/**
 * The move-explainer card: a move-triggered, evidence-grounded explanation of a
 * NOTABLE daily price move (|change| >= 5%). The change % and direction are
 * Go-owned (computed from the quote, never the LLM's); the explanation is the
 * LLM's ONE hedged Chinese sentence (`llm:true`, AI-labeled) or a canned Go-built
 * line (`llm:false`, the data-only fallback when the LLM is off / over the daily
 * cap / errored). Evidence chips link to their source.
 *
 * Hides entirely (renders null) when the move is NOT significant (sub-threshold
 * 200 with `significant:false`), the symbol is unknown (404 → null), or the fetch
 * fails — never a broken or misleading card on a quiet day.
 *
 * ASYNC: `/movement` returns instantly. The first significant response is
 * RENDERED immediately — even while `prose_status === 'generating'`, the canned
 * Go line is shown (no lingering skeleton). While `'generating'` we poll every
 * {@link POLL_INTERVAL_MS} (cap {@link MAX_POLLS}) to UPGRADE: each poll updates
 * `data` so the canned line silently swaps to the LLM sentence (and the AI badge
 * appears when `llm:true`) WITHOUT flashing the skeleton. Polling stops on any
 * terminal status. BACKWARD-COMPATIBLE: absent `prose_status` (older synchronous
 * backend) → render as final, no polling — exactly today's behavior.
 */
export function MovementCard({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const [data, setData] = useState<MovementResponse | null>(null);
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
      getMovement(ticker, lang, c.signal).then(
        r => {
          if (!active) return;
          // Hide on a quiet day: no data (404→null), a sub-threshold move
          // (significant:false), or a missing explanation.
          if (!r || !r.significant || !r.explanation?.trim()) {
            setStatus('hidden');
            return;
          }
          // Render the (possibly canned) explanation NOW — never stay in the
          // skeleton while generating. On a re-poll this silently swaps the
          // canned line for the upgraded LLM sentence (data updates, status
          // stays 'ready' → no skeleton flash).
          setData(r);
          setStatus('ready');
          // Keep polling to upgrade while the LLM gen is in flight (cap-guarded).
          // BACKWARD-COMPAT: absent prose_status ⇒ this is never true ⇒ no poll.
          if (r.prose_status === 'generating' && polls < MAX_POLLS) {
            polls += 1;
            timer = setTimeout(tick, POLL_INTERVAL_MS);
          }
        },
        () => {
          if (!active) return;
          if (c.signal.aborted) return;
          setStatus('hidden'); // error → hide
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
    return (
      <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
        <div className="mb-3 flex flex-wrap items-center gap-2">
          <h2 className={cx('flex items-center gap-1.5 text-[14px] font-bold', t.text)}>
            <Activity size={15} className={dark ? 'text-sky-300' : 'text-sky-500'} />
            {tr('move.title')}
          </h2>
        </div>
        <div className="space-y-2" aria-hidden>
          <div className={cx('h-3 rounded', t.skel)} style={{width: '88%'}} />
          <div className={cx('h-3 rounded', t.skel)} style={{width: '64%'}} />
        </div>
      </section>
    );
  }

  // status === 'ready' — data is non-null and significant.
  const d = data!;
  const up = d.direction === 'up';
  const col = up
    ? dark
      ? 'text-emerald-400'
      : 'text-emerald-600'
    : dark
      ? 'text-rose-400'
      : 'text-rose-500';
  const pct = `${up ? '+' : '-'}${Math.abs(d.change_pct).toFixed(1)}%`;
  const evidence = d.evidence ?? [];

  return (
    <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-2.5 flex flex-wrap items-center gap-2">
        <h2 className={cx('flex items-center gap-1.5 text-[14px] font-bold', t.text)}>
          <Activity size={15} className={dark ? 'text-sky-300' : 'text-sky-500'} />
          {tr('move.title')}
        </h2>
        {/* Go-owned move %/direction — the colored headline number. */}
        <span className={cx('inline-flex items-center gap-1 text-[15px] font-bold tabular-nums', col)}>
          {up ? <TrendingUp size={15} /> : <TrendingDown size={15} />}
          {pct}
        </span>
        {d.session && (
          <span className={cx('rounded-md px-1.5 py-0.5 text-[10px] font-semibold', t.chip, t.chipText)}>
            {d.session}
          </span>
        )}
        {d.llm && (
          <span
            className={cx(
              'rounded-md px-1.5 py-0.5 text-[10px] font-bold',
              dark ? 'bg-sky-500/15 text-sky-300' : 'bg-sky-50 text-sky-600',
            )}
          >
            {tr('move.aiBadge')}
          </span>
        )}
        {d.as_of && (
          <span className={cx('ml-auto text-[10.5px]', t.faint)}>
            {timeAgo(d.as_of)} {tr('common.ago')}
          </span>
        )}
      </div>

      {/* The explanation: LLM hedged sentence (Markdown) or the canned data-only line. */}
      <Markdown>{d.explanation ?? ''}</Markdown>

      {/* Attributed evidence chips — each links to its source. */}
      {evidence.length > 0 && (
        <div className="mt-3 flex flex-col gap-1.5">
          <span className={cx('text-[10.5px] font-semibold uppercase tracking-wide', t.faint)}>
            {tr('move.evidence')}
          </span>
          <div className="flex flex-wrap gap-1.5">
            {evidence.map((e, i) =>
              e.url ? (
                <a
                  key={i}
                  href={e.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  title={tr(`move.type.${e.type}`)}
                  className={cx(
                    'inline-flex max-w-full items-center gap-1.5 rounded-lg border px-2 py-1 text-[11.5px] transition-colors',
                    t.border,
                    t.sub,
                    dark ? 'hover:border-sky-400 hover:text-sky-300' : 'hover:border-sky-400 hover:text-sky-600',
                  )}
                >
                  <EvidenceIcon type={e.type} />
                  <span className="truncate">{e.title}</span>
                </a>
              ) : (
                <span
                  key={i}
                  title={tr(`move.type.${e.type}`)}
                  className={cx(
                    'inline-flex max-w-full items-center gap-1.5 rounded-lg border px-2 py-1 text-[11.5px]',
                    t.border,
                    t.sub,
                  )}
                >
                  <EvidenceIcon type={e.type} />
                  <span className="truncate">{e.title}</span>
                </span>
              ),
            )}
          </div>
        </div>
      )}

      <p className={cx('mt-2.5 text-[10.5px]', t.faint)}>
        {d.llm ? tr('move.disclaimer') : tr('move.disclaimerData')}
      </p>
    </section>
  );
}
