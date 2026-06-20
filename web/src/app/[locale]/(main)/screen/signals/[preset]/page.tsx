import type {Metadata} from 'next';
import {notFound} from 'next/navigation';
import {ScanLine} from 'lucide-react';
import {SITE_URL, langAlternates} from '@/lib/config';
import {isLocale, LOCALES} from '@/lib/locale';
import {ogImageMeta} from '@/lib/og';
import {SIGNAL_SCREEN_PRESETS, signalPresetByKey} from '@/lib/signalPresets';
import {SignalsScreen} from '@/components/SignalsScreen';

/** Pre-render every signal preset × locale at build time. */
export function generateStaticParams(): {locale: string; preset: string}[] {
  return LOCALES.flatMap(locale => SIGNAL_SCREEN_PRESETS.map(p => ({locale, preset: p.key})));
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string; preset: string}>;
}): Promise<Metadata> {
  const {locale, preset} = await params;
  const p = signalPresetByKey(preset);
  if (!p) return {title: 'Signal Screener'};
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';
  const title = zh ? p.titleZh : p.titleEn;
  const desc = zh ? p.descZh : p.descEn;
  const path = `/screen/signals/${p.key}`;
  return {
    title: {absolute: `${title} · Tickwind`},
    description: desc,
    alternates: langAlternates(path, loc),
    openGraph: {
      type: 'website',
      title,
      description: desc.slice(0, 110),
      url: `${SITE_URL}/${loc}${path}`,
      images: [
        ogImageMeta({
          lang: loc,
          eyebrow: zh ? '信号筛选' : 'Signal Screener',
          title,
          subtitle: desc.slice(0, 54),
        }),
      ],
    },
  };
}

/**
 * Curated signal-screener landing page (pSEO): one preset's fixed filter pre-applied
 * on the deterministic signals screener. The per-preset title + intro are
 * server-rendered for crawlers; the live ranked list is the interactive SignalsScreen.
 */
export default async function SignalPresetPage({
  params,
}: {
  params: Promise<{locale: string; preset: string}>;
}) {
  const {locale, preset} = await params;
  const p = signalPresetByKey(preset);
  if (!p) notFound();
  const zh = (isLocale(locale) ? locale : 'en') === 'zh';

  return (
    <div className="mx-auto max-w-3xl">
      <header className="mb-4">
        <h1 className="flex items-center gap-2 text-[22px] font-bold tracking-tight text-slate-900 dark:text-slate-100">
          <ScanLine size={20} className="text-violet-500 dark:text-violet-300" />
          {zh ? p.titleZh : p.titleEn}
        </h1>
        <p className="mt-1 text-[13px] text-slate-500 dark:text-slate-400">{zh ? p.descZh : p.descEn}</p>
      </header>

      <SignalsScreen initialDirection={p.direction ?? ''} initialSignal={p.signal ?? ''} hideHeader />
    </div>
  );
}
