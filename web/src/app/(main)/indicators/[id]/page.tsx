import type {Metadata} from 'next';
import Link from 'next/link';
import {notFound} from 'next/navigation';
import {LayoutGrid} from 'lucide-react';
import {
  getIndicators,
  indicatorBySlug,
  indicatorSlug,
  type Indicator,
} from '@/lib/api';
import {SITE_URL, langAlternates} from '@/lib/config';
import {ogImageMeta, type OgParams} from '@/lib/og';
import {LocalizedTitle} from '@/components/LocalizedTitle';
import {ShareCardButton} from '@/components/ShareCardButton';

// SSR with ISR: the catalog is static (embedded metadata) and only changes on a
// deploy that updates the dataset, so mirror the catalog page's long window.
export const revalidate = 86400;

/** Chinese domain label for the eyebrow / badge (data domain → 中文 term). */
const DOMAIN_ZH: Record<string, string> = {
  technical: '技术指标',
  fundamental: '基本面',
  sentiment: '情绪指标',
};

/** Pre-render every catalog record (282) so each page is static / ISR. */
export async function generateStaticParams(): Promise<{id: string}[]> {
  try {
    const data = await getIndicators({}, AbortSignal.timeout(5000));
    return data.indicators.map(ind => ({id: indicatorSlug(ind.id)}));
  } catch {
    // API unavailable at build → emit no params; pages render on-demand via ISR.
    return [];
  }
}

/** Fetches the catalog (best-effort) and resolves the record for a URL slug. */
async function lookup(slug: string): Promise<Indicator | null> {
  try {
    const data = await getIndicators({}, AbortSignal.timeout(5000));
    return indicatorBySlug(data.indicators, slug) ?? null;
  } catch {
    return null;
  }
}

/**
 * The English / Chinese display name (English-default per the owner principle;
 * the Chinese name comes from name_zh). Used in titles + headers.
 */
function displayNames(ind: Indicator): {en: string; zh: string} {
  return {
    en: ind.name_en,
    zh: ind.name_zh || ind.name_en,
  };
}

/** Builds the English / Chinese browser-tab titles for an indicator. */
function titles(ind: Indicator): {en: string; zh: string} {
  const n = displayNames(ind);
  const enQualifier = ind.abbr && ind.abbr !== n.en ? `${n.en} (${ind.abbr})` : n.en;
  return {
    en: `${enQualifier} — Definition, Formula & How to Read · Tickwind`,
    zh: `${n.zh}${ind.abbr ? ` ${ind.abbr}` : ''} —— 定义 / 计算公式 / 解读 · 潮汐 Tickwind`,
  };
}

/** First non-empty descriptive line (definition → interpretation → formula). */
function snippet(ind: Indicator): string {
  return ind.definition || ind.interpretation || ind.formula || '';
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{id: string}>;
}): Promise<Metadata> {
  const {id} = await params;
  const ind = await lookup(id);
  if (!ind) return {title: 'Indicator'};

  const n = displayNames(ind);
  const tt = titles(ind);
  const domainZh = DOMAIN_ZH[ind.domain] ?? ind.domain_name;
  // Canonical/hreflang path from the record's OWN slug (not the raw param), so
  // it stays correct even if slug lookup is ever loosened to normalize input.
  const path = `/indicators/${indicatorSlug(ind.id)}`;
  const snip = snippet(ind);

  return {
    // English-default tab title (LocalizedTitle swaps zh); Chinese keywords live
    // in description/keywords for the targeting.
    title: {absolute: tt.en},
    description: `${n.zh}${ind.abbr ? `(${ind.abbr})` : ''}的定义、计算公式与解读要点 —— ${snip} ${n.en} for US stocks: definition, formula and how to read it. 公开知识,不构成投资建议。`,
    keywords: [
      n.zh,
      n.en,
      ind.abbr,
      `${n.zh} 公式`,
      `${n.zh} 是什么`,
      `美股${domainZh}`,
      `${n.en} formula`,
    ].filter(Boolean) as string[],
    alternates: langAlternates(path),
    openGraph: {
      type: 'article',
      title: tt.en,
      url: `${SITE_URL}${path}`,
      images: [
        ogImageMeta({
          eyebrow: domainZh,
          title: `${n.zh}${ind.abbr ? ` ${ind.abbr}` : ''}`,
          subtitle: snip.slice(0, 64),
        }),
      ],
    },
  };
}

/**
 * Per-indicator detail page (pSEO): one indicator's definition, formula, default
 * parameters and interpretation from the public-knowledge `/v1/indicators`
 * catalog. Server-rendered so crawlers get the full content; the inactive
 * language is hidden by the [data-i18n] CSS keyed to <html lang>, the tab title
 * swapped by LocalizedTitle. Renders ONLY fields present in the dataset (empty
 * definitions fall back to formula + interpretation — nothing is invented).
 * Unknown slug → notFound().
 */
export default async function IndicatorRoute({params}: {params: Promise<{id: string}>}) {
  const {id} = await params;

  // Need the full catalog both to resolve the record and to build the related
  // list, so fetch once. A transient API failure → notFound (ISR refills later).
  let all: Indicator[] = [];
  try {
    all = (await getIndicators({}, AbortSignal.timeout(5000))).indicators;
  } catch {
    all = [];
  }
  const ind = indicatorBySlug(all, id);
  if (!ind) notFound();

  const slug = indicatorSlug(ind.id);
  const n = displayNames(ind);
  const tt = titles(ind);
  const domainZh = DOMAIN_ZH[ind.domain] ?? ind.domain_name;
  const core = ind.priority === 'P0';

  // Definition is shown when present; otherwise we fall back to the formula and
  // interpretation (both rendered below regardless), never inventing copy.
  const hasDefinition = !!ind.definition;

  // Related: same subcategory first, then fill from the same domain, excluding
  // self. Capped at 8 for a clean internal-linking block.
  const related: Indicator[] = [];
  const seen = new Set<string>([ind.id]);
  for (const pool of [
    all.filter(x => x.subcategory === ind.subcategory),
    all.filter(x => x.domain === ind.domain),
  ]) {
    for (const x of pool) {
      if (seen.has(x.id) || related.length >= 8) continue;
      seen.add(x.id);
      related.push(x);
    }
  }

  const description = snippet(ind);
  // Propagation organ: a branded, shareable indicator card (Chinese-first, like
  // the OG image) so the glossary entry can spread on social.
  const shareCard: OgParams = {
    kind: 'page',
    eyebrow: domainZh,
    title: ind.abbr ? `${n.zh} ${ind.abbr}` : n.zh,
    subtitle: description || undefined,
  };
  const ld = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'DefinedTerm',
        name: n.en,
        ...(ind.abbr ? {alternateName: ind.abbr} : {}),
        ...(description ? {description} : {}),
        inDefinedTermSet: {
          '@type': 'DefinedTermSet',
          name: 'Tickwind Stock Indicator Library',
          url: `${SITE_URL}/indicators`,
        },
        url: `${SITE_URL}/indicators/${slug}`,
      },
      {
        '@type': 'BreadcrumbList',
        itemListElement: [
          {'@type': 'ListItem', position: 1, name: 'Tickwind', item: SITE_URL},
          {'@type': 'ListItem', position: 2, name: 'Indicators', item: `${SITE_URL}/indicators`},
          {
            '@type': 'ListItem',
            position: 3,
            name: n.en,
            item: `${SITE_URL}/indicators/${slug}`,
          },
        ],
      },
    ],
  };

  return (
    <article className="mx-auto max-w-3xl">
      <LocalizedTitle en={tt.en} zh={tt.zh} />
      <script type="application/ld+json" dangerouslySetInnerHTML={{__html: JSON.stringify(ld)}} />

      <nav className="mb-4 text-[12px] text-slate-500 dark:text-slate-400" aria-label="Breadcrumb">
        <Link href="/" className="hover:underline">
          <span data-i18n="zh">首页</span>
          <span data-i18n="en">Home</span>
        </Link>
        <span className="mx-1.5">/</span>
        <Link href="/indicators" className="hover:underline">
          <span data-i18n="zh">指标库</span>
          <span data-i18n="en">Indicators</span>
        </Link>
      </nav>

      <header className="mb-4">
        <div className="flex items-start justify-between gap-3">
          <h1 className="flex flex-wrap items-center gap-x-2 gap-y-1 text-[24px] font-bold tracking-tight text-slate-900 dark:text-slate-100">
            <LayoutGrid size={20} className="text-teal-600 dark:text-teal-300" />
            {/* English-default name; zh leads for Chinese users (data-i18n CSS). */}
            <span data-i18n="zh">{n.zh}</span>
            <span data-i18n="en">{n.en}</span>
            {ind.abbr && (
              <span className="text-[15px] font-medium text-slate-500 dark:text-slate-400">
                {ind.abbr}
              </span>
            )}
          </h1>
          {/* propagation organ: save a branded indicator card */}
          <ShareCardButton card={shareCard} />
        </div>
        {/* The other-language name on a secondary line, so both are on the page. */}
        {n.zh !== n.en && (
          <p className="mt-1 text-[13px] text-slate-500 dark:text-slate-400">
            <span data-i18n="zh">{n.en}</span>
            <span data-i18n="en">{n.zh}</span>
          </p>
        )}

        <div className="mt-3 flex flex-wrap items-center gap-2">
          <span className="inline-block rounded-full bg-teal-50 px-2.5 py-0.5 text-[11px] font-semibold text-teal-700 dark:bg-teal-500/15 dark:text-teal-300">
            {domainZh}
            <span className="ml-1 text-teal-600/70 dark:text-teal-300/70">{ind.domain_name}</span>
          </span>
          <span
            className={
              core
                ? 'inline-block rounded-full bg-teal-600 px-2.5 py-0.5 text-[11px] font-bold text-white dark:bg-teal-500'
                : 'inline-block rounded-full bg-slate-100 px-2.5 py-0.5 text-[11px] font-semibold text-slate-500 dark:bg-slate-800 dark:text-slate-300'
            }
          >
            {ind.priority}
            <span data-i18n="zh">
              {' '}
              {core ? '核心' : ''}
            </span>
            <span data-i18n="en">{core ? ' Core' : ''}</span>
          </span>
          {ind.subcategory && (
            <span className="inline-block rounded-full bg-slate-100 px-2.5 py-0.5 text-[11px] font-medium text-slate-500 dark:bg-slate-800 dark:text-slate-300">
              {ind.subcategory}
            </span>
          )}
          <span className="text-[11.5px] text-slate-400 dark:text-slate-500">
            <span data-i18n="zh">适用:美股</span>
            <span data-i18n="en">Applies to: US stocks</span>
          </span>
        </div>
      </header>

      {/* Definition — falls back to formula + interpretation when empty. */}
      {hasDefinition && (
        <section className="mb-5">
          <h2 className="mb-1.5 text-[12px] font-semibold uppercase tracking-wide text-slate-400 dark:text-slate-500">
            <span data-i18n="zh">定义</span>
            <span data-i18n="en">Definition</span>
          </h2>
          <p className="text-[14px] leading-relaxed text-slate-700 dark:text-slate-200">
            {ind.definition}
          </p>
        </section>
      )}

      {ind.formula && (
        <section className="mb-5">
          <h2 className="mb-1.5 text-[12px] font-semibold uppercase tracking-wide text-slate-400 dark:text-slate-500">
            <span data-i18n="zh">计算公式</span>
            <span data-i18n="en">Formula</span>
          </h2>
          <pre className="overflow-x-auto whitespace-pre-wrap break-words rounded-xl bg-slate-50 px-3.5 py-3 font-mono text-[12.5px] leading-relaxed text-slate-800 dark:bg-slate-900 dark:text-slate-100">
            {ind.formula}
          </pre>
        </section>
      )}

      {ind.default_params != null && (
        <section className="mb-5">
          <h2 className="mb-1.5 text-[12px] font-semibold uppercase tracking-wide text-slate-400 dark:text-slate-500">
            <span data-i18n="zh">默认参数</span>
            <span data-i18n="en">Default parameters</span>
          </h2>
          <pre className="overflow-x-auto whitespace-pre-wrap break-words rounded-xl bg-slate-50 px-3.5 py-3 font-mono text-[12.5px] leading-relaxed text-slate-800 dark:bg-slate-900 dark:text-slate-100">
            {JSON.stringify(ind.default_params, null, 2)}
          </pre>
        </section>
      )}

      {ind.interpretation && (
        <section className="mb-5">
          <h2 className="mb-1.5 text-[12px] font-semibold uppercase tracking-wide text-slate-400 dark:text-slate-500">
            <span data-i18n="zh">解读要点</span>
            <span data-i18n="en">How to read it</span>
          </h2>
          <p className="text-[14px] leading-relaxed text-slate-700 dark:text-slate-200">
            {ind.interpretation}
          </p>
        </section>
      )}

      {/* Metadata chips: inputs / output shape / library hint — real fields only. */}
      {(ind.output_type || (ind.inputs && ind.inputs.length > 0) || ind.talib_or_lib) && (
        <div className="mb-5 flex flex-wrap gap-1.5">
          {ind.inputs && ind.inputs.length > 0 && (
            <MetaChip labelZh="输入" labelEn="Inputs" value={ind.inputs.join(', ')} />
          )}
          {ind.output_type && <MetaChip labelZh="输出" labelEn="Output" value={ind.output_type} />}
          {ind.talib_or_lib && (
            <MetaChip labelZh="库" labelEn="Library" value={ind.talib_or_lib} mono />
          )}
        </div>
      )}

      <div className="mb-6 rounded-xl border border-slate-200 bg-slate-50 p-3 text-[12px] text-slate-500 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-400">
        {ind.data_source && (
          <span className="mr-1.5 font-semibold text-slate-600 dark:text-slate-300">
            <span data-i18n="zh">数据来源:{ind.data_source}。</span>
            <span data-i18n="en">Data source: {ind.data_source}. </span>
          </span>
        )}
        <span data-i18n="zh">公开知识,不构成投资建议。</span>
        <span data-i18n="en">Public knowledge, not investment advice.</span>
      </div>

      {/* CTAs: back to the library + see it computed on a stock. */}
      <div className="mb-7 flex flex-wrap gap-2">
        <Link
          href="/indicators"
          className="rounded-lg border border-slate-200 px-3 py-1.5 text-[12.5px] font-medium text-slate-600 hover:bg-slate-50 dark:border-slate-800 dark:text-slate-300 dark:hover:bg-slate-900"
        >
          <span data-i18n="zh">← 返回指标库</span>
          <span data-i18n="en">← Back to all indicators</span>
        </Link>
        <Link
          href="/indicators"
          className="rounded-lg bg-teal-600 px-3 py-1.5 text-[12.5px] font-semibold text-white hover:bg-teal-700 dark:bg-teal-500 dark:hover:bg-teal-600"
        >
          <span data-i18n="zh">在个股页查看该指标的实时计算</span>
          <span data-i18n="en">See it computed on a stock</span>
        </Link>
      </div>

      {related.length > 0 && (
        <section className="mb-6">
          <h2 className="mb-2.5 text-[15px] font-bold text-slate-900 dark:text-slate-100">
            <span data-i18n="zh">相关指标</span>
            <span data-i18n="en">Related indicators</span>
          </h2>
          <div className="grid gap-2 sm:grid-cols-2">
            {related.map(r => (
              <RelatedCard key={r.id} ind={r} />
            ))}
          </div>
        </section>
      )}

      <p className="mt-6 text-center text-[11px] text-slate-400 dark:text-slate-500">
        <span data-i18n="zh">参考元数据 —— 公开、通用的指标定义。非投资建议。</span>
        <span data-i18n="en">
          Reference metadata — public, well-known indicator definitions. Not investment advice.
        </span>
      </p>
    </article>
  );
}

/** A small key:value metadata chip (bilingual label via the [data-i18n] CSS). */
function MetaChip({
  labelZh,
  labelEn,
  value,
  mono,
}: {
  labelZh: string;
  labelEn: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <span className="inline-flex items-center gap-1 rounded-md bg-slate-100 px-2 py-0.5 text-[11px] dark:bg-slate-800">
      <span className="text-slate-400 dark:text-slate-500">
        <span data-i18n="zh">{labelZh}</span>
        <span data-i18n="en">{labelEn}</span>
      </span>
      <span className={`text-slate-600 dark:text-slate-300 ${mono ? 'font-mono' : ''}`}>{value}</span>
    </span>
  );
}

/** One related-indicator link card (internal linking → its detail page). */
function RelatedCard({ind}: {ind: Indicator}) {
  return (
    <Link
      href={`/indicators/${indicatorSlug(ind.id)}`}
      className="block rounded-xl border border-slate-200 px-3 py-2.5 hover:border-teal-300 hover:bg-slate-50 dark:border-slate-800 dark:hover:border-teal-500/40 dark:hover:bg-slate-900"
    >
      <div className="flex items-center gap-1.5 text-[13px] font-semibold text-slate-800 dark:text-slate-100">
        <span data-i18n="zh">{ind.name_zh || ind.name_en}</span>
        <span data-i18n="en">{ind.name_en}</span>
        {ind.abbr && (
          <span className="text-[11px] font-medium text-slate-400 dark:text-slate-500">
            {ind.abbr}
          </span>
        )}
      </div>
      {ind.subcategory && (
        <div className="mt-0.5 text-[11px] text-slate-400 dark:text-slate-500">{ind.subcategory}</div>
      )}
    </Link>
  );
}
