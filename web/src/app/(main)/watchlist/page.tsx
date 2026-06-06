import {Board} from '@/components/Board';

export const metadata = {title: 'Your watchlist'};

/** The signed-in user's personal watchlist + its news and discussion. */
export default function WatchlistPage() {
  return <Board variant="watchlist" />;
}
