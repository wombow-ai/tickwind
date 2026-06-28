import type {Metadata} from 'next';
import {ApiError, getStock, type Security} from '@/lib/api';
import {SITE_URL, isDemoReportTicker, langAlternates} from '@/lib/config';
import {isLocale, LOCALES} from '@/lib/locale';
import {popularTickers} from '@/lib/pseo';
import {DeepResearchView} from '@/components/DeepResearchView';

interface Params {
  params: Promise<{locale: string; ticker: string}>;
}

// ISR: the SSR SHELL (metadata only) is cacheable per ticker. The deep report
// itself is fetched CLIENT-SIDE with the user's Bearer token — it is gated
// (auth + 1/day quota), so it is never fetched at build time or on the server.
export const revalidate = 600;

/**
 * Pre-render ONLY the popular-ticker subset × locale (same bounded set as the
 * parent stock page); everything else stays dynamic ISR (`dynamicParams`
 * defaults to true). Best-effort: `[]` on API failure so the build never breaks.
 * Note: this only pre-renders the static SHELL — the gated deep report is a
 * client fetch, so nothing here calls the gated endpoint at build time.
 */
export async function generateStaticParams(): Promise<{locale: string; ticker: string}[]> {
  try {
    const tickers = await popularTickers();
    return LOCALES.flatMap(locale =>
      tickers.map(ticker => ({locale, ticker: encodeURIComponent(ticker)})),
    );
  } catch {
    return [];
  }
}

/** Server-side security lookup for a richer title. Null on miss / non-US. */
async function fetchSecurity(t: string): Promise<Security | null> {
  try {
    return await getStock(t, AbortSignal.timeout(5000));
  } catch (e) {
    if (e instanceof ApiError) return null;
    return null;
  }
}

export async function generateMetadata({params}: Params): Promise<Metadata> {
  const {locale, ticker} = await params;
  const t = decodeURIComponent(ticker).toUpperCase();
  const loc = isLocale(locale) ? locale : 'en';
  const sec = await fetchSecurity(t);
  const name = sec?.name && sec.name !== t ? sec.name : t;
  const demo = isDemoReportTicker(t); // the evergreen free example report — indexable + no login copy
  const title =
    loc === 'zh' ? `${name} · AI 深度研报` : `${name} · AI Deep Research`;
  const description = demo
    ? loc === 'zh'
      ? `${name} 的 AI 深度研报(完整示例,免费):估值、基本面、技术、资金面与情绪面,数字均来自公开数据。非投资建议。`
      : `A complete AI deep-research example for ${name}: valuation, fundamentals, technicals, smart-money and sentiment — every figure from public data, free to read. Not investment advice.`
    : loc === 'zh'
      ? `${name} 的 AI 深度研报:估值、基本面、技术、资金面与情绪面分析,数字均来自公开数据。登录解锁。非投资建议。`
      : `AI deep-research report for ${name}: valuation, fundamentals, technicals, smart-money and sentiment — every figure from public data. Log in to unlock. Not investment advice.`;
  return {
    title,
    description,
    alternates: langAlternates(`/stock/${encodeURIComponent(t)}/research`, loc),
    // The demo report is a complete public example → indexable (a showcase SEO page). Every
    // other report is gated/per-user → noindex,follow (crawlers still reach the stock page).
    robots: demo ? {index: true, follow: true} : {index: false, follow: true},
  };
}

/**
 * The dedicated AI Deep Research route. A thin server shell (metadata + the
 * locale-aware client view); the actual report is fetched client-side with the
 * Supabase token inside {@link DeepResearchView}, which also renders the anon
 * login gate / loading / 429 / data-only states. Nothing gated runs on the
 * server or at build time.
 */
export default async function DeepResearchPage({params}: Params) {
  const {ticker} = await params;
  const t = decodeURIComponent(ticker).toUpperCase();
  // JSON-LD breadcrumb back to the public stock page (the report itself is
  // noindex, but the crumb keeps the link graph coherent).
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const jsonLd = {
    '@context': 'https://schema.org',
    '@type': 'BreadcrumbList',
    itemListElement: [
      {'@type': 'ListItem', position: 1, name: t, item: `${SITE_URL}/${loc}/stock/${encodeURIComponent(t)}`},
      {
        '@type': 'ListItem',
        position: 2,
        name: loc === 'zh' ? 'AI 深度研报' : 'AI Deep Research',
        item: `${SITE_URL}/${loc}/stock/${encodeURIComponent(t)}/research`,
      },
    ],
  };
  return (
    <>
      <script type="application/ld+json" dangerouslySetInnerHTML={{__html: JSON.stringify(jsonLd)}} />
      <DeepResearchView ticker={t} />
    </>
  );
}
