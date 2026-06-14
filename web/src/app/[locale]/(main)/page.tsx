import type {Metadata} from 'next';
import Link from '@/components/LocalLink';
import {APP_NAME, APP_TAGLINE, SITE_URL, langAlternates} from '@/lib/config';
import {ogImageMeta} from '@/lib/og';
import {isLocale} from '@/lib/locale';
import {HomeHub} from '@/components/HomeHub';

// The tab title in each language, chosen server-side by the route locale in
// generateMetadata so each locale URL ships its own <title>. The Chinese
// keywords still live in the description, keywords and the JSON-LD below.
const TITLE_EN = 'Tickwind · US Stocks, Congress Trades, Options Flow & 13F';
const TITLE_ZH = '潮汐 Tickwind · 美股实时行情 / 国会山股神 / 期权异动 / 13F大佬持仓 / 财报';

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string}>;
}): Promise<Metadata> {
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  return {
    title: {absolute: loc === 'zh' ? TITLE_ZH : TITLE_EN},
    description:
      'Tickwind 是面向中文投资者的美股数据平台:实时行情、SEC 内部人买入、国会山股神(议员持仓交易)、13D/13G 机构举牌、财报日历、期权异动、轧空雷达与 AI 中文速览。Data-first US-stock tracker for Chinese-speaking investors.',
    keywords: ['美股', '美股行情', '国会山股神', '美股内部人买入', '财报日历', '期权异动', '轧空', 'AI 股票速览', '13F 持仓', 'US stocks', 'congress trading'],
    alternates: langAlternates('/', loc),
    openGraph: {
      type: 'website',
      title: loc === 'zh' ? TITLE_ZH : TITLE_EN,
      url: `${SITE_URL}/${loc}`,
      images: [
        ogImageMeta({
          lang: loc,
          eyebrow: loc === 'zh' ? '中文美股数据台' : 'US-stock data desk',
          title:
            loc === 'zh'
              ? '美股实时行情 · 国会交易 · 13F · 期权异动'
              : 'US stocks · congress trades · 13F · options flow',
          subtitle:
            loc === 'zh'
              ? '数据优先,免费看清美股'
              : 'Data-first US-stock tracker — free',
        }),
      ],
    },
  };
}

// Server-rendered links to every public board — real content + internal linking
// for crawlers, and a useful directory for humans. Each entry carries both
// languages; the SSR intro renders only the active locale (chosen from the route
// segment), so /en and /zh ship genuinely distinct single-language HTML.
const DIRECTORY: {href: string; zh: string; en: string; descZh: string; descEn: string}[] = [
  {href: '/smart-money', zh: '聪明钱 · 国会山股神', en: 'Smart Money · Congress', descZh: '国会议员交易 + 13D/13G 机构举牌', descEn: 'Congressional trades + 13D/13G stakes'},
  {href: '/opportunities', zh: '机会榜 · 内部人买入', en: 'Opportunities · Insider Buys', descZh: 'SEC Form 4 高管增持的小盘股', descEn: 'Small-caps insiders are buying (SEC Form 4)'},
  {href: '/hot', zh: '热门 & 飙升', en: 'Hot & Surging', descZh: '社媒讨论热度榜(含 WSB)', descEn: 'Social-buzz leaderboard (incl. WSB)'},
  {href: '/screen', zh: '选股器', en: 'Screener', descZh: '按价格 / 涨跌幅 / 市值筛选全美股', descEn: 'Filter all US stocks by price / change / cap'},
  {href: '/calendar/earnings', zh: '财报日历', en: 'Earnings Calendar', descZh: '今日及未来财报(预估 EPS)', descEn: 'Upcoming reports with est. EPS'},
  {href: '/unusual', zh: '期权异动榜', en: 'Unusual Options', descZh: '全市场成交最活跃的期权合约', descEn: 'The most-traded options across the market'},
  {href: '/calendar/macro', zh: '大事件时间线', en: 'Events Timeline', descZh: 'FOMC / CPI 等宏观日历', descEn: 'Macro calendar — FOMC, CPI & more'},
  {href: '/discussion?tab=community', zh: '社区讨论', en: 'Community', descZh: '个股 & 大盘讨论区', descEn: 'Per-stock & market-wide discussion'},
  {href: '/guide', zh: '新手指南', en: 'Guides', descZh: '美股数据怎么查:国会山股神 / 13F / 期权异动', descEn: 'How to track congress, 13F, options & more'},
];

/**
 * Data-first home: a live client hub (Markets strip + Hot/News/Discussion
 * modules) over a server-rendered intro + board directory. The intro gives
 * crawlers real, keyword-rich content and internal links — the live modules
 * stream client-side and aren't indexable on their own. The SSR intro renders
 * only the active locale's copy so /en and /zh are distinct single-language HTML.
 */
export default async function HomePage({
  params,
}: {
  params: Promise<{locale: string}>;
}) {
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const ld = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'WebSite',
        name: APP_NAME,
        alternateName: '潮汐美股',
        url: SITE_URL,
        description: APP_TAGLINE,
        inLanguage: ['zh-CN', 'en'],
        potentialAction: {
          '@type': 'SearchAction',
          target: {'@type': 'EntryPoint', urlTemplate: `${SITE_URL}/search?q={search_term_string}`},
          'query-input': 'required name=search_term_string',
        },
      },
      {'@type': 'Organization', name: APP_NAME, url: SITE_URL, description: APP_TAGLINE},
    ],
  };

  return (
    <>
      <script type="application/ld+json" dangerouslySetInnerHTML={{__html: JSON.stringify(ld)}} />
      <HomeHub />

      <section className="mx-auto mt-10 max-w-5xl border-t border-slate-200 pt-6 dark:border-slate-800">
        <h2 className="text-[15px] font-bold text-slate-800 dark:text-slate-100">
          {loc === 'zh' ? 'Tickwind —— 中文投资者的美股数据台' : 'Tickwind — a data-first US-stock tracker'}
        </h2>
        {loc === 'zh' ? (
          <p className="mt-2 max-w-3xl text-[13px] leading-relaxed text-slate-500 dark:text-slate-400">
            一站看清美股:实时行情与盘前盘后、SEC 内部人买入、
            <strong className="font-semibold">国会山股神</strong>(美国国会议员持仓交易)、13D/13G 机构举牌、
            财报日历、期权异动与最大痛点、轧空雷达,以及由 AI 生成的中文个股速览与盘前晨报。
            数据来自 SEC、FINRA、Cboe 等公开来源,不构成投资建议。
          </p>
        ) : (
          <p className="mt-2 max-w-3xl text-[13px] leading-relaxed text-slate-500 dark:text-slate-400">
            Everything on US stocks in one place: real-time quotes including pre- and post-market, SEC insider
            buying, <strong className="font-semibold">congressional stock trades</strong>, 13D/13G activist stakes,
            the earnings calendar, unusual options activity &amp; max pain, and a short-squeeze radar — plus
            AI-written stock digests and a daily morning briefing. Data from public sources (SEC, FINRA, Cboe);
            not investment advice.
          </p>
        )}
        <nav className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-4" aria-label="Board directory">
          {DIRECTORY.map(d => (
            <Link
              key={d.href}
              href={d.href}
              className="rounded-xl border border-slate-200 p-3 transition hover:border-teal-400 dark:border-slate-800"
            >
              <div className="text-[12.5px] font-semibold text-slate-800 dark:text-slate-100">
                {loc === 'zh' ? d.zh : d.en}
              </div>
              <div className="mt-0.5 text-[11px] text-slate-500 dark:text-slate-400">
                {loc === 'zh' ? d.descZh : d.descEn}
              </div>
            </Link>
          ))}
        </nav>
      </section>
    </>
  );
}
