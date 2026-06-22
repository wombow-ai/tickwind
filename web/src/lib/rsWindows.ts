/**
 * The relative-strength leaderboards surfaced at `/screen/relative-strength/{window}` (pSEO). Each
 * ranks Tickwind's tracked universe by trailing EXCESS return vs the S&P 500 (SPY) over one window —
 * the market-wide view of the per-stock relative-strength card (`GET /v1/screen/relative-strength`).
 * Every number is Go-computed from public daily candles and is a DESCRIPTIVE historical statistic —
 * never a forecast or advice. A window key maps 1:1 to the backend's `?window=` value.
 */

/** One RS leaderboard: a URL slug + the backend window value + bilingual copy. */
export interface RSWindowPreset {
  /** URL slug under `/screen/relative-strength/{key}`, lower-case (e.g. "3m"). */
  key: string;
  /** The backend `?window=` value (e.g. "3M"). */
  window: '1M' | '3M' | '6M' | '1Y';
  titleZh: string;
  titleEn: string;
  descZh: string;
  descEn: string;
}

/** The window catalog. Order = display order on the hub + cross-link rail. */
export const RS_WINDOWS: readonly RSWindowPreset[] = [
  {
    key: '1m',
    window: '1M',
    titleZh: '美股相对强弱榜 · 近 1 个月(跑赢大盘)',
    titleEn: 'Strongest US Stocks vs the Market · 1-Month',
    descZh: '按近 1 个月相对标普 500(SPY)的超额收益排名 —— 跑赢大盘最多的美股。描述性历史统计,非投资建议。',
    descEn: 'US stocks ranked by their 1-month excess return vs the S&P 500 (SPY) — who is outpacing the market. A descriptive historical statistic, not investment advice.',
  },
  {
    key: '3m',
    window: '3M',
    titleZh: '美股相对强弱榜 · 近 3 个月(跑赢大盘)',
    titleEn: 'Strongest US Stocks vs the Market · 3-Month',
    descZh: '按近 3 个月相对标普 500(SPY)的超额收益排名 —— 跑赢大盘最多的美股。描述性历史统计,非投资建议。',
    descEn: 'US stocks ranked by their 3-month excess return vs the S&P 500 (SPY) — who is outpacing the market. A descriptive historical statistic, not investment advice.',
  },
  {
    key: '6m',
    window: '6M',
    titleZh: '美股相对强弱榜 · 近 6 个月(跑赢大盘)',
    titleEn: 'Strongest US Stocks vs the Market · 6-Month',
    descZh: '按近 6 个月相对标普 500(SPY)的超额收益排名 —— 跑赢大盘最多的美股。描述性历史统计,非投资建议。',
    descEn: 'US stocks ranked by their 6-month excess return vs the S&P 500 (SPY) — who is outpacing the market. A descriptive historical statistic, not investment advice.',
  },
  {
    key: '1y',
    window: '1Y',
    titleZh: '美股相对强弱榜 · 近 1 年(跑赢大盘)',
    titleEn: 'Strongest US Stocks vs the Market · 1-Year',
    descZh: '按近 1 年相对标普 500(SPY)的超额收益排名 —— 跑赢大盘最多的美股。描述性历史统计,非投资建议。',
    descEn: 'US stocks ranked by their 1-year excess return vs the S&P 500 (SPY) — who is outpacing the market. A descriptive historical statistic, not investment advice.',
  },
] as const;

/** Resolves an RS window by its URL slug, or `undefined` for an unknown slug. */
export function rsWindowByKey(key: string): RSWindowPreset | undefined {
  return RS_WINDOWS.find(w => w.key === key);
}
