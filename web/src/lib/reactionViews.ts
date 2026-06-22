/**
 * The earnings-reaction leaderboards surfaced at `/screen/earnings-reaction/{view}` (pSEO). Each ranks
 * Tickwind's tracked universe by how a stock has historically moved around its earnings — the
 * market-wide view of the per-stock earnings-reaction card (`GET /v1/screen/earnings-reaction`). Every
 * number is Go-computed from public daily candles + SEC 8-K (item 2.02) dates and is a DESCRIPTIVE
 * historical statistic (typical move size / up-rate over PAST earnings) — never a forecast or advice.
 * A view key maps 1:1 to the backend's `?view=` value (already a URL-safe slug).
 */

/** One earnings-reaction leaderboard: a URL slug (= the backend view) + bilingual copy. */
export interface ReactionViewPreset {
  /** URL slug under `/screen/earnings-reaction/{key}` — identical to the backend `?view=` value. */
  key: 'most-volatile' | 'highest-up-rate';
  titleZh: string;
  titleEn: string;
  descZh: string;
  descEn: string;
}

/** The view catalog. Order = display order on the hub + cross-link rail. */
export const REACTION_VIEWS: readonly ReactionViewPreset[] = [
  {
    key: 'most-volatile',
    titleZh: '美股财报波动最大榜',
    titleEn: 'Most Volatile US Stocks on Earnings',
    descZh: '按历次财报前后约 2 个交易日价格波动的典型幅度排名 —— 财报季波动最大的美股。描述性历史统计,非预测、非投资建议。',
    descEn: 'US stocks ranked by the typical size of their ~2-session price move around past earnings — the biggest earnings-season movers. A descriptive historical statistic, not a forecast or investment advice.',
  },
  {
    key: 'highest-up-rate',
    titleZh: '美股财报后最常上涨榜',
    titleEn: 'US Stocks That Most Often Rise After Earnings',
    descZh: '按历次财报前后股价上涨的频率排名 —— 财报后最常上涨的美股。每只附样本数(历次财报次数)。描述性历史统计,非预测、非投资建议。',
    descEn: 'US stocks ranked by how often they rose around their past earnings — who has reacted up most reliably. Each carries its sample count (number of past earnings). A descriptive historical statistic, not a forecast or investment advice.',
  },
] as const;

/** Resolves an earnings-reaction view by its URL slug, or `undefined` for an unknown slug. */
export function reactionViewByKey(key: string): ReactionViewPreset | undefined {
  return REACTION_VIEWS.find(v => v.key === key);
}
