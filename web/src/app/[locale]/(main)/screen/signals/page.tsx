import type {Metadata} from 'next';
import {langAlternates} from '@/lib/config';
import {isLocale} from '@/lib/locale';
import {ogImageMeta} from '@/lib/og';
import {SignalsScreen} from '@/components/SignalsScreen';

// English browser-tab title is the default (crawlers + the English UI); Chinese
// keywords stay in description/keywords. A STATIC segment under /screen, so it takes
// precedence over the dynamic /screen/[preset] price-screener landing pages.
const TITLE_EN = 'Signal Screener · Find Stocks by Technical Signal · Tickwind';
const TITLE_ZH = '信号筛选器 · 按技术信号筛选美股 · Tickwind';

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
      'Screen US stocks by deterministic technical signals — golden/death cross, RSI oversold/overbought, MACD cross — each computed from public daily data with a traceable basis, never advice. 美股技术信号筛选器：金叉/死叉、RSI 超买超卖、MACD 交叉，全部基于公开日线数据确定性计算，每条都有可溯源依据，不构成投资建议。',
    keywords: [
      'stock screener',
      'signal screener',
      'golden cross stocks',
      'RSI oversold screener',
      'MACD cross',
      '美股筛选器',
      '信号筛选',
      '金叉股票',
      'RSI 超卖',
    ],
    alternates: langAlternates('/screen/signals', loc),
    openGraph: {
      images: [
        ogImageMeta({
          lang: loc,
          eyebrow: loc === 'zh' ? '信号筛选' : 'Signal Screener',
          title: loc === 'zh' ? '按技术信号筛选美股' : 'Screen Stocks by Technical Signal',
          subtitle:
            loc === 'zh'
              ? '金叉/死叉 · RSI 超买超卖 · MACD 交叉 · 确定性计算'
              : 'Golden/death cross · RSI · MACD cross · deterministic',
        }),
      ],
    },
  };
}

export default function SignalScreenPage() {
  return <SignalsScreen />;
}
