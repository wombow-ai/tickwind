import type {Metadata} from 'next';
import Link from '@/components/LocalLink';
import {notFound} from 'next/navigation';
import {Flame} from 'lucide-react';
import {
  getNewsBatch,
  getTopics,
  type HotTopic,
  type NewsItem,
} from '@/lib/api';
import {SITE_URL, langAlternates} from '@/lib/config';
import {isLocale, LOCALES} from '@/lib/locale';
import {ogImageMeta} from '@/lib/og';

// Topics trend through the day → ISR with a 30-min window: the build-time set is
// captured by generateStaticParams, and new/cooled topics regenerate on demand.
export const revalidate = 1800;

/** Pre-render every trending topic × locale at build time. Best-effort: an API
 *  failure yields no params, so the route stays dynamic (never breaks the build).
 *  `dynamicParams` defaults true → new/unknown-but-fetchable keys ISR-generate. */
export async function generateStaticParams(): Promise<{locale: string; key: string}[]> {
  try {
    const data = await getTopics(AbortSignal.timeout(5000));
    return LOCALES.flatMap(locale =>
      (data.topics ?? []).map(t => ({locale, key: t.key})),
    );
  } catch {
    // API unavailable at build → emit no params; pages render on-demand via ISR.
    return [];
  }
}

/** Fetches the topics snapshot (best-effort) and resolves the topic for a key. */
async function lookup(key: string): Promise<HotTopic | null> {
  try {
    const data = await getTopics(AbortSignal.timeout(5000));
    return (data.topics ?? []).find(t => t.key === key) ?? null;
  } catch {
    return null;
  }
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string; key: string}>;
}): Promise<Metadata> {
  const {locale, key} = await params;
  const topic = await lookup(key);
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';
  const path = `/topic/${key}`;

  // Genuinely-empty topic (unknown key, or trended-off with no tickers): keep it
  // out of the index but still let the page render (notFound() handles the body).
  if (!topic || topic.related_tickers.length === 0) {
    return {
      title: zh ? '话题 · Tickwind' : 'Topic · Tickwind',
      robots: {index: false, follow: true},
    };
  }

  // The label is a news-derived theme (likely single-language) — rendered verbatim;
  // only the surrounding chrome is localized.
  const label = topic.label;
  const title = zh ? `${label} · 相关美股 | Tickwind` : `${label} · US Stocks & News | Tickwind`;
  const description = zh
    ? `与“${label}”相关的美股个股与最新新闻 —— 实时报价、关联股票与新闻聚合。延迟数据,仅供参考,不构成投资建议。`
    : `US stocks and the latest news tied to ${label} — live quotes, related tickers and aggregated headlines. Delayed data, for reference only, not investment advice.`;

  return {
    title: {absolute: title},
    description,
    alternates: langAlternates(path, loc),
    openGraph: {
      type: 'website',
      title,
      description: description.slice(0, 110),
      url: `${SITE_URL}/${loc}${path}`,
      images: [
        ogImageMeta({
          lang: loc,
          eyebrow: zh ? '主题' : 'Topic',
          title: label,
          subtitle: zh ? '相关美股与新闻' : 'Related US stocks & news',
        }),
      ],
    },
  };
}

/**
 * Trending-topic landing page (pSEO): the US stocks + recent news tied to one Hot
 * Topic (key from the URL). The crawlable core — h1 (the news-derived topic label
 * + a localized suffix), a per-locale intro, the related tickers as internal links
 * into `/stock/{t}`, and recent topic-scoped headlines — is SERVER-rendered for the
 * active locale (chosen from the route segment), so /en and /zh are distinct,
 * indexable HTML. The label is a proper-noun theme: rendered verbatim, never
 * machine-translated; only the chrome is localized. An unknown key or a topic that
 * has cooled off with no related tickers → notFound().
 */
export default async function TopicRoute({
  params,
}: {
  params: Promise<{locale: string; key: string}>;
}) {
  const {locale, key} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';

  const topic = await lookup(key);
  // Genuinely empty (unknown key, or trended off with no tickers) → 404. This also
  // keeps the page out of the index (notFound() serves the 404 not-found page).
  if (!topic || topic.related_tickers.length === 0) notFound();

  const label = topic.label;
  const path = `/topic/${key}`;
  const tickers = topic.related_tickers;

  // Topic-scoped recent headlines, server-fetched (best-effort: a failure → no
  // news section, never throws to the route). Uses the `?topic=` filter on the
  // batched news endpoint, scoped to this topic's related tickers.
  let news: NewsItem[] = [];
  try {
    const r = await getNewsBatch(tickers, 12, AbortSignal.timeout(8000), key);
    news = r.news ?? [];
  } catch {
    news = [];
  }

  // JSON-LD: a CollectionPage wrapping an ItemList of the related tickers (each a
  // locale-prefixed /stock URL) + a BreadcrumbList (Tickwind → Hot → topic). All
  // `item`/`url` locale-prefixed to match the canonical (the FIXED pattern).
  const ld = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'CollectionPage',
        name: label,
        url: `${SITE_URL}/${loc}${path}`,
        mainEntity: {
          '@type': 'ItemList',
          name: label,
          numberOfItems: tickers.length,
          itemListElement: tickers.map((tk, i) => ({
            '@type': 'ListItem',
            position: i + 1,
            name: tk,
            url: `${SITE_URL}/${loc}/stock/${encodeURIComponent(tk)}`,
          })),
        },
      },
      {
        '@type': 'BreadcrumbList',
        itemListElement: [
          {'@type': 'ListItem', position: 1, name: 'Tickwind', item: `${SITE_URL}/${loc}`},
          {
            '@type': 'ListItem',
            position: 2,
            name: zh ? '热门' : 'Hot',
            item: `${SITE_URL}/${loc}/hot`,
          },
          {'@type': 'ListItem', position: 3, name: label, item: `${SITE_URL}/${loc}${path}`},
        ],
      },
    ],
  };

  const heading = zh ? `${label} · 相关美股` : `${label} · Stocks & News`;
  const intro = zh
    ? `与“${label}”相关的美股个股与最新新闻。`
    : `US stocks and the latest news related to ${label}.`;

  return (
    <article className="mx-auto max-w-3xl">
      <script type="application/ld+json" dangerouslySetInnerHTML={{__html: JSON.stringify(ld)}} />

      <nav className="mb-4 text-[12px] text-slate-500 dark:text-slate-400" aria-label="Breadcrumb">
        <Link href="/" className="hover:underline">
          {zh ? '首页' : 'Home'}
        </Link>
        <span className="mx-1.5">/</span>
        <Link href="/hot" className="hover:underline">
          {zh ? '热门' : 'Hot'}
        </Link>
      </nav>

      <header className="mb-5">
        <h1 className="flex flex-wrap items-center gap-x-2 gap-y-1 text-[24px] font-bold tracking-tight text-slate-900 dark:text-slate-100">
          <Flame size={20} className="text-amber-500 dark:text-amber-300" />
          {heading}
          {topic.momentum > 1 && (
            <span className="rounded-full bg-amber-50 px-2 py-0.5 text-[11px] font-semibold text-amber-700 dark:bg-amber-500/15 dark:text-amber-300">
              {zh ? '升温中' : 'Heating up'}
            </span>
          )}
        </h1>
        <p className="mt-1.5 text-[13.5px] leading-relaxed text-slate-600 dark:text-slate-300">
          {intro}
        </p>
      </header>

      {/* Crawlable core: the related tickers as internal links into /stock/{t}. */}
      <section className="mb-7">
        <h2 className="mb-2.5 text-[12px] font-semibold uppercase tracking-wide text-slate-400 dark:text-slate-500">
          {zh ? '相关股票' : 'Related stocks'}
        </h2>
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
          {tickers.map(tk => (
            <Link
              key={tk}
              href={`/stock/${encodeURIComponent(tk)}`}
              className="flex items-center justify-center rounded-xl border border-slate-200 px-3 py-2.5 text-[14px] font-bold text-slate-900 transition hover:border-amber-300 hover:bg-slate-50 dark:border-slate-800 dark:text-slate-100 dark:hover:border-amber-500/40 dark:hover:bg-slate-900"
            >
              {tk}
            </Link>
          ))}
        </div>
      </section>

      {/* Recent topic-scoped headlines (server-rendered; omitted when none). */}
      {news.length > 0 && (
        <section className="mb-7">
          <h2 className="mb-2.5 text-[12px] font-semibold uppercase tracking-wide text-slate-400 dark:text-slate-500">
            {zh ? '相关新闻' : 'Related news'}
          </h2>
          <ul className="space-y-2.5">
            {news.map(n => (
              <li key={`${n.ticker}:${n.id}`}>
                <a
                  href={n.url}
                  target="_blank"
                  rel="noopener noreferrer nofollow"
                  className="block rounded-xl border border-slate-200 px-3.5 py-2.5 transition hover:border-amber-300 hover:bg-slate-50 dark:border-slate-800 dark:hover:border-amber-500/40 dark:hover:bg-slate-900"
                >
                  <span className="text-[13.5px] font-semibold leading-snug text-slate-800 dark:text-slate-100">
                    {(zh && n.headline_zh) || n.headline}
                  </span>
                  <span className="mt-1 block text-[11.5px] text-slate-400 dark:text-slate-500">
                    {n.ticker} · {n.source}
                  </span>
                </a>
              </li>
            ))}
          </ul>
        </section>
      )}

      <p className="mt-6 text-center text-[11px] text-slate-400 dark:text-slate-500">
        {zh
          ? '数据延迟 · 仅供参考 · 非投资建议'
          : 'Delayed data · For reference only · Not investment advice'}
      </p>
    </article>
  );
}
