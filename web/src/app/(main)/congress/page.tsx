import type {Metadata} from 'next';
import {CongressBoard} from '@/components/CongressBoard';

export const metadata: Metadata = {
  title: 'Congress trading',
  description:
    "The latest U.S. House stock-trade disclosures (Periodic Transaction Reports) from the official, public-domain House Clerk dataset — who filed, when, and a link to each official filing. Not investment advice.",
};

/** Public Congress trading board (House Clerk Periodic Transaction Reports). */
export default function CongressPage() {
  return (
    <div className="mx-auto max-w-3xl">
      <CongressBoard />
    </div>
  );
}
