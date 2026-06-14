import type {Metadata} from 'next';
import {langAlternates} from '@/lib/config';
import {isLocale} from '@/lib/locale';
import {Screener} from '@/components/Screener';

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string}>;
}): Promise<Metadata> {
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  return {
    title: 'Stock screener',
    description:
      'Filter US stocks by price, daily % change, and trading session over the whole market. Delayed quotes. Not investment advice.',
    alternates: langAlternates('/screen', loc),
  };
}

/** Public stock screener over the whole-US universe quote cache. */
export default function ScreenPage() {
  return (
    <div className="mx-auto max-w-3xl">
      <Screener />
    </div>
  );
}
