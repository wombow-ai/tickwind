import type {Metadata} from 'next';
import {SITE_URL} from '@/lib/config';
import {UnusualOptions} from '@/components/UnusualOptions';

export const metadata: Metadata = {
  title: '期权异动榜 · 全市场期权成交龙虎榜 | Unusual Options Activity',
  description:
    '美股期权异动榜：今日全市场成交最活跃的看涨/看跌期权合约，按单合约成交量排名，附量比(成交/未平仓)、行权价、到期日、隐波。看清主力资金正在涌入哪里。Unusual options activity across heavily-optioned US stocks — ranked by single-contract volume. 数据延迟约15分钟(Cboe)，不构成投资建议。',
  keywords: ['期权异动', '美股期权', '期权成交量', '量比', 'unusual options activity', 'options flow', '期权龙虎榜', '看涨期权', '看跌期权'],
  alternates: {canonical: `${SITE_URL}/unusual`},
};

/** Whole-market unusual options-activity board. */
export default function UnusualPage() {
  return <UnusualOptions />;
}
