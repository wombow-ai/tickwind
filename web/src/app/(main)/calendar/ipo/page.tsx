import type {Metadata} from 'next';
import {langAlternates} from '@/lib/config';
import {ogImageMeta} from '@/lib/og';
import {IPOCalendar} from '@/components/IPOCalendar';
import {LocalizedTitle} from '@/components/LocalizedTitle';

// English browser-tab title is the default (crawlers + the English UI); Chinese
// keywords stay in description/keywords. LocalizedTitle swaps in the zh title.
const TITLE_EN = 'US IPO Calendar · Upcoming & Recently Priced IPOs · Tickwind';
const TITLE_ZH = '美股 IPO 日历 · 新股上市 · 潮汐 Tickwind';

export const metadata: Metadata = {
  title: {absolute: TITLE_EN},
  description:
    '美股 IPO 日历 / 新股上市：近期已定价、即将上市与新近申报的美国 IPO，含交易所、发行价、股数与募资金额，逐条链接个股页。Track upcoming and recently-priced US IPOs from the Nasdaq calendar. 数据延迟，仅供参考，不构成投资建议。',
  keywords: [
    '美股 IPO 日历',
    '新股上市',
    '美股打新',
    '即将上市',
    'IPO calendar',
    'upcoming IPOs',
    'recently priced IPOs',
    'Nasdaq IPO',
  ],
  alternates: langAlternates('/calendar/ipo'),
  openGraph: {
    images: [
      ogImageMeta({
        eyebrow: '美股 IPO 日历',
        title: '新股上市 · 即将上市 · 近期定价',
        subtitle: '来自 Nasdaq 的美股 IPO 日历',
      }),
    ],
  },
};

/** Public US IPO calendar (IPO tab of the unified calendar; Nasdaq, delayed/display-only). */
export default function IPOCalendarPage() {
  return (
    <>
      <LocalizedTitle en={TITLE_EN} zh={TITLE_ZH} />
      <IPOCalendar />
    </>
  );
}
