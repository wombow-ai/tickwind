import type {Metadata} from 'next';
import {Suspense} from 'react';
import {ChatHub} from '@/components/ChatHub';
import {isLocale} from '@/lib/locale';

interface Params {
  params: Promise<{locale: string}>;
}

export async function generateMetadata({params}: Params): Promise<Metadata> {
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  return {
    title: loc === 'zh' ? 'AI 对话' : 'AI Chat',
    // Per-user, Pro-gated app surface — keep it out of the index.
    robots: {index: false, follow: false},
  };
}

/**
 * The unified AI chat hub (Product C). A thin shell; the gated, per-user hub (sidebar +
 * thread + user-data tools) is rendered client-side in ChatHub. Wrapped in Suspense for
 * useSearchParams (?ticker= warm-start).
 */
export default function ChatHubPage() {
  return (
    <Suspense fallback={null}>
      <ChatHub />
    </Suspense>
  );
}
