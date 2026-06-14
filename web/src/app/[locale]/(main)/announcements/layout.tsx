import type {Metadata} from 'next';
import {langAlternates} from '@/lib/config';
import {isLocale} from '@/lib/locale';

// The announcements page itself is a `'use client'` component (it reads the
// theme hook), so it cannot export `generateMetadata`. This Server Component
// layout supplies the per-locale canonical + hreflang the client page can't,
// and otherwise just renders its children.
export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string}>;
}): Promise<Metadata> {
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  return {
    title: "What's new",
    description: 'Product updates and release notes for Tickwind.',
    alternates: langAlternates('/announcements', loc),
  };
}

export default function AnnouncementsLayout({
  children,
}: Readonly<{children: React.ReactNode}>) {
  return children;
}
