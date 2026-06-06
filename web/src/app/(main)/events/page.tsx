import type {Metadata} from 'next';
import {EventsTimeline} from '@/components/EventsTimeline';

export const metadata: Metadata = {
  title: 'Events timeline',
  description:
    'Upcoming market-moving events — Fed (FOMC) rate decisions, key US economic releases (CPI, jobs report), and notable world events. For context, not investment advice.',
};

/** Public major-events timeline (macro + world). */
export default function EventsPage() {
  return <EventsTimeline />;
}
