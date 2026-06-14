import {permanentRedirect} from 'next/navigation';

/** Congress trading merged into the smart-money board (2026-06-10). */
export default function CongressPage() {
  permanentRedirect('/smart-money?tab=congress');
}
