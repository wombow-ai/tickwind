import Link from 'next/link';
import {themeNoFlashScript} from '@/lib/theme';

// The framework not-found that renders OUTSIDE any `[locale]` segment (e.g. an
// unmatched top-level path, or a bare path the proxy didn't prefix). Because
// `<html>`/`<body>` now live in `[locale]/layout.tsx` and the root layout is a
// pass-through, this page must render its OWN complete document or the browser
// gets a malformed (html/body-less) 404. The locale-scoped not-found
// (`[locale]/not-found.tsx`) handles `notFound()` thrown inside the app shell.
export default function NotFound() {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        {/* Apply the persisted theme before paint to avoid a light→dark flash. */}
        <script dangerouslySetInnerHTML={{__html: themeNoFlashScript}} />
      </head>
      <body className="antialiased">
        <main className="flex min-h-screen flex-col items-center justify-center px-6 text-center">
          <p className="text-[40px] font-bold tracking-tight text-slate-900 dark:text-slate-100">
            404
          </p>
          <p className="mt-2 text-[15px] text-slate-600 dark:text-slate-300">
            页面不存在 · Page not found
          </p>
          <Link
            href="/en"
            className="mt-6 inline-block rounded-full bg-teal-600 px-5 py-2.5 text-[14px] font-semibold text-white hover:bg-teal-700 dark:bg-teal-500 dark:text-slate-950 dark:hover:bg-teal-400"
          >
            返回首页 · Back to Tickwind
          </Link>
        </main>
      </body>
    </html>
  );
}
