import type {Metadata} from 'next';
import {BriefingView} from '@/components/BriefingView';

export const metadata: Metadata = {
  title: '盘前晨报',
  description: '每日 AI 盘前晨报：指数、涨跌焦点、今日财报与聪明钱动向，一分钟读完。',
};

/** Daily AI morning-briefing page (content generated server-side, once a day). */
export default function BriefingPage() {
  return <BriefingView />;
}
