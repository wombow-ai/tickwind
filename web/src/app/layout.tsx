import type {Metadata} from 'next';
import './globals.css';
import {AuthProvider} from '@/lib/auth';
import {APP_NAME, APP_TAGLINE, SITE_URL} from '@/lib/config';
import {ThemeProvider, themeNoFlashScript} from '@/lib/theme';
import {ToastProvider} from '@/components/ui/Toast';

const DESCRIPTION =
  'Live all-session stock prices, SEC filings, news and the chatter you follow — one calm page per stock.';

export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title: {
    default: `${APP_NAME} — ${APP_TAGLINE}`,
    template: `%s · ${APP_NAME}`,
  },
  description: DESCRIPTION,
  applicationName: APP_NAME,
  alternates: {canonical: '/'},
  openGraph: {
    type: 'website',
    siteName: APP_NAME,
    url: SITE_URL,
    title: `${APP_NAME} — ${APP_TAGLINE}`,
    description: DESCRIPTION,
  },
  twitter: {
    card: 'summary',
    title: `${APP_NAME} — ${APP_TAGLINE}`,
    description: DESCRIPTION,
  },
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
