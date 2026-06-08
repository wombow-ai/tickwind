import type {Candle} from '@/lib/api';

/** Periods we roll daily candles up into (client-side, no refetch). */
export type Timeframe = 'W' | 'M' | 'Q' | 'Y';

/**
 * Bucket key for a daily bar's date. Same-period bars share a key:
 * - W: the Monday of that ISO week (no year-boundary ambiguity)
 * - M: year+month · Q: year+quarter · Y: year
 */
function bucketKey(iso: string, period: Timeframe): string {
  const d = new Date(iso);
  const y = d.getUTCFullYear();
  const mo = d.getUTCMonth(); // 0-11
  switch (period) {
    case 'Y':
      return `${y}`;
    case 'Q':
      return `${y}-Q${Math.floor(mo / 3)}`;
    case 'M':
      return `${y}-${mo}`;
    case 'W': {
      const dow = d.getUTCDay(); // 0=Sun..6=Sat
      const back = (dow + 6) % 7; // days since Monday
      const monday = new Date(d);
      monday.setUTCDate(d.getUTCDate() - back);
      return `W${monday.toISOString().slice(0, 10)}`;
    }
  }
}

/**
 * Aggregates ascending daily candles into W/M/Q/Y OHLCV bars. Open + time come
 * from the first bar in each bucket, close from the last, high/low are the
 * extremes, volume is summed. Input MUST be sorted ascending by time (the API
 * returns it that way); aggregation preserves that order.
 */
export function aggregate(candles: Candle[], period: Timeframe): Candle[] {
  const buckets = new Map<string, Candle>();
  for (const c of candles) {
    const key = bucketKey(c.time, period);
    const b = buckets.get(key);
    if (!b) {
      buckets.set(key, {
        time: c.time,
        open: c.open,
        high: c.high,
        low: c.low,
        close: c.close,
        volume: c.volume,
      });
    } else {
      if (c.high > b.high) b.high = c.high;
      if (c.low < b.low) b.low = c.low;
      b.close = c.close; // last bar in the bucket
      b.volume += c.volume;
    }
  }
  return Array.from(buckets.values());
}
