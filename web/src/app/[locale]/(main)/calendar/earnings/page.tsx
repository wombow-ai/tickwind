import type {Metadata} from 'next';
import {langAlternates} from '@/lib/config';
import {isLocale} from '@/lib/locale';
import {EarningsCalendar} from '@/components/EarningsCalendar';

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string}>;
}): Promise<Metadata> {
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  return {
    title: 'Earnings calendar',
    description:
      'Upcoming US company earnings, grouped by day — pre-market vs after-hours timing and consensus EPS estimates. Data from Finnhub. Not investment advice.',
    alternates: langAlternates('/calendar/earnings', loc),
  };
}

/** Public market-wide earnings calendar (Earnings tab of the unified calendar). */
export default function EarningsCalendarPage() {
  return <EarningsCalendar />;
}
