import type {Metadata} from 'next';
import './globals.css';
import {SiteHeader} from '@/components/SiteHeader';
import {APP_NAME, APP_TAGLINE} from '@/lib/config';

export const metadata: Metadata = {
  title: `${APP_NAME} — ${APP_TAGLINE}`,
  description:
    'A personal stock dashboard: watchlist board and per-stock SEC filings timeline.',
};

export default function RootLayout({
  children,
}: Readonly<{children: React.ReactNode}>) {
  return (
    <html lang="en" className="h-full">
      <body className="flex min-h-full flex-col antialiased">
        <SiteHeader />
        <main className="mx-auto w-full max-w-5xl flex-1 px-4 py-8 sm:px-6">
          {children}
        </main>
        <footer className="border-t border-white/10 py-6">
          <div className="mx-auto max-w-5xl px-4 text-center text-xs text-zinc-600 sm:px-6">
            {APP_NAME} · data from SEC EDGAR · for personal use
          </div>
        </footer>
      </body>
    </html>
  );
}
