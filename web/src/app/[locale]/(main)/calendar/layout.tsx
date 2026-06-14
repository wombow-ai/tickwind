import {CalendarTabs} from '@/components/CalendarTabs';

/**
 * Shared shell for the unified calendar: a tab row (Earnings · Macro · IPO) over
 * each independently-indexable subpath. Per-subpath SSR metadata lives on the
 * individual pages so each URL keeps its own title/description/hreflang.
 */
export default function CalendarLayout({
  children,
}: Readonly<{children: React.ReactNode}>) {
  return (
    <>
      <CalendarTabs />
      {children}
    </>
  );
}
