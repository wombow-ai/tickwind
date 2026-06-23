'use client';

import {FileText} from 'lucide-react';
import {useEffect, useState} from 'react';
import Link from '@/components/LocalLink';
import {getMaterialFeed, type MaterialFeedEvent} from '@/lib/api';

/** Shorten a long SEC item label to its first clause (e.g. the 5.02 label runs to ~150 chars); the
 * full canonical label stays available as the `title` tooltip. Display-only — the data is untouched. */
function shortLabel(full: string): string {
  const head = full.split(/[;:]/)[0].trim();
  return head.length > 0 ? head : full;
}

/**
 * Client-self-healing market-wide material-events feed. The pSEO page SSR-fetches best-effort (crawlable
 * rows + JSON-LD when the tunnel cooperates), but Vercel's server-side fetch through the Cloudflare
 * tunnel is unreliable and the feed cache is cold after each backend restart — so the SSR fetch can bake
 * an empty page that sticks in the ISR cache. This renders the SSR `initial` rows when present (SEO) +
 * re-fetches from the browser on mount (the reliable path). Deploy-gotcha #7. Every figure is a Go-owned
 * SEC fact (form, dates, item codes + canonical labels, filing link) — facts only, no LLM, no advice.
 */
export function MaterialFeedList({
  item,
  initial,
  zh,
}: {
  item?: string; // optional 8-K item-code filter (a category page)
  initial: MaterialFeedEvent[];
  zh: boolean;
}) {
  const [events, setEvents] = useState<MaterialFeedEvent[]>(initial);
  const [loading, setLoading] = useState(initial.length === 0);

  useEffect(() => {
    const c = new AbortController();
    getMaterialFeed(item, 80, c.signal)
      .then(r => {
        if (c.signal.aborted) return;
        if (r.events && r.events.length > 0) setEvents(r.events);
        setLoading(false);
      })
      .catch(() => setLoading(false));
    return () => c.abort();
  }, [item]);

  if (events.length > 0) {
    return (
      <div className="tw-fade space-y-2.5">
        {events.map((e, i) => (
          <Row key={`${e.ticker}-${e.accession_url}-${i}`} e={e} zh={zh} />
        ))}
      </div>
    );
  }
  if (loading) {
    return <div className="h-40 animate-pulse rounded-2xl bg-slate-100 dark:bg-slate-900" />;
  }
  return (
    <div className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-10 text-center dark:border-slate-800 dark:bg-slate-900">
      <FileText size={22} className="mx-auto mb-2 text-slate-300 dark:text-slate-600" />
      <p className="text-[14px] font-semibold text-slate-700 dark:text-slate-200">
        {zh ? '暂无近期事件' : 'No recent events'}
      </p>
      <p className="mt-1 text-[12.5px] text-slate-500 dark:text-slate-400">
        {zh
          ? '该类别近期无重大 8-K 申报。事件每小时刷新一次,稍后再来查看。'
          : 'No notable 8-K filings in this category recently. The feed refreshes hourly — check back shortly.'}
      </p>
      <Link
        href="/events"
        className="mt-3 inline-block rounded-lg bg-amber-600 px-3 py-1.5 text-[12.5px] font-semibold text-white transition hover:bg-amber-700 dark:bg-amber-500 dark:hover:bg-amber-600"
      >
        {zh ? '查看全部事件 →' : 'See all events →'}
      </Link>
    </div>
  );
}

/** One event card → the ticker links into the stock page; each NOTABLE 8-K item shows its Go-owned
 * canonical label (shortened, full on hover); the SEC filing link opens the official index. Every
 * field is a disclosed corporate-filing fact, never advice. */
function Row({e, zh}: {e: MaterialFeedEvent; zh: boolean}) {
  return (
    <div className="rounded-2xl border border-slate-200 bg-white p-3.5 dark:border-slate-800 dark:bg-slate-950">
      <div className="flex items-center justify-between gap-3">
        <Link
          href={`/stock/${encodeURIComponent(e.ticker)}`}
          className="text-[15px] font-bold text-slate-900 hover:text-amber-600 dark:text-slate-100 dark:hover:text-amber-400"
        >
          {e.ticker}
        </Link>
        <span className="shrink-0 text-[11.5px] tabular-nums text-slate-400 dark:text-slate-500">
          {zh ? '申报 ' : 'Filed '}
          {e.filed_date}
        </span>
      </div>
      <div className="mt-2 flex flex-wrap gap-1.5">
        {e.items.map(it => (
          <span
            key={it.code}
            title={zh ? it.label_zh : it.label_en}
            className="rounded-md bg-amber-50 px-2 py-0.5 text-[12px] font-medium text-amber-700 dark:bg-amber-500/10 dark:text-amber-300"
          >
            {shortLabel(zh ? it.label_zh : it.label_en)}
          </span>
        ))}
      </div>
      <a
        href={e.accession_url}
        target="_blank"
        rel="noopener noreferrer"
        className="mt-2 inline-block text-[12px] font-medium text-slate-500 hover:text-amber-600 dark:text-slate-400 dark:hover:text-amber-400"
      >
        {e.form} · {zh ? '查看 SEC 申报 →' : 'View SEC filing →'}
      </a>
    </div>
  );
}
