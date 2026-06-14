/**
 * Curated screener "landing page" presets — high-intent, mega-cap-free movement /
 * price queries that the `/v1/screen` endpoint can answer correctly within its
 * 200-row, server-side cap. Each preset is a fixed {@link ScreenParams} bundle
 * surfaced at `/screen/{key}` as a ranked, internally-linked pSEO page.
 *
 * Every `params` field maps 1:1 to a backend-supported knob (verified against the
 * Go `getScreen` handler in `internal/api/api.go`):
 *   - `sort`    ∈ {change_desc (default), change_asc, price_desc, price_asc}
 *               — NO volume sort exists, so there is no "most active" preset.
 *   - `session` ∈ {pre, regular, post, overnight, closed}
 *               — matched (EqualFold) against the stock's CURRENT session, so a
 *                 session-scoped preset only returns rows while that session is
 *                 live; off-hours it renders the empty state (ISR refills later).
 *   - `minPrice`/`maxPrice`/`minChange`/`maxChange` — numeric filters.
 *
 * The price universe deliberately EXCLUDES S&P mega-caps (a known Alpaca-snapshot
 * quirk), so every preset is movement/price-based where a mega-cap-free, ≤200-row
 * list is still meaningful — no "blue-chip"/"mega-cap" preset (the data can't
 * back it).
 */

import type {ScreenParams} from '@/lib/api';

/** One curated screener preset: a URL slug + bilingual copy + fixed filters. */
export interface ScreenPreset {
  /** URL slug under `/screen/{key}` — stable, kebab-case, used in routing + SEO. */
  key: string;
  titleZh: string;
  titleEn: string;
  /** One-line intro / meta description (per locale). */
  descZh: string;
  descEn: string;
  /** Fixed screener filters — every field is backend-supported (see file doc). */
  params: ScreenParams;
}

/**
 * The preset catalog (~9). Order = display order on the hub. A larger result cap
 * (100) is requested per page than the interactive screener's default so the
 * ranked landing list is meatier (still under the server's 200 hard cap).
 */
export const SCREEN_PRESETS: readonly ScreenPreset[] = [
  {
    key: 'top-gainers',
    titleZh: '美股今日涨幅榜',
    titleEn: 'Top Gaining US Stocks Today',
    descZh: '按当日涨跌幅排序的美股涨幅榜 —— 实时筛选全市场涨势最强的个股。行情延迟,仅供参考,非投资建议。',
    descEn: "Today's biggest US stock gainers, ranked by daily % change across the whole market. Delayed quotes. Not investment advice.",
    params: {sort: 'change_desc', limit: 100},
  },
  {
    key: 'top-losers',
    titleZh: '美股今日跌幅榜',
    titleEn: 'Top Losing US Stocks Today',
    descZh: '按当日跌幅排序的美股跌幅榜 —— 实时筛选全市场跌势最深的个股。行情延迟,仅供参考,非投资建议。',
    descEn: "Today's biggest US stock losers, ranked by daily % decline across the whole market. Delayed quotes. Not investment advice.",
    params: {sort: 'change_asc', limit: 100},
  },
  {
    key: 'penny-movers',
    titleZh: '美股低价股涨幅榜(5 美元以下)',
    titleEn: 'Penny Stock Gainers Under $5',
    descZh: '5 美元以下低价美股的今日涨幅榜 —— 实时筛选低价股中涨势最强的个股。行情延迟,仅供参考,非投资建议。',
    descEn: "Today's top-gaining US penny stocks priced under $5, ranked by daily % change. Delayed quotes. Not investment advice.",
    params: {maxPrice: 5, sort: 'change_desc', limit: 100},
  },
  {
    key: 'penny-losers',
    titleZh: '美股低价股跌幅榜(5 美元以下)',
    titleEn: 'Penny Stock Losers Under $5',
    descZh: '5 美元以下低价美股的今日跌幅榜 —— 实时筛选低价股中跌势最深的个股。行情延迟,仅供参考,非投资建议。',
    descEn: "Today's biggest-falling US penny stocks priced under $5, ranked by daily % decline. Delayed quotes. Not investment advice.",
    params: {maxPrice: 5, sort: 'change_asc', limit: 100},
  },
  {
    key: 'small-cap-breakouts',
    titleZh: '美股小盘异动榜(20 美元以下涨超 10%)',
    titleEn: 'Small-Cap Breakouts (Under $20, Up 10%+)',
    descZh: '20 美元以下、当日涨幅超过 10% 的美股异动榜 —— 实时筛选低价大涨的小盘股。行情延迟,仅供参考,非投资建议。',
    descEn: 'US stocks priced under $20 that are up more than 10% today — low-priced small-cap breakouts, ranked by daily % change. Delayed quotes. Not investment advice.',
    params: {maxPrice: 20, minChange: 10, sort: 'change_desc', limit: 100},
  },
  {
    key: 'big-decliners',
    titleZh: '美股大跌榜(单日跌超 10%)',
    titleEn: 'Big Decliners (Down 10%+ Today)',
    descZh: '当日跌幅超过 10% 的美股大跌榜 —— 实时筛选全市场跌势最深的个股。行情延迟,仅供参考,非投资建议。',
    descEn: 'US stocks down more than 10% today, ranked by daily % decline across the whole market. Delayed quotes. Not investment advice.',
    params: {maxChange: -10, sort: 'change_asc', limit: 100},
  },
  {
    key: 'premarket-movers',
    titleZh: '美股盘前异动榜',
    titleEn: 'Premarket Movers',
    descZh: '盘前交易时段涨幅最强的美股 —— 仅在盘前时段(美东 4:00–9:30)有数据,其余时间显示空榜。行情延迟,仅供参考,非投资建议。',
    descEn: 'US stocks gaining the most in the premarket session — populated only during premarket hours (4:00–9:30 ET); empty otherwise. Delayed quotes. Not investment advice.',
    params: {session: 'pre', sort: 'change_desc', limit: 100},
  },
  {
    key: 'afterhours-movers',
    titleZh: '美股盘后异动榜',
    titleEn: 'After-Hours Movers',
    descZh: '盘后交易时段涨幅最强的美股 —— 仅在盘后时段(美东 16:00–20:00)有数据,其余时间显示空榜。行情延迟,仅供参考,非投资建议。',
    descEn: 'US stocks gaining the most in the after-hours session — populated only during after-hours (16:00–20:00 ET); empty otherwise. Delayed quotes. Not investment advice.',
    params: {session: 'post', sort: 'change_desc', limit: 100},
  },
  {
    key: 'overnight-movers',
    titleZh: '美股夜盘异动榜',
    titleEn: 'Overnight Movers',
    descZh: '夜盘交易时段涨幅最强的美股 —— 仅在夜盘时段有数据,其余时间显示空榜。行情延迟,仅供参考,非投资建议。',
    descEn: 'US stocks gaining the most in the overnight session — populated only during overnight trading; empty otherwise. Delayed quotes. Not investment advice.',
    params: {session: 'overnight', sort: 'change_desc', limit: 100},
  },
] as const;

/** Resolves a preset by its URL slug, or `undefined` for an unknown slug. */
export function presetByKey(key: string): ScreenPreset | undefined {
  return SCREEN_PRESETS.find(p => p.key === key);
}
