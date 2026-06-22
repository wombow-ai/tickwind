import type {Metadata} from 'next';
import Link from '@/components/LocalLink';
import {Scale} from 'lucide-react';
import {COMPARE_PAIRS, pairSlug} from '@/lib/compare';
import {langAlternates} from '@/lib/config';
import {isLocale} from '@/lib/locale';

export const revalidate = 86400;

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string}>;
}): Promise<Metadata> {
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';
  return {
    title: {absolute: zh ? '股票对比 · 并排比较美股估值与基本面 | Tickwind' : 'Compare Stocks — Side-by-Side US Stock Comparison | Tickwind'},
    description: zh
      ? '并排对比任意两只美股的市值、市盈率、市净率、营收、净利润与每股收益 —— 全部基于公开数据。仅供参考,不构成投资建议。'
      : 'Compare any two US stocks side by side — market cap, P/E, P/B, revenue, net income and EPS, all from public data. For reference only, not investment advice.',
    alternates: langAlternates('/compare', loc),
  };
}

/**
 * The `/compare` hub: a crawlable landing that introduces side-by-side stock comparison and links
 * the curated rivalries (each → `/compare/{a}-vs-{b}`). Internal-linking + an entry point for the
 * pSEO comparison pages. Single-locale server render.
 */
export default async function CompareHub({params}: {params: Promise<{locale: string}>}) {
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';

  return (
    <div className="mx-auto max-w-3xl">
      <header className="mb-6">
        <h1 className="flex items-center gap-2 text-[24px] font-bold tracking-tight text-slate-900 dark:text-slate-100">
          <Scale size={20} className="text-teal-500 dark:text-teal-300" />
          {zh ? '股票对比' : 'Compare Stocks'}
        </h1>
        <p className="mt-1.5 text-[13.5px] leading-relaxed text-slate-600 dark:text-slate-300">
          {zh
            ? '并排对比两只美股的估值与基本面 —— 市值、市盈率、市净率、营收、净利润、每股收益,全部基于公开数据。'
            : 'Two US stocks side by side — market cap, P/E, P/B, revenue, net income and EPS, all from public data.'}
        </p>
      </header>

      <section>
        <h2 className="mb-2.5 text-[12px] font-semibold uppercase tracking-wide text-slate-400 dark:text-slate-500">
          {zh ? '热门对比' : 'Popular comparisons'}
        </h2>
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
          {COMPARE_PAIRS.map(([a, b]) => (
            <Link
              key={pairSlug(a, b)}
              href={`/compare/${pairSlug(a, b)}`}
              className="flex items-center justify-center gap-1.5 rounded-xl border border-slate-200 px-3 py-2.5 text-[14px] font-bold text-slate-900 transition hover:border-teal-300 hover:bg-slate-50 dark:border-slate-800 dark:text-slate-100 dark:hover:border-teal-500/40 dark:hover:bg-slate-900"
            >
              {a} <span className="text-[11px] font-normal text-slate-400 dark:text-slate-500">vs</span> {b}
            </Link>
          ))}
        </div>
      </section>

      <p className="mt-7 text-center text-[11px] text-slate-400 dark:text-slate-500">
        {zh ? '数据延迟 · 公开数据 · 仅供参考 · 非投资建议' : 'Delayed data · Public sources · For reference only · Not investment advice'}
      </p>
    </div>
  );
}
