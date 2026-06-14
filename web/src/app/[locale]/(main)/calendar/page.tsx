import {redirect} from 'next/navigation';

/** `/{locale}/calendar` lands on the Earnings tab — the first subpath. */
export default async function CalendarPage({
  params,
}: {
  params: Promise<{locale: string}>;
}) {
  const {locale} = await params;
  redirect(`/${locale}/calendar/earnings`);
}
