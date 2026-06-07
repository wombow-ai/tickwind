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
    'nav.notes': 'Notes',
    'nav.community': 'Community',
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
    // feed pages (News / Discussion)
    'feed.titleNews': 'Market news',
    'feed.titleFiltered': 'Filtered news',
    'feed.subNews': 'The latest headlines across the most-watched US stocks.',
    'feed.subDiscussion':
      'What people are saying about the most-watched US stocks — StockTwits, Bluesky and more.',
    'feed.subFiltered': 'Latest headlines about “{t}”.',
    'feed.allNews': '← All news',
    'feed.emptyFiltered': 'No recent news for this topic',
    'feed.emptyFilteredSub': 'Try another topic or see all news.',
    'feed.emptyNewsSub': 'Headlines will appear here as they break.',
    'feed.emptyChatterSub': 'Posts will show up here as they come in.',
    // board (Markets / Watchlist)
    'board.titleWatchlist': 'Your watchlist',
    'board.subWatchlist': 'Your stocks — prices, news and chatter in one place.',
    'board.subMarkets': 'Live prices, then the news and chatter across the market.',
    'board.addStock': 'Add a stock…',
    'board.loginTitle': 'Log in to see your watchlist',
    'board.loginSub': 'Track your own stocks and clip links from anywhere — free.',
    'board.emptyTitle': 'Your board is calm and empty',
    'board.emptySub': 'Add a ticker to follow its price, news and chatter.',
    'board.showFeed': 'Show news & discussion',
    'board.hideFeed': 'Hide news & discussion',
    'board.emptyNewsSub': 'Headlines about the stocks you follow will land here.',
    'board.emptyChatterSub': 'Posts from StockTwits and Reddit will show up here.',
    'board.loginToBuild': 'Log in to build your watchlist',
    'board.added': 'Added {t}',
    'board.already': '{t} is already on your watchlist',
    'board.addFailed': "Couldn't add {t}",
    'board.removed': 'Removed {t}',
    'board.undo': 'Undo',
    'board.removeFailed': "Couldn't remove {t}",
    'footer.product': 'Product',
    'footer.account': 'Account',
    'footer.blurb': "Read every tick. See where the market's blowing.",
    'footer.copyright': '© 2026 Tickwind. Not investment advice.',
    'footer.tagline': 'Made for the curious investor.',
    'stock.waiting': 'Waiting for price…',
    'stock.loginAdd': 'Log in to track {t} and save links',
    'stock.loginAddSub': 'Keep a watchlist and clip posts from anywhere — free.',
    // pulse bar (detail-page buzz + sentiment chips)
    'pulse.buzz': 'Reddit buzz',
    'pulse.mentions24h': 'mentions / 24h',
    'pulse.rank': 'rank #{n}',
    'pulse.vs24h': 'vs 24h ago',
    'pulse.upvotes': 'upvotes',
    'pulse.sentiment': 'News sentiment',
    'pulse.across': 'across {n} recent articles',
    // k-line chart
    'kline.title': 'Price & indicators',
    'kline.empty': 'No price history yet',
    'kline.footer': 'Daily OHLC · MA/MACD/RSI computed locally. Not investment advice.',
    // comments
    'comments.tab': 'Comments',
    'comments.title': 'Community',
    'comments.subtitle': "What people are saying — across all stocks.",
    'comments.disclaimer':
      'User opinions — not investment advice, and not endorsed by Tickwind. Be civil; report anything abusive.',
    'comments.placeholder': 'Share your view…',
    'comments.post': 'Post',
    'comments.empty': 'No comments yet — be the first.',
    'comments.loginToPost': 'Log in to join the discussion.',
    'comments.report': 'Report',
    'comments.reported': 'Reported — thanks for flagging.',
    // settings
    'settings.signInTitle': 'Sign in to continue',
    'settings.signInSub': 'Your settings live with your account.',
    'settings.signedIn': 'Signed in',
    'settings.appearance': 'Appearance',
    'settings.theme': 'Theme',
    'settings.themeDark': 'Dark',
    'settings.themeLight': 'Light',
    'settings.themeHint': '— switch any time.',
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
    'stock.notesPlaceholder': 'Add a private note or thesis…',
    'stock.noNotes': 'No notes yet',
    'stock.noNotesSub': 'Jot down your thesis, price targets, or anything to remember.',
    // notes page
    'notes.title': 'My notes',
    'notes.subtitle': 'Your private notes & opinions — across stocks and dates.',
    'notes.empty': 'No notes yet',
    'notes.emptySub': 'Write your first note below.',
    'notes.composePlaceholder': 'Write a note… (optionally tag a $TICKER)',
    'notes.add': 'Add note',
    'notes.delete': 'Delete',
    'notes.pin': 'Pin',
    'notes.unpin': 'Unpin',
    'notes.edited': 'edited',
    'notes.list': 'List',
    'notes.calendar': 'Calendar',
    'notes.dayNote': 'Note for this day…',
    'notes.dayEmpty': 'No notes for this day.',
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
    'nav.notes': '笔记',
    'nav.community': '社区',
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
    // feed pages
    'feed.titleNews': '市场新闻',
    'feed.titleFiltered': '筛选新闻',
    'feed.subNews': '热门美股的最新头条。',
    'feed.subDiscussion': '人们在 StockTwits、Bluesky 等平台讨论热门美股的内容。',
    'feed.subFiltered': '关于「{t}」的最新头条。',
    'feed.allNews': '← 全部新闻',
    'feed.emptyFiltered': '该话题暂无近期新闻',
    'feed.emptyFilteredSub': '换个话题，或查看全部新闻。',
    'feed.emptyNewsSub': '有新头条时会显示在这里。',
    'feed.emptyChatterSub': '有新讨论时会显示在这里。',
    // board
    'board.titleWatchlist': '我的自选',
    'board.subWatchlist': '你的自选股 —— 价格、新闻与讨论一站式呈现。',
    'board.subMarkets': '实时价格，以及全市场的新闻与讨论。',
    'board.addStock': '添加股票…',
    'board.loginTitle': '登录以查看你的自选',
    'board.loginSub': '随时随地跟踪你的自选股与收藏链接 —— 免费。',
    'board.emptyTitle': '你的看板还空着',
    'board.emptySub': '添加一只股票，跟踪它的价格、新闻与讨论。',
    'board.showFeed': '显示新闻与讨论',
    'board.hideFeed': '隐藏新闻与讨论',
    'board.emptyNewsSub': '你关注的股票的新闻会显示在这里。',
    'board.emptyChatterSub': '来自 StockTwits 和 Reddit 的讨论会显示在这里。',
    'board.loginToBuild': '登录以创建你的自选',
    'board.added': '已添加 {t}',
    'board.already': '{t} 已在你的自选中',
    'board.addFailed': '添加 {t} 失败',
    'board.removed': '已移除 {t}',
    'board.undo': '撤销',
    'board.removeFailed': '移除 {t} 失败',
    'footer.product': '产品',
    'footer.account': '账户',
    'footer.blurb': '读懂每一次跳动，看清市场风向。',
    'footer.copyright': '© 2026 Tickwind。非投资建议。',
    'footer.tagline': '为好奇的投资者而做。',
    'stock.waiting': '正在等待价格…',
    'stock.loginAdd': '登录以关注 {t} 并保存链接',
    'stock.loginAddSub': '随时随地维护自选股、收藏帖子 —— 免费。',
    // pulse bar
    'pulse.buzz': 'Reddit 热度',
    'pulse.mentions24h': '次提及 / 24h',
    'pulse.rank': '排名 #{n}',
    'pulse.vs24h': '较 24h 前',
    'pulse.upvotes': '次赞',
    'pulse.sentiment': '新闻情绪',
    'pulse.across': '基于 {n} 篇近期文章',
    // k-line chart
    'kline.title': 'K线与指标',
    'kline.empty': '暂无价格历史',
    'kline.footer': '日K线 · MA/MACD/RSI 本地计算。非投资建议。',
    // comments
    'comments.tab': '评论',
    'comments.title': '社区讨论',
    'comments.subtitle': '大家都在聊什么 —— 跨所有股票。',
    'comments.disclaimer': '用户观点 —— 非投资建议，不代表 Tickwind 立场。请文明发言；如有不当内容请举报。',
    'comments.placeholder': '分享你的看法…',
    'comments.post': '发布',
    'comments.empty': '还没有评论 —— 来抢沙发。',
    'comments.loginToPost': '登录后参与讨论。',
    'comments.report': '举报',
    'comments.reported': '已举报，谢谢反馈。',
    // settings
    'settings.signInTitle': '请先登录',
    'settings.signInSub': '你的设置与账户绑定。',
    'settings.signedIn': '已登录',
    'settings.appearance': '外观',
    'settings.theme': '主题',
    'settings.themeDark': '深色',
    'settings.themeLight': '浅色',
    'settings.themeHint': '—— 随时可切换。',
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
    'stock.notesPlaceholder': '写下你的备注或观点…',
    'stock.noNotes': '还没有笔记',
    'stock.noNotesSub': '记下你的投资逻辑、目标价，或任何想记住的内容。',
    // notes page
    'notes.title': '我的笔记',
    'notes.subtitle': '你的私人笔记与观点 —— 跨股票与日期。',
    'notes.empty': '还没有笔记',
    'notes.emptySub': '在下方写下你的第一条笔记。',
    'notes.composePlaceholder': '写一条笔记…（可选：标注 $股票代码）',
    'notes.add': '添加笔记',
    'notes.delete': '删除',
    'notes.pin': '置顶',
    'notes.unpin': '取消置顶',
    'notes.edited': '已编辑',
    'notes.list': '列表',
    'notes.calendar': '日历',
    'notes.dayNote': '当天的笔记…',
    'notes.dayEmpty': '当天还没有笔记。',
    'common.ago': '前',
  },
};
