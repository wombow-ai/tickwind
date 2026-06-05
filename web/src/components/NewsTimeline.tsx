import type {NewsItem} from '@/lib/api';
import {formatPublishedDate, toDateTimeAttrFull} from '@/lib/format';

interface NewsTimelineProps {
  news: NewsItem[];
}

/**
 * A vertical timeline of company-news articles, most recent first. Each entry
 * shows its source and published time, the headline as a link out to the
 * article, and a truncated summary when one is available.
 */
export function NewsTimeline({news}: NewsTimelineProps) {
  return (
    <ol className="relative space-y-1 border-l border-white/10 pl-6">
      {news.map((item, index) => (
        <NewsRow key={`${item.id || item.url}-${index}`} item={item} />
      ))}
    </ol>
  );
}

/** A single timeline entry. */
function NewsRow({item}: {item: NewsItem}) {
  const dateTime = toDateTimeAttrFull(item.published);
  return (
    <li className="group relative -ml-6 rounded-lg pl-6 pr-3 py-3 transition hover:bg-white/[0.03]">
      <span
        aria-hidden
        className="absolute -left-[5px] top-5 h-2.5 w-2.5 rounded-full border-2 border-zinc-950 bg-zinc-600 transition group-hover:bg-sky-400"
      />
      <div className="flex flex-wrap items-center gap-2">
        {item.source ? (
          <span className="text-xs font-semibold text-zinc-400">
            {item.source}
          </span>
        ) : null}
        <time
          dateTime={dateTime}
          className="text-xs font-medium text-zinc-500"
        >
          {formatPublishedDate(item.published)}
        </time>
      </div>
      <a
        href={item.url}
        target="_blank"
        rel="noopener noreferrer"
        className="mt-1.5 flex items-start gap-1.5 text-sm font-medium text-zinc-200 hover:text-sky-300 focus:outline-none focus-visible:underline"
      >
        <span className="flex-1">{item.headline}</span>
        <span
          aria-hidden
          className="mt-0.5 shrink-0 text-zinc-600 transition group-hover:text-sky-400"
        >
          ↗
        </span>
      </a>
      {item.summary ? (
        <p className="mt-1 line-clamp-2 text-sm text-zinc-500">
          {item.summary}
        </p>
      ) : null}
    </li>
  );
}
