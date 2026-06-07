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
    'events.all': 'All',
    'events.footer':
      'Sources: BLS, Federal Reserve + curated. Times in your local zone. Not investment advice.',
    // topics + shared states
    'topics.title': 'HOT TOPICS',
    'states.error': 'Something went wrong',
    'states.errorTitle': 'The wind dropped for a moment',
    'states.errorSub': "We couldn't load this feed. Check your connection and try again.",
    'states.retry': 'Try again',
    // hot list / leaderboards
    'hot.surging': 'Surging',
    'hot.blurbHot':
      'The most-discussed US stocks across Reddit right now — ranked by buzz and momentum.',
    'hot.blurbSurging':
      'Stocks whose chatter is accelerating fastest — the biggest 24h jumps in mentions, not just the loudest.',
    'hot.emptySub': 'The leaderboard refreshes every few minutes — check back shortly.',
    'hot.footer': 'Buzz via ApeWisdom (Reddit mentions). Not investment advice.',
    // auth
    'auth.titleSignup': 'Create your account',
    'auth.titleLogin': 'Welcome back',
    'auth.subSignup': 'Track your own watchlist and clip links — free.',
    'auth.subLogin': 'Log in to your watchlist and saved links.',
    'auth.google': 'Continue with Google',
    'auth.or': 'or',
    'auth.email': 'Email',
    'auth.password': 'Password',
    'auth.busy': 'One moment…',
    'auth.createAccount': 'Create account',
    'auth.haveAccount': 'Already have an account?',
    'auth.noAccount': "Don't have an account?",
    'auth.pwTooShort': 'Password must be at least 6 characters.',
    'auth.genericError': 'Something went wrong.',
    'auth.checkEmail': 'Check your email to confirm your account, then log in.',
    'stock.collecting': 'Collecting data for this stock…',
    'stock.collectingSub':
      "First time we've tracked it — price, news and filings arrive in about a minute. This page updates on its own.",
    'stock.filings': 'Filings',
    'stock.savedLinks': 'Saved links',
    'stock.noNewsSub': 'New headlines will land here first.',
    'stock.noChatterSub': 'Posts from StockTwits, Bluesky and more will show up here.',
    'stock.noFilings': 'No recent filings',
    'stock.noFilingsSub': 'Filings appear here as soon as they hit SEC EDGAR.',
    'stock.noClips': 'No saved links yet',
    'stock.noClipsSub': 'Found something good elsewhere? Paste a link to clip it here.',
    'stock.clipPlaceholder': 'Paste an X / Xiaohongshu / TikTok link…',
    'stock.save': 'Save',
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
    'events.all': '全部',
    'events.footer': '来源：BLS、美联储 + 精选。时间按你的本地时区显示。非投资建议。',
    // topics + shared states
    'topics.title': '热点话题',
    'states.error': '出错了',
    'states.errorTitle': '信号中断了一下',
    'states.errorSub': '未能加载此内容，请检查网络连接后重试。',
    'states.retry': '重试',
    // hot list / leaderboards
    'hot.surging': '飙升',
    'hot.blurbHot': '当前 Reddit 上讨论最多的美股 —— 按热度与势头排名。',
    'hot.blurbSurging': '讨论热度加速最快的股票 —— 24 小时提及增幅最大，而非单纯最热闹的。',
    'hot.emptySub': '榜单每几分钟刷新一次 —— 稍后再来看看。',
    'hot.footer': '热度数据来自 ApeWisdom（Reddit 提及）。非投资建议。',
    // auth
    'auth.titleSignup': '创建账户',
    'auth.titleLogin': '欢迎回来',
    'auth.subSignup': '创建并管理你的自选股与收藏链接 —— 免费。',
    'auth.subLogin': '登录以查看你的自选与收藏。',
    'auth.google': '使用 Google 继续',
    'auth.or': '或',
    'auth.email': '邮箱',
    'auth.password': '密码',
    'auth.busy': '请稍候…',
    'auth.createAccount': '创建账户',
    'auth.haveAccount': '已有账户？',
    'auth.noAccount': '还没有账户？',
    'auth.pwTooShort': '密码至少需要 6 位。',
    'auth.genericError': '出错了。',
    'auth.checkEmail': '请查收邮件以确认账户，然后登录。',
    'stock.collecting': '正在收集这只股票的数据…',
    'stock.collectingSub': '这是我们首次收录该股票 —— 价格、新闻与申报约一分钟内到位，页面会自动更新。',
    'stock.filings': '申报',
    'stock.savedLinks': '收藏链接',
    'stock.noNewsSub': '新的相关新闻会最先出现在这里。',
    'stock.noChatterSub': '来自 StockTwits、Bluesky 等的讨论会显示在这里。',
    'stock.noFilings': '暂无近期申报',
    'stock.noFilingsSub': '申报一旦出现在 SEC EDGAR，就会显示在这里。',
    'stock.noClips': '还没有收藏链接',
    'stock.noClipsSub': '在别处看到好内容？粘贴链接即可收藏到这里。',
    'stock.clipPlaceholder': '粘贴 X / 小红书 / TikTok 链接…',
    'stock.save': '保存',
    'common.ago': '前',
  },
};
