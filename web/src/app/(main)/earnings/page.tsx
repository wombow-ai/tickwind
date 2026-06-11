import type {Metadata} from 'next';
import {EarningsCalendar} from '@/components/EarningsCalendar';

export const metadata: Metadata = {
  title: 'Earnings calendar',
  description:
    'Upcoming US company earnings, grouped by day — pre-market vs after-hours timing and consensus EPS estimates. Data from Finnhub. Not investment advice.',
};

/** Public market-wide earnings calendar page. */
export default function EarningsPage() {
  return <EarningsCalendar />;
}
