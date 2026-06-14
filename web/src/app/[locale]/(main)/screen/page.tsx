import type {Metadata} from 'next';
import Link from '@/components/LocalLink';
import {langAlternates} from '@/lib/config';
import {isLocale} from '@/lib/locale';
import {SCREEN_PRESETS} from '@/lib/presets';
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
export default async function ScreenPage({
  params,
}: {
  params: Promise<{locale: string}>;
}) {
  const {locale} = await params;
  const zh = (isLocale(locale) ? locale : 'en') === 'zh';
  return (
    <div className="mx-auto max-w-3xl">
      <Screener />

      {/* Curated preset landing pages — internal links into the pSEO screener family. */}
      <section className="mt-8">
        <h2 className="mb-2.5 text-[15px] font-bold text-slate-900 dark:text-slate-100">
          {zh ? '热门筛选榜单' : 'Popular screener lists'}
        </h2>
        <div className="grid gap-2 sm:grid-cols-2">
          {SCREEN_PRESETS.map(p => (
            <Link
              key={p.key}
              href={`/screen/${p.key}`}
              className="block rounded-xl border border-slate-200 px-3 py-2.5 hover:border-sky-300 hover:bg-slate-50 dark:border-slate-800 dark:hover:border-sky-500/40 dark:hover:bg-slate-900"
            >
              <div className="text-[13px] font-semibold text-slate-800 dark:text-slate-100">
                {zh ? p.titleZh : p.titleEn}
              </div>
            </Link>
          ))}
        </div>
      </section>
    </div>
  );
}
