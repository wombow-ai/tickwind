import type {Metadata} from 'next';
import {ApiError, getFundamentals, getQuote, getStock, type Fundamentals, type Security} from '@/lib/api';
import {SITE_URL, langAlternates} from '@/lib/config';
import {isLocale, LOCALES} from '@/lib/locale';
import {popularTickers} from '@/lib/pseo';
import {StockView} from '@/components/StockView';
import {RelatedStocks} from '@/components/RelatedStocks';

interface Params {
  params: Promise<{locale: string; ticker: string}>;
}

// ISR: cache the SSR shell (metadata + JSON-LD) per ticker for 10 min. Live
// prices still stream client-side inside StockView.
export const revalidate = 600;

/**
 * Pre-render ONLY the popular ticker subset (POPULAR_TICKERS ∪ hot/surging/wsb
 * ∪ opportunities — ~hundreds) × the two locales. The full ~6,700-symbol quote
 * universe stays DYNAMIC ISR (`dynamicParams` defaults to true), so the build
 * never balloons. Best-effort: `[]` on API failure so the build never breaks
 * (mirrors the guide/indicators `generateStaticParams`).
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

/** Server-side security lookup for richer metadata + JSON-LD. Null on miss. */
async function fetchSecurity(t: string): Promise<Security | null> {
  try {
    return await getStock(t, AbortSignal.timeout(5000));
  } catch {
    return null; // non-US / uningested / API down → render with the ticker only
  }
}

/** Server-side fundamentals for the financials JSON-LD. Null for non-US / no data. */
async function fetchFundamentals(t: string): Promise<Fundamentals | null> {
  try {
    return await getFundamentals(t, AbortSignal.timeout(5000));
  } catch {
    return null;
  }
}

/**
 * Whether a rejected fetch should still be treated as "content present" — i.e.
 * we FAIL OPEN. Only a definitive 404 (the symbol genuinely has no data) is a
 * "no content" signal; ANY other failure (network error → status 0, timeout/
 * abort, 5xx) means "couldn't tell", so we keep the page indexable rather than
 * risk deindexing a real page on a transient hiccup.
 */
function isDefinitive404(reason: unknown): boolean {
  return reason instanceof ApiError && reason.status === 404;
}

/**
 * Defense-in-depth thin-content check: a page is "thin" only when the ticker
 * definitively has NEITHER a live quote NOR fundamentals — i.e. an obscure /
 * delisted symbol an ISR crawl reached, whose page would be essentially empty.
 * Quote present = a positive live price; fundamentals present = any XBRL record.
 * Fails OPEN on transient errors (a non-404 rejection counts as "present").
 * Returns true ⇒ caller should add `robots: noindex`.
 */
async function isThin(t: string): Promise<boolean> {
  const signal = AbortSignal.timeout(4000);
  const [quote, fund] = await Promise.allSettled([
    getQuote(t, signal),
    getFundamentals(t, signal),
  ]);
  const noQuote =
    quote.status === 'fulfilled'
      ? !(quote.value?.price > 0)
      : isDefinitive404(quote.reason);
  const noFund =
    fund.status === 'fulfilled' ? !fund.value : isDefinitive404(fund.reason);
  return noQuote && noFund;
}

export async function generateMetadata({params}: Params): Promise<Metadata> {
  const {locale, ticker} = await params;
  const t = decodeURIComponent(ticker).toUpperCase();
  const loc = isLocale(locale) ? locale : 'en';
  const [sec, thin] = await Promise.all([fetchSecurity(t), isThin(t)]);
  const hasName = !!sec?.name && sec.name !== t;
  return {
    title: hasName ? `${t} · ${sec!.name}` : t,
    description: `Live all-session price, SEC filings, fundamentals (市值/市盈率/营收), news and discussion for ${sec?.name || t}.`,
    alternates: langAlternates(`/stock/${encodeURIComponent(t)}`, loc),
    // Thin (no quote AND no fundamentals) → keep it out of the index but let
    // crawlers follow links out. Real pages get the default (indexable).
    ...(thin ? {robots: {index: false, follow: true}} : {}),
  };
}

/** Public per-stock page (data-first, SEO-friendly: rich metadata + JSON-LD). */
export default async function StockPage({params}: Params) {
  const {locale, ticker} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const t = decodeURIComponent(ticker).toUpperCase();
  const [sec, fund] = await Promise.all([fetchSecurity(t), fetchFundamentals(t)]);
  const name = sec?.name || t;
  const stockUrl = `${SITE_URL}/${loc}/stock/${encodeURIComponent(t)}`;

  // Structured data — separate blocks per type; we only emit fields we actually
  // have (partial markup = zero rich-result lift).
  const jsonLd: Record<string, unknown>[] = [
    {
      '@context': 'https://schema.org',
      '@type': 'Corporation',
      name,
      tickerSymbol: t,
      url: stockUrl,
    },
    {
      '@context': 'https://schema.org',
      '@type': 'BreadcrumbList',
      itemListElement: [
        {'@type': 'ListItem', position: 1, name: 'Markets', item: `${SITE_URL}/${loc}`},
        {'@type': 'ListItem', position: 2, name, item: stockUrl},
      ],
    },
  ];

  // Key financials as a Dataset — the schema.org-blessed shape for packaged
  // financial data (better for AI/SERP than a strained FinancialProduct). Only
  // the measures we actually have are included; the block is omitted if none.
  const measures: Record<string, unknown>[] = [];
  if (fund) {
    if (fund.market_cap != null)
      measures.push({'@type': 'PropertyValue', name: 'Market capitalization', value: Math.round(fund.market_cap), unitText: fund.currency});
    if (fund.pe != null)
      measures.push({'@type': 'PropertyValue', name: 'Price-to-earnings ratio (P/E)', value: Number(fund.pe.toFixed(2))});
    if (fund.revenue)
      measures.push({'@type': 'PropertyValue', name: `Revenue (${fund.period})`, value: Math.round(fund.revenue), unitText: fund.currency});
    if (fund.net_income)
      measures.push({'@type': 'PropertyValue', name: `Net income (${fund.period})`, value: Math.round(fund.net_income), unitText: fund.currency});
  }
  if (measures.length > 0) {
    jsonLd.push({
      '@context': 'https://schema.org',
      '@type': 'Dataset',
      name: `${name} key financials`,
      description: `Market cap, P/E, revenue and net income for ${name}${fund?.period ? ` (${fund.period})` : ''}, derived from SEC filings.`,
      url: stockUrl,
      creator: {'@type': 'Organization', name: 'Tickwind'},
      variableMeasured: measures,
    });
  }

  return (
    <>
      {jsonLd.map((obj, i) => (
        <script
          key={i}
          type="application/ld+json"
          dangerouslySetInnerHTML={{__html: JSON.stringify(obj)}}
        />
      ))}
      <StockView ticker={t} />
      <RelatedStocks ticker={t} locale={loc} />
    </>
  );
}
