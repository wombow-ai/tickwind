import type {Metadata} from 'next';
import {FeedPage} from '@/components/FeedPage';

export const metadata: Metadata = {
  title: 'Discussion',
  description:
    'What people are saying about the most-watched US stocks — StockTwits, Bluesky and more.',
};

/** Public aggregated discussion feed. */
export default function DiscussionPage() {
  return <FeedPage kind="discussion" />;
}
