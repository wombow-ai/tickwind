/**
 * Curated SIGNAL-screener "landing page" presets — high-intent technical-signal
 * queries ("golden cross stocks", "oversold stocks") surfaced at
 * `/screen/signals/{key}` as SEO landing pages that pre-apply a filter on the
 * deterministic signals screener (`/v1/screen/signals`). Each preset is a fixed
 * {direction, signal} bundle + bilingual copy. The signal ids map 1:1 to the Go
 * signals layer (technical.ma-cross / technical.rsi / …). EN-first per the owner
 * principle; not investment advice.
 */

/** One curated signal-screener preset: a URL slug + bilingual copy + a fixed filter. */
export interface SignalScreenPreset {
  /** URL slug under `/screen/signals/{key}` — stable, kebab-case. */
  key: string;
  titleEn: string;
  titleZh: string;
  /** One-line intro / meta description (per locale). */
  descEn: string;
  descZh: string;
  /** Pre-applied screener filters (empty = any). */
  direction?: string;
  signal?: string;
}

export const SIGNAL_SCREEN_PRESETS: readonly SignalScreenPreset[] = [
  {
    key: 'golden-cross',
    titleEn: 'Golden Cross Stocks Today',
    titleZh: '今日金叉股票',
    descEn: 'US stocks whose 50-day moving average just crossed above their 200-day — the classic golden cross, computed deterministically from public daily price data. Delayed data, not investment advice.',
    descZh: '50日均线刚上穿200日均线的美股 —— 经典金叉,基于公开日线数据确定性计算。数据延迟,不构成投资建议。',
    signal: 'technical.ma-cross',
    direction: 'bullish',
  },
  {
    key: 'death-cross',
    titleEn: 'Death Cross Stocks Today',
    titleZh: '今日死叉股票',
    descEn: 'US stocks whose 50-day moving average just crossed below their 200-day — the death cross, computed deterministically from public daily price data. Delayed data, not investment advice.',
    descZh: '50日均线刚下穿200日均线的美股 —— 死叉,基于公开日线数据确定性计算。数据延迟,不构成投资建议。',
    signal: 'technical.ma-cross',
    direction: 'bearish',
  },
  {
    key: 'rsi-oversold',
    titleEn: 'Oversold US Stocks (RSI below 30)',
    titleZh: '超卖美股 (RSI 低于 30)',
    descEn: 'US stocks with RSI below 30 — a disclosed oversold condition computed from public daily price data. Delayed data, not investment advice.',
    descZh: 'RSI 低于 30 的美股 —— 基于公开日线数据计算的超卖状态。数据延迟,不构成投资建议。',
    signal: 'technical.rsi',
    direction: 'bullish',
  },
  {
    key: 'rsi-overbought',
    titleEn: 'Overbought US Stocks (RSI above 70)',
    titleZh: '超买美股 (RSI 高于 70)',
    descEn: 'US stocks with RSI above 70 — a disclosed overbought condition computed from public daily price data. Delayed data, not investment advice.',
    descZh: 'RSI 高于 70 的美股 —— 基于公开日线数据计算的超买状态。数据延迟,不构成投资建议。',
    signal: 'technical.rsi',
    direction: 'bearish',
  },
  {
    key: 'bullish-signals',
    titleEn: 'US Stocks With Bullish Technical Signals',
    titleZh: '出现看多技术信号的美股',
    descEn: 'US stocks showing any bullish technical signal right now — golden cross, RSI oversold, MACD bullish cross and more, each a deterministic rule over public daily data. Not investment advice.',
    descZh: '当前出现任意看多技术信号的美股 —— 金叉、RSI 超卖、MACD 金叉等,每条都是对公开日线数据的确定性规则。不构成投资建议。',
    direction: 'bullish',
  },
  {
    key: 'bearish-signals',
    titleEn: 'US Stocks With Bearish Technical Signals',
    titleZh: '出现看空技术信号的美股',
    descEn: 'US stocks showing any bearish technical signal right now — death cross, RSI overbought, MACD bearish cross and more, each a deterministic rule over public daily data. Not investment advice.',
    descZh: '当前出现任意看空技术信号的美股 —— 死叉、RSI 超买、MACD 死叉等,每条都是对公开日线数据的确定性规则。不构成投资建议。',
    direction: 'bearish',
  },
];

export function signalPresetByKey(key: string): SignalScreenPreset | undefined {
  return SIGNAL_SCREEN_PRESETS.find(p => p.key === key);
}
