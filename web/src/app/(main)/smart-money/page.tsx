import type {Metadata} from 'next';
import {SITE_URL} from '@/lib/config';
import {SmartMoneyTabs, type SmartMoneyTab} from '@/components/SmartMoneyTabs';

export const metadata: Metadata = {
  title: '聪明钱 · 国会山股神 & 机构举牌 | Smart Money',
  description:
    '国会山股神来了：美国国会议员股票交易披露（佩洛西等）+ SEC 13D/13G 机构举牌与维权持仓，中英对照、逐条链接官方申报。Track U.S. congressional stock trades and institutional 13D/13G stakes side by side. 公开数据，不构成投资建议。',
  keywords: ['国会山股神', '佩洛西持仓', '美国国会议员股票交易', '13D 举牌', '机构持仓', 'congress trading tracker', '13D 13G filings'],
  alternates: {canonical: `${SITE_URL}/smart-money`},
};

/** Merged institutional (13D/13G) + Congress trading board. */
export default async function SmartMoneyPage({
  searchParams,
}: {
  searchParams: Promise<{tab?: string}>;
}) {
  const sp = await searchParams;
  const initial: SmartMoneyTab = sp.tab === 'congress' ? 'congress' : 'institutional';
  return <SmartMoneyTabs initial={initial} />;
}
