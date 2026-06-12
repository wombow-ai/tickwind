/**
 * Curated SEO landing pages ("guides") — one per high-value Chinese keyword
 * cluster, each funnelling to a live board. These give crawlers real,
 * keyword-rich, internally-linked content for queries the board pages don't
 * target head-on (question-style and long-tail terms), per the pSEO plan.
 *
 * Content is bilingual (zh primary for the keyword targeting, en parallel);
 * pages render both and hide the inactive language with the [data-i18n] CSS,
 * matching the homepage intro. Every guide carries the "public data, not advice"
 * disclaimer the project requires.
 */

export interface GuideFAQ {
  qZh: string;
  aZh: string;
  qEn: string;
  aEn: string;
}

export interface Guide {
  slug: string;
  /** Browser-tab + OG title (English default; LocalizedTitle swaps zh). */
  titleEn: string;
  titleZh: string;
  /** Meta description (Chinese-led for the keyword targeting). */
  descZh: string;
  descEn: string;
  keywords: string[];
  h1Zh: string;
  h1En: string;
  /** Intro paragraphs (rendered in order). */
  bodyZh: string[];
  bodyEn: string[];
  /** Call-to-action into the live board. */
  cta: {href: string; labelZh: string; labelEn: string};
  /** Q&A — also emitted as FAQPage structured data. */
  faq: GuideFAQ[];
  /** Other guide slugs to cross-link. */
  related: string[];
}

export const GUIDES: Guide[] = [
  {
    slug: 'congress-stock-tracker',
    titleEn: 'Congress Stock Tracker — U.S. Lawmakers’ Trades · Tickwind',
    titleZh: '国会山股神 · 美国国会议员持仓与交易查询 · 潮汐 Tickwind',
    descZh:
      '国会山股神怎么查?美国国会议员(含佩洛西)依《STOCK 法案》披露的股票交易,逐条对应官方申报、可按议员与个股筛选。公开数据,不构成投资建议。',
    descEn:
      'Track U.S. congressional stock trades disclosed under the STOCK Act, linked to the official filings. Public data, not investment advice.',
    keywords: ['国会山股神', '佩洛西持仓', '美国国会议员股票交易', '议员持仓查询', 'congress stock tracker', 'STOCK Act'],
    h1Zh: '国会山股神:美国国会议员持仓与交易查询',
    h1En: 'Congress Stock Tracker: U.S. Lawmakers’ Trades',
    bodyZh: [
      '“国会山股神”指的是美国国会参众两院议员的股票交易。根据 2012 年《STOCK 法案》,议员及其配偶在买卖股票后必须在 45 天内公开申报,这些定期交易报告(PTR)是公开数据。市场之所以关注,是因为部分议员的择时表现长期跑赢大盘,佩洛西(Nancy Pelosi)夫妇的科技股仓位尤其常被讨论。',
      'Tickwind 汇总众议院文书办公室的公开 PTR 申报,逐条标注交易议员、个股、买/卖方向与申报日期,并链接到官方原文。你可以据此观察资金流向,但披露有最多 45 天延迟、且只标交易区间金额——它是一个信号来源,而非买卖建议。',
    ],
    bodyEn: [
      'The “Congress stock tracker” follows the stock trades of U.S. House and Senate members. Under the 2012 STOCK Act, lawmakers must disclose trades within 45 days; these Periodic Transaction Reports (PTRs) are public. Markets watch them because some members have historically timed trades well — the Pelosi household’s tech positions are the most-discussed.',
      'Tickwind aggregates the House Clerk’s public PTR filings, tagging each with the lawmaker, ticker, buy/sell side and filing date, and links to the official source. Disclosures lag up to 45 days and show only amount ranges, so treat them as one signal — not advice.',
    ],
    cta: {href: '/smart-money', labelZh: '查看国会交易看板', labelEn: 'Open the Congress board'},
    faq: [
      {
        qZh: '什么是国会山股神?',
        aZh: '指美国国会议员依《STOCK 法案》公开披露的股票交易;因部分议员择时表现突出而受市场关注。',
        qEn: 'What is the “Congress stock tracker”?',
        aEn: 'It tracks U.S. lawmakers’ stock trades disclosed under the STOCK Act — watched because some members have timed trades notably well.',
      },
      {
        qZh: '议员交易披露有多及时?',
        aZh: '《STOCK 法案》要求交易后 45 天内申报,因此数据存在最多 45 天延迟,且只披露金额区间。',
        qEn: 'How timely are the disclosures?',
        aEn: 'The STOCK Act requires filing within 45 days, so the data lags up to 45 days and shows only amount ranges.',
      },
    ],
    related: ['insider-buying', '13f-whale-watching'],
  },
  {
    slug: 'insider-buying',
    titleEn: 'US Insider Buying — Executive Open-Market Buys · Tickwind',
    titleZh: '美股内部人买入 · 高管增持信号查询 · 潮汐 Tickwind',
    descZh:
      '美股内部人买入怎么查?从 SEC Form 4 申报中筛出高管、董事在公开市场真金白银增持的个股(尤其小盘股),逐条链接官方申报。公开数据,不构成投资建议。',
    descEn:
      'Find U.S. insider buying from SEC Form 4 filings — executives and directors buying their own stock on the open market. Public data, not advice.',
    keywords: ['美股内部人买入', '高管增持', '内部人交易查询', 'SEC Form 4', 'insider buying', '小盘股增持'],
    h1Zh: '美股内部人买入:高管增持信号查询',
    h1En: 'US Insider Buying: Executive Open-Market Buys',
    bodyZh: [
      '内部人买入指公司高管、董事或大股东在公开市场用自有资金买入自家股票。相比卖出(原因很多),公开市场的“增持”信号更纯粹——他们比任何人都了解公司,愿意自掏腰包买入,往往被视为对前景有信心。这类交易通过 SEC Form 4 在两个工作日内申报,属公开数据。',
      'Tickwind 持续扫描每日 Form 4 申报,只保留代码 P(公开市场买入),并聚焦市值较小、信号更显著的个股,展示买入人数、合计金额与官方申报链接。请注意:增持不等于股价必涨,它只是众多基本面线索之一。',
    ],
    bodyEn: [
      'Insider buying is when a company’s executives, directors or large holders buy their own stock on the open market with their own money. Unlike selling (which has many motives), an open-market buy is a cleaner signal — insiders know the business best and are putting cash in. These trades are disclosed on SEC Form 4 within two business days and are public.',
      'Tickwind continuously scans daily Form 4 filings, keeps only code P (open-market buys), and focuses on smaller-cap names where the signal is sharper — showing the number of buyers, total value and the official filing link. A buy is not a guarantee of upside; it is one fundamental clue among many.',
    ],
    cta: {href: '/opportunities', labelZh: '查看内部人买入榜', labelEn: 'Open the Opportunity board'},
    faq: [
      {
        qZh: '在哪里查美股内部人买入?',
        aZh: '内部人交易通过 SEC Form 4 申报。Tickwind 机会榜筛出其中“公开市场买入”(代码 P)的小盘股并链接原文。',
        qEn: 'Where can I find U.S. insider buying?',
        aEn: 'Insider trades are filed on SEC Form 4. Tickwind’s Opportunity board surfaces open-market buys (code P) in small-caps and links the filing.',
      },
      {
        qZh: '内部人买入是利好吗?',
        aZh: '通常被视为管理层有信心的信号,但不保证股价上涨,应结合基本面综合判断。',
        qEn: 'Is insider buying bullish?',
        aEn: 'It’s often read as a confidence signal, but it doesn’t guarantee upside — weigh it with the fundamentals.',
      },
    ],
    related: ['congress-stock-tracker', '13f-whale-watching'],
  },
  {
    slug: 'unusual-options',
    titleEn: 'Unusual Options Activity — Most-Traded US Options · Tickwind',
    titleZh: '美股期权异动 · 全市场期权成交龙虎榜 · 潮汐 Tickwind',
    descZh:
      '美股期权异动怎么看?今日全市场成交最活跃的看涨/看跌合约,按单合约成交量排名,附量比(成交/未平仓)、行权价、到期日。数据延迟约15分钟(Cboe),不构成投资建议。',
    descEn:
      'See unusual options activity across US stocks — the most-traded call/put contracts ranked by volume, with volume/OI, strike and expiry. ~15-min delayed (Cboe), not advice.',
    keywords: ['美股期权异动', '期权成交量', '量比', '期权龙虎榜', 'unusual options activity', 'options flow'],
    h1Zh: '美股期权异动榜:主力资金流向哪里',
    h1En: 'Unusual Options Activity: Where the Flow Is Going',
    bodyZh: [
      '期权异动指某些合约的成交量异常放大,常被用来观察主力资金的方向与情绪。一个关键指标是“量比”——当日成交量除以未平仓量(OI):量比远大于 1,意味着大量新仓在今天涌入,而非旧仓换手,信号更值得留意。',
      'Tickwind 扫描全市场流动性较好的期权,按单合约成交量排名,展示看涨/看跌、行权价、到期日与量比。期权是杠杆工具、且异动未必代表“聪明钱”——它是一个观察情绪的窗口,不构成交易建议。数据来自 Cboe,约延迟 15 分钟。',
    ],
    bodyEn: [
      'Unusual options activity means a contract’s volume spikes abnormally — often used to read where flow and sentiment are pointing. A key tell is the volume/open-interest ratio: well above 1 means a lot of new positions opened today rather than old ones changing hands, which is more notable.',
      'Tickwind scans heavily-traded options market-wide, ranks by single-contract volume, and shows call/put, strike, expiry and the volume/OI ratio. Options are leveraged and unusual flow isn’t necessarily “smart money” — it’s a sentiment window, not advice. Data is ~15-min delayed from Cboe.',
    ],
    cta: {href: '/unusual', labelZh: '查看期权异动榜', labelEn: 'Open the Unusual Options board'},
    faq: [
      {
        qZh: '期权“量比”是什么意思?',
        aZh: '量比 = 当日成交量 ÷ 未平仓量(OI)。远大于 1 说明今天有大量新建仓位,异动更值得关注。',
        qEn: 'What does the options volume/OI ratio mean?',
        aEn: 'It’s the day’s volume divided by open interest. Well above 1 means lots of new positions opened today — a more meaningful spike.',
      },
      {
        qZh: '期权异动数据多实时?',
        aZh: 'Tickwind 的期权数据来自 Cboe,约延迟 15 分钟,仅供观察、不构成投资建议。',
        qEn: 'How real-time is the options data?',
        aEn: 'Tickwind’s options data is ~15 minutes delayed (Cboe), for observation only — not investment advice.',
      },
    ],
    related: ['short-squeeze-radar', 'insider-buying'],
  },
  {
    slug: '13f-whale-watching',
    titleEn: '13F Whale Watching — Top Funds’ Quarterly Holdings · Tickwind',
    titleZh: '13F 大佬持仓 · 顶级基金季度持仓跟踪 · 潮汐 Tickwind',
    descZh:
      '13F 大佬持仓怎么查?跟踪伯克希尔(巴菲特)、Scion(Michael Burry)等知名基金每季度向 SEC 申报的 13F 持仓与环比加减仓,逐条链接官方申报。公开数据,不构成投资建议。',
    descEn:
      'Track 13F holdings of famous funds (Berkshire/Buffett, Scion/Burry and more) — quarterly SEC filings with quarter-over-quarter changes. Public data, not advice.',
    keywords: ['13F 持仓', '大佬持仓', '巴菲特持仓', '机构持仓查询', '13F filings', 'whale watching'],
    h1Zh: '13F 大佬持仓:跟踪顶级基金的季度持仓',
    h1En: '13F Whale Watching: Top Funds’ Quarterly Holdings',
    bodyZh: [
      '管理超过 1 亿美元的机构,必须在每个季度结束后 45 天内向 SEC 申报 13F 报告,披露其美股持仓。这让普通投资者得以“抄作业”——看巴菲特的伯克希尔、Michael Burry 的 Scion、Bill Ackman 的 Pershing Square 等知名基金这一季买了什么、加减了哪些仓位。',
      'Tickwind 解析这些 13F 申报,按基金汇总持仓,并计算环比(QoQ)加仓/减仓/新建/清仓,链接官方原文。注意:13F 有最多 45 天延迟、只含季末快照、不含做空与海外仓位——它告诉你“季度末他们持有什么”,而不是“现在该买什么”。',
    ],
    bodyEn: [
      'Institutions managing over $100M must file a 13F with the SEC within 45 days of each quarter-end, disclosing their U.S. holdings. That lets ordinary investors look over the shoulders of names like Buffett’s Berkshire, Michael Burry’s Scion and Bill Ackman’s Pershing Square — what they bought and how positions changed.',
      'Tickwind parses these 13F filings, aggregates holdings per fund, computes quarter-over-quarter adds/trims/new/exits, and links the source. Note: 13F lags up to 45 days, is a quarter-end snapshot, and excludes shorts and non-U.S. positions — it shows what they held, not what to buy now.',
    ],
    cta: {href: '/smart-money', labelZh: '查看 13F 持仓看板', labelEn: 'Open the 13F board'},
    faq: [
      {
        qZh: '13F 报告是什么?',
        aZh: '管理超 1 亿美元的机构须在季末 45 天内向 SEC 申报的美股持仓清单,属公开数据。',
        qEn: 'What is a 13F report?',
        aEn: 'A public SEC filing of U.S. holdings that institutions managing over $100M must submit within 45 days of quarter-end.',
      },
      {
        qZh: '能看到巴菲特最新持仓吗?',
        aZh: '能看到最近一期 13F 申报的伯克希尔持仓及环比变化,但有最多 45 天延迟、为季末快照。',
        qEn: 'Can I see Buffett’s latest holdings?',
        aEn: 'You can see Berkshire’s most recent 13F holdings and changes — but it lags up to 45 days and is a quarter-end snapshot.',
      },
    ],
    related: ['congress-stock-tracker', 'insider-buying'],
  },
  {
    slug: 'short-squeeze-radar',
    titleEn: 'Short Squeeze Radar — High Short Interest US Stocks · Tickwind',
    titleZh: '美股轧空雷达 · 高做空比例个股查询 · 潮汐 Tickwind',
    descZh:
      '美股轧空(逼空)怎么看?结合 FINRA 做空比例、社媒讨论热度(含 WSB)观察潜在轧空候选。数据来自公开来源,不构成投资建议。',
    descEn:
      'Spot potential short squeezes in US stocks using FINRA short interest plus social buzz (incl. WSB). Public data, not investment advice.',
    keywords: ['美股轧空', '逼空', '做空比例', 'short squeeze', '空头回补', 'WSB'],
    h1Zh: '美股轧空雷达:高做空比例与逼空信号',
    h1En: 'Short Squeeze Radar: High Short Interest Signals',
    bodyZh: [
      '轧空(short squeeze)指做空者被迫回补、推动股价快速上涨的现象。当一只股票的做空比例(short interest)很高、可借券紧张,又遇到利好或社媒集中买入时,空头回补可能形成连锁上涨——2021 年的 GME 是最著名的例子。',
      'Tickwind 在个股页展示 FINRA 双月披露的做空比例与回补天数(days-to-cover),并结合社媒讨论热度榜(含 WSB)帮你发现讨论升温的高做空标的。轧空难以预测、风险极高——这些数据用于观察,不构成任何交易建议。',
    ],
    bodyEn: [
      'A short squeeze is when short sellers are forced to buy back, driving the price up fast. When a stock has high short interest and tight borrow, then meets good news or concentrated social buying, short covering can cascade — GME in 2021 is the famous example.',
      'Tickwind shows FINRA’s bi-monthly short interest and days-to-cover on each stock page, and pairs it with a social-buzz leaderboard (incl. WSB) to surface heavily-shorted names getting attention. Squeezes are unpredictable and very risky — this is for observation, not advice.',
    ],
    cta: {href: '/hot', labelZh: '查看热度榜(含 WSB)', labelEn: 'Open the Hot board (incl. WSB)'},
    faq: [
      {
        qZh: '什么是轧空(short squeeze)?',
        aZh: '高做空个股遇到上涨时,空头被迫回补买入、进一步推高股价的连锁现象。',
        qEn: 'What is a short squeeze?',
        aEn: 'When a heavily-shorted stock rises, shorts are forced to buy back, pushing the price up further in a cascade.',
      },
      {
        qZh: '在哪看美股做空比例?',
        aZh: 'Tickwind 个股页展示 FINRA 双月披露的做空比例与回补天数,并配社媒热度榜辅助观察。',
        qEn: 'Where can I see U.S. short interest?',
        aEn: 'Tickwind shows FINRA’s bi-monthly short interest and days-to-cover on each stock page, alongside a social-buzz board.',
      },
    ],
    related: ['unusual-options', '13f-whale-watching'],
  },
];

/** Returns the guide for a slug, or undefined. */
export function guideBySlug(slug: string): Guide | undefined {
  return GUIDES.find(g => g.slug === slug);
}
