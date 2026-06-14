import LocalLink from '@/components/LocalLink';

// Locale-scoped not-found: handles `notFound()` thrown inside the `[locale]`
// shell (guide/[slug], indicators/[id], fund/[slug], congress/member/[slug],
// …). It's wrapped by `[locale]/layout.tsx` (which renders `<html>`/`<body>` +
// providers), so this renders content ONLY. Bilingual via the app's `[data-i18n]`
// dual-render convention, keyed to `<html lang>`.
export default function LocaleNotFound() {
  return (
    <main className="flex min-h-[60vh] flex-col items-center justify-center px-6 text-center">
      <p className="text-[40px] font-bold tracking-tight text-slate-900 dark:text-slate-100">
        404
      </p>
      <p className="mt-2 text-[15px] text-slate-600 dark:text-slate-300">
        <span data-i18n="zh">页面不存在</span>
        <span data-i18n="en">Page not found</span>
      </p>
      <LocalLink
        href="/"
        className="mt-6 inline-block rounded-full bg-teal-600 px-5 py-2.5 text-[14px] font-semibold text-white hover:bg-teal-700 dark:bg-teal-500 dark:text-slate-950 dark:hover:bg-teal-400"
      >
        <span data-i18n="zh">返回首页</span>
        <span data-i18n="en">Back to Tickwind</span>
      </LocalLink>
    </main>
  );
}
