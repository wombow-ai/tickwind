import type {Metadata} from 'next';
import Link from 'next/link';
import {APP_NAME, APP_TAGLINE, SITE_URL} from '@/lib/config';
import {HomeHub} from '@/components/HomeHub';

export const metadata: Metadata = {
  description:
    'Tickwind 是面向中文投资者的美股数据平台:实时行情、SEC 内部人买入、国会山股神(议员持仓交易)、13D/13G 机构举牌、财报日历、期权异动、轧空雷达与 AI 中文速览。Data-first US-stock tracker for Chinese-speaking investors.',
  keywords: ['美股', '美股行情', '国会山股神', '美股内部人买入', '财报日历', '期权异动', '轧空', 'AI 股票速览', 'US stocks', 'congress trading'],
};

// Server-rendered links to every public board — real content + internal linking
// for crawlers, and a useful directory for humans. Keyword anchors in zh.
const DIRECTORY: {href: string; zh: string; desc: string}[] = [
  {href: '/smart-money', zh: '聪明钱 · 国会山股神', desc: '国会议员交易 + 13D/13G 机构举牌'},
  {href: '/opportunities', zh: '机会榜 · 内部人买入', desc: 'SEC Form 4 高管增持的小盘股'},
  {href: '/hot', zh: '热门 & 飙升', desc: '社媒讨论热度榜(含 WSB)'},
  {href: '/screen', zh: '选股器', desc: '按价格 / 涨跌幅 / 市值筛选全美股'},
  {href: '/earnings', zh: '财报日历', desc: '今日及未来财报(预估 EPS)'},
  {href: '/briefing', zh: '盘前晨报', desc: '每日 AI 中文盘前简报'},
  {href: '/events', zh: '大事件时间线', desc: 'FOMC / CPI 等宏观日历'},
  {href: '/community', zh: '社区讨论', desc: '个股 & 大盘讨论区'},
];

/**
 * Data-first home: a live client hub (Markets strip + Hot/News/Discussion
 * modules) over a server-rendered intro + board directory. The intro gives
 * crawlers real, keyword-rich content and internal links — the live modules
 * stream client-side and aren't indexable on their own.
 */
export default function HomePage() {
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
          Tickwind —— 中文投资者的美股数据台
        </h2>
        <p className="mt-2 max-w-3xl text-[13px] leading-relaxed text-slate-500 dark:text-slate-400">
          一站看清美股:实时行情与盘前盘后、SEC 内部人买入、
          <strong className="font-semibold">国会山股神</strong>(美国国会议员持仓交易)、13D/13G 机构举牌、
          财报日历、期权异动与最大痛点、轧空雷达,以及由 AI 生成的中文个股速览与盘前晨报。
          数据来自 SEC、FINRA、Cboe 等公开来源,不构成投资建议。
        </p>
        <nav className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-4" aria-label="板块导航">
          {DIRECTORY.map(d => (
            <Link
              key={d.href}
              href={d.href}
              className="rounded-xl border border-slate-200 p-3 transition hover:border-teal-400 dark:border-slate-800"
            >
              <div className="text-[12.5px] font-semibold text-slate-800 dark:text-slate-100">{d.zh}</div>
              <div className="mt-0.5 text-[11px] text-slate-500 dark:text-slate-400">{d.desc}</div>
            </Link>
          ))}
        </nav>
      </section>
    </>
  );
}
