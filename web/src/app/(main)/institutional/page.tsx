import type {Metadata} from 'next';
import {InstitutionalBoard} from '@/components/InstitutionalBoard';

export const metadata: Metadata = {
  title: 'Institutional & activist filings',
  description:
    'Recent SEC Schedule 13D/13G beneficial-ownership filings — which institutions and activists took >5% stakes in which companies. 13D = active stake, 13G = passive. Public-domain SEC data. Not investment advice.',
};

/** Public institutional / activist board (SEC 13D/13G beneficial ownership). */
export default function InstitutionalPage() {
  return (
    <div className="mx-auto max-w-3xl">
      <InstitutionalBoard />
    </div>
  );
}
