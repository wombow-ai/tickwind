import type {Metadata} from 'next';
import {HotList} from '@/components/HotList';

export const metadata: Metadata = {
  title: 'Hot stocks',
  description:
    'The most-discussed US stocks across Reddit right now, ranked by buzz and momentum.',
};

/** Public trending-leaderboard page. */
export default function HotPage() {
  return <HotList />;
}
