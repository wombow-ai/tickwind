/**
 * Internal-link helpers for the per-stock "Related" footer (SEO supply): given a
 * ticker, surface the theme {@link ZONES} it belongs to, its sector/theme peers,
 * and meaningful `/compare` pairs. This is what makes the ~6,000 /stock pages
 * cross-link the (built-but-dormant) /compare engine + /zone hubs so Google can
 * actually discover them.
 *
 * ANTI-HALLUCINATION: pure editorial structure — only real, currently US-listed
 * common-stock symbols, ZERO market numbers (every figure on the linked pages is
 * fetched live/Go-computed). Nothing here is advice.
 */

import {ZONES} from './zones';

/** A zone a ticker belongs to (for a `/zone/{key}` link with its title). */
export interface ZoneRef {
  key: string;
  titleEn: string;
  titleZh: string;
}

/**
 * Curated sector/theme peer sets (richer than the compare pairs — 3-6 rivals each).
 * Keys + values are high-confidence, currently US-listed common stocks. Symmetry is
 * NOT required; {@link peersFor} fills gaps from zone co-membership. Order matters
 * (most-relevant peer first) since callers slice the head.
 */
const PEER_MAP: Record<string, string[]> = {
  // Big tech / internet
  AAPL: ['MSFT', 'GOOGL', 'AMZN', 'META'],
  MSFT: ['AAPL', 'GOOGL', 'ORCL', 'CRM'],
  GOOGL: ['META', 'MSFT', 'AMZN', 'AAPL'],
  META: ['GOOGL', 'NFLX', 'SNAP', 'PINS'],
  AMZN: ['GOOGL', 'MSFT', 'SHOP', 'WMT'],
  NFLX: ['DIS', 'WBD', 'META', 'CMCSA'],
  DIS: ['NFLX', 'CMCSA', 'WBD', 'PARA'],
  ORCL: ['MSFT', 'CRM', 'IBM'],
  ADBE: ['CRM', 'MSFT', 'NOW'],
  UBER: ['LYFT', 'DASH', 'ABNB'],
  SHOP: ['AMZN', 'ETSY', 'MELI'],
  // Semiconductors / AI hardware
  NVDA: ['AMD', 'AVGO', 'INTC', 'TSM'],
  AMD: ['NVDA', 'INTC', 'AVGO', 'QCOM'],
  INTC: ['AMD', 'NVDA', 'TSM', 'QCOM'],
  AVGO: ['QCOM', 'AMD', 'MRVL', 'TXN'],
  QCOM: ['AVGO', 'ARM', 'NVDA', 'AMD'],
  ARM: ['QCOM', 'NVDA', 'AVGO'],
  TSM: ['INTC', 'NVDA', 'ASML', 'AMAT'],
  MRVL: ['AVGO', 'QCOM', 'NVDA'],
  MU: ['WDC', 'STX', 'AVGO'],
  AMAT: ['LRCX', 'KLAC', 'ASML'],
  LRCX: ['AMAT', 'KLAC', 'ASML'],
  KLAC: ['AMAT', 'LRCX', 'ASML'],
  ASML: ['AMAT', 'LRCX', 'TSM'],
  ANET: ['CSCO', 'JNPR', 'NVDA'],
  CSCO: ['ANET', 'JNPR', 'HPE'],
  DELL: ['HPE', 'SMCI', 'HPQ'],
  HPE: ['DELL', 'SMCI', 'CSCO'],
  SMCI: ['DELL', 'HPE', 'NVDA'],
  // Software / cloud / security
  CRM: ['NOW', 'ADBE', 'ORCL', 'MSFT'],
  NOW: ['CRM', 'WDAY', 'SNOW'],
  SNOW: ['DDOG', 'PLTR', 'MDB', 'NET'],
  DDOG: ['SNOW', 'NET', 'MDB'],
  PLTR: ['SNOW', 'NOW', 'CRWD'],
  CRWD: ['PANW', 'ZS', 'S', 'NET'],
  PANW: ['CRWD', 'ZS', 'FTNT'],
  ZS: ['CRWD', 'PANW', 'NET'],
  NET: ['DDOG', 'CRWD', 'FSLY'],
  // Payments / financials
  V: ['MA', 'AXP', 'PYPL'],
  MA: ['V', 'AXP', 'PYPL'],
  AXP: ['V', 'MA', 'COF'],
  PYPL: ['V', 'MA', 'COF'],
  JPM: ['BAC', 'WFC', 'C', 'GS'],
  BAC: ['JPM', 'WFC', 'C'],
  WFC: ['JPM', 'BAC', 'C'],
  C: ['JPM', 'BAC', 'WFC'],
  GS: ['MS', 'JPM', 'C'],
  MS: ['GS', 'JPM', 'SCHW'],
  SCHW: ['MS', 'GS', 'JPM'],
  // Consumer / retail
  KO: ['PEP', 'MNST', 'KDP'],
  PEP: ['KO', 'MNST', 'KDP'],
  MCD: ['SBUX', 'CMG', 'YUM'],
  SBUX: ['MCD', 'CMG', 'YUM'],
  NKE: ['LULU', 'SKX', 'UAA'],
  LULU: ['NKE', 'SKX'],
  WMT: ['TGT', 'COST', 'AMZN'],
  TGT: ['WMT', 'COST', 'DG'],
  COST: ['WMT', 'BJ', 'TGT'],
  HD: ['LOW', 'WMT'],
  LOW: ['HD', 'WMT'],
  PG: ['CL', 'KMB', 'KO'],
  CL: ['PG', 'KMB'],
  // Healthcare / pharma / med-tech
  LLY: ['NVO', 'PFE', 'MRK', 'ABBV'],
  NVO: ['LLY', 'PFE', 'MRK'],
  PFE: ['MRK', 'ABBV', 'JNJ', 'BMY'],
  MRK: ['PFE', 'ABBV', 'JNJ', 'BMY'],
  ABBV: ['JNJ', 'PFE', 'MRK', 'BMY'],
  JNJ: ['PFE', 'MRK', 'ABBV'],
  BMY: ['PFE', 'MRK', 'ABBV'],
  UNH: ['CVS', 'CI', 'ELV', 'HUM'],
  CVS: ['UNH', 'CI', 'WBA'],
  ISRG: ['SYK', 'MDT', 'BSX'],
  SYK: ['ISRG', 'MDT', 'BSX', 'ZBH'],
  MDT: ['SYK', 'BSX', 'ABT'],
  BSX: ['MDT', 'SYK', 'ABT'],
  AMGN: ['GILD', 'BIIB', 'REGN', 'VRTX'],
  GILD: ['AMGN', 'BIIB', 'MRK'],
  VRTX: ['REGN', 'AMGN', 'GILD'],
  REGN: ['VRTX', 'AMGN', 'GILD'],
  // Energy
  XOM: ['CVX', 'COP', 'SHEL'],
  CVX: ['XOM', 'COP', 'OXY'],
  COP: ['OXY', 'EOG', 'XOM'],
  OXY: ['COP', 'CVX', 'EOG'],
  EOG: ['COP', 'OXY', 'XOM'],
  // Autos / EV
  TSLA: ['RIVN', 'LCID', 'F', 'GM'],
  RIVN: ['LCID', 'TSLA', 'F'],
  LCID: ['RIVN', 'TSLA'],
  F: ['GM', 'TSLA', 'STLA'],
  GM: ['F', 'TSLA', 'STLA'],
  // Media / telecom
  CMCSA: ['DIS', 'NFLX', 'WBD', 'T'],
  WBD: ['NFLX', 'DIS', 'PARA'],
  T: ['VZ', 'TMUS', 'CMCSA'],
  VZ: ['T', 'TMUS'],
  TMUS: ['VZ', 'T'],
  // Industrials / airlines / defense
  CAT: ['DE', 'CMI', 'HON'],
  DE: ['CAT', 'CNH', 'AGCO'],
  BA: ['RTX', 'LMT', 'GE'],
  RTX: ['BA', 'LMT', 'NOC', 'GD'],
  LMT: ['RTX', 'NOC', 'GD', 'LHX'],
  NOC: ['LMT', 'RTX', 'GD'],
  DAL: ['UAL', 'AAL', 'LUV'],
  UAL: ['DAL', 'AAL', 'LUV'],
  AAL: ['DAL', 'UAL', 'LUV'],
};

/** Zone co-members of `ticker` (same layer first, then other layers), de-duped, self excluded. */
function zonePeers(ticker: string): string[] {
  const T = ticker.toUpperCase();
  const out: string[] = [];
  const seen = new Set<string>([T]);
  for (const z of ZONES) {
    for (const layer of z.layers) {
      if (!layer.tickers.some(t => t.ticker.toUpperCase() === T)) continue;
      for (const t of layer.tickers) {
        const p = t.ticker.toUpperCase();
        if (!seen.has(p)) {
          seen.add(p);
          out.push(p);
        }
      }
    }
  }
  return out;
}

/**
 * The zones a ticker appears in (for `/zone/{key}` links). A ticker can sit in
 * several zones (e.g. NVDA → AI + Quantum) — all are returned, in catalog order.
 */
export function tickerToZones(ticker: string): ZoneRef[] {
  const T = ticker.toUpperCase();
  const out: ZoneRef[] = [];
  for (const z of ZONES) {
    if (z.layers.some(l => l.tickers.some(t => t.ticker.toUpperCase() === T))) {
      out.push({key: z.key, titleEn: z.titleEn, titleZh: z.titleZh});
    }
  }
  return out;
}

/**
 * Up to `max` peer tickers for a stock: curated {@link PEER_MAP} first, topped up
 * with zone co-members. Returns `[]` for an unknown/obscure ticker (the footer
 * then renders nothing rather than a thin/empty section).
 */
export function peersFor(ticker: string, max = 6): string[] {
  const T = ticker.toUpperCase();
  const out: string[] = [];
  const seen = new Set<string>([T]);
  const push = (p: string) => {
    const u = p.toUpperCase();
    if (!seen.has(u)) {
      seen.add(u);
      out.push(u);
    }
  };
  for (const p of PEER_MAP[T] ?? []) push(p);
  for (const p of zonePeers(T)) {
    if (out.length >= max) break;
    push(p);
  }
  return out.slice(0, max);
}
