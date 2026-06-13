import type {Metadata} from 'next';
import {langAlternates} from '@/lib/config';
import {EarningsCalendar} from '@/components/EarningsCalendar';

export const metadata: Metadata = {
  title: 'Earnings calendar',
  description:
    'Upcoming US company earnings, grouped by day — pre-market vs after-hours timing and consensus EPS estimates. Data from Finnhub. Not investment advice.',
  alternates: langAlternates('/calendar/earnings'),
};

/** Public market-wide earnings calendar (Earnings tab of the unified calendar). */
export default function EarningsCalendarPage() {
  return <EarningsCalendar />;
}
