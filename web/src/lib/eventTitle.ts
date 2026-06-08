// Chinese labels for recurring, schedulable market events. The Go side emits
// English titles plus a stable `subtype` enum (internal/events); we translate by
// subtype so an upstream wording tweak never breaks the mapping. Unknown
// subtypes — or English — fall back to the original title, so an event is never
// blanked. Note: `earnings` here is the BLS "Real Earnings" release, NOT
// corporate earnings (the timeline carries macro events, not company reports).
const ZH: Record<string, string> = {
  fomc: '美联储利率决议',
  cpi: '美国CPI（消费者物价指数）',
  nfp: '美国非农就业报告',
  ppi: '美国PPI（生产者物价指数）',
  gdp: '美国GDP',
  jobs: '美国JOLTS职位空缺',
  eci: '美国雇佣成本指数',
  earnings: '美国实际收入',
  election: '美国中期选举',
  worldcup: '世界杯',
};

/**
 * Localised event title. For zh it returns the curated Chinese label for a known
 * subtype; otherwise (unknown subtype, or lang !== 'zh') it returns the original
 * English title unchanged.
 */
export function eventTitle(subtype: string, fallback: string, lang: string): string {
  if (lang === 'zh') {
    const zh = ZH[subtype];
    if (zh) return zh;
  }
  return fallback;
}
