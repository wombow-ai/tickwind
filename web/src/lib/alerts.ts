import {type Alert} from '@/lib/api';

// Shared alert presentation — used by both the per-stock AlertsPanel and the global
// AlertsCenter so the two surfaces can't drift (they previously rendered a pct_move
// alert differently: one with the ± prefix, one without).

/** Price/event alert kinds (the create picker on the stock page). new_filing + earnings_soon are
 * thresholdless events; the others take a price/percent threshold. */
export const PRICE_KINDS = ['price_above', 'price_below', 'pct_move', 'new_filing', 'earnings_soon'] as const;

/** Deterministic signal-condition alert kinds (self-describing — no threshold needed). */
export const SIGNAL_KINDS = [
  'golden_cross',
  'death_cross',
  'rsi_oversold',
  'rsi_overbought',
  'signal_bullish',
  'signal_bearish',
] as const;

/** Thresholdless kinds: new_filing, earnings_soon + every signal kind ignore the threshold field. */
export function isThresholdless(kind: string): boolean {
  return kind === 'new_filing' || kind === 'earnings_soon' || (SIGNAL_KINDS as readonly string[]).includes(kind);
}

/** Maps an alert kind to its i18n label key. */
export function kindLabelKey(kind: string): string {
  switch (kind) {
    case 'price_above':
      return 'alerts.priceAbove';
    case 'price_below':
      return 'alerts.priceBelow';
    case 'pct_move':
      return 'alerts.pctMove';
    case 'golden_cross':
      return 'alerts.goldenCross';
    case 'death_cross':
      return 'alerts.deathCross';
    case 'rsi_oversold':
      return 'alerts.rsiOversold';
    case 'rsi_overbought':
      return 'alerts.rsiOverbought';
    case 'signal_bullish':
      return 'alerts.signalBullish';
    case 'signal_bearish':
      return 'alerts.signalBearish';
    case 'earnings_soon':
      return 'alerts.earningsSoon';
    default:
      return 'alerts.newFiling';
  }
}

/**
 * Human label for an alert. pct_move fires on the ABSOLUTE day move in EITHER
 * direction (the evaluator takes |move|), so it renders with a ± prefix; price
 * thresholds render with a $; thresholdless kinds render the bare label.
 */
export function describeAlert(a: Alert, tr: (k: string) => string): string {
  const k = tr(kindLabelKey(a.kind));
  if (isThresholdless(a.kind)) return k;
  if (a.kind === 'pct_move') return `${k} ±${a.threshold}%`;
  return `${k} $${a.threshold}`;
}
