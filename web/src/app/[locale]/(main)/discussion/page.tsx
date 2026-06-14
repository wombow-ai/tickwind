import type {Metadata} from 'next';
import {langAlternates} from '@/lib/config';
import {isLocale} from '@/lib/locale';
import {DiscussionTabs} from '@/components/DiscussionTabs';

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string}>;
}): Promise<Metadata> {
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  return {
    title: 'Discussion',
    description:
      'What people are saying about the most-watched US stocks — StockTwits, Bluesky and more.',
    alternates: langAlternates('/discussion', loc),
  };
}

/** Public discussion shell: aggregated social feed + the global community board. */
export default function DiscussionPage() {
  return <DiscussionTabs />;
}
