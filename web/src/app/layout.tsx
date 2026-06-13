import type {Metadata} from 'next';
import './globals.css';
import {AuthProvider} from '@/lib/auth';
import {APP_NAME, APP_TAGLINE, SITE_URL} from '@/lib/config';
import {ogImageMeta} from '@/lib/og';
import {langNoFlashScript} from '@/lib/i18n';
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
    images: [
      ogImageMeta({
        eyebrow: '中文美股数据台',
        title: '美股实时行情 · 国会交易 · 13F · 期权异动',
        subtitle: '数据优先,免费看清美股 — 行情/SEC内部人/国会山股神/财报',
      }),
    ],
  },
  twitter: {
    card: 'summary_large_image',
    title: `${APP_NAME} — ${APP_TAGLINE}`,
    description: DESCRIPTION,
    images: [ogImageMeta({eyebrow: '中文美股数据台', title: '美股实时行情 · 国会交易 · 13F · 期权异动'}).url],
  },
};

export default function RootLayout({
  children,
}: Readonly<{children: React.ReactNode}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        {/* Apply the persisted theme + language before paint to avoid a flash. */}
        <script dangerouslySetInnerHTML={{__html: themeNoFlashScript}} />
        <script dangerouslySetInnerHTML={{__html: langNoFlashScript}} />
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
