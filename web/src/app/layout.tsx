import type {Metadata} from 'next';
import './globals.css';
import {APP_NAME, APP_TAGLINE, SITE_URL} from '@/lib/config';
import {ogImageMeta} from '@/lib/og';

const DESCRIPTION =
  'Live all-session stock prices, SEC filings, news and the chatter you follow — one calm page per stock.';

// Global metadata (metadataBase + defaults) applies to every page. The actual
// <html>/<body> + provider stack live in `[locale]/layout.tsx` — all pages are
// under `[locale]`, so this root layout is a minimal pass-through (the standard
// next-intl pattern for a path-prefixed locale segment).
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

export default function RootLayout({children}: Readonly<{children: React.ReactNode}>) {
  return children;
}
