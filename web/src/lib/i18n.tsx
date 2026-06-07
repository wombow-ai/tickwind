'use client';

import {useCallback, useSyncExternalStore} from 'react';

/**
 * Lightweight i18n (English / 简体中文) mirroring the theme pattern: the chosen
 * language lives on the <html lang> attribute (set before paint by
 * {@link langNoFlashScript}) as the single source of truth, read via
 * useSyncExternalStore so it's hydration-safe and shared across all subscribers.
 * UI chrome strings are translated via {@link useT}; data (prices, headlines,
 * company names) is shown as-sourced.
 */
export type Lang = 'en' | 'zh';

const STORAGE_KEY = 'tw-lang';
const EVENT = 'tw-lang-change';

/** Inline script applying the persisted language before first paint. */
export const langNoFlashScript = `(function(){try{var l=localStorage.getItem('${STORAGE_KEY}');if(l==='zh'||l==='en'){document.documentElement.lang=l;}}catch(e){}})();`;

function subscribe(cb: () => void): () => void {
  window.addEventListener(EVENT, cb);
  window.addEventListener('storage', cb);
  return () => {
    window.removeEventListener(EVENT, cb);
    window.removeEventListener('storage', cb);
  };
}

function getSnapshot(): Lang {
  return document.documentElement.lang === 'zh' ? 'zh' : 'en';
}

function getServerSnapshot(): Lang {
  return 'en'; // SSR renders English; the no-flash script reconciles on the client.
}

/** Current language + setters. Hydration-safe (reads the <html lang> attribute). */
export function useLang(): {lang: Lang; setLang: (l: Lang) => void; toggle: () => void} {
  const lang = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
  const setLang = useCallback((next: Lang) => {
    document.documentElement.lang = next;
    try {
      localStorage.setItem(STORAGE_KEY, next);
    } catch {
      // Private mode / storage disabled: language still applies for this session.
    }
    window.dispatchEvent(new Event(EVENT));
  }, []);
  const toggle = useCallback(() => setLang(getSnapshot() === 'zh' ? 'en' : 'zh'), [setLang]);
  return {lang, setLang, toggle};
}

/**
 * Returns a translator `t(key)` for the current language, falling back to the
 * English string, then the key itself, so a missing translation never blanks
 * the UI.
 */
export function useT(): (key: string) => string {
  const {lang} = useLang();
  return useCallback((key: string) => dict[lang][key] ?? dict.en[key] ?? key, [lang]);
}

// dict holds the UI-chrome strings. Keep keys dotted-namespaced; add new keys to
// BOTH maps. Missing zh keys gracefully fall back to en.
const dict: Record<Lang, Record<string, string>> = {
  en: {
    // top nav
    'nav.markets': 'Markets',
    'nav.hot': 'Hot',
    'nav.news': 'News',
    'nav.opportunities': 'Opportunities',
    'nav.events': 'Events',
    'nav.watchlist': 'Watchlist',
    'nav.whatsnew': "What's new",
    'nav.login': 'Log in',
    'nav.signup': 'Sign up',
    'nav.search': 'Search a stock…',
    'nav.myWatchlist': 'My watchlist',
    'nav.settings': 'Settings',
    'nav.signout': 'Sign out',
    'nav.account': 'Account menu',
    // home hub
    'home.title': 'Markets today',
    'home.subtitle':
      'Live prices, then the trends, news and chatter across the market — all in one place.',
    'home.hotTopics': 'Hot Topics',
    'mod.hotStocks': 'Hot stocks',
    'mod.opportunity': 'Opportunity',
    'mod.guru': 'Guru-watch',
    'mod.news': 'News',
    'mod.discussion': 'Discussion',
    'mod.fullBoard': 'Full board',
    'mod.seeAll': 'See all',
    'mod.moreNews': 'More news',
    'mod.joinIn': 'Join in',
    'mod.noData': 'No data yet',
    'mod.noSignals': 'No signals yet',
    'mod.noPosts': 'No posts yet',
    'mod.noNews': 'No news yet',
    'mod.noChatter': 'No chatter yet',
    'common.ago': 'ago',
  },
  zh: {
    // top nav
    'nav.markets': '行情',
    'nav.hot': '热门',
    'nav.news': '新闻',
    'nav.opportunities': '机会',
    'nav.events': '大事件',
    'nav.watchlist': '自选',
    'nav.whatsnew': '更新',
    'nav.login': '登录',
    'nav.signup': '注册',
    'nav.search': '搜索股票…',
    'nav.myWatchlist': '我的自选',
    'nav.settings': '设置',
    'nav.signout': '退出登录',
    'nav.account': '账户菜单',
    // home hub
    'home.title': '今日市场',
    'home.subtitle': '实时价格，以及全市场的趋势、新闻与讨论 —— 一站式呈现。',
    'home.hotTopics': '热点话题',
    'mod.hotStocks': '热门股票',
    'mod.opportunity': '机会榜',
    'mod.guru': '大V观点',
    'mod.news': '新闻',
    'mod.discussion': '讨论',
    'mod.fullBoard': '完整榜单',
    'mod.seeAll': '查看全部',
    'mod.moreNews': '更多新闻',
    'mod.joinIn': '参与讨论',
    'mod.noData': '暂无数据',
    'mod.noSignals': '暂无信号',
    'mod.noPosts': '暂无内容',
    'mod.noNews': '暂无新闻',
    'mod.noChatter': '暂无讨论',
    'common.ago': '前',
  },
};
