import type {Metadata} from 'next';
import {getStock, type Security} from '@/lib/api';
import {SITE_URL} from '@/lib/config';
import {StockView} from '@/components/StockView';

interface Params {
  params: Promise<{ticker: string}>;
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

export async function generateMetadata({params}: Params): Promise<Metadata> {
  const {ticker} = await params;
  const t = decodeURIComponent(ticker).toUpperCase();
  const sec = await fetchSecurity(t);
  const hasName = !!sec?.name && sec.name !== t;
  return {
    title: hasName ? `${t} · ${sec!.name}` : t,
    description: `Live all-session price, SEC filings, fundamentals (市值/市盈率/营收), news and discussion for ${sec?.name || t}.`,
    alternates: {canonical: `${SITE_URL}/stock/${encodeURIComponent(t)}`},
  };
}

/** Public per-stock page (data-first, SEO-friendly: rich metadata + JSON-LD). */
export default async function StockPage({params}: Params) {
  const {ticker} = await params;
  const t = decodeURIComponent(ticker).toUpperCase();
  const sec = await fetchSecurity(t);
  const name = sec?.name || t;
  const stockUrl = `${SITE_URL}/stock/${encodeURIComponent(t)}`;

  // Structured data — separate blocks per type (partial markup = zero rich-result
  // lift, so we only emit fields we actually have). Corporation + BreadcrumbList
  // are always valid from the ticker/name; richer FinancialProduct is deferred
  // (needs server-fetched price/financials).
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
        {'@type': 'ListItem', position: 1, name: 'Markets', item: SITE_URL},
        {'@type': 'ListItem', position: 2, name, item: stockUrl},
      ],
    },
  ];

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
