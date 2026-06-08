import type {Metadata} from 'next';
import {TopicPage} from '@/components/TopicPage';

export async function generateMetadata({
  searchParams,
}: {
  searchParams: Promise<{label?: string}>;
}): Promise<Metadata> {
  const {label} = await searchParams;
  return {
    title: label ? `${label} — Topic` : 'Topic',
    description: label
      ? `Stocks and news tied to ${label}.`
      : 'Stocks and news for a trending market topic.',
  };
}

/** Trending-topic landing page: stocks + news for one Hot Topic (key from the URL). */
export default async function TopicRoute({
  params,
  searchParams,
}: {
  params: Promise<{key: string}>;
  searchParams: Promise<{label?: string}>;
}) {
  const {key} = await params;
  const {label} = await searchParams;
  return <TopicPage topicKey={key} label={label} />;
}
