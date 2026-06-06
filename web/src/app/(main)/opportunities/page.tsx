import type {Metadata} from 'next';
import {OpportunityBoard} from '@/components/OpportunityBoard';

export const metadata: Metadata = {
  title: 'Opportunity board',
  description:
    'Small-cap US stocks where company insiders are buying on the open market, surfaced from SEC Form 4 filings. Not investment advice.',
};

/** Public Opportunity board (small-cap insider-buy signals). */
export default function OpportunitiesPage() {
  return <OpportunityBoard />;
}
