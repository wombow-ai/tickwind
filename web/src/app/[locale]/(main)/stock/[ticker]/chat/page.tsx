import type {Metadata} from 'next';
import {ChatThread} from '@/components/ChatThread';
import {langAlternates} from '@/lib/config';
import {isLocale, LOCALES} from '@/lib/locale';
import {popularTickers} from '@/lib/pseo';

interface Params {
  params: Promise<{locale: string; ticker: string}>;
}

// ISR: only the SSR shell (metadata) is cacheable per ticker. The thread itself is
// Pro-gated + per-user, fetched client-side with the Supabase token inside ChatThread —
// nothing gated runs on the server or at build time.
export const revalidate = 600;

export async function generateStaticParams(): Promise<{locale: string; ticker: string}[]> {
  try {
    const tickers = await popularTickers();
    return LOCALES.flatMap(locale => tickers.map(ticker => ({locale, ticker: encodeURIComponent(ticker)})));
  } catch {
    return [];
  }
}

export async function generateMetadata({params}: Params): Promise<Metadata> {
  const {locale, ticker} = await params;
  const t = decodeURIComponent(ticker).toUpperCase();
  const loc = isLocale(locale) ? locale : 'en';
  const title = loc === 'zh' ? `${t} · 个性化 AI 分析` : `${t} · Personalized AI analysis`;
  return {
    title,
    description:
      loc === 'zh'
        ? `就 ${t} 向 AI 提问 —— 答案基于公开数据,数字均有出处。Pro 功能。非投资建议。`
        : `Ask an AI anything about ${t} — answers grounded in public data, every figure sourced. Pro feature. Not investment advice.`,
    alternates: langAlternates(`/stock/${encodeURIComponent(t)}/chat`, loc),
    // Gated, per-user content — keep it out of the index but let crawlers follow back.
    robots: {index: false, follow: true},
  };
}

/**
 * The personalized AI chat route (Product B). A thin server shell; the gated, per-user
 * thread is rendered + fetched entirely client-side in {@link ChatThread} (login + Pro
 * gates live there).
 */
export default async function ChatPage({params}: Params) {
  const {ticker} = await params;
  const t = decodeURIComponent(ticker).toUpperCase();
  return <ChatThread ticker={t} />;
}
