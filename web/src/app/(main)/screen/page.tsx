import type {Metadata} from 'next';
import {Screener} from '@/components/Screener';

export const metadata: Metadata = {
  title: 'Stock screener',
  description:
    'Filter US stocks by price, daily % change, and trading session over the whole market. Delayed quotes. Not investment advice.',
};

/** Public stock screener over the whole-US universe quote cache. */
export default function ScreenPage() {
  return (
    <div className="mx-auto max-w-3xl">
      <Screener />
    </div>
  );
}
