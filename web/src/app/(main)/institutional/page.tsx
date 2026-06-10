import {permanentRedirect} from 'next/navigation';

/** Institutional/activist filings merged into the smart-money board (2026-06-10). */
export default function InstitutionalPage() {
  permanentRedirect('/smart-money?tab=institutional');
}
