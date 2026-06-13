import {redirect} from 'next/navigation';

/** `/calendar` lands on the Earnings tab — the first subpath. */
export default function CalendarPage() {
  redirect('/calendar/earnings');
}
