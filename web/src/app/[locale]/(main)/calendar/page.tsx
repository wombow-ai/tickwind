import {redirect} from 'next/navigation';

/** `/{locale}/calendar` lands on the Events (macro timeline) tab — the first subpath. */
export default async function CalendarPage({
  params,
}: {
  params: Promise<{locale: string}>;
}) {
  const {locale} = await params;
  redirect(`/${locale}/calendar/macro`);
}
