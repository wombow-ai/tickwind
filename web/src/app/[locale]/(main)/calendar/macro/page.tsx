import type {Metadata} from 'next';
import {langAlternates} from '@/lib/config';
import {isLocale} from '@/lib/locale';
import {EventsTimeline} from '@/components/EventsTimeline';
import {RateCutOdds} from '@/components/RateCutOdds';

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string}>;
}): Promise<Metadata> {
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  return {
    title: 'Events timeline',
    description:
      'Upcoming market-moving events — Fed (FOMC) rate decisions, key US economic releases (CPI, jobs report), and notable world events. For context, not investment advice.',
    alternates: langAlternates('/calendar/macro', loc),
  };
}

/**
 * Public macro events + rate-cut odds (Macro tab of the unified calendar).
 * Rate-cut odds (prediction markets) render below the events timeline — kept off
 * the homepage per the owner; this macro page is their contextual home.
 */
export default function MacroCalendarPage() {
  return (
    <>
      <EventsTimeline />
      <RateCutOdds />
    </>
  );
}
