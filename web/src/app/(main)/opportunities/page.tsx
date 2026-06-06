import type {Metadata} from 'next';
import {GuruRail} from '@/components/GuruRail';
import {OpportunityBoard} from '@/components/OpportunityBoard';

export const metadata: Metadata = {
  title: 'Opportunity board',
  description:
    'Small-cap US stocks where company insiders are buying on the open market, surfaced from SEC Form 4 filings, alongside what independent finance writers are publishing. Not investment advice.',
};

/** Public Opportunity board (small-cap insider-buy signals) + the Guru-watch rail. */
export default function OpportunitiesPage() {
  return (
    <div className="grid gap-8 lg:grid-cols-3">
      <div className="lg:col-span-2">
        <OpportunityBoard />
      </div>
      <div className="lg:col-span-1">
        <GuruRail />
      </div>
    </div>
  );
}
