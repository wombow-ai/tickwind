import type {Metadata} from 'next';
import {AlertsCenter} from '@/components/AlertsCenter';

export const metadata: Metadata = {
  title: 'Alerts',
  description: 'Your price and event alerts across all stocks — triggered and active, in one place.',
};

/** Cross-stock alerts hub (auth-gated inside the component). */
export default function AlertsPage() {
  return <AlertsCenter />;
}
