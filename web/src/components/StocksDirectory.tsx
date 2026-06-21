'use client';

import {LayoutGrid} from 'lucide-react';
import {useEffect, useState} from 'react';
import Link from '@/components/LocalLink';
import {getUniverseSymbols} from '@/lib/api';

/**
 * Self-healing A–Z directory views. The `/stocks` hub and `/stocks/[letter]` pages are ISR
 * (hourly) and pre-render the ticker universe server-side — but a build that coincides with a
 * backend restart bakes an EMPTY universe (the documented deploy gotcha), and the stale empty
 * page is served until the next revalidate. To make the directory resilient, these components
 * render the SERVER list when present (so SEO/HTML still carries the links the common case),
 * and only when it's empty do they CLIENT-fetch `/v1/universe/symbols` and fill in — so a
 * degraded bake self-heals on first view instead of showing "no stocks". Mirrors the
 * indicator-library self-heal.
 */

/** First-letter bucket key (uppercase A–Z), or '' for non-alpha leads (digits/symbols). */
function letterKey(t: string): string {
  if (!t) return '';
  const c = t[0].toUpperCase();
  return c >= 'A' && c <= 'Z' ? c : '';
}

/** The quote-bearing tickers for one A–Z letter, sorted — the client-side mirror of pseo.tickersForLetter. */
function filterByLetter(all: string[], letter: string): string[] {
  const up = letter.toUpperCase();
  return all.filter(t => letterKey(t) === up).sort((a, b) => a.localeCompare(b));
}

/**
 * The per-letter ticker grid (used by /stocks/[letter]). `initial` is the server-rendered list;
 * when it's empty the component heals from the live universe before falling back to the empty
 * state. `count` line + grid here so they stay consistent after a heal.
 */
export function LetterGrid({letter, initial, zh}: {letter: string; initial: string[]; zh: boolean}) {
  const up = letter.toUpperCase();
  const [tickers, setTickers] = useState<string[]>(initial);
  const [healing, setHealing] = useState(initial.length === 0);

  useEffect(() => {
    if (initial.length > 0) return; // server already shipped the list — nothing to heal
    let active = true;
    // `healing` already starts true (initial state) when the server list was empty, so no
    // synchronous setState needed here — go straight to the fetch.
    (async () => {
      try {
        const all = await getUniverseSymbols(AbortSignal.timeout(8000));
        if (active) setTickers(filterByLetter(all, letter));
      } catch {
        /* keep empty → empty state */
      } finally {
        if (active) setHealing(false);
      }
    })();
    return () => {
      active = false;
    };
  }, [initial, letter]);

  if (tickers.length > 0) {
    return (
      <>
        <p className="mb-3 text-[12.5px] text-slate-500 dark:text-slate-400">
          {zh ? `共 ${tickers.length.toLocaleString()} 只` : `${tickers.length.toLocaleString()} stocks`}
        </p>
        <section>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 md:grid-cols-4">
            {tickers.map(tk => (
              <Link
                key={tk}
                href={`/stock/${encodeURIComponent(tk)}`}
                className="flex items-center justify-center rounded-xl border border-slate-200 px-3 py-2.5 text-[14px] font-bold text-slate-900 transition hover:border-sky-300 hover:bg-slate-50 dark:border-slate-800 dark:text-slate-100 dark:hover:border-sky-500/40 dark:hover:bg-slate-900"
              >
                {tk}
              </Link>
            ))}
          </div>
        </section>
      </>
    );
  }

  if (healing) {
    // Healing shimmer — a grid of placeholder tiles while the client fetch resolves.
    return (
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 md:grid-cols-4" aria-busy="true">
        {Array.from({length: 12}).map((_, i) => (
          <div
            key={i}
            className="h-[42px] animate-pulse rounded-xl border border-slate-200 bg-slate-100 dark:border-slate-800 dark:bg-slate-900"
          />
        ))}
      </div>
    );
  }

  return (
    <div className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-10 text-center dark:border-slate-800 dark:bg-slate-900">
      <LayoutGrid size={22} className="mx-auto mb-2 text-slate-300 dark:text-slate-600" />
      <p className="text-[14px] font-semibold text-slate-700 dark:text-slate-200">
        {zh ? `暂无以 ${up} 开头的股票` : `No stocks starting with ${up} right now`}
      </p>
      <p className="mt-1 text-[12.5px] text-slate-500 dark:text-slate-400">
        {zh ? '稍后再来查看,或浏览其他字母。' : 'Check back shortly, or browse another letter.'}
      </p>
      <Link
        href="/stocks"
        className="mt-3 inline-block rounded-lg bg-sky-600 px-3 py-1.5 text-[12.5px] font-semibold text-white transition hover:bg-sky-700 dark:bg-sky-500 dark:hover:bg-sky-600"
      >
        {zh ? '返回 A–Z 目录 →' : 'Back to the A–Z directory →'}
      </Link>
    </div>
  );
}

/**
 * The 26-letter index with per-letter counts (used by the /stocks hub). `initialCounts` maps a
 * lowercase letter → count (server-rendered). When the total is 0 (empty bake) it heals from the
 * live universe. The tiles always link out regardless of count, so this is purely the count fill.
 */
export function LetterCounts({
  letters,
  initialCounts,
  zh,
}: {
  letters: readonly string[];
  initialCounts: Record<string, number>;
  zh: boolean;
}) {
  const initialTotal = Object.values(initialCounts).reduce((n, v) => n + v, 0);
  const [counts, setCounts] = useState<Record<string, number>>(initialCounts);

  useEffect(() => {
    if (initialTotal > 0) return; // server already has counts
    let active = true;
    (async () => {
      try {
        const all = await getUniverseSymbols(AbortSignal.timeout(8000));
        const next: Record<string, number> = {};
        for (const t of all) {
          const k = letterKey(t);
          if (!k) continue;
          const lc = k.toLowerCase();
          next[lc] = (next[lc] ?? 0) + 1;
        }
        if (active) setCounts(next);
      } catch {
        /* keep the dashes */
      }
    })();
    return () => {
      active = false;
    };
  }, [initialTotal]);

  const total = Object.values(counts).reduce((n, v) => n + v, 0);

  return (
    <>
      {total > 0 && (
        <p className="mb-3 text-[12.5px] text-slate-500 dark:text-slate-400">
          {zh ? `共收录 ${total.toLocaleString()} 只有报价的美股` : `${total.toLocaleString()} quote-bearing US stocks indexed`}
        </p>
      )}
      <div className="grid grid-cols-3 gap-2 sm:grid-cols-4 md:grid-cols-6">
        {letters.map(letter => {
          const count = counts[letter] ?? 0;
          return (
            <Link
              key={letter}
              href={`/stocks/${letter}`}
              className="flex flex-col items-center justify-center gap-0.5 rounded-xl border border-slate-200 px-3 py-3 transition hover:border-sky-300 hover:bg-slate-50 dark:border-slate-800 dark:hover:border-sky-500/40 dark:hover:bg-slate-900"
            >
              <span className="text-[18px] font-bold uppercase text-slate-900 dark:text-slate-100">{letter}</span>
              <span className="text-[11px] tabular-nums text-slate-400 dark:text-slate-500">
                {count > 0 ? count.toLocaleString() : '—'}
              </span>
            </Link>
          );
        })}
      </div>
    </>
  );
}
