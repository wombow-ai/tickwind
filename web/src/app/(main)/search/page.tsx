import type {Metadata} from 'next';
import {SearchResults} from '@/components/SearchResults';

export const metadata: Metadata = {
  title: 'Search',
  description: 'Search stocks and ETFs by ticker or company name.',
};

/** Search-results landing page for the nav search box (Enter → /search?q=…). */
export default async function SearchPage({
  searchParams,
}: {
  searchParams: Promise<{q?: string}>;
}) {
  const sp = await searchParams;
  return <SearchResults q={sp.q ?? ''} />;
}
