import type {Metadata} from 'next';
import {GuruRail} from '@/components/GuruRail';
import {LocalizedTitle} from '@/components/LocalizedTitle';
import {OpportunityBoard} from '@/components/OpportunityBoard';
import {WsbBoard} from '@/components/WsbBoard';

// English browser-tab title is the default (crawlers + the English UI); Chinese
// keywords stay in description/keywords. LocalizedTitle swaps in zh.
const TITLE_EN = 'Opportunity Board · US Insider Open-Market Buys · Tickwind';
const TITLE_ZH = '机会榜 · 美股内部人买入 · 高管增持 · 潮汐 Tickwind';

export const metadata: Metadata = {
  title: {absolute: TITLE_EN},
  description:
    '美股内部人买入雷达：从 SEC Form 4 申报中挖出高管/董事公开市场增持的小盘股，配合独立财经作者的最新观点。US insider-buying signals from SEC Form 4 filings. 公开数据，不构成投资建议。',
  keywords: ['美股内部人买入', '高管增持', '内部人交易', 'Form 4', 'insider buying', '小盘股机会'],
};

/** Public Opportunity board (small-cap insider-buy signals) + the Guru-watch rail. */
export default function OpportunitiesPage() {
  return (
    <div className="grid gap-8 lg:grid-cols-3">
      <LocalizedTitle en={TITLE_EN} zh={TITLE_ZH} />
      <div className="lg:col-span-2">
        <OpportunityBoard />
      </div>
      <div className="flex flex-col gap-8 lg:col-span-1">
        <WsbBoard />
        <GuruRail />
      </div>
    </div>
  );
}
