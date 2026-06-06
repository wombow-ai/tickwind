import type {Metadata} from 'next';
import './globals.css';
import {AuthProvider} from '@/lib/auth';
import {APP_NAME, APP_TAGLINE} from '@/lib/config';
import {ThemeProvider, themeNoFlashScript} from '@/lib/theme';
import {ToastProvider} from '@/components/ui/Toast';

export const metadata: Metadata = {
  title: {
    default: `${APP_NAME} — ${APP_TAGLINE}`,
    template: `%s · ${APP_NAME}`,
  },
  description:
    'Live all-session stock prices, SEC filings, news and the chatter you follow — one calm page per stock.',
};

export default function RootLayout({
  children,
}: Readonly<{children: React.ReactNode}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        {/* Apply the persisted theme before paint to avoid a flash. */}
        <script dangerouslySetInnerHTML={{__html: themeNoFlashScript}} />
      </head>
      <body className="antialiased">
        <ThemeProvider>
          <AuthProvider>
            <ToastProvider>{children}</ToastProvider>
          </AuthProvider>
        </ThemeProvider>
      </body>
    </html>
  );
}
