import type {Metadata} from 'next';
import {getFundamentals, getStock, type Fundamentals, type Security} from '@/lib/api';
import {SITE_URL, langAlternates} from '@/lib/config';
import {isLocale} from '@/lib/locale';
import {StockView} from '@/components/StockView';

interface Params {
  params: Promise<{locale: string; ticker: string}>;
}

// ISR: cache the SSR shell (metadata + JSON-LD) per ticker for 10 min. Live
// prices still stream client-side inside StockView.
export const revalidate = 600;

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

export async function generateMetadata({params}: Params): Promise<Metadata> {
  const {locale, ticker} = await params;
  const t = decodeURIComponent(ticker).toUpperCase();
  const loc = isLocale(locale) ? locale : 'en';
  const sec = await fetchSecurity(t);
  const hasName = !!sec?.name && sec.name !== t;
  return {
    title: hasName ? `${t} · ${sec!.name}` : t,
    description: `Live all-session price, SEC filings, fundamentals (市值/市盈率/营收), news and discussion for ${sec?.name || t}.`,
    alternates: langAlternates(`/stock/${encodeURIComponent(t)}`, loc),
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
    </>
  );
}
