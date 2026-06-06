'use client';

import {useEffect, useState} from 'react';
import {getNews, getQuote, type NewsItem, type Quote} from '@/lib/api';
import {formatPrice, formatPublishedDate} from '@/lib/format';

/** Live demo card: real price + session + a few headlines from the public API. */
export function AuroraDemo({ticker}: {ticker: string}) {
  const [quote, setQuote] = useState<Quote | null>(null);
  const [news, setNews] = useState<NewsItem[]>([]);
  const [state, setState] = useState<'loading' | 'ready' | 'error'>('loading');

  useEffect(() => {
    const controller = new AbortController();
    Promise.all([
      getQuote(ticker, controller.signal).catch(() => null),
      getNews(ticker, 3, controller.signal)
        .then(r => r.news)
        .catch(() => [] as NewsItem[]),
    ]).then(
      ([q, n]) => {
        if (controller.signal.aborted) return;
        setQuote(q);
        setNews(n);
        setState('ready');
      },
      () => {
        if (!controller.signal.aborted) setState('error');
      },
    );
    return () => controller.abort();
  }, [ticker]);

  return (
    <div className="rounded-3xl border border-slate-200 bg-white/80 p-6 shadow-xl shadow-indigo-600/5 backdrop-blur">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="font-mono text-lg font-bold text-slate-900">{ticker}</span>
          <span className="rounded-md bg-slate-100 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-slate-500">
            US
          </span>
        </div>
        {quote ? <SessionPill session={quote.session} /> : null}
      </div>

      <div className="mt-3 flex items-baseline gap-2">
        <span className="font-mono text-4xl font-bold tracking-tight tabular-nums text-slate-900">
          {state === 'loading' ? (
            <Shimmer width="8rem" />
          ) : quote ? (
            `$${formatPrice(quote.price)}`
          ) : (
            '—'
          )}
        </span>
        <span className="text-sm text-slate-500">Apple Inc.</span>
      </div>

      <div className="mt-5 space-y-1.5">
        {state === 'loading' ? (
          <>
            <Shimmer width="100%" />
            <Shimmer width="80%" />
          </>
        ) : news.length === 0 ? (
          <p className="text-sm text-slate-400">No fresh news right now.</p>
        ) : (
          news.map(n => (
            <a
              key={n.id}
              href={n.url}
              target="_blank"
              rel="noopener noreferrer"
              className="block rounded-xl px-3 py-2 transition hover:bg-slate-50"
            >
              <span className="text-xs text-slate-400">
                {n.source} · {formatPublishedDate(n.published)}
              </span>
              <span className="mt-0.5 block text-sm font-medium text-slate-700">
                {n.headline}
              </span>
            </a>
          ))
        )}
      </div>

      <p className="mt-4 text-center text-xs text-slate-400">
        Live data from Tickwind — this is what every stock page looks like.
      </p>
    </div>
  );
}

function SessionPill({session}: {session: string}) {
  const map: Record<string, [string, string]> = {
    pre: ['Pre-market', 'bg-amber-100 text-amber-700'],
    regular: ['Open', 'bg-emerald-100 text-emerald-700'],
    post: ['After-hours', 'bg-indigo-100 text-indigo-700'],
    overnight: ['Overnight', 'bg-violet-100 text-violet-700'],
    closed: ['Closed', 'bg-slate-100 text-slate-500'],
  };
  const [label, cls] = map[session] ?? [session, 'bg-slate-100 text-slate-500'];
  return (
    <span className={`rounded-full px-2.5 py-1 text-xs font-semibold ${cls}`}>
      {label}
    </span>
  );
}

function Shimmer({width}: {width: string}) {
  return (
    <span
      className="inline-block h-4 animate-pulse rounded bg-slate-200 align-middle"
      style={{width}}
    />
  );
}
