import type {Metadata} from 'next';
import {type IndicatorsResponse} from '@/lib/api';
import {localCatalog} from '@/lib/catalog';
import {langAlternates} from '@/lib/config';
import {isLocale} from '@/lib/locale';
import {ogImageMeta} from '@/lib/og';
import {IndicatorLibraryShell} from '@/components/IndicatorLibraryShell';

// The catalog is static (embedded metadata), so a long ISR window is plenty —
// it only changes on a deploy that updates the dataset.
export const revalidate = 86400;

// Per-locale browser-tab title, chosen server-side by the route locale in
// generateMetadata; Chinese SEO keywords stay in description/keywords.
const TITLE_EN = 'Stock Indicator Library — Technical, Fundamental & Sentiment · Tickwind';
const TITLE_ZH = '美股指标大全 · 技术 / 基本面 / 情绪指标库 · 潮汐 Tickwind';

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string}>;
}): Promise<Metadata> {
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  return {
    title: {absolute: loc === 'zh' ? TITLE_ZH : TITLE_EN},
    description:
      '美股指标大全:技术指标(RSI、MACD、布林带、KDJ)、基本面指标(市盈率 PE、市净率、ROE、毛利率)与情绪指标的完整参考库 —— 每个指标含定义、计算公式、默认参数与解读要点。A searchable reference of US-stock technical, fundamental and sentiment indicators with formulas. 公开知识,不构成投资建议。',
    keywords: [
      '美股技术指标',
      '美股基本面指标',
      '股票指标大全',
      'RSI',
      'MACD',
      '市盈率',
      'PE',
      '布林带',
      'ROE',
      'stock indicators',
      'technical indicators',
    ],
    alternates: langAlternates('/indicators', loc),
    openGraph: {
      images: [
        ogImageMeta(
          loc === 'zh'
            ? {
                lang: 'zh',
                eyebrow: '指标库',
                title: '美股指标大全 · 技术 / 基本面 / 情绪',
                subtitle: '含公式、默认参数与解读要点的可检索指标参考',
              }
            : {
                lang: 'en',
                eyebrow: 'Indicator library',
                title: 'US-stock indicators · technical / fundamental / sentiment',
                subtitle: 'A searchable reference with formulas, default params & how to read them',
              },
        ),
      ],
    },
  };
}

/**
 * The browsable stock-indicator library (SSR/ISR, high pSEO value). Fetches the
 * stock-applicable catalog server-side so the full content is in the initial
 * HTML for crawlers; the client component then handles instant search/filter
 * over the embedded set. A slow/down API degrades to an empty catalog rather
 * than failing the render.
 */
export default async function IndicatorsPage() {
  let data: IndicatorsResponse = {
    count: 0,
    total: 0,
    indicators: [],
    facets: {domains: [], priorities: [], subcategories: []},
  };
  data = localCatalog({}); // bundled static catalog — Vercel can't fetch the 150KB endpoint SSR

  return (
    <div className="mx-auto max-w-5xl">
      <IndicatorLibraryShell
        indicators={data.indicators}
        facets={data.facets}
        total={data.total}
      />
    </div>
  );
}
