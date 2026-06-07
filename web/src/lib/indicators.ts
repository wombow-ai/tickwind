/**
 * Technical-indicator math, computed client-side from a daily close/volume
 * series (oldest → newest). Each function emits `null` during its warmup so the
 * chart leaves gaps rather than drawing wrong values. Formulas follow the
 * StockCharts / TradingView conventions — SMA-seeded EMA, Wilder-smoothed RSI,
 * population-σ Bollinger — so values match mainstream platforms. Pure functions,
 * no dependencies. Indicators are path-dependent (EMA/RSI), so always compute
 * over the FULL history then slice to the visible window — never recompute from
 * only the zoomed slice.
 */

export type Series = (number | null)[];

/** Simple moving average over the trailing `period` closes (null for the first period-1). */
export function sma(closes: number[], period: number): Series {
  const out: Series = new Array(closes.length).fill(null);
  if (period <= 0) return out;
  let sum = 0;
  for (let i = 0; i < closes.length; i++) {
    sum += closes[i];
    if (i >= period) sum -= closes[i - period];
    if (i >= period - 1) out[i] = sum / period;
  }
  return out;
}

/**
 * Exponential moving average. k = 2/(period+1); the first value is seeded with
 * the SMA of the first `period` closes (the StockCharts/MACD standard — NOT the
 * first price), placed at index period-1.
 */
export function ema(closes: number[], period: number): Series {
  const out: Series = new Array(closes.length).fill(null);
  if (period <= 0 || closes.length < period) return out;
  const k = 2 / (period + 1);
  let seed = 0;
  for (let i = 0; i < period; i++) seed += closes[i];
  seed /= period;
  out[period - 1] = seed;
  let prev = seed;
  for (let i = period; i < closes.length; i++) {
    prev = closes[i] * k + prev * (1 - k);
    out[i] = prev;
  }
  return out;
}

export interface MacdResult {
  macd: Series;
  signal: Series;
  histogram: Series;
}

/**
 * MACD: line = EMA(fast)−EMA(slow); signal = EMA(signalPeriod) of the MACD line
 * (computed over only the defined MACD points, then mapped back); histogram =
 * line − signal.
 */
export function macd(closes: number[], fast = 12, slow = 26, signalPeriod = 9): MacdResult {
  const n = closes.length;
  const emaFast = ema(closes, fast);
  const emaSlow = ema(closes, slow);

  const macdLine: Series = new Array(n).fill(null);
  const definedVals: number[] = [];
  const definedIdx: number[] = [];
  for (let i = 0; i < n; i++) {
    const f = emaFast[i];
    const s = emaSlow[i];
    if (f !== null && s !== null) {
      const m = f - s;
      macdLine[i] = m;
      definedVals.push(m);
      definedIdx.push(i);
    }
  }

  // EMA(signalPeriod) over ONLY the defined MACD values, mapped back to indices.
  const signalCompact = ema(definedVals, signalPeriod);
  const signal: Series = new Array(n).fill(null);
  for (let j = 0; j < definedIdx.length; j++) {
    signal[definedIdx[j]] = signalCompact[j];
  }

  const histogram: Series = new Array(n).fill(null);
  for (let i = 0; i < n; i++) {
    const m = macdLine[i];
    const sg = signal[i];
    if (m !== null && sg !== null) histogram[i] = m - sg;
  }
  return {macd: macdLine, signal, histogram};
}

/**
 * RSI via Wilder's smoothing (NOT a simple SMA and NOT a 2/(N+1) EMA). The first
 * avgGain/avgLoss is a simple average of the first `period` deltas; subsequent
 * values use avg = (prevAvg·(period-1) + current)/period. RSI is path-dependent,
 * so feed it the full history for convergence.
 */
export function rsi(closes: number[], period = 14): Series {
  const n = closes.length;
  const out: Series = new Array(n).fill(null);
  if (n < period + 1) return out;

  let gain = 0;
  let loss = 0;
  for (let i = 1; i <= period; i++) {
    const d = closes[i] - closes[i - 1];
    if (d >= 0) gain += d;
    else loss -= d;
  }
  let avgGain = gain / period;
  let avgLoss = loss / period;
  out[period] = rsiFrom(avgGain, avgLoss);

  for (let i = period + 1; i < n; i++) {
    const d = closes[i] - closes[i - 1];
    const g = d > 0 ? d : 0;
    const l = d < 0 ? -d : 0;
    avgGain = (avgGain * (period - 1) + g) / period;
    avgLoss = (avgLoss * (period - 1) + l) / period;
    out[i] = rsiFrom(avgGain, avgLoss);
  }
  return out;
}

function rsiFrom(avgGain: number, avgLoss: number): number {
  if (avgLoss === 0) return 100; // no losses → fully overbought (avoid /0)
  if (avgGain === 0) return 0;
  const rs = avgGain / avgLoss;
  return 100 - 100 / (1 + rs);
}

export interface BollingerResult {
  middle: Series;
  upper: Series;
  lower: Series;
}

/**
 * Bollinger Bands: middle = SMA(period); upper/lower = middle ± mult·σ, where σ
 * is the POPULATION standard deviation (÷period, not period-1 — Bollinger's
 * stated convention) over the same trailing window.
 */
export function bollinger(closes: number[], period = 20, mult = 2): BollingerResult {
  const n = closes.length;
  const middle: Series = new Array(n).fill(null);
  const upper: Series = new Array(n).fill(null);
  const lower: Series = new Array(n).fill(null);
  for (let i = period - 1; i < n; i++) {
    let sum = 0;
    for (let j = i - period + 1; j <= i; j++) sum += closes[j];
    const m = sum / period;
    let variance = 0;
    for (let j = i - period + 1; j <= i; j++) variance += (closes[j] - m) ** 2;
    variance /= period; // population σ²
    const sd = Math.sqrt(variance);
    middle[i] = m;
    upper[i] = m + mult * sd;
    lower[i] = m - mult * sd;
  }
  return {middle, upper, lower};
}
