import type {Metadata} from 'next';
import {StockView} from '@/components/StockView';

interface Params {
  params: Promise<{ticker: string}>;
}

export async function generateMetadata({params}: Params): Promise<Metadata> {
  const {ticker} = await params;
  const t = decodeURIComponent(ticker).toUpperCase();
  return {
    title: t,
    description: `Live all-session price, SEC filings, news and discussion for ${t}.`,
  };
}

/** Public per-stock page (data-first, SEO-friendly title). */
export default async function StockPage({params}: Params) {
  const {ticker} = await params;
  const t = decodeURIComponent(ticker).toUpperCase();
  return <StockView ticker={t} />;
}
