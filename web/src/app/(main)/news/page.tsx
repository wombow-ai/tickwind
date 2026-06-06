import type {Metadata} from 'next';
import {FeedPage} from '@/components/FeedPage';

export const metadata: Metadata = {
  title: 'Market news',
  description:
    'The latest news headlines across the most-watched US stocks, in one feed.',
};

/** Public aggregated market-news feed, optionally filtered to a hot topic. */
export default async function NewsPage({
  searchParams,
}: {
  searchParams: Promise<{topic?: string; label?: string}>;
}) {
  const sp = await searchParams;
  return <FeedPage kind="news" topic={sp.topic} topicLabel={sp.label} />;
}
