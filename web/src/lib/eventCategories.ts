/**
 * The corporate material-events feed surfaced at `/events` (all notable filings) and
 * `/events/{category}` (one 8-K item type) — pSEO over the market-wide `GET /v1/material-feed`. Each
 * category maps 1:1 to a high-signal SEC 8-K item code (the backend's `?item=` filter). Every field on
 * the feed is a Go-owned SEC fact (form, dates, item codes + canonical labels, filing link) — facts
 * only, NO LLM, NO advice. The copy describes the FILING TYPE, never a recommendation.
 */

/** One event category: a URL slug + the 8-K item code it filters to + bilingual copy. */
export interface EventCategory {
  key: string; // URL slug under `/events/{key}`
  item: string; // the backend 8-K item code (?item=)
  titleZh: string;
  titleEn: string;
  descZh: string;
  descEn: string;
}

/** The category catalog. Order = display order on the hub + cross-link rail. */
export const EVENT_CATEGORIES: readonly EventCategory[] = [
  {
    key: 'leadership-changes',
    item: '5.02',
    titleZh: '美股高管变动 — 近期董事/高管 8-K',
    titleEn: 'US Stock Leadership Changes — Recent Officer & Director 8-Ks',
    descZh: '近期 SEC 8-K 第 5.02 项申报 —— 追踪美股的董事/高管离任、任命与薪酬安排。披露性公司申报事实,非投资建议。',
    descEn: 'Recent SEC 8-K item 5.02 filings — director and officer departures, appointments, and compensation changes across tracked US stocks. A disclosed corporate-filing fact, not investment advice.',
  },
  {
    key: 'material-agreements',
    item: '1.01',
    titleZh: '美股重大协议 — 近期 8-K 申报',
    titleEn: 'US Stock Material Agreements — Recent 8-K Filings',
    descZh: '近期 SEC 8-K 第 1.01 项申报 —— 追踪美股签订的重大确定性协议。披露性公司申报事实,非投资建议。',
    descEn: 'Recent SEC 8-K item 1.01 filings — entries into material definitive agreements across tracked US stocks. A disclosed corporate-filing fact, not investment advice.',
  },
  {
    key: 'new-debt',
    item: '2.03',
    titleZh: '美股新增债务 — 近期 8-K 申报',
    titleEn: 'US Stocks Taking On New Debt — Recent 8-K Filings',
    descZh: '近期 SEC 8-K 第 2.03 项申报 —— 追踪美股新增的直接财务义务/重大债务。披露性公司申报事实,非投资建议。',
    descEn: 'Recent SEC 8-K item 2.03 filings — creation of a direct financial obligation (new material debt) across tracked US stocks. A disclosed corporate-filing fact, not investment advice.',
  },
  {
    key: 'bankruptcy',
    item: '1.03',
    titleZh: '美股破产/接管申报 (8-K 1.03)',
    titleEn: 'US Stock Bankruptcy & Receivership Filings (8-K 1.03)',
    descZh: '近期 SEC 8-K 第 1.03 项申报 —— 追踪美股的破产或接管。披露性公司申报事实,非投资建议。',
    descEn: 'Recent SEC 8-K item 1.03 filings — bankruptcy or receivership across tracked US stocks. A disclosed corporate-filing fact, not investment advice.',
  },
  {
    key: 'restatements',
    item: '4.02',
    titleZh: '美股财报重述 — 8-K 4.02 申报',
    titleEn: 'US Stock Financial Restatements — Non-Reliance 8-Ks (4.02)',
    descZh: '近期 SEC 8-K 第 4.02 项申报 —— 追踪美股声明前期财报不可依赖(重述)。披露性公司申报事实,非投资建议。',
    descEn: 'Recent SEC 8-K item 4.02 filings — non-reliance on previously issued financial statements (restatements) across tracked US stocks. A disclosed corporate-filing fact, not investment advice.',
  },
] as const;

/** Resolves an event category by its URL slug, or `undefined` for an unknown slug. */
export function eventCategoryByKey(key: string): EventCategory | undefined {
  return EVENT_CATEGORIES.find(c => c.key === key);
}
