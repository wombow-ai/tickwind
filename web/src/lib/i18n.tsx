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
    // opportunity board
    'opp.title': 'Opportunity board',
    'opp.subtitle':
      'Small-cap US stocks where company insiders are buying on the open market — surfaced from SEC Form 4 filings.',
    'opp.disclaimer':
      'Rows are surfaced by data signals (insider open-market buying), not recommendations. Inclusion is not a rating or a suggestion to buy. Small-caps can be volatile and illiquid. Not investment advice.',
    'opp.empty': 'No insider-buy signals yet',
    'opp.emptySub': 'The board fills in as recent SEC Form 4 open-market buys are filed.',
    'opp.smallCap': 'Small cap',
    'opp.viewFiling': 'View SEC filing',
    'opp.footer': 'Insider data from public SEC EDGAR filings. Not investment advice.',
    // guru rail
    'guru.title': 'Guru-watch',
    'guru.subtitle':
      'What independent finance writers are publishing — and the tickers they name. Opinions for context, linked to the source.',
    'guru.empty': 'No guru posts yet',
    'guru.emptySub': 'The rail fills in as curated writers publish new pieces.',
    'guru.source': 'Source',
    'guru.footer':
      'Third-party opinions from public RSS feeds, shown for context. Not an endorsement. Not investment advice.',
    // wsb
    'wsb.title': 'WSB Trending',
    'wsb.hotList': 'Hot list',
    'wsb.mentions': 'mentions',
    'wsb.footer': 'Source: ApeWisdom · discussion volume, not advice.',
    // events
    'events.title': 'Events timeline',
    'events.subtitle':
      'Upcoming market-moving events — Fed decisions, key US data (CPI, jobs), and notable world events. For context, not advice.',
    'events.empty': 'No upcoming events',
    'events.emptySub': 'Check back soon.',
    'events.high': 'High',
    'events.footer':
      'Sources: BLS, Federal Reserve + curated. Times in your local zone. Not investment advice.',
    // topics + shared states
    'topics.title': 'HOT TOPICS',
    'states.error': 'Something went wrong',
    'states.retry': 'Try again',
    'stock.collecting': 'Collecting data for this stock…',
    'stock.collectingSub':
      "First time we've tracked it — price, news and filings arrive in about a minute. This page updates on its own.",
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
    // opportunity board
    'opp.title': '机会榜',
    'opp.subtitle': '内部人正在公开市场买入的美国小盘股 —— 来自 SEC Form 4 申报。',
    'opp.disclaimer':
      '榜单由数据信号（内部人公开买入）生成，并非推荐；列入不代表评级或买入建议。小盘股可能波动较大、流动性较差。非投资建议。',
    'opp.empty': '暂无内部人买入信号',
    'opp.emptySub': '随着近期 SEC Form 4 公开买入申报陆续公布，榜单会逐步填充。',
    'opp.smallCap': '小盘股',
    'opp.viewFiling': '查看 SEC 申报',
    'opp.footer': '内部人数据来自公开的 SEC EDGAR 申报。非投资建议。',
    // guru rail
    'guru.title': '大V观点',
    'guru.subtitle': '独立财经作者在写什么 —— 以及他们提到的股票。观点仅供参考，附原文链接。',
    'guru.empty': '暂无大V内容',
    'guru.emptySub': '随着精选作者发布新文章，这里会陆续更新。',
    'guru.source': '原文',
    'guru.footer': '来自公开 RSS 的第三方观点，仅供参考，不代表认可。非投资建议。',
    // wsb
    'wsb.title': 'WSB 热议',
    'wsb.hotList': '完整榜单',
    'wsb.mentions': '次提及',
    'wsb.footer': '来源：ApeWisdom · 讨论热度，非投资建议。',
    // events
    'events.title': '大事件时间线',
    'events.subtitle': '即将到来的市场大事件 —— 美联储决议、关键美国数据（CPI、非农）以及重要世界事件。仅供参考，非投资建议。',
    'events.empty': '暂无即将到来的事件',
    'events.emptySub': '稍后再来看看。',
    'events.high': '重要',
    'events.footer': '来源：BLS、美联储 + 精选。时间按你的本地时区显示。非投资建议。',
    // topics + shared states
    'topics.title': '热点话题',
    'states.error': '出错了',
    'states.retry': '重试',
    'stock.collecting': '正在收集这只股票的数据…',
    'stock.collectingSub': '这是我们首次收录该股票 —— 价格、新闻与申报约一分钟内到位，页面会自动更新。',
    'common.ago': '前',
  },
};
