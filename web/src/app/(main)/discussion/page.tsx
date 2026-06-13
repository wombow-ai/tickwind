import type {Metadata} from 'next';
import {DiscussionTabs} from '@/components/DiscussionTabs';

export const metadata: Metadata = {
  title: 'Discussion',
  description:
    'What people are saying about the most-watched US stocks — StockTwits, Bluesky and more.',
};

/** Public discussion shell: aggregated social feed + the global community board. */
export default function DiscussionPage() {
  return <DiscussionTabs />;
}
