import type {Metadata} from 'next';
import {langAlternates} from '@/lib/config';
import {isLocale} from '@/lib/locale';
import {ogImageMeta} from '@/lib/og';
import {LocalizedTitle} from '@/components/LocalizedTitle';
import {SmartMoneyTabs, type SmartMoneyTab} from '@/components/SmartMoneyTabs';

// English browser-tab title is the default (crawlers + the English UI); Chinese
// keywords stay in description/keywords. LocalizedTitle swaps in zh.
const TITLE_EN = 'Smart Money · Congress Trades, 13F & Activist Filings · Tickwind';
const TITLE_ZH = '聪明钱 · 国会山股神 · 13F 大佬持仓 · 机构举牌 · 潮汐 Tickwind';

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
      '国会山股神来了：美国国会议员股票交易披露（佩洛西等）+ SEC 13D/13G 机构举牌与维权持仓，中英对照、逐条链接官方申报。Track U.S. congressional stock trades and institutional 13D/13G stakes side by side. 公开数据，不构成投资建议。',
    keywords: ['国会山股神', '佩洛西持仓', '美国国会议员股票交易', '13D 举牌', '机构持仓', 'congress trading tracker', '13D 13G filings'],
    alternates: langAlternates('/smart-money', loc),
    openGraph: {
      images: [
        ogImageMeta({
          lang: loc,
          eyebrow: loc === 'zh' ? '聪明钱' : 'Smart Money',
          title:
            loc === 'zh'
              ? '国会山股神 · 13F 大佬持仓 · 机构举牌'
              : 'Congress Trades · 13F Whales · Activist Filings',
          subtitle:
            loc === 'zh'
              ? '跟踪美国国会议员交易与机构持仓变动'
              : 'Tracking U.S. congressional trades and institutional holdings',
        }),
      ],
    },
  };
}

/** Merged institutional (13D/13G) + Congress trading board. */
export default async function SmartMoneyPage({
  searchParams,
}: {
  searchParams: Promise<{tab?: string}>;
}) {
  const sp = await searchParams;
  const initial: SmartMoneyTab =
    sp.tab === 'congress' ? 'congress' : sp.tab === 'institutional' ? 'institutional' : '13f';
  return (
    <>
      <LocalizedTitle en={TITLE_EN} zh={TITLE_ZH} />
      <SmartMoneyTabs initial={initial} />
    </>
  );
}
