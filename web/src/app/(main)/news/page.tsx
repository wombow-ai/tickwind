import type {Metadata} from 'next';
import {FeedPage} from '@/components/FeedPage';

export const metadata: Metadata = {
  title: 'Market news',
  description:
    'The latest news headlines across the most-watched US stocks, in one feed.',
};

/** Public aggregated market-news feed. */
export default function NewsPage() {
  return <FeedPage kind="news" />;
}
