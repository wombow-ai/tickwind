import type {Metadata} from 'next';
import {AlertTriangle, Layers} from 'lucide-react';
import {notFound} from 'next/navigation';
import Link from '@/components/LocalLink';
import {ZoneLayers} from '@/components/ZoneLayers';
import {ZoneStack} from '@/components/ZoneStack';
import {SITE_URL, langAlternates} from '@/lib/config';
import {isLocale, LOCALES} from '@/lib/locale';
import {ogImageMeta} from '@/lib/og';
import {ZONES, zoneByKey, zoneTickers} from '@/lib/zones';

// Curated structure (no live data in the HTML beyond the static editorial layers),
// so a long revalidate is plenty — config changes ship via a deploy. Prices hydrate
// client-side.
export const revalidate = 3600;

/** Pre-render every zone × locale at build time. */
export function generateStaticParams(): {locale: string; key: string}[] {
  return LOCALES.flatMap(locale => ZONES.map(z => ({locale, key: z.key})));
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string; key: string}>;
}): Promise<Metadata> {
  const {locale, key} = await params;
  const z = zoneByKey(key);
  if (!z) return {title: 'Zone'};
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';
  const title = zh ? z.titleZh : z.titleEn;
  const desc = zh ? z.descZh : z.descEn;
  const path = `/zone/${z.key}`;
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
          eyebrow: zh ? '主题专区' : 'Theme Zone',
          title,
          subtitle: (zh ? z.taglineZh : z.taglineEn).slice(0, 60),
        }),
      ],
    },
  };
}

/**
 * Curated investment-theme zone (pSEO): a vertical, chokepoint-flagged map of a
 * theme's public companies, layer by layer. Server-rendered single-locale (the
 * editorial structure is the crawlable content); per-ticker prices hydrate live
 * client-side from Go. Curated structure + Go-owned numbers = anti-hallucination
 * safe. Unknown slug → notFound().
 */
export default async function ZoneRoute({
  params,
}: {
  params: Promise<{locale: string; key: string}>;
}) {
  const {locale, key} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';
  const z = zoneByKey(key);
  if (!z) notFound();

  const title = zh ? z.titleZh : z.titleEn;
  const tagline = zh ? z.taglineZh : z.taglineEn;
  const desc = zh ? z.descZh : z.descEn;
  const path = `/zone/${z.key}`;
  const tickers = zoneTickers(z);

  const ld = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'ItemList',
        name: title,
        description: desc,
        numberOfItems: tickers.length,
        itemListElement: tickers.map((tk, i) => ({
          '@type': 'ListItem',
          position: i + 1,
          name: tk,
          url: `${SITE_URL}/${loc}/stock/${encodeURIComponent(tk)}`,
        })),
      },
      {
        '@type': 'BreadcrumbList',
        itemListElement: [
          {'@type': 'ListItem', position: 1, name: 'Tickwind', item: `${SITE_URL}/${loc}`},
          {'@type': 'ListItem', position: 2, name: zh ? '主题专区' : 'Zones', item: `${SITE_URL}/${loc}/zone`},
          {'@type': 'ListItem', position: 3, name: title, item: `${SITE_URL}/${loc}${path}`},
        ],
      },
    ],
  };

  const others = ZONES.filter(o => o.key !== z.key);

  return (
    <article className="mx-auto max-w-3xl">
      <script type="application/ld+json" dangerouslySetInnerHTML={{__html: JSON.stringify(ld)}} />

      <nav className="mb-4 text-[12px] text-slate-500 dark:text-slate-400" aria-label="Breadcrumb">
        <Link href="/" className="hover:underline">
          {zh ? '首页' : 'Home'}
        </Link>
        <span className="mx-1.5">/</span>
        <Link href="/zone" className="hover:underline">
          {zh ? '主题专区' : 'Zones'}
        </Link>
      </nav>

      <header className="mb-5">
        <h1 className="flex items-center gap-2 text-[24px] font-bold tracking-tight text-slate-900 dark:text-slate-100">
          <Layers size={20} className="text-violet-600 dark:text-violet-300" />
          {title}
        </h1>
        <p className="mt-1 text-[12.5px] font-medium text-violet-700 dark:text-violet-300">{tagline}</p>
        <p className="mt-2 text-[13.5px] leading-relaxed text-slate-600 dark:text-slate-300">{desc}</p>
      </header>

      {z.speculative && (
        <div className="mb-5 flex items-start gap-2 rounded-xl border border-amber-300 bg-amber-50 px-3 py-2.5 text-[12.5px] text-amber-800 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-200">
          <AlertTriangle size={15} className="mt-0.5 shrink-0" />
          <span>
            {zh
              ? '高度投机主题:本专区多为临床期 / 早期 / 高估值个股,波动与归零风险极大。仅供研究,非投资建议。'
              : 'Highly speculative theme: many names here are clinical-stage / early / extreme-multiple, with large drawdown and total-loss risk. For research only — not investment advice.'}
          </span>
        </div>
      )}

      {/* AI flagship: a funnel/"cake" diagram of the ordered layers (simple, intuitive
          overview) above the detailed per-layer cards. tenx-theme zones are sibling
          sub-themes (no hierarchy), so the funnel is gated to the flagship. */}
      {z.kind === 'ai-flagship' && <ZoneStack zone={z} zh={zh} />}

      <ZoneLayers zone={z} zh={zh} />

      <p className="mt-6 text-center text-[11px] text-slate-400 dark:text-slate-500">
        {zh
          ? '层级为人工策展 · 价格为公开数据实时(延迟)行情 · 仅供参考 · 非投资建议'
          : 'Curated structure · live delayed prices from public data · for reference only · not investment advice'}
      </p>

      <section className="mt-8">
        <h2 className="mb-2.5 text-[15px] font-bold text-slate-900 dark:text-slate-100">
          {zh ? '更多专区' : 'More zones'}
        </h2>
        <div className="grid gap-2 sm:grid-cols-2">
          {others.map(o => (
            <Link
              key={o.key}
              href={`/zone/${o.key}`}
              className="block rounded-xl border border-slate-200 px-3 py-2.5 hover:border-violet-300 hover:bg-slate-50 dark:border-slate-800 dark:hover:border-violet-500/40 dark:hover:bg-slate-900"
            >
              <div className="text-[13px] font-semibold text-slate-800 dark:text-slate-100">{zh ? o.titleZh : o.titleEn}</div>
            </Link>
          ))}
          <Link
            href="/zone"
            className="block rounded-xl border border-dashed border-slate-300 px-3 py-2.5 text-[13px] font-medium text-slate-600 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-900"
          >
            {zh ? '全部专区 →' : 'All zones →'}
          </Link>
        </div>
      </section>
    </article>
  );
}
