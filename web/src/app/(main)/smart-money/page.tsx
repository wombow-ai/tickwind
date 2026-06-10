import type {Metadata} from 'next';
import {SmartMoneyTabs, type SmartMoneyTab} from '@/components/SmartMoneyTabs';

export const metadata: Metadata = {
  title: 'Smart money',
  description:
    'Follow the big money: recent SEC Schedule 13D/13G institutional & activist stakes, and official U.S. House stock-trade disclosures (Periodic Transaction Reports) — side by side. Public-domain data, linked to each official filing. Not investment advice.',
};

/** Merged institutional (13D/13G) + Congress trading board. */
export default async function SmartMoneyPage({
  searchParams,
}: {
  searchParams: Promise<{tab?: string}>;
}) {
  const sp = await searchParams;
  const initial: SmartMoneyTab = sp.tab === 'congress' ? 'congress' : 'institutional';
  return <SmartMoneyTabs initial={initial} />;
}
