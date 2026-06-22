/**
 * The dividend leaderboards surfaced at `/screen/dividends/{view}` (pSEO). Each ranks Tickwind's
 * tracked universe of dividend payers by one Go-computed metric — the market-wide view of the
 * per-stock dividend card (`GET /v1/screen/dividends`). Every figure is computed from SEC-filed annual
 * figures (+ the live price for yield) and is DESCRIPTIVE (yield/payout/coverage/growth) — there is
 * deliberately NO blended "dividend-safety grade", and the copy ranks BY a metric, it never calls a
 * stock "best". A view key maps 1:1 to the backend's `?view=` value (already a URL-safe slug).
 */

/** The dividend metric a view ranks by (the column the leaderboard emphasizes). */
export type DividendMetric = 'yield' | 'payout' | 'growth' | 'coverage';

/** One dividend leaderboard: a URL slug (= the backend view), its ranking metric, + bilingual copy. */
export interface DividendViewPreset {
  key: 'highest-yield' | 'fastest-growing' | 'best-covered' | 'lowest-payout';
  primary: DividendMetric;
  titleZh: string;
  titleEn: string;
  descZh: string;
  descEn: string;
}

/** The view catalog. Order = display order on the hub + cross-link rail. */
export const DIVIDEND_VIEWS: readonly DividendViewPreset[] = [
  {
    key: 'highest-yield',
    primary: 'yield',
    titleZh: '美股股息率最高榜',
    titleEn: 'Highest Dividend Yield US Stocks',
    descZh: '按最近一期股息率(年度分红 ÷ 市值)排名的美股分红股。每只附派息率以供参考。描述性历史/在档统计,非投资建议。',
    descEn: 'US dividend payers ranked by their trailing dividend yield (annual dividends ÷ market cap), with the payout ratio for context. A descriptive as-filed statistic, not investment advice.',
  },
  {
    key: 'fastest-growing',
    primary: 'growth',
    titleZh: '美股分红增长最快榜',
    titleEn: 'Fastest-Growing US Dividends',
    descZh: '按分红同比增长(本年度 vs 上一年度 SEC 申报的分红)排名的美股。描述性在档统计,非预测、非投资建议。',
    descEn: 'US stocks ranked by their year-over-year dividend growth (this fiscal year vs last, from SEC filings). A descriptive as-filed statistic, not a forecast or investment advice.',
  },
  {
    key: 'best-covered',
    primary: 'coverage',
    titleZh: '美股分红现金流覆盖最佳榜',
    titleEn: 'US Dividends Best Covered by Free Cash Flow',
    descZh: '按自由现金流对分红的覆盖倍数(FCF ÷ 分红)排名 —— 覆盖倍数越高,分红被自由现金流覆盖得越充分。描述性在档统计,非投资建议。',
    descEn: 'US dividend payers ranked by how many times free cash flow covers the payout (FCF ÷ dividends) — a higher multiple means the dividend is more fully covered by free cash flow. A descriptive statistic, not investment advice.',
  },
  {
    key: 'lowest-payout',
    primary: 'payout',
    titleZh: '美股派息率最低榜',
    titleEn: 'US Stocks with the Lowest Dividend Payout Ratio',
    descZh: '按派息率(分红 ÷ 净利润)由低到高排名 —— 派出盈利占比最小的分红股(仅含正派息率)。描述性在档统计,非投资建议。',
    descEn: 'US dividend payers ranked by the lowest dividend payout ratio (dividends ÷ net income) — those paying out the smallest share of earnings (positive ratios only). A descriptive statistic, not investment advice.',
  },
] as const;

/** Resolves a dividend view by its URL slug, or `undefined` for an unknown slug. */
export function dividendViewByKey(key: string): DividendViewPreset | undefined {
  return DIVIDEND_VIEWS.find(v => v.key === key);
}
