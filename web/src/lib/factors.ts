/**
 * The four FACTOR LEADERBOARDS surfaced at `/screen/factors/{factor}` (pSEO). Each ranks Tickwind's
 * tracked universe by ONE factor's percentile — the market-wide view of the per-stock multi-factor
 * scorecard (`GET /v1/screen/factors`). Every percentile is Go-computed from the public quote +
 * SEC-XBRL fundamentals and is DESCRIPTIVE — there is no blended composite score and no
 * rating/recommendation (the no-advice line). A factor key maps 1:1 to the backend's
 * `indicators.ValidFactor` set.
 */

/** One factor leaderboard: a URL slug (= the backend factor key) + bilingual copy. */
export interface FactorPreset {
  /** URL slug under `/screen/factors/{key}` — equals the backend `?factor=` value. */
  key: 'value' | 'growth' | 'quality' | 'momentum';
  titleZh: string;
  titleEn: string;
  /** One-line intro / meta description (per locale). */
  descZh: string;
  descEn: string;
  /** Which Go-computed sub-metrics feed this factor's percentile (shown as a "based on" note). */
  basisZh: string;
  basisEn: string;
}

/** The factor catalog. Order = display order on the hub + cross-link rail. */
export const FACTOR_PRESETS: readonly FactorPreset[] = [
  {
    key: 'value',
    titleZh: '美股价值因子排行榜(最便宜)',
    titleEn: 'US Stocks Ranked by Value (Cheapest)',
    descZh: '按价值因子百分位排序的美股榜单 —— 估值越低(P/E、P/B、P/S 越便宜)百分位越高。相对全市场的描述性百分位,非评级、非投资建议。',
    descEn: 'US stocks ranked by their value-factor percentile vs the tracked universe — the cheaper the valuation (lower P/E, P/B, P/S), the higher the percentile. A descriptive percentile, not a rating or investment advice.',
    basisZh: '基于市盈率、市净率、市销率(越低越便宜)',
    basisEn: 'Based on P/E, P/B and P/S (lower = cheaper)',
  },
  {
    key: 'growth',
    titleZh: '美股成长因子排行榜(增长最快)',
    titleEn: 'US Stocks Ranked by Growth (Fastest-Growing)',
    descZh: '按成长因子百分位排序的美股榜单 —— 营收与盈利同比增速越高百分位越高。相对全市场的描述性百分位,非评级、非投资建议。',
    descEn: 'US stocks ranked by their growth-factor percentile vs the tracked universe — faster year-over-year revenue and earnings growth means a higher percentile. A descriptive percentile, not a rating or investment advice.',
    basisZh: '基于营收同比、盈利同比增速',
    basisEn: 'Based on revenue and earnings YoY growth',
  },
  {
    key: 'quality',
    titleZh: '美股质量因子排行榜',
    titleEn: 'US Stocks Ranked by Quality',
    descZh: '按质量因子百分位排序的美股榜单 —— 盈利能力与财务稳健度越高百分位越高。相对全市场的描述性百分位,非评级、非投资建议。',
    descEn: 'US stocks ranked by their quality-factor percentile vs the tracked universe — stronger profitability and balance-sheet health means a higher percentile. A descriptive percentile, not a rating or investment advice.',
    basisZh: '基于 ROE、ROIC、EBIT 利润率、Piotroski F 评分',
    basisEn: 'Based on ROE, ROIC, EBIT margin and the Piotroski F-score',
  },
  {
    key: 'momentum',
    titleZh: '美股动量因子排行榜(最强)',
    titleEn: 'US Stocks Ranked by Momentum (Strongest)',
    descZh: '按动量因子百分位排序的美股榜单 —— 近一年总回报越高百分位越高。相对全市场的描述性百分位,非评级、非投资建议。',
    descEn: 'US stocks ranked by their momentum-factor percentile vs the tracked universe — a stronger trailing one-year total return means a higher percentile. A descriptive percentile, not a rating or investment advice.',
    basisZh: '基于近一年总股东回报(TSR)',
    basisEn: 'Based on the trailing 1-year total shareholder return (TSR)',
  },
] as const;

/** Resolves a factor preset by its URL slug, or `undefined` for an unknown slug. */
export function factorByKey(key: string): FactorPreset | undefined {
  return FACTOR_PRESETS.find(p => p.key === key);
}
