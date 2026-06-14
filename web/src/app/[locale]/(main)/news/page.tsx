import type {Metadata} from 'next';
import {langAlternates} from '@/lib/config';
import {isLocale} from '@/lib/locale';
import {FeedPage} from '@/components/FeedPage';

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string}>;
}): Promise<Metadata> {
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  return {
    title: 'Market news',
    description:
      'The latest news headlines across the most-watched US stocks, in one feed.',
    alternates: langAlternates('/news', loc),
  };
}

/** Public aggregated market-news feed, optionally filtered to a hot topic. */
export default async function NewsPage({
  searchParams,
}: {
  searchParams: Promise<{topic?: string; label?: string}>;
}) {
  const sp = await searchParams;
  return <FeedPage kind="news" topic={sp.topic} topicLabel={sp.label} />;
}
