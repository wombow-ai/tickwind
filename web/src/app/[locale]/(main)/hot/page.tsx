import type {Metadata} from 'next';
import {langAlternates} from '@/lib/config';
import {isLocale} from '@/lib/locale';
import {HotList} from '@/components/HotList';

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string}>;
}): Promise<Metadata> {
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  return {
    title: 'Hot stocks',
    description:
      'The most-discussed US stocks across Reddit right now, ranked by buzz and momentum.',
    alternates: langAlternates('/hot', loc),
  };
}

/** Public trending-leaderboard page. */
export default function HotPage() {
  return <HotList />;
}
