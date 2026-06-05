import type {Post} from '@/lib/api';
import {formatPublishedDate, toDateTimeAttrFull} from '@/lib/format';

interface SocialFeedProps {
  posts: Post[];
}

/**
 * A vertical timeline of social posts (e.g. StockTwits), most recent first.
 * Each entry shows the author, source and time, with the post body linking out
 * to the discussion.
 */
export function SocialFeed({posts}: SocialFeedProps) {
  return (
    <ol className="relative space-y-1 border-l border-white/10 pl-6">
      {posts.map((post, index) => (
        <SocialRow key={`${post.id}-${index}`} post={post} />
      ))}
    </ol>
  );
}

/** A single social post entry. */
function SocialRow({post}: {post: Post}) {
  const dateTime = toDateTimeAttrFull(post.created_at);
  return (
    <li className="group relative -ml-6 rounded-lg pl-6 pr-3 py-3 transition hover:bg-white/[0.03]">
      <span
        aria-hidden
        className="absolute -left-[5px] top-5 h-2.5 w-2.5 rounded-full border-2 border-zinc-950 bg-zinc-600 transition group-hover:bg-emerald-400"
      />
      <div className="flex flex-wrap items-center gap-2">
        {post.author ? (
          <span className="text-xs font-semibold text-zinc-300">
            @{post.author}
          </span>
        ) : null}
        <SourceBadge source={post.source} />
        <time dateTime={dateTime} className="text-xs font-medium text-zinc-500">
          {formatPublishedDate(post.created_at)}
        </time>
      </div>
      <a
        href={post.url}
        target="_blank"
        rel="noopener noreferrer"
        className="mt-1.5 flex items-start gap-1.5 text-sm text-zinc-200 hover:text-emerald-300 focus:outline-none focus-visible:underline"
      >
        <span className="flex-1 line-clamp-4 whitespace-pre-line">
          {post.body}
        </span>
        <span
          aria-hidden
          className="mt-0.5 shrink-0 text-zinc-600 transition group-hover:text-emerald-400"
        >
          ↗
        </span>
      </a>
    </li>
  );
}

/** A small pill naming the social source. */
function SourceBadge({source}: {source: string}) {
  const label =
    source === 'stocktwits'
      ? 'StockTwits'
      : source === 'reddit'
        ? 'Reddit'
        : source;
  return (
    <span className="rounded bg-white/5 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-zinc-400">
      {label}
    </span>
  );
}
