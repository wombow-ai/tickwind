'use client';

import {useEffect, useState} from 'react';
import {getNews, getQuote, type NewsItem, type Quote} from '@/lib/api';
import {formatPrice, formatPublishedDate} from '@/lib/format';

/** Live demo card — friendly, rounded, warm. */
export function BreezeDemo({ticker}: {ticker: string}) {
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
    <div className="rounded-[2rem] border border-emerald-100 bg-white p-7 shadow-2xl shadow-emerald-600/10">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2.5">
          <span className="grid h-10 w-10 place-items-center rounded-2xl bg-emerald-100 font-mono text-sm font-extrabold text-emerald-700">
            {ticker.slice(0, 2)}
          </span>
          <div className="leading-tight">
            <div className="font-mono text-base font-bold text-zinc-900">
              {ticker}
            </div>
            <div className="text-xs text-zinc-400">Apple Inc.</div>
          </div>
        </div>
        {quote ? <SessionPill session={quote.session} /> : null}
      </div>

      <div className="mt-5 font-mono text-5xl font-extrabold tracking-tight tabular-nums text-zinc-900">
        {state === 'loading' ? (
          <Shimmer width="9rem" />
        ) : quote ? (
          `$${formatPrice(quote.price)}`
        ) : (
          '—'
        )}
      </div>

      <div className="mt-6 space-y-2">
        {state === 'loading' ? (
          <>
            <Shimmer width="100%" />
            <Shimmer width="75%" />
          </>
        ) : news.length === 0 ? (
          <p className="text-sm text-zinc-400">No fresh news right now.</p>
        ) : (
          news.map(n => (
            <a
              key={n.id}
              href={n.url}
              target="_blank"
              rel="noopener noreferrer"
              className="block rounded-2xl bg-emerald-50/60 px-4 py-3 transition hover:bg-emerald-50"
            >
              <span className="text-xs font-medium text-emerald-700/70">
                {n.source} · {formatPublishedDate(n.published)}
              </span>
              <span className="mt-0.5 block text-sm font-semibold text-zinc-800">
                {n.headline}
              </span>
            </a>
          ))
        )}
      </div>
    </div>
  );
}

function SessionPill({session}: {session: string}) {
  const map: Record<string, [string, string]> = {
    pre: ['Pre-market', 'bg-amber-100 text-amber-700'],
    regular: ['Open now', 'bg-emerald-500 text-white'],
    post: ['After-hours', 'bg-sky-100 text-sky-700'],
    overnight: ['Overnight', 'bg-violet-100 text-violet-700'],
    closed: ['Closed', 'bg-zinc-100 text-zinc-500'],
  };
  const [label, cls] = map[session] ?? [session, 'bg-zinc-100 text-zinc-500'];
  return (
    <span className={`rounded-full px-3 py-1.5 text-xs font-bold ${cls}`}>
      {label}
    </span>
  );
}

function Shimmer({width}: {width: string}) {
  return (
    <span
      className="inline-block h-5 animate-pulse rounded-lg bg-emerald-100 align-middle"
      style={{width}}
    />
  );
}
