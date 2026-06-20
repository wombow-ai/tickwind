import type {Metadata} from 'next';
import {Layers} from 'lucide-react';
import Link from '@/components/LocalLink';
import {SITE_URL, langAlternates} from '@/lib/config';
import {isLocale, LOCALES} from '@/lib/locale';
import {ogImageMeta} from '@/lib/og';
import {ZONES} from '@/lib/zones';

export function generateStaticParams(): {locale: string}[] {
  return LOCALES.map(locale => ({locale}));
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string}>;
}): Promise<Metadata> {
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';
  const title = zh ? '主题专区 — AI 产业链 · 10倍股主题' : 'Theme Zones — The AI Stack & 10x Themes';
  const desc = zh
    ? '按产业链层级与投资主题策展的美股专区 —— AI 五层蛋糕(供电/芯片/基础设施/大模型/应用)与卡脖子环节,以及更多高成长主题。结构为人工策展,价格为公开数据实时行情。非投资建议。'
    : "Curated US-stock zones by value-chain layer and investment theme — the AI five-layer stack (energy / chips / infrastructure / models / applications) with chokepoints flagged, plus more high-growth themes. Curated structure, live delayed prices. Not investment advice.";
  return {
    title: {absolute: `${title} · Tickwind`},
    description: desc,
    alternates: langAlternates('/zone', loc),
    openGraph: {
      type: 'website',
      title,
      description: desc.slice(0, 110),
      url: `${SITE_URL}/${loc}/zone`,
      images: [ogImageMeta({lang: loc, eyebrow: zh ? '主题专区' : 'Theme Zones', title, subtitle: zh ? 'AI 产业链 · 卡脖子 · 10倍股' : 'The AI stack · chokepoints · 10x themes'})],
    },
  };
}

/** Theme-zones hub (pSEO): lists the curated zones, each a layer-by-layer map of a
 *  theme's public companies. Single-locale render. */
export default async function ZoneHub({params}: {params: Promise<{locale: string}>}) {
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';

  const ld = {
    '@context': 'https://schema.org',
    '@type': 'CollectionPage',
    name: zh ? '主题专区' : 'Theme Zones',
    hasPart: ZONES.map(z => ({
      '@type': 'CollectionPage',
      name: zh ? z.titleZh : z.titleEn,
      url: `${SITE_URL}/${loc}/zone/${z.key}`,
    })),
  };

  return (
    <article className="mx-auto max-w-3xl">
      <script type="application/ld+json" dangerouslySetInnerHTML={{__html: JSON.stringify(ld)}} />
      <header className="mb-5">
        <h1 className="flex items-center gap-2 text-[24px] font-bold tracking-tight text-slate-900 dark:text-slate-100">
          <Layers size={20} className="text-violet-600 dark:text-violet-300" />
          {zh ? '主题专区' : 'Theme Zones'}
        </h1>
        <p className="mt-2 text-[13.5px] leading-relaxed text-slate-600 dark:text-slate-300">
          {zh
            ? '按产业链层级与投资主题策展的美股专区。每个专区把一个主题的上市公司一层层拆开,并标出供应链「卡脖子」环节。层级为人工策展,价格为公开数据实时(延迟)行情。非投资建议。'
            : "Curated US-stock zones by value-chain layer and investment theme. Each zone maps a theme's public companies layer by layer, with the supply-chain chokepoints flagged. Curated structure; live delayed prices. Not investment advice."}
        </p>
      </header>

      <div className="grid gap-3 sm:grid-cols-2">
        {ZONES.map(z => (
          <Link
            key={z.key}
            href={`/zone/${z.key}`}
            className="group block rounded-2xl border border-slate-200 bg-white p-4 transition hover:border-violet-300 hover:shadow-sm dark:border-slate-800 dark:bg-slate-950 dark:hover:border-violet-500/40"
          >
            <div className="flex items-center gap-2">
              <h2 className="text-[15.5px] font-bold text-slate-900 dark:text-slate-100">{zh ? z.titleZh : z.titleEn}</h2>
              {z.speculative && (
                <span className="rounded-full bg-amber-100 px-1.5 py-0.5 text-[10px] font-bold uppercase text-amber-700 dark:bg-amber-500/15 dark:text-amber-300">
                  {zh ? '投机' : 'Speculative'}
                </span>
              )}
            </div>
            <p className="mt-1 text-[12px] font-medium text-violet-700 dark:text-violet-300">{zh ? z.taglineZh : z.taglineEn}</p>
            <p className="mt-1.5 line-clamp-3 text-[12.5px] leading-relaxed text-slate-500 dark:text-slate-400">{zh ? z.descZh : z.descEn}</p>
          </Link>
        ))}
      </div>
    </article>
  );
}
