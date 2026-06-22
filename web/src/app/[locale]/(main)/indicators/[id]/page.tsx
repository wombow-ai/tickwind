import type {Metadata} from 'next';
import Link from '@/components/LocalLink';
import {notFound} from 'next/navigation';
import {LayoutGrid} from 'lucide-react';
import {
  getIndicators,
  HISTORYABLE_INDICATOR_IDS,
  indicatorBySlug,
  indicatorSlug,
  type Indicator,
} from '@/lib/api';
import {SITE_URL, POPULAR_TICKERS, langAlternates} from '@/lib/config';
import {isLocale, LOCALES} from '@/lib/locale';
import {ogImageMeta, type OgParams} from '@/lib/og';
import {IndicatorHistoryChart} from '@/components/IndicatorHistoryChart';
import {ShareCardButton} from '@/components/ShareCardButton';

// Indicators that have a server-side time series get a live example chart on the detail page.
const HISTORY_CHARTABLE = new Set<string>(HISTORYABLE_INDICATOR_IDS);
// A liquid, always-available reference stock for the worked example.
const EXAMPLE_TICKER = 'AAPL';

// SSR with ISR: the catalog is static (embedded metadata) and only changes on a
// deploy that updates the dataset, so mirror the catalog page's long window.
export const revalidate = 86400;

/** Chinese domain label for the eyebrow / badge (data domain → 中文 term). */
const DOMAIN_ZH: Record<string, string> = {
  technical: '技术指标',
  fundamental: '基本面',
  sentiment: '情绪指标',
};

/** Pre-render every catalog record (282) × locale so each page is static / ISR. */
export async function generateStaticParams(): Promise<{locale: string; id: string}[]> {
  try {
    const data = await getIndicators({}, AbortSignal.timeout(5000));
    return LOCALES.flatMap(locale =>
      data.indicators.map(ind => ({locale, id: indicatorSlug(ind.id)})),
    );
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
  params: Promise<{locale: string; id: string}>;
}): Promise<Metadata> {
  const {locale, id} = await params;
  const ind = await lookup(id);
  if (!ind) return {title: 'Indicator'};

  const loc = isLocale(locale) ? locale : 'en';
  const n = displayNames(ind);
  const tt = titles(ind);
  const domainZh = DOMAIN_ZH[ind.domain] ?? ind.domain_name;
  // Canonical/hreflang path from the record's OWN slug (not the raw param), so
  // it stays correct even if slug lookup is ever loosened to normalize input.
  const path = `/indicators/${indicatorSlug(ind.id)}`;
  const snip = snippet(ind);

  return {
    // Locale-matched tab title; Chinese keywords live in description/keywords.
    title: {absolute: loc === 'zh' ? tt.zh : tt.en},
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
    alternates: langAlternates(path, loc),
    openGraph: {
      type: 'article',
      title: loc === 'zh' ? tt.zh : tt.en,
      url: `${SITE_URL}/${loc}${path}`,
      images: [
        ogImageMeta({
          lang: loc,
          eyebrow: loc === 'zh' ? domainZh : ind.domain_name,
          title:
            loc === 'zh'
              ? `${n.zh}${ind.abbr ? ` ${ind.abbr}` : ''}`
              : `${n.en}${ind.abbr ? ` ${ind.abbr}` : ''}`,
          subtitle: snip.slice(0, 64),
        }),
      ],
    },
  };
}

/**
 * Per-indicator detail page (pSEO): one indicator's definition, formula, default
 * parameters and interpretation from the public-knowledge `/v1/indicators`
 * catalog. Server-rendered so crawlers get the full content; only the active
 * locale's chrome (chosen from the route segment) is emitted, the per-locale tab
 * title set in generateMetadata, so /en and /zh are distinct single-language
 * HTML. Renders ONLY fields present in the dataset (empty definitions fall back
 * to formula + interpretation — nothing is invented). Unknown slug → notFound().
 */
export default async function IndicatorRoute({
  params,
}: {
  params: Promise<{locale: string; id: string}>;
}) {
  const {locale, id} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';

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
  const path = `/indicators/${slug}`;
  const n = displayNames(ind);
  const domainZh = DOMAIN_ZH[ind.domain] ?? ind.domain_name;
  // Domain label in the active locale (zh term vs the dataset's English name).
  const domainLabel = zh ? domainZh : ind.domain_name;
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
  // Propagation organ: a branded, shareable indicator card. Server component (no
  // useLang) → English-default chrome per the single-language-defaults-English
  // principle. The subtitle reuses the existing definition snippet.
  const shareCard: OgParams = {
    kind: 'page',
    eyebrow: ind.domain_name,
    title: ind.abbr ? `${n.en} ${ind.abbr}` : n.en,
    subtitle: description || undefined,
  };
  const ld = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'DefinedTerm',
        name: zh ? n.zh : n.en,
        ...(ind.abbr ? {alternateName: ind.abbr} : {}),
        ...(description ? {description} : {}),
        inDefinedTermSet: {
          '@type': 'DefinedTermSet',
          name: 'Tickwind Stock Indicator Library',
          url: `${SITE_URL}/${loc}/indicators`,
        },
        url: `${SITE_URL}/${loc}${path}`,
      },
      {
        '@type': 'BreadcrumbList',
        itemListElement: [
          {'@type': 'ListItem', position: 1, name: 'Tickwind', item: `${SITE_URL}/${loc}`},
          {'@type': 'ListItem', position: 2, name: zh ? '指标库' : 'Indicators', item: `${SITE_URL}/${loc}/indicators`},
          {
            '@type': 'ListItem',
            position: 3,
            name: zh ? n.zh : n.en,
            item: `${SITE_URL}/${loc}${path}`,
          },
        ],
      },
    ],
  };

  return (
    <article className="mx-auto max-w-3xl">
      <script type="application/ld+json" dangerouslySetInnerHTML={{__html: JSON.stringify(ld)}} />

      <nav className="mb-4 text-[12px] text-slate-500 dark:text-slate-400" aria-label="Breadcrumb">
        <Link href="/" className="hover:underline">
          {zh ? '首页' : 'Home'}
        </Link>
        <span className="mx-1.5">/</span>
        <Link href="/indicators" className="hover:underline">
          {zh ? '指标库' : 'Indicators'}
        </Link>
      </nav>

      <header className="mb-4">
        <div className="flex items-start justify-between gap-3">
          <h1 className="flex flex-wrap items-center gap-x-2 gap-y-1 text-[24px] font-bold tracking-tight text-slate-900 dark:text-slate-100">
            <LayoutGrid size={20} className="text-teal-600 dark:text-teal-300" />
            {zh ? n.zh : n.en}
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
            {zh ? n.en : n.zh}
          </p>
        )}

        <div className="mt-3 flex flex-wrap items-center gap-2">
          <span className="inline-block rounded-full bg-teal-50 px-2.5 py-0.5 text-[11px] font-semibold text-teal-700 dark:bg-teal-500/15 dark:text-teal-300">
            {domainLabel}
          </span>
          <span
            className={
              core
                ? 'inline-block rounded-full bg-teal-600 px-2.5 py-0.5 text-[11px] font-bold text-white dark:bg-teal-500'
                : 'inline-block rounded-full bg-slate-100 px-2.5 py-0.5 text-[11px] font-semibold text-slate-500 dark:bg-slate-800 dark:text-slate-300'
            }
          >
            {ind.priority}
            {core ? (zh ? ' 核心' : ' Core') : ''}
          </span>
          {ind.subcategory && (
            <span className="inline-block rounded-full bg-slate-100 px-2.5 py-0.5 text-[11px] font-medium text-slate-500 dark:bg-slate-800 dark:text-slate-300">
              {ind.subcategory}
            </span>
          )}
          <span className="text-[11.5px] text-slate-400 dark:text-slate-500">
            {zh ? '适用:美股' : 'Applies to: US stocks'}
          </span>
        </div>
      </header>

      {/* Live worked example: this indicator's real time series on a reference stock. Only
          for the indicators with a server-side history; client-fetched (Go-owns-the-numbers),
          so the static pSEO page gains a live, anti-hallucination-safe chart. */}
      {HISTORY_CHARTABLE.has(ind.id) && (
        <section className="mb-6">
          <h2 className="mb-1.5 text-[12px] font-semibold uppercase tracking-wide text-slate-400 dark:text-slate-500">
            {zh ? '走势示例' : 'Live example'}
          </h2>
          <p className="mb-2 text-[12.5px] leading-relaxed text-slate-500 dark:text-slate-400">
            {zh
              ? `${n.zh}在 ${EXAMPLE_TICKER} 上的历史走势,用真实日线数据计算 —— 可切换时间范围。`
              : `${n.en} on ${EXAMPLE_TICKER}, computed from real daily price data — switch the range to zoom.`}
          </p>
          <div className="rounded-2xl border border-slate-200 p-3 dark:border-slate-800">
            <IndicatorHistoryChart ticker={EXAMPLE_TICKER} id={ind.id} range="1Y" />
          </div>
          <p className="mt-1.5 text-[11px] text-slate-400 dark:text-slate-500">
            {zh ? '延迟数据 · 仅供参考 · 非投资建议' : 'Delayed data · For reference only · Not investment advice'}
          </p>
        </section>
      )}

      {/* Definition — falls back to formula + interpretation when empty. */}
      {hasDefinition && (
        <section className="mb-5">
          <h2 className="mb-1.5 text-[12px] font-semibold uppercase tracking-wide text-slate-400 dark:text-slate-500">
            {zh ? '定义' : 'Definition'}
          </h2>
          <p className="text-[14px] leading-relaxed text-slate-700 dark:text-slate-200">
            {ind.definition}
          </p>
        </section>
      )}

      {ind.formula && (
        <section className="mb-5">
          <h2 className="mb-1.5 text-[12px] font-semibold uppercase tracking-wide text-slate-400 dark:text-slate-500">
            {zh ? '计算公式' : 'Formula'}
          </h2>
          <pre className="overflow-x-auto whitespace-pre-wrap break-words rounded-xl bg-slate-50 px-3.5 py-3 font-mono text-[12.5px] leading-relaxed text-slate-800 dark:bg-slate-900 dark:text-slate-100">
            {ind.formula}
          </pre>
        </section>
      )}

      {ind.default_params != null && (
        <section className="mb-5">
          <h2 className="mb-1.5 text-[12px] font-semibold uppercase tracking-wide text-slate-400 dark:text-slate-500">
            {zh ? '默认参数' : 'Default parameters'}
          </h2>
          <pre className="overflow-x-auto whitespace-pre-wrap break-words rounded-xl bg-slate-50 px-3.5 py-3 font-mono text-[12.5px] leading-relaxed text-slate-800 dark:bg-slate-900 dark:text-slate-100">
            {JSON.stringify(ind.default_params, null, 2)}
          </pre>
        </section>
      )}

      {ind.interpretation && (
        <section className="mb-5">
          <h2 className="mb-1.5 text-[12px] font-semibold uppercase tracking-wide text-slate-400 dark:text-slate-500">
            {zh ? '解读要点' : 'How to read it'}
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
            <MetaChip label={zh ? '输入' : 'Inputs'} value={ind.inputs.join(', ')} />
          )}
          {ind.output_type && <MetaChip label={zh ? '输出' : 'Output'} value={ind.output_type} />}
          {ind.talib_or_lib && (
            <MetaChip label={zh ? '库' : 'Library'} value={ind.talib_or_lib} mono />
          )}
        </div>
      )}

      <div className="mb-6 rounded-xl border border-slate-200 bg-slate-50 p-3 text-[12px] text-slate-500 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-400">
        {ind.data_source && (
          <span className="mr-1.5 font-semibold text-slate-600 dark:text-slate-300">
            {zh ? `数据来源:${ind.data_source}。` : `Data source: ${ind.data_source}. `}
          </span>
        )}
        {zh ? '公开知识,不构成投资建议。' : 'Public knowledge, not investment advice.'}
      </div>

      {/* Activation funnel: turn a glossary reader into a product user by sending
          them to this indicator's LIVE computed value on real stocks (deep-link to
          the per-stock indicators panel via the #indicators anchor). */}
      <section className="mb-6 rounded-xl border border-teal-200 bg-teal-50/60 p-4 dark:border-teal-500/20 dark:bg-teal-500/5">
        <p className="mb-2.5 text-[13.5px] font-semibold text-slate-800 dark:text-slate-100">
          {zh
            ? `在热门美股上查看${n.zh}的实时计算值 →`
            : `See ${n.en} computed live on popular US stocks →`}
        </p>
        <div className="flex flex-wrap gap-1.5">
          {POPULAR_TICKERS.slice(0, 5).map(tk => (
            <Link
              key={tk}
              href={`/stock/${tk}#indicators`}
              className="rounded-lg bg-teal-600 px-2.5 py-1 text-[12.5px] font-bold tabular-nums text-white transition hover:bg-teal-700 dark:bg-teal-500 dark:hover:bg-teal-600"
            >
              {tk}
            </Link>
          ))}
          <Link
            href="/screen"
            className="rounded-lg border border-slate-300 px-2.5 py-1 text-[12.5px] font-medium text-slate-600 transition hover:bg-white dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-900"
          >
            {zh ? '全部美股 →' : 'All stocks →'}
          </Link>
        </div>
      </section>

      <div className="mb-7">
        <Link
          href="/indicators"
          className="inline-block rounded-lg border border-slate-200 px-3 py-1.5 text-[12.5px] font-medium text-slate-600 hover:bg-slate-50 dark:border-slate-800 dark:text-slate-300 dark:hover:bg-slate-900"
        >
          {zh ? '← 返回指标库' : '← Back to all indicators'}
        </Link>
      </div>

      {related.length > 0 && (
        <section className="mb-6">
          <h2 className="mb-2.5 text-[15px] font-bold text-slate-900 dark:text-slate-100">
            {zh ? '相关指标' : 'Related indicators'}
          </h2>
          <div className="grid gap-2 sm:grid-cols-2">
            {related.map(r => (
              <RelatedCard key={r.id} ind={r} zh={zh} />
            ))}
          </div>
        </section>
      )}

      <p className="mt-6 text-center text-[11px] text-slate-400 dark:text-slate-500">
        {zh
          ? '参考元数据 —— 公开、通用的指标定义。非投资建议。'
          : 'Reference metadata — public, well-known indicator definitions. Not investment advice.'}
      </p>
    </article>
  );
}

/** A small key:value metadata chip with an active-locale label. */
function MetaChip({
  label,
  value,
  mono,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <span className="inline-flex items-center gap-1 rounded-md bg-slate-100 px-2 py-0.5 text-[11px] dark:bg-slate-800">
      <span className="text-slate-400 dark:text-slate-500">{label}</span>
      <span className={`text-slate-600 dark:text-slate-300 ${mono ? 'font-mono' : ''}`}>{value}</span>
    </span>
  );
}

/** One related-indicator link card (internal linking → its detail page). */
function RelatedCard({ind, zh}: {ind: Indicator; zh: boolean}) {
  return (
    <Link
      href={`/indicators/${indicatorSlug(ind.id)}`}
      className="block rounded-xl border border-slate-200 px-3 py-2.5 hover:border-teal-300 hover:bg-slate-50 dark:border-slate-800 dark:hover:border-teal-500/40 dark:hover:bg-slate-900"
    >
      <div className="flex items-center gap-1.5 text-[13px] font-semibold text-slate-800 dark:text-slate-100">
        {zh ? ind.name_zh || ind.name_en : ind.name_en}
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
