'use client';

import {useEffect} from 'react';
import {useLang} from '@/lib/i18n';

/**
 * Syncs the browser-tab title (document.title) to the active UI language. The
 * page's server-rendered <title> stays English (the crawl/default value), so
 * English users — and search engines — never get a Chinese tab; Chinese users
 * get the zh title once hydrated. Renders nothing.
 */
export function LocalizedTitle({en, zh}: {en: string; zh: string}) {
  const {lang} = useLang();
  useEffect(() => {
    document.title = lang === 'zh' ? zh : en;
  }, [lang, en, zh]);
  return null;
}
