import type {Metadata} from 'next';
import Link from '@/components/LocalLink';
import {langAlternates} from '@/lib/config';
import {GUIDES} from '@/lib/guides';
import {isLocale} from '@/lib/locale';

const TITLE_EN = 'Guides — How to Track US Stocks, Congress, 13F & Options · Tickwind';
const TITLE_ZH = '指南 · 美股数据怎么查:国会山股神 / 13F / 期权异动 / 内部人买入 · 潮汐 Tickwind';

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
      '美股数据查询指南:国会山股神、13F 大佬持仓、美股内部人买入、期权异动、轧空雷达 —— 每个主题怎么看、在哪查,链接到对应的实时看板。公开数据,不构成投资建议。',
    keywords: ['美股查询', '国会山股神', '13F 持仓', '美股内部人买入', '期权异动', '美股轧空'],
    alternates: langAlternates('/guide', loc),
  };
}

/** Hub of the keyword landing pages — real internal linking for crawlers. */
export default async function GuideHub({
  params,
}: {
  params: Promise<{locale: string}>;
}) {
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  return (
    <div className="mx-auto max-w-3xl">
      <header className="mb-6">
        <h1 className="text-[26px] font-bold tracking-tight text-slate-900 dark:text-slate-100">
          {loc === 'zh' ? '指南:美股数据怎么查' : 'Guides: How to Track US Stocks'}
        </h1>
        <p className="mt-1.5 text-[13.5px] text-slate-500 dark:text-slate-400">
          {loc === 'zh'
            ? '每个信号怎么看、在哪查 —— 每篇都链接到对应的实时看板。'
            : 'What each signal means and where to find it — each links to its live board.'}
        </p>
      </header>

      <ul className="grid gap-3 sm:grid-cols-2">
        {GUIDES.map(g => (
          <li key={g.slug}>
            <Link
              href={`/guide/${g.slug}`}
              className="block h-full rounded-2xl border border-slate-200 p-4 transition hover:border-teal-400 dark:border-slate-800"
            >
              <div className="text-[14px] font-semibold text-slate-900 dark:text-slate-100">
                {loc === 'zh' ? g.h1Zh : g.h1En}
              </div>
              <p className="mt-1 line-clamp-2 text-[12.5px] leading-relaxed text-slate-500 dark:text-slate-400">
                {loc === 'zh' ? g.descZh : g.descEn}
              </p>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
