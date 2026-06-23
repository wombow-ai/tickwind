/**
 * Typed client for the Tickwind backend JSON API.
 *
 * All requests target {@link API_BASE}, read from `NEXT_PUBLIC_API_BASE`.
 * Public market-data calls are open; per-user calls (watchlist, clips) take a
 * Supabase access token, sent as `Authorization: Bearer <token>`.
 */

/** Base URL of the Tickwind API (no trailing slash). */
export const API_BASE: string = (
  process.env.NEXT_PUBLIC_API_BASE ?? 'http://localhost:8080'
).replace(/\/+$/, '');

/** A tracked instrument, as returned by `GET /v1/stocks/{ticker}`. */
export interface Security {
  ticker: string;
  /** SEC Central Index Key. Omitted by the API for non-US instruments. */
  cik?: string;
  name: string;
  /** Listing market: `US`, `HK`, or `KR`. */
  market: string;
}

/** A regulatory disclosure (e.g. 8-K, 10-Q, Form 4). */
export interface Filing {
  ticker: string;
  /** SEC form type, e.g. `8-K`, `10-Q`, `4`. */
  form: string;
  title: string;
  /** RFC 3339 timestamp of when the filing was filed. */
  filed_at: string;
  accession_no: string;
  /** Canonical SEC URL for the filing. */
  url: string;
}

/** Envelope returned by `GET /v1/stocks/{ticker}/filings`. */
export interface FilingsResponse {
  ticker: string;
  count: number;
  filings: Filing[];
}

/** A company-news article (e.g. from Finnhub). */
export interface NewsItem {
  ticker: string;
  /** Stable upstream identifier for the article. */
  id: string;
  headline: string;
  /** AI-translated Simplified-Chinese headline; absent until translated. */
  headline_zh?: string;
  /** Short blurb; may be empty when the source omits one. */
  summary: string;
  /** Publisher name, e.g. `Reuters`. */
  source: string;
  /** Canonical URL of the article. */
  url: string;
  /** RFC 3339 timestamp of when the article was published. */
  published: string;
}

/** Envelope returned by `GET /v1/stocks/{ticker}/news`. */
export interface NewsResponse {
  ticker: string;
  count: number;
  news: NewsItem[];
}

/** A social-media post about a security (e.g. from StockTwits). */
export interface Post {
  ticker: string;
  /** `<source>:<rawid>`, stable per post. */
  id: string;
  /** Source network, e.g. `stocktwits`. */
  source: string;
  author: string;
  body: string;
  url: string;
  /** RFC 3339 timestamp of when the post was created. */
  created_at: string;
}

/** Envelope returned by `GET /v1/stocks/{ticker}/social`. */
export interface SocialResponse {
  ticker: string;
  count: number;
  posts: Post[];
}

/**
 * A per-ticker numeric "pulse": mention-momentum (`buzz`, e.g. ApeWisdom) or
 * news sentiment (`sentiment`, e.g. Alpha Vantage). Each source fills only the
 * fields for its facet; the rest are absent.
 */
export interface Signal {
  ticker: string;
  /** Provider, e.g. `apewisdom` | `alphavantage`. */
  source: string;
  /** Which facet this signal carries: `buzz` | `sentiment`. */
  kind: string;
  /** Buzz: current mention count. */
  mentions?: number;
  /** Buzz: mentions 24h earlier (for momentum). */
  mentions_prev?: number;
  /** Buzz: popularity rank (1 = most mentioned). */
  rank?: number;
  /** Buzz: rank 24h earlier. */
  rank_prev?: number;
  /** Buzz: total upvotes. */
  upvotes?: number;
  /** Sentiment: average score in [-1, 1]. */
  score?: number;
  /** Sentiment: human label, e.g. `Somewhat-Bullish`. */
  label?: string;
  /** Sentiment: number of articles aggregated. */
  sample_size?: number;
  /** RFC 3339 timestamp of the last update. */
  updated_at: string;
}

/** Envelope returned by `GET /v1/stocks/{ticker}/signals`. */
export interface SignalsResponse {
  ticker: string;
  count: number;
  signals: Signal[];
}

/**
 * One row of the trending leaderboard — the most-discussed US stocks
 * market-wide, ranked by a "heat" score (discussion volume × momentum).
 */
export interface HotStock {
  /** Which board this row belongs to: `hot` | `surging`. */
  board: string;
  ticker: string;
  /** Company / instrument name. */
  name: string;
  /** Rank within the board, 1 = top. */
  rank: number;
  /** Discussion volume — mentions in the last 24h. */
  mentions: number;
  /** Mentions in the same window 24h earlier. */
  mentions_prev: number;
  /** Mention growth vs 24h ago as a fraction (0.2 = +20%). */
  change: number;
  /** Total upvotes on those mentions. */
  upvotes: number;
  /** This board's ranking score (volume×momentum for hot, momentum for surging). */
  score: number;
  updated_at: string;
  /** Live price, joined from the universe cache; absent when unknown. */
  price?: number;
  /** Day change %, null when prev close is unknown or implausible. */
  change_pct?: number | null;
}

/** Envelope returned by `GET /v1/hot`. */
export interface HotResponse {
  board: string;
  count: number;
  stocks: HotStock[];
}

/**
 * Trading session a quote belongs to, in US market terms. Unknown values from
 * the backend are tolerated by widening to `string` at the type boundary.
 */
export type Session =
  | 'pre'
  | 'regular'
  | 'post'
  | 'overnight'
  | 'closed'
  | (string & {});

/** A latest price, as returned by `GET /v1/stocks/{ticker}/quote`. */
export interface Quote {
  ticker: string;
  price: number;
  /** Previous trading day's close, for the day's change. Absent when unknown. */
  prev_close?: number;
  /** Regular-session close (live regular price during hours; the day's close
   *  after). In pre/post sessions, `price` is the extended price shown against
   *  this. Absent when unknown. */
  regular_close?: number;
  /** Trading session the price was observed in. */
  session: Session;
  /** Upstream data provider, e.g. `alpaca`. */
  source: string;
  /** RFC 3339 timestamp of when the price was observed. */
  at: string;
}

/** Health payload returned by `GET /healthz`. */
export interface Health {
  status: string;
  service: string;
}

/** Error envelope returned by the API on non-2xx responses. */
interface ApiErrorBody {
  error?: string;
}

/** Error thrown when an API request fails or returns a non-2xx status. */
export class ApiError extends Error {
  readonly status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

/** Builds request headers, attaching a bearer token when provided. */
function authHeaders(
  base: Record<string, string>,
  token?: string | null,
): Record<string, string> {
  return token ? {...base, Authorization: `Bearer ${token}`} : base;
}

/**
 * Wraps `fetch` to retry EXACTLY ONCE on a transient network failure.
 *
 * Background: a cold (never-warmed) ticker's first on-demand research request
 * intermittently gets reset by the Cloudflare Tunnel hop at ~3s with an empty
 * reply — `fetch()` itself rejects with a `TypeError` (the browser sees a
 * network error, NOT an HTTP status). The fact sheet caches on/after that first
 * attempt, so an immediate retry succeeds. This is an infra-layer behavior we
 * can't fix from the app, so we mitigate it client-side here.
 *
 * Retry policy — deliberately narrow:
 * - Retry ONLY when `fetch()` REJECTS with a transient network error. Once a
 *   `Response` object is returned (ANY HTTP status, incl. 4xx/5xx) it is
 *   returned unchanged — the caller maps it exactly as today (401 → anon,
 *   429 → quota, 404 → notfound, …). A 429 is NEVER retried.
 * - An `AbortError` (the signal aborted, or was already aborted) is a deliberate
 *   cancel, not a transient failure — it is rethrown immediately, no retry.
 * - At most ONE retry (2 total attempts), after a short fixed backoff. If the
 *   signal aborts during the backoff wait, the timer is cleared and the
 *   `AbortError` is rethrown instead of retrying.
 *
 * Restricted to idempotent GETs (it never re-sends a body/method), so a retry
 * can't double-submit a mutation.
 */
async function fetchWithRetryOnce(
  input: string,
  init: RequestInit,
  backoffMs = 500,
): Promise<Response> {
  const signal = init.signal ?? undefined;
  try {
    return await fetch(input, init);
  } catch (e) {
    // A deliberate cancel (already-aborted signal or abort mid-flight) surfaces
    // as an AbortError / an aborted signal — rethrow, do NOT retry.
    if (isAbort(e, signal)) throw e;
    // Otherwise this is a transient network reject (TypeError / connection
    // reset / empty reply). Wait a short backoff, then retry exactly once.
    await abortableDelay(backoffMs, signal);
    return await fetch(input, init);
  }
}

/** True when a rejection represents a deliberate abort rather than a network error. */
function isAbort(e: unknown, signal?: AbortSignal): boolean {
  return (
    (signal?.aborted ?? false) ||
    (e instanceof DOMException && e.name === 'AbortError') ||
    (e instanceof Error && e.name === 'AbortError')
  );
}

/**
 * Resolves after `ms`, or rejects with an `AbortError` as soon as `signal`
 * aborts (clearing the timer). Used for the single retry backoff so a cancel
 * during the wait never triggers the retry.
 */
function abortableDelay(ms: number, signal?: AbortSignal): Promise<void> {
  if (signal?.aborted) {
    return Promise.reject(new DOMException('Aborted', 'AbortError'));
  }
  return new Promise<void>((resolve, reject) => {
    const timer = setTimeout(() => {
      signal?.removeEventListener('abort', onAbort);
      resolve();
    }, ms);
    const onAbort = () => {
      clearTimeout(timer);
      reject(new DOMException('Aborted', 'AbortError'));
    };
    signal?.addEventListener('abort', onAbort, {once: true});
  });
}

/**
 * Performs a typed GET against the API and parses the JSON body.
 *
 * @param path Absolute API path beginning with `/`.
 * @param signal Optional abort signal to cancel the request.
 * @param token Optional Supabase access token for per-user endpoints.
 * @throws {ApiError} If the network call fails or the status is not 2xx.
 */
async function getJson<T>(
  path: string,
  signal?: AbortSignal,
  token?: string | null,
): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`${API_BASE}${path}`, {
      headers: authHeaders({Accept: 'application/json'}, token),
      signal,
    });
  } catch {
    throw new ApiError(`network error contacting ${API_BASE}${path}`, 0);
  }

  if (!res.ok) {
    let detail = res.statusText;
    try {
      const body = (await res.json()) as ApiErrorBody;
      if (body.error) {
        detail = body.error;
      }
    } catch {
      // Non-JSON error body; fall back to the status text.
    }
    throw new ApiError(detail, res.status);
  }

  return (await res.json()) as T;
}

/**
 * Like {@link getJson}, but retries the request EXACTLY ONCE on a transient
 * network failure (see {@link fetchWithRetryOnce}). Behaviorally identical to
 * `getJson` otherwise: any returned HTTP status (401/429/404/5xx) is mapped the
 * same way and is NEVER retried, and an `AbortError` propagates unchanged. Used
 * by the on-demand research GETs, where a cold ticker's first request can be
 * reset at the Cloudflare Tunnel hop and an immediate retry succeeds.
 *
 * @throws {ApiError} If the network call fails (after the single retry) or the
 *   status is not 2xx.
 */
async function getJsonWithRetry<T>(
  path: string,
  signal?: AbortSignal,
  token?: string | null,
  init?: RequestInit,
): Promise<T> {
  let res: Response;
  try {
    res = await fetchWithRetryOnce(`${API_BASE}${path}`, {
      headers: authHeaders({Accept: 'application/json'}, token),
      signal,
      ...(init ?? {}),
    });
  } catch (e) {
    // Re-throw a deliberate cancel so callers can ignore it; only a real
    // transient network failure (both attempts rejected) becomes ApiError(0).
    if (isAbort(e, signal)) throw e;
    throw new ApiError(`network error contacting ${API_BASE}${path}`, 0);
  }

  if (!res.ok) {
    let detail = res.statusText;
    try {
      const body = (await res.json()) as ApiErrorBody;
      if (body.error) {
        detail = body.error;
      }
    } catch {
      // Non-JSON error body; fall back to the status text.
    }
    throw new ApiError(detail, res.status);
  }

  return (await res.json()) as T;
}

/**
 * Performs a typed POST with a JSON body and parses the JSON response.
 *
 * @throws {ApiError} If the network call fails or the status is not 2xx.
 */
async function postJson<T>(
  path: string,
  body: unknown,
  signal?: AbortSignal,
  token?: string | null,
): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`${API_BASE}${path}`, {
      method: 'POST',
      headers: authHeaders(
        {'Content-Type': 'application/json', Accept: 'application/json'},
        token,
      ),
      body: JSON.stringify(body),
      signal,
    });
  } catch {
    throw new ApiError(`network error contacting ${API_BASE}${path}`, 0);
  }

  if (!res.ok) {
    let detail = res.statusText;
    try {
      const data = (await res.json()) as ApiErrorBody;
      if (data.error) {
        detail = data.error;
      }
    } catch {
      // Non-JSON error body; fall back to the status text.
    }
    throw new ApiError(detail, res.status);
  }

  return (await res.json()) as T;
}

/**
 * Performs a typed DELETE and parses the JSON response.
 *
 * @throws {ApiError} If the network call fails or the status is not 2xx.
 */
async function deleteJson<T>(
  path: string,
  signal?: AbortSignal,
  token?: string | null,
): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`${API_BASE}${path}`, {
      method: 'DELETE',
      headers: authHeaders({Accept: 'application/json'}, token),
      signal,
    });
  } catch {
    throw new ApiError(`network error contacting ${API_BASE}${path}`, 0);
  }

  if (!res.ok) {
    let detail = res.statusText;
    try {
      const data = (await res.json()) as ApiErrorBody;
      if (data.error) {
        detail = data.error;
      }
    } catch {
      // Non-JSON error body; fall back to the status text.
    }
    throw new ApiError(detail, res.status);
  }

  return (await res.json()) as T;
}

async function patchJson<T>(
  path: string,
  body: unknown,
  signal?: AbortSignal,
  token?: string | null,
): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`${API_BASE}${path}`, {
      method: 'PATCH',
      headers: authHeaders(
        {'Content-Type': 'application/json', Accept: 'application/json'},
        token,
      ),
      body: JSON.stringify(body),
      signal,
    });
  } catch {
    throw new ApiError(`network error contacting ${API_BASE}${path}`, 0);
  }

  if (!res.ok) {
    let detail = res.statusText;
    try {
      const data = (await res.json()) as ApiErrorBody;
      if (data.error) {
        detail = data.error;
      }
    } catch {
      // Non-JSON error body; fall back to the status text.
    }
    throw new ApiError(detail, res.status);
  }

  return (await res.json()) as T;
}

/** Normalizes a user-supplied ticker into the API's canonical form. */
function normalizeTicker(ticker: string): string {
  return ticker.trim().toUpperCase();
}

/** Fetches a single security by ticker. */
export function getStock(
  ticker: string,
  signal?: AbortSignal,
): Promise<Security> {
  return getJson<Security>(
    `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}`,
    signal,
  );
}

/**
 * Fetches the latest price for a ticker.
 *
 * @throws {ApiError} With status 404 when no quote is available yet (e.g. no
 *   upstream price key is configured); callers should render this gracefully.
 */
export function getQuote(ticker: string, signal?: AbortSignal): Promise<Quote> {
  return getJson<Quote>(
    `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/quote`,
    signal,
  );
}

/**
 * Asks the backend to stream this ticker's price in real time (it joins the live
 * WebSocket subscription, within the free-tier cap). Fire-and-forget — call when
 * a stock detail page opens so its quote stays fresh. Public; failures are ignored.
 */
export function subscribeLive(ticker: string, signal?: AbortSignal): Promise<{ok: boolean}> {
  return postJson<{ok: boolean}>(
    `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/subscribe`,
    {},
    signal,
  );
}

/** Envelope returned by `GET /v1/stocks/{ticker}/bars`. */
export interface BarsResponse {
  ticker: string;
  /** Recent daily closing prices, oldest first. Empty when unavailable. */
  closes: number[];
  /** 52-week high / low (from the daily candle cache); absent when unavailable. */
  year_high?: number;
  year_low?: number;
}

/** Fetches recent daily closes for a ticker's trend sparkline. */
export function getBars(
  ticker: string,
  signal?: AbortSignal,
): Promise<BarsResponse> {
  return getJson<BarsResponse>(
    `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/bars`,
    signal,
  );
}

/** One daily OHLC candle (+ volume) for the K-line chart. */
export interface Candle {
  /** RFC3339 bar date. */
  time: string;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
}

/** Envelope returned by `GET /v1/stocks/{ticker}/candles`. */
export interface CandlesResponse {
  ticker: string;
  candles: Candle[];
}

/**
 * Fetches OHLC candles for the K-line chart. Default = ~5y of daily bars; pass a
 * resolution (5Min/15Min/1Hour) for intraday (1D/5D) views.
 */
export function getCandles(
  ticker: string,
  signal?: AbortSignal,
  resolution?: string,
): Promise<CandlesResponse> {
  const path = `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/candles`;
  return getJson<CandlesResponse>(
    resolution ? `${path}?resolution=${encodeURIComponent(resolution)}` : path,
    signal,
  );
}

/** Reported XBRL figures + price-derived metrics from `GET /v1/stocks/{t}/fundamentals`. */
export interface Fundamentals {
  ticker: string;
  name?: string;
  currency: string;
  shares: number;
  revenue: number;
  net_income: number;
  eps_diluted: number;
  equity: number;
  period: string; // e.g. "FY2025"
  as_of: string;
  price: number;
  market_cap: number | null;
  pe: number | null; // null for loss-makers
  pb: number | null;
}

/** Fetches SEC-XBRL fundamentals (market cap, P/E, revenue, net income). Rejects (404) for non-US / no data. */
export function getFundamentals(
  ticker: string,
  signal?: AbortSignal,
): Promise<Fundamentals> {
  return getJson<Fundamentals>(
    `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/fundamentals`,
    signal,
  );
}

/** Envelope returned by `GET /v1/bars?tickers=...`. */
export interface BarsBatchResponse {
  /** Map from ticker to its recent daily closes (oldest first). */
  bars: Record<string, number[]>;
}

/** Fetches daily-close series for several tickers in one batched request. */
export function getBarsBatch(
  tickers: readonly string[],
  signal?: AbortSignal,
): Promise<BarsBatchResponse> {
  const q = tickers.map(normalizeTicker).join(',');
  return getJson<BarsBatchResponse>(
    `/v1/bars?tickers=${encodeURIComponent(q)}`,
    signal,
  );
}

/** Envelope returned by `GET /v1/news?tickers=...` (merged home feed). */
export interface NewsFeedResponse {
  count: number;
  news: NewsItem[];
}

/** Envelope returned by `GET /v1/social?tickers=...` (merged home feed). */
export interface SocialFeedResponse {
  count: number;
  posts: Post[];
}

/** Fetches recent news merged across several tickers, newest first. */
export function getNewsBatch(
  tickers: readonly string[],
  perTicker = 6,
  signal?: AbortSignal,
  topic?: string,
): Promise<NewsFeedResponse> {
  const q = tickers.map(normalizeTicker).join(',');
  let path =
    `/v1/news?tickers=${encodeURIComponent(q)}&limit=${encodeURIComponent(String(perTicker))}`;
  if (topic) path += `&topic=${encodeURIComponent(topic)}`;
  return getJson<NewsFeedResponse>(path, signal);
}

/** Fetches recent social posts merged across several tickers, newest first. */
export function getSocialBatch(
  tickers: readonly string[],
  perTicker = 6,
  signal?: AbortSignal,
): Promise<SocialFeedResponse> {
  const q = tickers.map(normalizeTicker).join(',');
  return getJson<SocialFeedResponse>(
    `/v1/social?tickers=${encodeURIComponent(q)}&limit=${encodeURIComponent(String(perTicker))}`,
    signal,
  );
}

/**
 * Fetches recent filings for a ticker, most recent first.
 *
 * @param limit Maximum number of filings to return (defaults to 25).
 */
export function getFilings(
  ticker: string,
  limit = 25,
  signal?: AbortSignal,
): Promise<FilingsResponse> {
  const path =
    `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}` +
    `/filings?limit=${encodeURIComponent(String(limit))}`;
  return getJson<FilingsResponse>(path, signal);
}

/**
 * Fetches recent company news for a ticker, most recent first.
 *
 * @param limit Maximum number of articles to return (defaults to 25).
 */
export function getNews(
  ticker: string,
  limit = 25,
  signal?: AbortSignal,
): Promise<NewsResponse> {
  const path =
    `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}` +
    `/news?limit=${encodeURIComponent(String(limit))}`;
  return getJson<NewsResponse>(path, signal);
}

/**
 * Fetches recent social posts for a ticker, most recent first.
 *
 * @param limit Maximum number of posts to return (defaults to 30).
 */
export function getSocial(
  ticker: string,
  limit = 30,
  signal?: AbortSignal,
): Promise<SocialResponse> {
  const path =
    `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}` +
    `/social?limit=${encodeURIComponent(String(limit))}`;
  return getJson<SocialResponse>(path, signal);
}

/**
 * Fetches the per-ticker numeric pulse (buzz / sentiment) from every signal
 * source. Returns one entry per source; the list may be empty.
 */
export function getSignals(
  ticker: string,
  signal?: AbortSignal,
): Promise<SignalsResponse> {
  const path =
    `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/signals`;
  return getJson<SignalsResponse>(path, signal);
}

/**
 * Fetches one trending board, top first.
 *
 * @param board `hot` (most discussed, default) or `surging` (biggest risers).
 * @param limit Maximum number of stocks to return (defaults to 40).
 */
export function getHot(
  board = 'hot',
  limit = 40,
  signal?: AbortSignal,
): Promise<HotResponse> {
  return getJson<HotResponse>(
    `/v1/hot?board=${encodeURIComponent(board)}&limit=${encodeURIComponent(String(limit))}`,
    signal,
  );
}

/** A trending news theme (one chip in the Hot Topics strip). */
export interface HotTopic {
  /** Stable id used for filtering (e.g. `ai_capex`). */
  key: string;
  /** Display label (e.g. `AI capex`). */
  label: string;
  /** Matching articles in the last 24h. */
  count: number;
  /** >1 = heating up vs the prior 24h. */
  momentum: number;
  /** Tickers most associated with the theme. */
  related_tickers: string[];
}

/** Envelope returned by `GET /v1/topics`. */
export interface TopicsResponse {
  generated_at: string;
  window: string;
  topics: HotTopic[];
}

// Resilient trending-topics fetch. The cold Cloudflare-tunnel hop intermittently returns a
// 200 with an EMPTY body (not a network reject — so the generic retry-on-reject misses it),
// which baked all /topic/[key] pages as the loading fallback on Vercel. So: retry until the
// snapshot is non-empty (covers both an empty 200 AND a reject), and DON'T Data-Cache it (a
// cached empty would persist). An in-flight promise still collapses a build burst to one call.
// The topic pages now render on-demand (no build prerender), where a single request is far less
// likely to hit the cold-empty reply than the concurrent build was.
let topicsInFlight: Promise<TopicsResponse> | null = null;
export function getTopics(signal?: AbortSignal): Promise<TopicsResponse> {
  if (topicsInFlight) return topicsInFlight;
  const promise = (async (): Promise<TopicsResponse> => {
    for (let attempt = 0; attempt < 3; attempt++) {
      try {
        const r = await getJson<TopicsResponse>('/v1/topics', signal);
        if (r.topics && r.topics.length > 0) return r;
      } catch {
        if (isAbort(undefined, signal)) break; // a deliberate timeout/cancel — stop
      }
      if (attempt < 2) await abortableDelay(400, signal).catch(() => {});
    }
    return {generated_at: '', window: '', topics: []}; // gave up → caller shows the (noindex) fallback
  })();
  topicsInFlight = promise;
  void promise.finally(() => {
    if (topicsInFlight === promise) topicsInFlight = null;
  });
  return promise;
}

/** One insider's open-market buy, for the Opportunity card's evidence. */
export interface OpportunityBuyer {
  name: string;
  title: string;
  date: string;
  value: number;
}

/** One row of the Opportunity board (small-cap with insider buying). */
export interface OpportunityStock {
  ticker: string;
  cik: number;
  company: string;
  price: number;
  market_cap: number;
  rank: number;
  buyers: number; // distinct insiders who bought
  buy_value: number; // total $ of those buys
  buy_count: number;
  last_buy_date: string;
  explainer: string; // "3 insiders bought $1.2M on the open market in the last 30 days"
  top_buyers: OpportunityBuyer[];
  filing_url: string; // link to the SEC filing — the trust anchor
  updated_at: string;
}

/** Envelope returned by `GET /v1/opportunities`. */
export interface OpportunitiesResponse {
  count: number;
  stocks: OpportunityStock[];
}

/**
 * Fetches the Opportunity board — small-cap US stocks with recent open-market
 * insider buying (SEC Form 4), ranked by conviction.
 */
export function getOpportunities(
  limit = 0,
  signal?: AbortSignal,
): Promise<OpportunitiesResponse> {
  const q = limit > 0 ? `?limit=${encodeURIComponent(String(limit))}` : '';
  return getJson<OpportunitiesResponse>(`/v1/opportunities${q}`, signal);
}

/** One curated-KOL ("guru") post in the Guru-watch rail. */
export interface GuruItem {
  author: string; // the writer / publication (e.g. "Serenity")
  title: string;
  url: string; // link to the source post
  teaser: string; // short fair-use snippet, never the full body
  published: string;
  tickers: string[]; // tickers the post mentions (cashtags)
}

/** Envelope returned by `GET /v1/gurus`. */
export interface GurusResponse {
  count: number;
  items: GuruItem[];
}

/**
 * Fetches the Guru-watch rail — recent posts from curated finance writers
 * (KOLs) with the tickers each mentions. Opinions for context, not advice.
 */
export function getGurus(limit = 0, signal?: AbortSignal): Promise<GurusResponse> {
  const q = limit > 0 ? `?limit=${encodeURIComponent(String(limit))}` : '';
  return getJson<GurusResponse>(`/v1/gurus${q}`, signal);
}

/** One symbol-directory match for search autocomplete. */
export interface SymbolMatch {
  ticker: string;
  name: string;
  exchange: string;
  country: string;
}

/** Envelope returned by `GET /v1/search`. */
export interface SearchResponse {
  count: number;
  results: SymbolMatch[];
}

/**
 * Searches the symbol directory (ticker + company name) for autocomplete.
 * Public endpoint; results are ranked best-first.
 */
export function searchSymbols(
  q: string,
  limit = 8,
  signal?: AbortSignal,
): Promise<SearchResponse> {
  const params = new URLSearchParams({q});
  if (limit > 0) params.set('limit', String(limit));
  return getJson<SearchResponse>(`/v1/search?${params.toString()}`, signal);
}

/** One market-moving event on the timeline. */
export interface EventItem {
  id: string;
  title: string;
  category: string; // "macro" | "world"
  subtype: string; // "fomc" | "nfp" | "cpi" | "worldcup" | ...
  start: string; // ISO-8601 UTC
  all_day: boolean;
  importance: string; // "high" | "med" | "low"
  region: string; // "US" | "Global" | country
  source_name: string;
  source_url: string;
}

/** Envelope returned by `GET /v1/events`. */
export interface EventsResponse {
  count: number;
  events: EventItem[];
}

/**
 * Fetches the major-events timeline (macro releases like FOMC/CPI/NFP + notable
 * world events), upcoming first. Public endpoint.
 */
export function getEvents(limit = 0, signal?: AbortSignal): Promise<EventsResponse> {
  const q = limit > 0 ? `?limit=${encodeURIComponent(String(limit))}` : '';
  return getJson<EventsResponse>(`/v1/events${q}`, signal);
}

/** A scheduled/reported company earnings event (Finnhub calendar). */
export interface Earning {
  ticker: string;
  /** ISO-8601 (the earnings date). */
  date: string;
  /** Reporting time: "bmo" (before open) | "amc" (after close) | "dmh" | "". */
  hour?: string;
  eps_estimate?: number;
  eps_actual?: number;
  revenue_estimate?: number;
  revenue_actual?: number;
  /**
   * How the stock has historically moved around earnings (Go-computed aggregate), present only for
   * tracked tickers with enough history. Descriptive — never a forecast.
   */
  reaction?: {avg_abs_move: number; up_rate: number; samples: number};
}

/** Envelope returned by `GET /v1/earnings`. */
export interface EarningsResponse {
  count: number;
  earnings: Earning[];
}

/** Envelope returned by `GET /v1/stocks/{ticker}/earnings`. */
export interface StockEarningsResponse {
  ticker: string;
  count: number;
  earnings: Earning[];
}

/**
 * Fetches the company earnings calendar within [from, to] (YYYY-MM-DD; both
 * optional — the server defaults to today .. +30d). Public endpoint.
 */
export function getEarnings(
  from?: string,
  to?: string,
  signal?: AbortSignal,
): Promise<EarningsResponse> {
  const p = new URLSearchParams();
  if (from) p.set('from', from);
  if (to) p.set('to', to);
  const q = p.toString();
  return getJson<EarningsResponse>(`/v1/earnings${q ? `?${q}` : ''}`, signal);
}

/**
 * Fetches recent/upcoming earnings rows for one ticker (ascending by date),
 * capped by `limit` (default 8). Public endpoint.
 */
export function getStockEarnings(
  ticker: string,
  limit = 8,
  signal?: AbortSignal,
): Promise<StockEarningsResponse> {
  const path = `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/earnings`;
  const q = limit > 0 ? `?limit=${encodeURIComponent(String(limit))}` : '';
  return getJson<StockEarningsResponse>(`${path}${q}`, signal);
}

/** One option contract in the OI leaderboard. */
export interface OptionContract {
  contract: string;
  type: string; // "C" | "P"
  strike: number;
  expiry: string; // YYYY-MM-DD
  oi: number;
  volume: number;
  iv: number; // implied vol, fraction (0.25 = 25%)
}

/** Per-stock delayed options overview from `GET /v1/stocks/{t}/options`. */
export interface OptionsView {
  ticker: string;
  pc_volume: number;
  pc_oi: number;
  max_pain?: number;
  expiry?: string;
  top_oi: OptionContract[];
  updated_at: string;
}

/**
 * Fetches the delayed (≈15-min, Cboe) options overview for a stock. Rejects
 * with 404 when the symbol has no listed options (non-US, etc.).
 */
export function getOptions(ticker: string, signal?: AbortSignal): Promise<OptionsView> {
  return getJson<OptionsView>(
    `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/options`,
    signal,
  );
}

/** One row of the whole-market unusual options-activity board. */
export interface UnusualContract {
  ticker: string;
  type: string; // "C" | "P"
  strike: number;
  expiry: string;
  volume: number;
  oi: number;
  vol_oi: number;
  iv: number;
}

/** Envelope from `GET /v1/options/unusual`. */
export interface UnusualResponse {
  count: number;
  updated_at: string;
  contracts: UnusualContract[];
}

/** Fetches the whole-market unusual options-activity board (delayed, Cboe). */
export function getUnusualOptions(signal?: AbortSignal): Promise<UnusualResponse> {
  return getJson<UnusualResponse>('/v1/options/unusual', signal);
}

/** The daily AI pre-market briefing from `GET /v1/briefing` (404 until generated). */
export interface Briefing {
  date: string; // ET day, YYYY-MM-DD
  text: string; // Chinese briefing body (markdown-ish sections)
  generated_at: string;
}

/** Fetches today's AI morning briefing; rejects 404 before generation. */
export function getBriefing(lang: string, signal?: AbortSignal): Promise<Briefing> {
  const q = lang === 'en' ? '?lang=en' : '';
  return getJson<Briefing>(`/v1/briefing${q}`, signal);
}

/** One position on a 13F whale-holdings fund card (from `GET /v1/13f`). */
export interface WhalePosition {
  ticker: string; // "" when the CUSIP has no US-equity match
  issuer: string;
  value: number; // whole USD
  shares: number;
  pct: number; // % of the fund's 13F portfolio value
  change: 'new' | 'add' | 'trim' | 'hold';
  chg_pct: number; // signed share change vs the prior quarter (%)
}

/** One famous fund's latest 13F snapshot with quarter-over-quarter tags. */
export interface FundHoldings {
  slug: string;
  name: string; // firm
  manager: string; // the person it's known for
  period: string; // quarter-end (as-of), YYYY-MM-DD
  filed: string; // filing date, YYYY-MM-DD
  count: number; // total positions in the filing
  value: number; // total 13F portfolio value (USD)
  positions: WhalePosition[];
}

/** The 13F whale-holdings board from `GET /v1/13f`. */
export interface ThirteenFBoard {
  funds: FundHoldings[];
  updated_at: string;
}

/** Fetches the 13F whale-holdings board (famous funds' latest quarterly holdings). */
export function getThirteenF(signal?: AbortSignal): Promise<ThirteenFBoard> {
  return getJson<ThirteenFBoard>('/v1/13f', signal);
}

/**
 * One famous fund that holds a given ticker, from the per-stock reverse 13F
 * lookup (`GET /v1/stocks/{t}/whales`). `weight` is the position's share of the
 * fund's 13F portfolio; `change` is the quarter-over-quarter move.
 */
export interface WhaleHolder {
  fund_slug: string; // links to /fund/{slug}
  fund_name: string; // firm, e.g. "Berkshire Hathaway"
  manager: string; // the person it's known for, e.g. "Warren Buffett"
  value: number; // position value in this fund (whole USD)
  weight: number; // % of the fund's 13F portfolio
  change: 'new' | 'add' | 'trim' | 'hold';
  period: string; // the fund's filing quarter-end (as-of), YYYY-MM-DD
}

/** Envelope returned by `GET /v1/stocks/{ticker}/whales`. */
export interface WhalesResponse {
  ticker: string;
  holders: WhaleHolder[];
}

/**
 * Fetches which tracked 13F funds hold a given ticker (the reverse "which whales
 * own this stock" lookup), largest position first. Public endpoint; an empty
 * `holders` list returns a 200, so callers should hide the chip when empty.
 */
export function getWhales(
  ticker: string,
  signal?: AbortSignal,
): Promise<WhalesResponse> {
  return getJson<WhalesResponse>(
    `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/whales`,
    signal,
  );
}

/**
 * Fetches one fund's latest 13F holdings by slug, for the fund pSEO page.
 * Resolves to `null` when the slug is unknown (the API 404s), so SSR callers can
 * render `notFound()`; other errors reject.
 */
export async function getFund(
  slug: string,
  signal?: AbortSignal,
): Promise<FundHoldings | null> {
  try {
    return await getJson<FundHoldings>(
      `/v1/13f/${encodeURIComponent(slug)}`,
      signal,
    );
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return null;
    throw e;
  }
}

/**
 * The AI digest for a stock from `GET /v1/stocks/{t}/summary` (cached daily).
 *
 * ASYNC contract: the endpoint returns INSTANTLY — when a background generation
 * is in flight `summary` is "" and `prose_status` is `"generating"`, and the
 * client polls until the prose fills in (see {@link AISummaryCard}). Any other
 * `prose_status` (or an absent field, from the older synchronous backend) is
 * terminal and the rendered `summary` is final.
 */
export interface AISummary {
  ticker: string;
  /** Bullet points ("- " lines) in the requested language; "" while
   *  `prose_status === 'generating'` or when there's no material yet. */
  summary: string;
  generated_at?: string;
  /**
   * The prose-generation lifecycle. Only `"generating"` means a background job
   * is filling `summary` in → keep polling. `"ready"` is final (may be "" → no
   * material → hide); `"quota_exhausted"`/`"llm_disabled"` are terminal → hide.
   * ABSENT on the older synchronous backend (treat exactly as today: non-empty
   * `summary` → show, empty → hide, no polling).
   */
  prose_status?: 'ready' | 'generating' | 'llm_disabled' | 'quota_exhausted';
}

/**
 * Fetches the stock's AI digest in the given UI language ("zh"|"en"; cached
 * per language, server-side). Rejects with 503 when no LLM is configured (hide
 * the card) and 429 when the daily generation budget is exhausted. Returns
 * instantly with `prose_status` when the prose is generated asynchronously (the
 * caller polls while `"generating"`); see {@link AISummary}.
 */
export function getSummary(
  ticker: string,
  lang: string,
  signal?: AbortSignal,
): Promise<AISummary> {
  const q = lang === 'en' ? '?lang=en' : '';
  return getJson<AISummary>(
    `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/summary${q}`,
    signal,
  );
}

/**
 * One labeled, source-attributed datum in a research section. `value` is the
 * already-formatted display string set in Go (e.g. "41.2x", "亏损", "$4.5T",
 * "—", or "数据不足" when `status !== 'ok'`) — the frontend never recomputes it.
 * `raw` is the underlying number when present. `source`/`source_url` are the
 * citation; `as_of` a freshness stamp. The numbers come exclusively from public
 * structured data — the LLM never sets a value (anti-hallucination contract).
 */
export interface ResearchFact {
  key: string;
  label_zh: string;
  label_en: string;
  /** Formatted display string; "数据不足" when not `ok`. */
  value: string;
  raw?: number;
  /** "%" | "x" | "price" | "USD" | "" (empty). */
  unit?: string;
  status: 'ok' | 'insufficient' | 'unsupported';
  /** Why the fact is not `ok`; verbatim from the source. Absent when `ok`. */
  reason?: string;
  /** Citation label, e.g. "SEC XBRL FY2024". */
  source: string;
  source_url?: string;
  /** Freshness stamp, e.g. "2024-09-28". */
  as_of?: string;
}

/** A source citation for a section: an in-page anchor and/or an external URL. */
export interface ResearchCitation {
  label: string;
  /** In-page section id, e.g. "#fundamentals". */
  anchor?: string;
  /** External source (SEC filing, member page, …). */
  url?: string;
}

/**
 * One report section: a bilingual title, its pre-formatted {@link ResearchFact}s,
 * the LLM's qualitative `prose` (empty when the LLM is off / over the daily cap /
 * the call failed), and a citations footer. The numbers live in `facts`; the
 * prose adds only words.
 */
export interface ResearchSection {
  key: string;
  title_zh: string;
  title_en: string;
  facts: ResearchFact[];
  /** Qualitative prose (Markdown); "" in the data-only report. */
  prose: string;
  citations: ResearchCitation[];
  /**
   * Two-sided 看多/看空 reading, set only on the `overview` section when the LLM
   * ran (absent in the data-only report). Each is a list of qualitative points
   * grounded in the report's facts — not a recommendation, not a number.
   */
  bull?: string[];
  bear?: string[];
  /**
   * Pro-paywall lock: true when this section's content was withheld from a free
   * viewer (the Go backend strips prose/facts and sets this at serve time). The UI
   * renders a locked "upgrade to unlock" card instead of the section body.
   */
  locked?: boolean;
}

/** Envelope returned by `GET /v1/stocks/{ticker}/research`. */
export interface ResearchReportResponse {
  ticker: string;
  name?: string;
  /** Newest underlying data date across sources (may be empty). */
  as_of: string;
  /** "$190.12 · alpaca · delayed · regular". */
  price_label: string;
  generated_at: string;
  /** The configured model id; "" when the LLM is disabled. */
  model: string;
  /** Whether prose is present (true) or this is the data-only report (false). */
  llm: boolean;
  /**
   * Prose-generation lifecycle for the now-ASYNC deep report (`?depth=deep`):
   * - `ready` — prose is present (`sections[].prose` + overview bull/bear); render the full report;
   * - `generating` — data-only NOW (Go-owned facts/citations/as_of/price_label/disclaimer all
   *   present, prose empty); a background generation is in flight → poll the same URL;
   * - `quota_exhausted` — over the monthly quota (1/user/month), no cached prose → data-only is final;
   * - `llm_disabled` — LLM off → data-only is final.
   * OPTIONAL: an OLDER (synchronous) backend omits this field — absent ⇒ treat as ready/done (no poll).
   * All four are HTTP 200; 401/404 still flow through {@link getDeepResearch} as today.
   */
  prose_status?: 'ready' | 'generating' | 'quota_exhausted' | 'llm_disabled';
  disclaimer: string;
  sections: ResearchSection[];
  /**
   * Pro-paywall: true when this is the FREE-tier teaser of a ready report — the
   * overview + first section are shown, the rest are `locked`. The UI shows an
   * "unlock the full report with Pro" CTA. Absent/false = full report (Pro viewer,
   * or the paywall is off).
   */
  paywall_locked?: boolean;
}

/**
 * Fetches the structured deep-research report for a ticker in the given UI
 * language ("zh"|"en"). Always 200 with a data-only report when the LLM is off /
 * over the daily cap (prose empty, `llm:false`); resolves to `null` only when the
 * symbol is unknown (the API 404s) so callers can hide the tab. Other errors reject.
 */
export async function getResearch(
  ticker: string,
  lang?: string,
  signal?: AbortSignal,
): Promise<ResearchReportResponse | null> {
  const q = lang === 'en' ? '?lang=en' : '';
  try {
    // Retry-once: a cold ticker's first research request can be reset at the
    // Cloudflare Tunnel hop; an immediate retry hits the now-cached fact sheet.
    return await getJsonWithRetry<ResearchReportResponse>(
      `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/research${q}`,
      signal,
    );
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return null;
    throw e;
  }
}

/**
 * Fetches the GATED **deep** research report (`?depth=deep`) for a ticker in the
 * given UI language. Unlike the public {@link getResearch}, this is an authed
 * call — it sends the Supabase access `token` as a Bearer header — and the
 * endpoint enforces a global per-user **1 generation/day** quota. The richer
 * prose comes back when the LLM ran (`llm:true`); when the provider is off /
 * rate-limited the backend still 200s with the Go-owned data-only report
 * (`llm:false`, prose empty), so the view always renders.
 *
 * The caller MUST branch on the thrown {@link ApiError} status rather than have
 * it swallowed, so the gate UX can react:
 * - **401** anon / invalid token → show the login gate (don't call this when
 *   logged out — pass a real token);
 * - **429** the daily generation quota is spent → show the "try tomorrow" note
 *   (a cached (user-agnostic ticker,day) report 200s for free, so 429 only on a
 *   NEW generation over quota);
 * - **404** unknown symbol → resolves to `null` (hide the report).
 * Other errors reject.
 */
export async function getDeepResearch(
  ticker: string,
  token: string | null,
  lang?: string,
  signal?: AbortSignal,
): Promise<ResearchReportResponse | null> {
  const p = new URLSearchParams({depth: 'deep'});
  if (lang === 'en') p.set('lang', 'en');
  try {
    // Retry-once: same cold-ticker Tunnel-reset mitigation as getResearch. Only
    // a transient network REJECT retries — a 401/429 Response is returned as-is
    // (never retried), so the daily generation quota is never burned by a retry.
    return await getJsonWithRetry<ResearchReportResponse>(
      `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/research?${p.toString()}`,
      signal,
      token,
    );
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return null;
    throw e; // 401 / 429 / other → caller branches on status
  }
}

/** The logged-in user's Pro entitlement (GET /v1/billing/me). */
export interface Entitlement {
  tier: 'free' | 'pro';
  current_period_end?: string;
  cancel_at_period_end?: boolean;
}

/**
 * Fetches the user's Pro entitlement. Returns {tier:'free'} when billing is not
 * configured (the endpoint 404s) so callers can treat "no billing" as free.
 */
export async function getEntitlement(token: string | null, signal?: AbortSignal): Promise<Entitlement> {
  try {
    return await getJson<Entitlement>('/v1/billing/me', signal, token);
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return {tier: 'free'};
    throw e;
  }
}

/** Starts a Stripe Checkout session for the chosen plan and returns the hosted URL. */
export async function createCheckout(
  interval: 'month' | 'year',
  token: string | null,
  signal?: AbortSignal,
): Promise<string> {
  const {url} = await postJson<{url: string}>(
    `/v1/billing/checkout?interval=${interval}`,
    {},
    signal,
    token,
  );
  return url;
}

/** Opens the Stripe Billing Portal (manage/cancel) and returns its URL. */
export async function createPortal(token: string | null, signal?: AbortSignal): Promise<string> {
  const {url} = await postJson<{url: string}>('/v1/billing/portal', {}, signal, token);
  return url;
}

// ── Product B: personalized AI chat (Pro-gated) ─────────────────────────────

/**
 * One ordered piece of an assistant answer: prose `text`, or a `widget` reference the
 * UI renders from the real store (the chat layer never ships a widget's numbers).
 */
export interface ChatBlock {
  kind: 'text' | 'widget';
  text?: string;
  widget?: string;
  params?: Record<string, string>;
}

/** The POST /chat response: the assistant's blocks + a meter readout + soft-state flags. */
export interface ChatResponse {
  blocks: ChatBlock[];
  disclaimer?: string;
  meter?: {used: number; limit: number};
  /** The monthly message cap was hit — `blocks` carries a soft note (HTTP is still 200). */
  limit_reached?: boolean;
  /** The global daily cap was hit — `blocks` carries a soft note (HTTP is still 200). */
  busy?: boolean;
  /** The thread exceeded its token budget — `blocks` says to start a new conversation. */
  thread_full?: boolean;
}

/** One persisted turn from GET /chat (assistant turns carry parsed blocks). */
export interface ChatHistoryMessage {
  role: 'user' | 'assistant';
  blocks?: ChatBlock[];
  text?: string;
  at: string;
}

/**
 * Sends one Product B chat turn (POST /v1/stocks/{ticker}/chat). Pro-gated server-side:
 * a non-Pro user gets a **402** ({@link ApiError} status 402) → the caller shows the
 * upgrade CTA. The monthly-cap / busy soft states return HTTP 200 with the note in
 * `blocks` + the matching flag set.
 */
export async function postChat(
  ticker: string,
  message: string,
  token: string | null,
  lang?: string,
  signal?: AbortSignal,
): Promise<ChatResponse> {
  return postJson<ChatResponse>(
    `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/chat`,
    {message, lang: lang === 'zh' ? 'zh' : 'en'},
    signal,
    token,
  );
}

/** The user's persisted chat thread for a ticker (GET /v1/stocks/{ticker}/chat). */
export async function getChatHistory(
  ticker: string,
  token: string | null,
  signal?: AbortSignal,
): Promise<ChatHistoryMessage[]> {
  const {messages} = await getJson<{messages: ChatHistoryMessage[]}>(
    `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/chat`,
    signal,
    token,
  );
  return messages ?? [];
}

/** Clears the user's chat thread for a ticker (the "new conversation" reset). */
export async function clearChat(ticker: string, token: string | null, signal?: AbortSignal): Promise<void> {
  await deleteJson<{ok: boolean}>(
    `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/chat`,
    signal,
    token,
  );
}

// ── Product C: the unified chat hub (conversations) ─────────────────────────

/** One conversation in the hub (the sidebar list shape). */
export interface Conversation {
  id: string;
  title: string;
  anchor_ticker?: string;
  updated_at: string;
}

/** Lists the user's conversations, newest-updated first (GET /v1/conversations). */
export async function listConversations(token: string | null, signal?: AbortSignal): Promise<Conversation[]> {
  const {conversations} = await getJson<{conversations: Conversation[]}>('/v1/conversations', signal, token);
  return conversations ?? [];
}

/** Creates a conversation (general, or stock-anchored when anchor_ticker is given). */
export async function createConversation(
  opts: {title?: string; anchorTicker?: string},
  token: string | null,
  signal?: AbortSignal,
): Promise<Conversation> {
  return postJson<Conversation>(
    '/v1/conversations',
    {title: opts.title ?? '', anchor_ticker: opts.anchorTicker ?? ''},
    signal,
    token,
  );
}

/** Renames a conversation (PATCH /v1/conversations/{id}). */
export async function renameConversation(id: string, title: string, token: string | null, signal?: AbortSignal): Promise<void> {
  await patchJson<{ok: boolean}>(`/v1/conversations/${encodeURIComponent(id)}`, {title}, signal, token);
}

/** Deletes a conversation + its messages (DELETE /v1/conversations/{id}). */
export async function deleteConversation(id: string, token: string | null, signal?: AbortSignal): Promise<void> {
  await deleteJson<{ok: boolean}>(`/v1/conversations/${encodeURIComponent(id)}`, signal, token);
}

/** Sends one chat turn to a conversation (POST /v1/conversations/{id}/chat). */
export async function postConvChat(
  id: string,
  message: string,
  token: string | null,
  lang?: string,
  signal?: AbortSignal,
): Promise<ChatResponse> {
  return postJson<ChatResponse>(
    `/v1/conversations/${encodeURIComponent(id)}/chat`,
    {message, lang: lang === 'zh' ? 'zh' : 'en'},
    signal,
    token,
  );
}

/**
 * Streaming variant of postConvChat (POST /v1/conversations/{id}/chat/stream): reads the
 * SSE response and invokes onToken for each content delta as the answer generates, then
 * resolves with the terminal payload — the AUTHORITATIVE advice-filtered blocks + meter.
 * The caller renders tokens live, then reconciles its accumulated text with the returned
 * blocks (so the anti-hallucination filter always wins). Throws on a transport error or a
 * server "error" event; the caller can fall back to the non-streaming postConvChat.
 */
export async function postConvChatStream(
  id: string,
  message: string,
  token: string | null,
  lang: string | undefined,
  onToken: (text: string) => void,
  signal?: AbortSignal,
): Promise<ChatResponse> {
  let res: Response;
  try {
    res = await fetch(`${API_BASE}/v1/conversations/${encodeURIComponent(id)}/chat/stream`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(token ? {Authorization: `Bearer ${token}`} : {}),
      },
      body: JSON.stringify({message, lang: lang === 'zh' ? 'zh' : 'en'}),
      signal,
    });
  } catch {
    throw new ApiError(`network error contacting ${API_BASE}`, 0);
  }
  if (!res.ok || !res.body) {
    throw new ApiError('chat stream failed', res.status);
  }
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buf = '';
  let done: ChatResponse | null = null;
  for (;;) {
    const {value, done: streamDone} = await reader.read();
    if (value) buf += decoder.decode(value, {stream: true});
    let sep: number;
    // SSE events are blank-line ("\n\n") separated; parse each complete event.
    while ((sep = buf.indexOf('\n\n')) >= 0) {
      const event = buf.slice(0, sep);
      buf = buf.slice(sep + 2);
      for (const line of event.split('\n')) {
        const s = line.trim();
        if (!s.startsWith('data:')) continue;
        const data = s.slice(5).trim();
        if (!data) continue;
        let ev: {type?: string; text?: string} & Partial<ChatResponse>;
        try {
          ev = JSON.parse(data);
        } catch {
          continue;
        }
        if (ev.type === 'token') {
          if (ev.text) onToken(ev.text);
        } else if (ev.type === 'error') {
          throw new ApiError('chat stream error', 502);
        } else if (ev.type === 'done') {
          done = {blocks: ev.blocks ?? [], disclaimer: ev.disclaimer, meter: ev.meter, limit_reached: ev.limit_reached, busy: ev.busy, thread_full: ev.thread_full};
        }
      }
    }
    if (streamDone) break;
  }
  if (!done) throw new ApiError('chat stream ended without a result', 0);
  return done;
}

/** Current monthly chat TOKEN usage (GET /v1/chat/usage) for the hub quota bar on load. */
export async function getChatUsage(token: string | null, signal?: AbortSignal): Promise<{used: number; limit: number}> {
  return getJson<{used: number; limit: number}>('/v1/chat/usage', signal, token);
}

/** A conversation's persisted messages (GET /v1/conversations/{id}/chat). */
export async function getConvHistory(id: string, token: string | null, signal?: AbortSignal): Promise<ChatHistoryMessage[]> {
  const {messages} = await getJson<{messages: ChatHistoryMessage[]}>(
    `/v1/conversations/${encodeURIComponent(id)}/chat`,
    signal,
    token,
  );
  return messages ?? [];
}

/**
 * One attributed evidence item behind a notable price move: a recent news
 * headline, a filing, or an insider buy. `title`/`url` are set in Go from the
 * typed source — never the LLM's (the LLM may reference these headlines but
 * invents none and writes no URL).
 */
export interface MovementEvidence {
  type: 'news' | 'filing' | 'insider';
  title: string;
  url?: string;
  /** RFC3339 timestamp of the source item. */
  time: string;
}

/**
 * Envelope returned by `GET /v1/stocks/{ticker}/movement`. The move-explainer:
 * `change_pct`/`direction` are Go-owned numbers (computed from the quote, NEVER
 * the LLM's). `significant` gates whether the move is notable enough (|change|
 * >= 5%) to explain — when `false`, there is no `explanation`/`evidence` and the
 * card hides. When `significant`, `explanation` is the LLM's ONE hedged Chinese
 * sentence (`llm:true`, with `disclaimer`) or a canned Go-built line (`llm:false`,
 * the data-only fallback when the LLM is off / over the daily cap / errored).
 *
 * ASYNC contract: the endpoint returns INSTANTLY. When `prose_status` is
 * `"generating"` the `explanation` is the CANNED Go line NOW (`llm:false`) and a
 * background LLM generation is in flight — render the canned line immediately
 * and poll to UPGRADE it (a later `"ready"` poll swaps in the better
 * `explanation` and, when `llm:true`, the AI badge). `"ready"` is terminal.
 */
export interface MovementResponse {
  ticker: string;
  significant: boolean;
  /** Day's change %, computed in Go from price vs prev close. */
  change_pct: number;
  direction: 'up' | 'down';
  session: string;
  /** Present only when `significant`. LLM-hedged or canned. */
  explanation?: string;
  /** Present only when `significant`. Attributed source items. */
  evidence?: MovementEvidence[];
  /** Whether the explanation is the LLM's sentence (true) or the canned line. */
  llm: boolean;
  /** The configured model id; "" when the LLM is disabled / canned line served. */
  model: string;
  /** RFC3339 quote timestamp. */
  as_of: string;
  /** "AI 生成 · 仅供参考 · 非投资建议" — present only when `llm`. */
  disclaimer?: string;
  /**
   * The prose-generation lifecycle. Only `"generating"` means the LLM upgrade is
   * still in flight (the canned `explanation` shows now → keep polling). Every
   * other value — `"ready"` (final), `"llm_disabled"`, `"quota_exhausted"` — is
   * terminal. ABSENT on the older synchronous backend (treat as today: render
   * the explanation as final, no polling).
   */
  prose_status?: 'ready' | 'generating' | 'llm_disabled' | 'quota_exhausted';
}

/**
 * Fetches the "why did this stock move today?" explainer for a ticker in the
 * given UI language ("zh"|"en"). Always 200 with the Go-owned number; when the
 * move is below threshold (or the LLM is off) it still resolves — the caller
 * checks `significant` and `llm` to decide what to render. Resolves to `null`
 * only when the symbol is unknown (the API 404s) so callers hide the card.
 */
export async function getMovement(
  ticker: string,
  lang?: string,
  signal?: AbortSignal,
): Promise<MovementResponse | null> {
  const q = lang === 'en' ? '?lang=en' : '';
  try {
    // Retry-once: another on-demand cold-ticker GET subject to the same
    // Cloudflare Tunnel reset as the research fetches.
    return await getJsonWithRetry<MovementResponse>(
      `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/movement${q}`,
      signal,
    );
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return null;
    throw e;
  }
}

/**
 * One parsed 8-K item code with its canonical labels. The labels are OWNED BY GO
 * (the item-code → meaning map lives server-side) — the anti-hallucination anchor.
 * Pick `label_en` / `label_zh` based on the current UI language.
 */
export interface MaterialEventItem {
  code: string;
  label_en: string;
  label_zh: string;
}

/**
 * One 8-K (current report) filing — a material corporate event. Every field
 * except `summary` is a Go-owned fact (form/dates/accession URL + parsed item
 * codes & labels). `summary` is the OPTIONAL LLM plain-language summary (absent
 * when the LLM is off / the source was too thin — the item labels alone still
 * render).
 */
export interface MaterialEvent {
  /** "8-K" or "8-K/A" (the amendment variant). */
  form: string;
  /** Whether this is an 8-K/A amendment to a prior 8-K. */
  amendment: boolean;
  /** SEC filing date (YYYY-MM-DD). */
  filed_date: string;
  /** Period-of-report / event date (YYYY-MM-DD), when present. */
  report_date?: string;
  /** Human-readable SEC filing index page. */
  accession_url: string;
  /** Parsed 8-K item codes with their canonical bilingual labels. */
  items: MaterialEventItem[];
  /** Optional LLM plain-language summary; absent when none was written. */
  summary?: string;
}

/**
 * Envelope returned by `GET /v1/stocks/{ticker}/material-events`. `filings` is
 * ALWAYS present and non-null (an existing company with no recent 8-Ks yields
 * `[]`). `llm` reports whether any summary was AI-written; `disclaimer` is present
 * only then. `source` is the data attribution ("SEC EDGAR"). 404 → null (the card
 * hides) only when the ticker/CIK can't be resolved.
 */
export interface MaterialEventsResponse {
  ticker: string;
  filings: MaterialEvent[];
  count: number;
  llm: boolean;
  model: string;
  source: string;
  /** RFC3339 generation timestamp (the as-of for the data). */
  generated_at: string;
  /** "AI 生成 · 仅供参考 · 非投资建议" — present only when `llm`. */
  disclaimer?: string;
}

/**
 * Fetches a company's recent 8-K material-event filings (with an optional AI
 * summary per filing) for a ticker in the given UI language ("zh"|"en"). Always
 * 200 with the Go-owned facts; resolves to `null` only when the symbol is unknown
 * (the API 404s) so the caller hides the card. An existing company with zero
 * recent 8-Ks resolves with `filings: []`.
 */
export async function getMaterialEvents(
  ticker: string,
  lang?: string,
  signal?: AbortSignal,
): Promise<MaterialEventsResponse | null> {
  const q = lang === 'en' ? '?lang=en' : '';
  try {
    // Retry-once: another on-demand cold-ticker GET subject to the same
    // Cloudflare Tunnel reset as the research fetches.
    return await getJsonWithRetry<MaterialEventsResponse>(
      `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/material-events${q}`,
      signal,
    );
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return null;
    throw e;
  }
}

/**
 * One open-market insider transaction (a buy or a sell) from a Form 4 filing —
 * the building block of the per-ticker insider-activity timeline. EVERY field is
 * a Go-owned fact parsed straight from the Form 4 XML (no LLM, no derived guess):
 * `value` = `shares` × `price`. `planned_10b5_1` is the best-effort Rule 10b5-1
 * planned-sale flag (always false for buys, and false for a sale lacking a
 * reliable indicator — never fabricated).
 */
export interface InsiderTransaction {
  /** "buy" (Form 4 code P) or "sell" (code S). */
  type: 'buy' | 'sell';
  /** Reporting insider's name as filed (e.g. "Cook Timothy D"). */
  owner: string;
  /** Insider role/title (filed officer title, else "Director"/"Officer", else ""). */
  role?: string;
  shares: number;
  price: number;
  /** shares × price (Go-computed from the source figures). */
  value: number;
  /** Transaction date (YYYY-MM-DD); filing date as a fallback when absent. */
  date: string;
  /** Affirmed Rule 10b5-1 planned sale (sells only). */
  planned_10b5_1: boolean;
  /** Human-readable SEC filing index page. */
  accession_url: string;
}

/**
 * Envelope returned by `GET /v1/stocks/{ticker}/insider-activity`.
 * `transactions` is ALWAYS present and non-null, newest first (an existing
 * company with no recent Form 4s yields `[]`). `buy_count`/`sell_count`/
 * `net_value` (buy $ − sell $) are cheap Go-owned aggregates. `source` is the
 * attribution ("SEC EDGAR Form 4"). 404 → null (the card hides) only when the
 * ticker/CIK can't be resolved.
 */
export interface InsiderActivityResponse {
  ticker: string;
  transactions: InsiderTransaction[];
  count: number;
  buy_count: number;
  sell_count: number;
  net_value: number;
  source: string;
  /** RFC3339 generation timestamp (the as-of for the data). */
  generated_at: string;
}

/**
 * Fetches a company's recent insider-activity timeline (Form 4 open-market buys
 * AND sells) for a ticker. Pure structured data — no LLM. Always 200 with the
 * Go-owned facts; resolves to `null` only when the symbol is unknown (the API
 * 404s) so the caller hides the card. An existing company with zero recent
 * Form 4s resolves with `transactions: []`.
 */
export async function getInsiderActivity(
  ticker: string,
  signal?: AbortSignal,
): Promise<InsiderActivityResponse | null> {
  try {
    return await getJson<InsiderActivityResponse>(
      `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/insider-activity`,
      signal,
    );
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return null;
    throw e;
  }
}

/** A symbol's latest FINRA short-interest row (twice-monthly settlement). */
export interface ShortInterest {
  symbol: string;
  name?: string;
  market?: string;
  settlement_date: string; // YYYY-MM-DD
  short_qty: number;
  prev_short_qty?: number;
  avg_daily_volume?: number;
  days_to_cover: number;
  change_pct: number;
}

/** One day's short-volume share, for the daily short trend sparkline. */
export interface DailyShortPoint {
  date: string; // YYYY-MM-DD
  short_pct: number; // % of the day's reported volume that was short
}

/**
 * Daily short-volume facet (FINRA short-sale volume) for a stock — the latest
 * day's short % plus a short recent history. A faster-cadence companion to the
 * twice-monthly {@link ShortInterest} settlement; absent (`null`) when the
 * symbol has no daily row.
 */
export interface DailyShort {
  short_pct: number;
  as_of: string; // YYYY-MM-DD
  history: DailyShortPoint[];
}

/**
 * Fetches the latest FINRA short data for a stock (squeeze radar). The
 * twice-monthly settlement {@link ShortInterest} (`short`) and the daily
 * short-volume facet ({@link DailyShort}, `daily`) are each `null` when the
 * symbol has no row (non-US, brand-new listings — note ETFs ARE covered, e.g.
 * SPY). Rejects with 404 only when the symbol is wholly unknown.
 */
export function getShort(
  ticker: string,
  signal?: AbortSignal,
): Promise<{ticker: string; short: ShortInterest | null; daily?: DailyShort | null}> {
  return getJson<{ticker: string; short: ShortInterest | null; daily?: DailyShort | null}>(
    `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/short`,
    signal,
  );
}

/** One row of the market-wide "today's short volume" board. */
export interface ShortVolumeStock {
  symbol: string;
  /** Short volume as a % of the day's total reported volume. */
  short_pct: number;
  short_volume: number;
  total_volume: number;
}

/** Envelope returned by `GET /v1/short-volume`. */
export interface ShortVolumeResponse {
  as_of: string; // YYYY-MM-DD
  count: number;
  stocks: ShortVolumeStock[];
}

/**
 * Fetches the market-wide "today's short volume" leaderboard (FINRA short-sale
 * volume), ranked highest short-% first and filtered to liquid names. Public
 * endpoint; the list is `[]` until the data source is ready.
 */
export function getShortVolume(
  limit = 50,
  signal?: AbortSignal,
): Promise<ShortVolumeResponse> {
  const q = limit > 0 ? `?limit=${encodeURIComponent(String(limit))}` : '';
  return getJson<ShortVolumeResponse>(`/v1/short-volume${q}`, signal);
}

/** One component (sub-indicator) feeding the fear & greed index. */
export interface SentimentComponent {
  name: string;
  score: number; // 0–100
  note: string;
}

/** One day's historical fear & greed score, for the trend sparkline. */
export interface SentimentPoint {
  date: string; // YYYY-MM-DD
  score: number; // 0–100
}

/** The Tickwind fear & greed index from `GET /v1/sentiment`. */
export interface Sentiment {
  score: number; // 0–100 (0 = extreme fear, 100 = extreme greed)
  label: string; // English label, e.g. "Greed"
  label_zh: string; // Chinese label, e.g. "贪婪"
  components: SentimentComponent[];
  updated_at: string; // RFC 3339
  history: SentimentPoint[];
}

/**
 * Fetches the Tickwind fear & greed index (own composite over VIX, momentum,
 * breadth, etc.). Public endpoint; component/history lists are `[]` until the
 * data source is ready.
 */
export function getSentiment(signal?: AbortSignal): Promise<Sentiment> {
  return getJson<Sentiment>('/v1/sentiment', signal);
}

/** One probability outcome (e.g. a rate-cut size) within a prediction market. */
export interface RateCutOutcome {
  label: string; // e.g. "-25bps"
  probability: number; // 0–1
}

/** One prediction market's rate-cut odds (e.g. from Kalshi / Polymarket). */
export interface RateCutMarket {
  source: string; // "Kalshi" | "Polymarket" | …
  question: string;
  as_of: string; // RFC 3339 / source-supplied
  outcomes: RateCutOutcome[];
}

/** Envelope returned by `GET /v1/ratecut`. */
export interface RateCutResponse {
  markets: RateCutMarket[];
  updated_at: string; // RFC 3339
}

/**
 * Fetches Fed rate-cut odds from prediction markets (Kalshi / Polymarket).
 * Public endpoint; `markets` is `[]` until the data source is ready. These are
 * prediction-market prices, not investment advice.
 */
export function getRateCut(signal?: AbortSignal): Promise<RateCutResponse> {
  return getJson<RateCutResponse>('/v1/ratecut', signal);
}

/** One maturity tenor's par yield on the Treasury curve (e.g. 2Y @ 4.09%). */
export interface MacroYield {
  tenor: string; // canonical short label, e.g. "2Y", "10Y", "3M"
  rate: number; // par yield, percent
}

/**
 * The latest U.S. Treasury daily par yield curve from `GET /v1/macro` — the
 * macro-context strip backing the 2s10s recession signal. `available` is false
 * (and `yields` empty) until the server-side cache is first filled; the strip
 * hides itself in that case. `spread_2s10s` is null when either the 2Y or 10Y
 * leg is missing (never fabricated). Real Treasury data only.
 */
export interface Macro {
  available: boolean;
  as_of: string; // YYYY-MM-DD, the curve's business day
  yields: MacroYield[];
  spread_2s10s: number | null; // 10Y − 2Y, percentage points (null if a leg is absent)
  inverted: boolean; // spread present and negative
  source: string; // "U.S. Treasury"
  source_zh: string; // "美国财政部"
  source_url: string;
  updated_at: string; // RFC 3339
}

/**
 * Fetches the latest U.S. Treasury daily par yield curve (server-driven cache;
 * refreshed ~12h). Public, keyless. Always resolves with a well-formed shape —
 * `available: false` + empty `yields` until the curve is loaded.
 */
export function getMacro(signal?: AbortSignal): Promise<Macro> {
  return getJson<Macro>('/v1/macro', signal);
}

/** A coin's best-effort spot price + 24h change. `null` (not 0) when the price
 * source was unavailable — never fabricated. */
export interface CryptoPrice {
  price: number; // spot price in USD
  change_24h: number; // 24h change, percent
}

/**
 * The latest crypto market-mood snapshot from `GET /v1/crypto` — the crypto
 * Fear & Greed index (0–100, alternative.me) plus best-effort BTC/ETH spot
 * prices (CoinGecko). Context for the crypto-linked equities COIN/MSTR/RIOT/MARA.
 * `available` is false until the server-side cache is first filled (the strip
 * hides itself). `btc`/`eth` are null when the price source was unavailable —
 * the F&G score alone is the feature, prices are never fabricated.
 */
export interface Crypto {
  available: boolean;
  score: number; // 0–100 (0 = extreme fear, 100 = extreme greed)
  label: string; // English classification, e.g. "Greed"
  label_zh: string; // Chinese classification, e.g. "贪婪"
  as_of: string; // YYYY-MM-DD, the index's day
  btc: CryptoPrice | null;
  eth: CryptoPrice | null;
  source: string; // "alternative.me"
  updated_at: string; // RFC 3339
}

/**
 * Fetches the latest crypto Fear & Greed index + best-effort BTC/ETH prices
 * (server-driven cache; refreshed ~hourly). Public, keyless. Always resolves
 * with a well-formed shape — `available: false` until the index is loaded.
 */
export function getCrypto(signal?: AbortSignal): Promise<Crypto> {
  return getJson<Crypto>('/v1/crypto', signal);
}

/** A U.S. House Periodic Transaction Report filing (official Clerk disclosure). */
export interface CongressFiling {
  name: string; // "Richard W. Allen"
  last: string;
  first: string;
  state: string; // "GA"
  district: string; // "12"
  filing_type: string; // "P" (periodic transaction report)
  year: number;
  filed_date: string; // ISO-8601
  doc_id: string;
  pdf_url: string; // official filing PDF
}

/** Envelope returned by `GET /v1/congress`. */
export interface CongressResponse {
  count: number;
  filings: CongressFiling[];
}

/**
 * Fetches the latest congressional stock-trade disclosures (House Clerk Periodic
 * Transaction Reports), newest first. Public endpoint; `limit` caps rows.
 */
export function getCongress(limit = 60, signal?: AbortSignal): Promise<CongressResponse> {
  const q = limit > 0 ? `?limit=${encodeURIComponent(String(limit))}` : '';
  return getJson<CongressResponse>(`/v1/congress${q}`, signal);
}

/**
 * One congressional trade in a ticker, from `GET /v1/stocks/{ticker}/congress`.
 * `type` is the disclosed transaction direction ("purchase" / "sale" /
 * "exchange" / …); `slug` deep-links to the member's detail page.
 */
export interface CongressTrade {
  member: string; // "Chip Roy"
  slug: string; // "chip-roy" — links to /congress/member/{slug}
  type: string; // "purchase" | "sale" | "exchange" | …
  amount_range: string; // e.g. "$1,001 - $15,000"
  tx_date: string; // transaction date (ISO-8601 / source-supplied)
}

/** Envelope returned by `GET /v1/stocks/{ticker}/congress`. */
export interface StockCongressResponse {
  ticker: string;
  trades: CongressTrade[];
}

/**
 * Fetches the congressional trades disclosed in a single ticker (House Clerk
 * PTRs), most recent first. Public endpoint; an empty disclosure history returns
 * `trades: []` with a 200, so callers should hide the section when empty.
 */
export function getStockCongress(
  ticker: string,
  signal?: AbortSignal,
): Promise<StockCongressResponse> {
  return getJson<StockCongressResponse>(
    `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/congress`,
    signal,
  );
}

/**
 * One transaction in a member's disclosure history, from
 * `GET /v1/congress/member/{slug}`. Fields mirror the House Clerk PTR columns.
 * Note: the live API serializes these in snake_case (verified against
 * `/v1/congress/member/mike-kelly`), so the keys are snake_case here despite the
 * PascalCase column names. `ticker` may be empty (the asset is a fund, bond, or
 * has no listed symbol).
 */
export interface MemberTx {
  owner: string; // "self" | "spouse" | "joint" | …
  asset: string; // human asset name
  ticker: string; // "" when the asset has no listed symbol
  asset_type: string; // "Stock" | "Bond" | …
  type: string; // "purchase" | "sale" | "exchange" | …
  partial: boolean;
  amount_low: number;
  amount_high: number;
  amount_range: string; // e.g. "$1,001 - $15,000"
  tx_date: string; // transaction date (ISO-8601 / source-supplied)
  notify_date: string; // filing / notification date
}

/** Envelope returned by `GET /v1/congress/member/{slug}`. */
export interface MemberResponse {
  slug: string;
  name: string;
  state: string;
  transactions: MemberTx[];
}

/**
 * Fetches one congressional member's full disclosure history by slug
 * ({@link congressSlug}). Resolves to `null` when the slug is unknown (the API
 * 404s), so SSR callers can render `notFound()`; other errors reject.
 */
export async function getCongressMember(
  slug: string,
  signal?: AbortSignal,
): Promise<MemberResponse | null> {
  try {
    return await getJson<MemberResponse>(
      `/v1/congress/member/${encodeURIComponent(slug)}`,
      signal,
    );
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return null;
    throw e;
  }
}

/** One dated sample on the follow-trade simulation equity curves (member vs SPY). */
export interface BacktestPoint {
  date: string; // YYYY-MM-DD
  member_pct: number; // cumulative follow-trade return, %
  spy_pct: number; // cumulative SPY buy-and-hold return, %
}

/**
 * Result of the conservative follow-trade SIMULATION for one member: each
 * disclosed BUY enters equal-weight at its disclosure-date close and is held (or
 * sold on a disclosed sale) to today, vs. an equal-dollar SPY buy-and-hold
 * baseline. A historical replay only — not realized returns, not advice.
 * `insufficient` is true when there isn't enough priced buy history to simulate.
 */
export interface Backtest {
  insufficient: boolean;
  member_return_pct: number;
  spy_return_pct: number;
  window_start: string; // YYYY-MM-DD ("" when insufficient)
  window_end: string; // YYYY-MM-DD ("" when insufficient)
  window_days: number;
  trades_used: number; // priced buy legs that entered the simulation
  trades_skipped: number; // buy legs dropped for missing price history
  tickers: string[] | null; // distinct simulated tickers (sorted)
  curve: BacktestPoint[] | null; // member-vs-SPY equity curve, oldest first
}

/** Envelope returned by `GET /v1/congress/member/{slug}/backtest`. */
export interface BacktestResponse {
  slug: string;
  name: string;
  backtest: Backtest;
}

/**
 * Fetches one member's follow-trade simulation. The endpoint always returns 200
 * (an unknown member / no price data → `backtest.insufficient: true`), so this
 * resolves to `null` only on a transport error the caller should ignore. SSR
 * callers can render the section conditionally on `insufficient`.
 */
export async function getCongressBacktest(
  slug: string,
  signal?: AbortSignal,
): Promise<BacktestResponse | null> {
  try {
    return await getJson<BacktestResponse>(
      `/v1/congress/member/${encodeURIComponent(slug)}/backtest`,
      signal,
    );
  } catch {
    return null; // a transient API error hides the section rather than breaking the page
  }
}

/**
 * Builds a member's URL slug from their display name, matching the backend's
 * Slugify exactly: lowercase, then collapse each run of non-alphanumeric
 * characters to a single `-`, then trim leading/trailing `-`. E.g.
 * "Suzan K. DelBene" → "suzan-k-delbene"; "Mike Kelly" → "mike-kelly".
 */
export function congressSlug(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
}

/** A SEC Schedule 13D/13G beneficial-ownership filing (institutional/activist stake). */
export interface InstitutionalFiling {
  form_type: string; // "SC 13D" | "SC 13D/A" | "SC 13G" | "SC 13G/A"
  cik: number;
  company: string; // subject issuer
  ticker?: string; // resolved from cik (links to the stock page) when known
  accession: string;
  filed_date: string; // YYYYMMDD
  activist: boolean; // true = 13D (active stake), false = 13G (passive)
  filer?: string; // the reporting institution
}

/** Envelope returned by `GET /v1/institutional`. */
export interface InstitutionalResponse {
  count: number;
  filings: InstitutionalFiling[];
}

/**
 * Fetches recent SEC 13D/13G beneficial-ownership filings, newest first. `type`
 * filters to activist 13D or passive 13G; `limit` caps rows. Public endpoint.
 */
export function getInstitutional(
  opts: {type?: '13d' | '13g'; limit?: number} = {},
  signal?: AbortSignal,
): Promise<InstitutionalResponse> {
  const p = new URLSearchParams();
  if (opts.type) p.set('type', opts.type);
  if (opts.limit != null) p.set('limit', String(opts.limit));
  const q = p.toString();
  return getJson<InstitutionalResponse>(`/v1/institutional${q ? `?${q}` : ''}`, signal);
}

/** One screener match from `GET /v1/screen`. change_pct is null when unknown. */
/** One major-market-index level (the real index, not an ETF proxy). */
export interface IndexQuote {
  symbol: string; // Yahoo-style, e.g. ^GSPC
  name?: string;
  price: number;
  prev_close?: number;
  source: string;
  at: string;
}

/** Envelope returned by `GET /v1/indices`. */
export interface IndicesResponse {
  count: number;
  indices: IndexQuote[];
}

/** Fetches the latest major US index levels for the homepage strip. */
export function getIndices(signal?: AbortSignal): Promise<IndicesResponse> {
  return getJson<IndicesResponse>('/v1/indices', signal);
}

export interface ScreenResult {
  ticker: string;
  price: number;
  prev_close?: number;
  change_pct: number | null;
  session: string;
}

/** Envelope returned by `GET /v1/screen`. */
export interface ScreenResponse {
  count: number;
  results: ScreenResult[];
}

/** Filters accepted by the screener (all optional; blanks are omitted). */
export interface ScreenParams {
  minPrice?: number;
  maxPrice?: number;
  minChange?: number;
  maxChange?: number;
  session?: string;
  sort?: string;
  limit?: number;
}

/**
 * Runs the stock screener over the whole-US universe quote cache (delayed IEX
 * prices). Only non-empty filters are sent. Public endpoint.
 */
export function getScreen(
  params: ScreenParams = {},
  signal?: AbortSignal,
): Promise<ScreenResponse> {
  const p = new URLSearchParams();
  if (params.minPrice != null) p.set('min_price', String(params.minPrice));
  if (params.maxPrice != null) p.set('max_price', String(params.maxPrice));
  if (params.minChange != null) p.set('min_change', String(params.minChange));
  if (params.maxChange != null) p.set('max_change', String(params.maxChange));
  if (params.session) p.set('session', params.session);
  if (params.sort) p.set('sort', params.sort);
  if (params.limit != null) p.set('limit', String(params.limit));
  const q = p.toString();
  return getJson<ScreenResponse>(`/v1/screen${q ? `?${q}` : ''}`, signal);
}

/** Envelope returned by `GET /v1/universe/symbols`. */
export interface UniverseSymbolsResponse {
  symbols: string[];
  count: number;
}

/**
 * The quote-bearing US ticker universe (~6,700 — every symbol the server has a
 * live price for; a strict subset of `/v1/symbols`' full ~16k SEC+Nasdaq
 * index). Public, no auth. Used by the pSEO sitemap to seed a content-bearing
 * `/stock/{t}` page per name (unlike `/v1/screen`, it is not capped at 200).
 */
export async function getUniverseSymbols(signal?: AbortSignal): Promise<string[]> {
  // The A–Z directory builds ~53 routes (26 letters × 2 locales + the hub) in one run, and
  // each needs this SAME ~8.7k-symbol universe. Route it through Next's Data Cache
  // (`revalidate` 1h) so those builds DEDUPE to a single network call — otherwise each fires
  // a cold ~59 KB request through the Cloudflare tunnel, the concurrent storm times out, and
  // the pages bake EMPTY (the bug behind the empty directory). `next` is ignored in the
  // browser, where the self-heal passes its own short-timeout signal (uncached, fine).
  const init = {next: {revalidate: 3600}} as RequestInit;
  const r = await getJsonWithRetry<UniverseSymbolsResponse>('/v1/universe/symbols', signal, null, init);
  return r.symbols ?? [];
}

/**
 * One indicator-catalog record, as returned by `GET /v1/indicators`. Mirrors the
 * backend dataset schema (see docs/indicators/SPEC.md). All fields are English
 * except `name_zh`. `formula` is shown verbatim (math/symbols preserved).
 */
export interface Indicator {
  id: string;
  /** `technical` | `fundamental` | `sentiment` (onchain is crypto-only, excluded). */
  domain: string;
  domain_name: string;
  subcategory: string;
  /** `P0` (MVP core) | `P1` (advanced) | `P2` (pro/long-tail). */
  priority: string;
  /** `stock` | `both` (crypto-only indicators are excluded server-side). */
  applies_to: string;
  name_en: string;
  name_zh: string;
  abbr: string;
  definition: string;
  formula: string;
  inputs?: string[] | null;
  /** Suggested default parameters; shape varies per indicator. */
  default_params?: unknown;
  /** TA-Lib function or library hint, where one exists. */
  talib_or_lib?: string;
  /** Render/served shape hint: overlay | oscillator | volume | ratio/value | … */
  output_type?: string;
  data_source: string;
  interpretation: string;
}

/** A value and how many catalog records carry it (for filter chips). */
export interface IndicatorFacet {
  value: string;
  count: number;
}

/** Facet counts over the whole stock-applicable catalog. */
export interface IndicatorFacets {
  domains: IndicatorFacet[];
  priorities: IndicatorFacet[];
  subcategories: IndicatorFacet[];
}

/** Envelope returned by `GET /v1/indicators`. */
export interface IndicatorsResponse {
  /** Number of indicators after filtering. */
  count: number;
  /** Total stock-applicable indicators in the catalog (unfiltered). */
  total: number;
  indicators: Indicator[];
  facets: IndicatorFacets;
}

/** Filters accepted by the indicator catalog (all optional). */
export interface IndicatorParams {
  domain?: string;
  priority?: string;
  subcategory?: string;
  /** Free-text query matched against names, abbreviation, and definition. */
  q?: string;
}

/**
 * Fetches the stock-applicable indicator catalog (static, embedded metadata).
 * Only non-empty filters are sent. Public endpoint — used by the `/indicators`
 * library page for SSR; client-side filtering then works over the embedded set.
 */
export function getIndicators(
  params: IndicatorParams = {},
  signal?: AbortSignal,
): Promise<IndicatorsResponse> {
  const p = new URLSearchParams();
  if (params.domain) p.set('domain', params.domain);
  if (params.priority) p.set('priority', params.priority);
  if (params.subcategory) p.set('subcategory', params.subcategory);
  if (params.q) p.set('q', params.q);
  const q = p.toString();
  if (q === '') {
    return cachedFullCatalog(signal); // the hot path: the full ~150KB catalog
  }
  return getJson<IndicatorsResponse>(`/v1/indicators?${q}`, signal);
}

// In-flight coalescing + Data Cache for the FULL indicator catalog (~150 KB). The library page
// AND every /indicators/[id] detail page (564 routes) fetch it at build; without this they each
// fire a cold 150 KB request and the concurrent storm times out → pages bake as the loading
// fallback (no content, fallback metadata) — exactly the universe/directory bug, here for the
// indicator catalog. A shared promise collapses one worker's burst to a single fetch; `next`
// caches it (the catalog is static, changes only on a dataset deploy) so it stays reliable.
let catalogInFlight: Promise<IndicatorsResponse> | null = null;
function cachedFullCatalog(signal?: AbortSignal): Promise<IndicatorsResponse> {
  if (catalogInFlight) return catalogInFlight;
  const init = {next: {revalidate: 86400}} as RequestInit;
  const promise = getJsonWithRetry<IndicatorsResponse>('/v1/indicators', signal, null, init);
  catalogInFlight = promise;
  void promise.finally(() => {
    if (catalogInFlight === promise) catalogInFlight = null;
  });
  return promise;
}

/**
 * Maps an indicator id to its clean, extension-safe URL slug for the
 * `/indicators/{slug}` pSEO pages: the dotted id has its `.` replaced by `-`
 * (e.g. `technical.rsi` → `technical-rsi`, `fundamental.pe-ttm` →
 * `fundamental-pe-ttm`). The raw id stays the stable key; a slug is never
 * reverse-engineered by un-replacing `-` (ids may already contain hyphens, e.g.
 * `pe-ttm`) — resolve a slug back to a record via {@link indicatorBySlug}, which
 * slugifies every catalog id and compares.
 */
export function indicatorSlug(id: string): string {
  return id.replace(/\./g, '-');
}

/**
 * Finds the catalog record whose slugified id equals `slug`, by slugifying every
 * id and comparing (NOT a naive un-replace, since ids can contain hyphens). The
 * `.`→`-` map is collision-free across the current 282-record catalog. Returns
 * `undefined` when no record matches (the caller then 404s).
 */
export function indicatorBySlug(indicators: Indicator[], slug: string): Indicator | undefined {
  return indicators.find(ind => indicatorSlug(ind.id) === slug);
}

/** One dated value in an indicator series (Phase 1 returns latest values only). */
export interface IndicatorPoint {
  date: string;
  value: number;
}

/**
 * A computed indicator for one stock: the embedded catalog metadata
 * ({@link Indicator}) plus the latest computed result. `status` is `ok` when a
 * headline {@link value} is present, `insufficient` when the inputs are missing
 * (render as "—"), or `unsupported` when the formula can't be computed here.
 */
export interface StockIndicator extends Indicator {
  status: 'ok' | 'insufficient' | 'unsupported';
  /** Why the indicator is not `ok`; absent when `ok`. */
  reason?: string;
  /** Headline scalar; present only when `status === 'ok'`. */
  value?: number;
  /** Display unit: `%` | `ratio` | `price` | `x` | `` (empty). */
  unit?: string;
  /** Extra lines for multi-line indicators (e.g. MACD: signal, hist). */
  extra?: Record<string, number>;
}

/** The market backdrop returned alongside per-stock indicators. */
export interface MarketContext {
  /** CBOE Volatility Index, when available. */
  vix?: number;
  /** CNN-style Fear & Greed gauge, when available. */
  fear_greed?: {score: number; label: string};
}

/** Envelope returned by `GET /v1/stocks/{ticker}/indicators`. */
export interface StockIndicatorsResponse {
  ticker: string;
  /** Newest underlying data date (may be empty). */
  as_of: string;
  /** Market backdrop; omitted when neither VIX nor Fear & Greed is available. */
  market_context?: MarketContext;
  /** The P0 stock-applicable set: ok first, then insufficient, then unsupported. */
  indicators: StockIndicator[];
}

/**
 * Fetches the latest computed indicators for a ticker. Resolves to `null` when
 * the symbol is unknown / has no data (the API 404s), so callers can hide the
 * panel; other errors reject.
 */
export async function getStockIndicators(
  ticker: string,
  signal?: AbortSignal,
): Promise<StockIndicatorsResponse | null> {
  try {
    return await getJson<StockIndicatorsResponse>(
      `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/indicators`,
      signal,
    );
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return null;
    throw e;
  }
}

/**
 * One deterministic posture signal derived from a Go-computed indicator. It is NOT
 * advice / a price target / a rating — it states a disclosed technical condition, and
 * `basis` cites the source indicator + value + threshold so it is fully traceable to a
 * number Go computed (never LLM-invented).
 */
export interface IndicatorSignal {
  /** Source indicator id, e.g. `"technical.rsi"`. */
  id: string;
  /** Human label, e.g. `"RSI oversold"`. */
  label: string;
  /** Posture direction. */
  direction: 'bullish' | 'bearish' | 'neutral';
  /** Traceability, e.g. `"RSI 27.4 < 30"`. */
  basis: string;
}

/** Envelope returned by `GET /v1/stocks/{ticker}/indicator-signals`. */
export interface IndicatorSignalsResponse {
  ticker: string;
  /** Newest underlying data date (may be empty). */
  as_of: string;
  /** The deterministic signals (a teaser when `paywall_locked`). */
  signals: IndicatorSignal[];
  /** Full signal count — drives the "unlock N more with Pro" CTA when truncated. */
  total_signals: number;
  /** True when the free-tier paywall truncated the list to a teaser. */
  paywall_locked?: boolean;
}

/**
 * Fetches the deterministic posture-signals for a ticker. Resolves to `null` when the
 * symbol is unknown / has no data (404), so callers can hide the card. Pass the user's
 * access token so a Pro viewer gets the full list once the signals paywall is live.
 */
export async function getIndicatorSignals(
  ticker: string,
  token?: string | null,
  signal?: AbortSignal,
): Promise<IndicatorSignalsResponse | null> {
  try {
    return await getJson<IndicatorSignalsResponse>(
      `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/indicator-signals`,
      signal,
      token,
    );
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return null;
    throw e;
  }
}

/** One screened stock: its ticker + the signals that matched the screen query. */
export interface SignalMatch {
  ticker: string;
  signals: IndicatorSignal[];
}

/** Envelope returned by `GET /v1/screen/signals` (the deterministic signals screener). */
export interface ScreenSignalsResponse {
  count: number;
  results: SignalMatch[];
  /** Full match count — drives the "N more with Pro" CTA when the teaser is truncated. */
  total_matches?: number;
  /** When the background scan was built (RFC3339); absent before the first scan. */
  as_of?: string;
  /** True when the screener is Pro-gated and the viewer is not Pro (results are a teaser). */
  paywall_locked?: boolean;
}

/**
 * Screens the whole universe for stocks whose deterministic signals match. Filters are
 * optional and AND-ed (e.g. signal=`technical.ma-cross` + direction=`bullish` → golden
 * crosses). Resolves to an empty result when the endpoint is unavailable (404), so the
 * page degrades gracefully. Pass the user token so a Pro viewer gets full results once
 * the screener paywall is live.
 */
export async function getScreenSignals(
  params: {direction?: string; signal?: string; limit?: number},
  token?: string | null,
  abort?: AbortSignal,
): Promise<ScreenSignalsResponse> {
  const q = new URLSearchParams();
  if (params.direction) q.set('direction', params.direction);
  if (params.signal) q.set('signal', params.signal);
  if (params.limit) q.set('limit', String(params.limit));
  const qs = q.toString();
  try {
    return await getJson<ScreenSignalsResponse>(
      `/v1/screen/signals${qs ? `?${qs}` : ''}`,
      abort,
      token,
    );
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return {count: 0, results: []};
    throw e;
  }
}

/**
 * Backtest of a signal rule on one ticker: a disclosed HISTORICAL statistic (win rate
 * / average forward return over `horizon` trading days / trade count / buy-and-hold
 * baseline), NOT a prediction or advice.
 */
export interface SignalBacktestResult {
  rule: string;
  horizon: number;
  trades: number;
  wins: number;
  win_rate: number; // 0..1
  avg_return: number; // %
  baseline: number; // buy-and-hold over the span, %
}

/** Envelope returned by `GET /v1/stocks/{ticker}/backtest`. */
export interface SignalBacktestResponse {
  ticker: string;
  result?: SignalBacktestResult;
  /** True when the screener is Pro-gated and the viewer is not Pro (hard-locked). */
  paywall_locked?: boolean;
}

/**
 * Backtests a signal rule over a ticker's daily candles. Resolves to `null` when the
 * ticker has no price history / not enough to backtest (404/422), so the widget can
 * show a graceful message. Pass the user token so a Pro viewer gets results once the
 * paywall is live.
 */
export async function getBacktest(
  ticker: string,
  rule: string,
  horizon: number,
  token?: string | null,
  abort?: AbortSignal,
): Promise<SignalBacktestResponse | null> {
  const q = new URLSearchParams({rule, horizon: String(horizon)});
  try {
    return await getJson<SignalBacktestResponse>(
      `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/backtest?${q.toString()}`,
      abort,
      token,
    );
  } catch (e) {
    if (e instanceof ApiError && (e.status === 404 || e.status === 422)) return null;
    throw e;
  }
}

/** One dated point of an indicator's time series. */
export interface IndicatorHistoryPoint {
  date: string; // YYYY-MM-DD
  value: number;
}

/**
 * One technical indicator computed across a ticker's full daily history — the
 * date-aligned line a chart draws (GET /v1/stocks/{ticker}/indicator-history).
 * `points` is the primary line; `lines` carries the extra aligned bands
 * (MACD signal/histogram, Bollinger upper/lower). Every value is Go-computed —
 * the chart's latest point equals the single-point indicator value.
 */
export interface IndicatorHistory {
  indicator: string;
  period?: number;
  unit: string; // % | price | ratio | x | usd | ""
  points: IndicatorHistoryPoint[];
  lines?: Record<string, IndicatorHistoryPoint[]>;
}

/** Indicator ids that have a server-side time-series history (mirrors indicators.HistoryableID). */
export const HISTORYABLE_INDICATOR_IDS = [
  'technical.sma-ma',
  'technical.ema',
  'technical.rsi',
  'technical.macd',
  'technical.boll',
  'technical.atr',
  'technical.stochastic-kdj',
] as const;

/**
 * Fetches one indicator's time series for charting. Resolves to `null` on any
 * 4xx/network error (unsupported id / no history / too short) so the chart can
 * degrade gracefully. Public, no auth (mirrors the free single-point indicators).
 */
export async function getIndicatorHistory(
  ticker: string,
  id: string,
  period?: number,
  signal?: AbortSignal,
): Promise<IndicatorHistory | null> {
  const q = new URLSearchParams({id});
  if (period && period > 0) q.set('period', String(period));
  try {
    const r = await getJson<{ticker: string; history?: IndicatorHistory}>(
      `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/indicator-history?${q.toString()}`,
      signal,
    );
    return r.history ?? null;
  } catch {
    return null;
  }
}

/** One calendar month's historical return stats (GET /v1/stocks/{t}/seasonality). */
export interface SeasonStat {
  month: number; // 1..12
  avg_return: number; // mean MoM % return
  median_return: number;
  win_rate: number; // 0..1
  years: number;
}

/** A ticker's month-of-year return seasonality — a disclosed historical statistic (Go-computed). */
export interface Seasonality {
  months: SeasonStat[];
  from_year: number;
  to_year: number;
  samples: number;
}

/**
 * Fetches a ticker's month-of-year seasonality. Resolves to `null` on any 4xx/network error
 * (no history / too short) so the card can hide gracefully. Public, no auth.
 */
export async function getSeasonality(ticker: string, signal?: AbortSignal): Promise<Seasonality | null> {
  try {
    const r = await getJson<{ticker: string; seasonality?: Seasonality}>(
      `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/seasonality`,
      signal,
    );
    return r.seasonality ?? null;
  } catch {
    return null;
  }
}

/** One trailing window of a stock's performance vs the benchmark (excess return, Go-computed). */
export interface RelStrengthWindow {
  label: string; // "1M","3M","6M","1Y"
  stock_return: number; // % over the window
  benchmark_return: number; // benchmark % over the same span
  relative: number; // stock_return - benchmark_return (excess, pp)
}

/** A ticker's trailing relative strength vs SPY — a disclosed historical statistic (Go-computed). */
export interface RelativeStrength {
  benchmark: string;
  as_of: string;
  windows: RelStrengthWindow[];
}

/**
 * Fetches a ticker's trailing relative strength vs SPY. Resolves to `null` on any 4xx/network
 * error (no history / too short / ticker is the benchmark) so the card can hide gracefully.
 * Public, no auth.
 */
export async function getRelativeStrength(
  ticker: string,
  signal?: AbortSignal,
): Promise<RelativeStrength | null> {
  try {
    const r = await getJson<{ticker: string; relative_strength?: RelativeStrength}>(
      `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/relative-strength`,
      signal,
    );
    return r.relative_strength ?? null;
  } catch {
    return null;
  }
}

/** One past earnings announcement + the stock's ~2-session reaction around it (Go-computed). */
export interface EarningsEvent {
  date: string; // YYYY-MM-DD (8-K item 2.02 filing date)
  move: number; // ~2-session close-to-close % around the announcement
}

/** A ticker's earnings-reaction history — a disclosed historical statistic (Go-computed). */
export interface EarningsReaction {
  events: EarningsEvent[]; // most recent first
  avg_move: number; // mean signed reaction %
  avg_abs_move: number; // mean magnitude % (typical size)
  up_rate: number; // fraction of events positive (0..1)
  samples: number;
}

/**
 * Fetches a ticker's earnings-reaction history. Resolves to `null` on any 4xx/network error
 * (no history / too few reports / no earnings filings) so the card can hide gracefully.
 * Public, no auth.
 */
export async function getEarningsReaction(
  ticker: string,
  signal?: AbortSignal,
): Promise<EarningsReaction | null> {
  try {
    const r = await getJson<{ticker: string; earnings_reaction?: EarningsReaction}>(
      `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/earnings-reaction`,
      signal,
    );
    return r.earnings_reaction ?? null;
  } catch {
    return null;
  }
}

/**
 * A stock's dividend profile (Go-computed from SEC annual figures + the live price). Every field is
 * descriptive — there is no "dividend-safety grade". Fields are absent when not computable; the whole
 * object is null for a non-payer.
 */
export interface DividendView {
  yield?: number; // %
  payout_ratio?: number; // %
  dps?: number; // $ per share
  fcf_coverage?: number; // x (free cash flow / dividends)
  yoy_growth?: number; // % YoY change in dividends paid
  period?: string; // fiscal year of the annual figures
}

/**
 * Fetches a ticker's dividend profile. Resolves to `null` on any 4xx/network error (a non-payer or no
 * fundamentals) so the card hides gracefully. Public, no auth.
 */
export async function getDividend(ticker: string, signal?: AbortSignal): Promise<DividendView | null> {
  try {
    const r = await getJson<{ticker: string; dividend?: DividendView}>(
      `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/dividend`,
      signal,
    );
    return r.dividend ?? null;
  } catch {
    return null;
  }
}

/** One factor's standing: a 0..100 percentile vs the tracked universe + how many sub-metrics fed it. */
export interface FactorScore {
  percentile: number;
  inputs: number;
}

/**
 * A stock's four INDEPENDENT factor percentiles vs Tickwind's tracked universe (Go-computed).
 * Descriptive only — there is deliberately no blended composite score and no rating.
 */
export interface Scorecard {
  value?: FactorScore;
  growth?: FactorScore;
  quality?: FactorScore;
  momentum?: FactorScore;
  population: number;
}

/** One stock's standing on a single factor in the market-wide leaderboard (Go-computed percentile). */
export interface FactorRank {
  ticker: string;
  percentile: number;
  inputs: number;
}

/** Envelope returned by `GET /v1/screen/factors`. */
export interface FactorScreenResponse {
  factor: string;
  count: number;
  results: FactorRank[];
  /** How many tracked stocks had a computable percentile for this factor (the leaderboard's denominator). */
  population: number;
  as_of?: string;
}

/**
 * The market-wide FACTOR LEADERBOARD: every tracked stock ranked by one factor's percentile
 * (value | growth | quality | momentum) vs the whole tracked universe. Each percentile is Go-computed
 * by the same scorecard path the per-stock card uses, as of the population's build time (`as_of`).
 * Descriptive only — no rating/advice. Public, no auth. Throws on a non-2xx/network error (callers
 * SSR-fetch with a try/catch → empty state).
 */
export function getFactorScreen(
  factor: string,
  limit?: number,
  signal?: AbortSignal,
): Promise<FactorScreenResponse> {
  const p = new URLSearchParams({factor});
  if (limit != null) p.set('limit', String(limit));
  return getJson<FactorScreenResponse>(`/v1/screen/factors?${p.toString()}`, signal);
}

/** One stock's standing on the relative-strength leaderboard: excess return vs SPY + the two legs. */
export interface RSRank {
  ticker: string;
  relative: number; // excess return (stock − benchmark), percentage points
  stock_return: number; // %
  benchmark_return: number; // %
}

/** Envelope returned by `GET /v1/screen/relative-strength`. */
export interface RSScreenResponse {
  window: string; // "1M" | "3M" | "6M" | "1Y"
  count: number;
  results: RSRank[];
  total: number; // how many tracked stocks had a computable excess return for this window
  as_of?: string;
}

/**
 * The market-wide RELATIVE-STRENGTH leaderboard: every tracked stock ranked by its trailing excess
 * return vs SPY over the window (1M | 3M | 6M | 1Y). Go-computed historical excess returns —
 * descriptive, no advice. Public, no auth. Throws on a non-2xx/network error (callers client-fetch
 * with a try/catch → empty/loading state).
 */
export function getRSScreen(window: string, limit?: number, signal?: AbortSignal): Promise<RSScreenResponse> {
  const p = new URLSearchParams({window});
  if (limit != null) p.set('limit', String(limit));
  return getJson<RSScreenResponse>(`/v1/screen/relative-strength?${p.toString()}`, signal);
}

/** One stock's standing on the earnings-reaction leaderboard: the typical move around its past
 * earnings + how often it rose, with the sample count (the disclosed basis). */
export interface ReactionRank {
  ticker: string;
  avg_abs_move: number; // mean magnitude % of the ~2-session move around earnings
  up_rate: number; // fraction of past earnings with a positive reaction (0..1)
  samples: number; // number of measurable past earnings reactions (>= 4)
}

/** Envelope returned by `GET /v1/screen/earnings-reaction`. */
export interface ERScreenResponse {
  view: string; // "most-volatile" | "highest-up-rate"
  count: number;
  results: ReactionRank[];
  total: number; // how many tracked stocks had a measurable earnings-reaction aggregate
  as_of?: string;
}

/**
 * The market-wide EARNINGS-REACTION leaderboard: every tracked stock ranked by how it has historically
 * moved around its earnings (view = most-volatile | highest-up-rate). Go-computed historical statistics
 * (typical ~2-session move size / up-rate over past earnings), each disclosed with its sample count —
 * descriptive, no advice/forecast. Public, no auth. Throws on a non-2xx/network error (callers
 * client-fetch with a try/catch → empty/loading state).
 */
export function getEarningsReactionScreen(
  view: string,
  limit?: number,
  signal?: AbortSignal,
): Promise<ERScreenResponse> {
  const p = new URLSearchParams({view});
  if (limit != null) p.set('limit', String(limit));
  return getJson<ERScreenResponse>(`/v1/screen/earnings-reaction?${p.toString()}`, signal);
}

/** One stock's standing on the dividend leaderboard: its full Go-computed dividend profile (so the UI
 * can show the ranked metric + context regardless of which view ranked it). Metrics are absent when
 * not computable. */
export interface DividendRank {
  ticker: string;
  yield?: number; // %
  payout_ratio?: number; // %
  dps?: number; // $ per share
  fcf_coverage?: number; // x (free cash flow / dividends)
  yoy_growth?: number; // % YoY change in dividends paid
  period?: string; // fiscal year of the annual figures
}

/** Envelope returned by `GET /v1/screen/dividends`. */
export interface DividendScreenResponse {
  view: string; // "highest-yield" | "fastest-growing" | "best-covered" | "lowest-payout"
  count: number;
  results: DividendRank[];
  total: number; // how many tracked payers had a computable metric for this view
  as_of?: string;
}

/**
 * The market-wide DIVIDEND leaderboard: every tracked payer ranked by one dividend metric (view =
 * highest-yield | fastest-growing | best-covered | lowest-payout). Go-computed descriptive figures
 * (yield/payout/coverage/growth) from SEC-filed annual figures + the live price — no "dividend-safety
 * grade"/rating/advice. Public, no auth. Throws on a non-2xx/network error (callers client-fetch with a
 * try/catch → empty/loading state).
 */
export function getDividendScreen(
  view: string,
  limit?: number,
  signal?: AbortSignal,
): Promise<DividendScreenResponse> {
  const p = new URLSearchParams({view});
  if (limit != null) p.set('limit', String(limit));
  return getJson<DividendScreenResponse>(`/v1/screen/dividends?${p.toString()}`, signal);
}

/** One parsed 8-K item code with its canonical Go-owned label. */
export interface MaterialEventItem {
  code: string;
  label_en: string;
  label_zh: string;
}

/** One notable 8-K on the market-wide material-events feed: the ticker + the Go-owned event facts
 * (form, dates, SEC filing link, and the filtered NOTABLE item codes). Facts only — no LLM. */
export interface MaterialFeedEvent {
  ticker: string;
  form: string;
  filed_date: string;
  report_date?: string;
  accession_url: string;
  items: MaterialEventItem[];
}

/** Envelope returned by `GET /v1/material-feed`. */
export interface MaterialFeedResponse {
  count: number;
  events: MaterialFeedEvent[];
  as_of?: string;
}

/**
 * The market-wide NOTABLE material-events feed: recent high-signal 8-K filings (leadership change,
 * M&A, bankruptcy, restatement, …) across the tracked universe, newest first. An optional 8-K item
 * code filters to one category (e.g. "5.02"). Go-owned facts + canonical labels — no LLM, no advice.
 * Public, no auth. Throws on a non-2xx/network error (callers client-fetch with a try/catch → empty
 * state).
 */
export function getMaterialFeed(item?: string, limit?: number, signal?: AbortSignal): Promise<MaterialFeedResponse> {
  const p = new URLSearchParams();
  if (item) p.set('item', item);
  if (limit != null) p.set('limit', String(limit));
  const q = p.toString();
  return getJson<MaterialFeedResponse>(`/v1/material-feed${q ? `?${q}` : ''}`, signal);
}

/**
 * Fetches a ticker's multi-factor scorecard (percentiles vs the tracked universe). Resolves to
 * `null` on any 4xx/network error (no fundamentals / population not ready) so the card can hide
 * gracefully. Returns the populationAsOf alongside for vintage disclosure. Public, no auth.
 */
export async function getScorecard(
  ticker: string,
  signal?: AbortSignal,
): Promise<{scorecard: Scorecard; populationAsOf: string} | null> {
  try {
    const r = await getJson<{ticker: string; population_as_of?: string; scorecard?: Scorecard}>(
      `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/scorecard`,
      signal,
    );
    return r.scorecard ? {scorecard: r.scorecard, populationAsOf: r.population_as_of ?? ''} : null;
  } catch {
    return null;
  }
}

// ── Per-user prefs (generic JSON UI-state blob) ──────────────────────────
//
// A small, generic per-user JSON-prefs surface. The blob is namespaced by
// top-level key (`{"indicators":{"ids":[...]}}`); the backend shallow-merges a
// PUT's top-level keys, so a client that only writes `indicators` never
// clobbers a future sibling pref. GET returns `{}` when the user has none.

/** The opaque per-user prefs blob. The indicators client owns the `indicators` key. */
export interface PrefsBlob {
  indicators?: {ids?: string[]};
  /** AI chat "Use my data" toggle. Absent → default ON; false → chat won't read the user's watchlist/holdings/notes. */
  chat_personal_data?: boolean;
  // Future sibling pref keys slot in here without a migration.
  [key: string]: unknown;
}

/**
 * Fetches the caller's stored prefs blob. Returns `{}` when none are stored
 * (the caller then falls back to localStorage / the default). Requires a token.
 */
export function getMyPrefs(
  token: string | null,
  signal?: AbortSignal,
): Promise<PrefsBlob> {
  return getJson<PrefsBlob>('/v1/me/prefs', signal, token);
}

/**
 * Shallow-merges the given top-level keys into the caller's stored prefs (the
 * backend merges and persists, returning 204 with no body). Pass only the keys
 * you own (e.g. `{indicators: {ids}}`). Requires a token.
 */
export async function putMyPrefs(
  token: string | null,
  blob: PrefsBlob,
  signal?: AbortSignal,
): Promise<void> {
  let res: Response;
  try {
    res = await fetch(`${API_BASE}/v1/me/prefs`, {
      method: 'PUT',
      headers: authHeaders(
        {'Content-Type': 'application/json', Accept: 'application/json'},
        token,
      ),
      body: JSON.stringify(blob),
      signal,
    });
  } catch {
    throw new ApiError(`network error contacting ${API_BASE}/v1/me/prefs`, 0);
  }
  if (!res.ok) {
    let detail = res.statusText;
    try {
      const data = (await res.json()) as ApiErrorBody;
      if (data.error) detail = data.error;
    } catch {
      // Non-JSON / empty error body; fall back to the status text.
    }
    throw new ApiError(detail, res.status);
  }
  // 204 No Content — nothing to parse.
}

/** A link a user saved to a ticker (private, per-user). */
export interface Clip {
  id: string;
  user_id: string;
  ticker: string;
  /** Page title fetched by the backend at save time. */
  title: string;
  url: string;
  /** RFC 3339 timestamp of when the clip was saved. */
  created_at: string;
}

/** Envelope returned by `GET /v1/stocks/{ticker}/clips`. */
export interface ClipsResponse {
  ticker: string;
  count: number;
  clips: Clip[];
}

/**
 * Saves a pasted link to a ticker (the "clipper"). The backend fetches the
 * page title and stores it as a private {@link Clip}, which is returned.
 * Requires authentication.
 */
export function clipLink(
  token: string | null,
  ticker: string,
  url: string,
  signal?: AbortSignal,
): Promise<Clip> {
  return postJson<Clip>(
    `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}/clip`,
    {url},
    signal,
    token,
  );
}

/** Fetches the caller's saved clips for a ticker. Requires authentication. */
export function getClips(
  token: string | null,
  ticker: string,
  limit = 50,
  signal?: AbortSignal,
): Promise<ClipsResponse> {
  const path =
    `/v1/stocks/${encodeURIComponent(normalizeTicker(ticker))}` +
    `/clips?limit=${encodeURIComponent(String(limit))}`;
  return getJson<ClipsResponse>(path, signal, token);
}

/** A user's private note/opinion (stock- and/or calendar-date-scoped). */
export interface Note {
  id: string;
  user_id: string;
  ticker?: string;
  /** "YYYY-MM-DD"; absent when undated. */
  note_date?: string;
  body: string;
  pinned: boolean;
  created_at: string;
  updated_at: string;
}

/** Envelope returned by `GET /v1/notes`. */
export interface NotesResponse {
  count: number;
  notes: Note[];
}

/** Lists the caller's notes — by ticker, by [from,to] date range, or all. */
export function getNotes(
  token: string | null,
  opts: {ticker?: string; from?: string; to?: string; limit?: number} = {},
  signal?: AbortSignal,
): Promise<NotesResponse> {
  const q = new URLSearchParams();
  if (opts.ticker) q.set('ticker', normalizeTicker(opts.ticker));
  if (opts.from) q.set('from', opts.from);
  if (opts.to) q.set('to', opts.to);
  q.set('limit', String(opts.limit ?? 200));
  return getJson<NotesResponse>(`/v1/notes?${q.toString()}`, signal, token);
}

/** Creates a note (stock- and/or date-scoped). Requires authentication. */
export function createNote(
  token: string | null,
  input: {ticker?: string; note_date?: string; body: string; pinned?: boolean},
  signal?: AbortSignal,
): Promise<Note> {
  return postJson<Note>('/v1/notes', input, signal, token);
}

/** Edits a note's body and/or pinned flag. Requires authentication. */
export function updateNote(
  token: string | null,
  id: string,
  patch: {body?: string; pinned?: boolean},
  signal?: AbortSignal,
): Promise<Note> {
  return patchJson<Note>(`/v1/notes/${encodeURIComponent(id)}`, patch, signal, token);
}

/** Deletes a note. Requires authentication. */
export function deleteNote(
  token: string | null,
  id: string,
  signal?: AbortSignal,
): Promise<{deleted: boolean}> {
  return deleteJson<{deleted: boolean}>(
    `/v1/notes/${encodeURIComponent(id)}`,
    signal,
    token,
  );
}

/** A per-user price/event alert on a ticker. */
export interface Alert {
  id: string;
  user_id: string;
  ticker: string;
  kind: string; // price_above | price_below | pct_move | new_filing
  threshold: number;
  active: boolean;
  created_at: string;
  triggered_at?: string; // set once the evaluator fires it
}

/** Envelope returned by `GET /v1/alerts`. */
export interface AlertsResponse {
  count: number;
  alerts: Alert[];
}

/** Lists the caller's alerts. Requires authentication. */
export function getAlerts(token: string | null, signal?: AbortSignal): Promise<AlertsResponse> {
  return getJson<AlertsResponse>('/v1/alerts', signal, token);
}

/** Creates a price/event alert. Requires authentication. */
export function createAlert(
  token: string | null,
  input: {ticker: string; kind: string; threshold: number},
  signal?: AbortSignal,
): Promise<Alert> {
  return postJson<Alert>('/v1/alerts', input, signal, token);
}

/** Deletes an alert. Requires authentication. */
export function deleteAlert(
  token: string | null,
  id: string,
  signal?: AbortSignal,
): Promise<{deleted: boolean}> {
  return deleteJson<{deleted: boolean}>(`/v1/alerts/${encodeURIComponent(id)}`, signal, token);
}

/** Re-arms a triggered alert (active again, trigger cleared). Requires auth. */
export function reactivateAlert(
  token: string | null,
  id: string,
  signal?: AbortSignal,
): Promise<{reactivated: boolean}> {
  return patchJson<{reactivated: boolean}>(
    `/v1/alerts/${encodeURIComponent(id)}`,
    {},
    signal,
    token,
  );
}

/** A user's position in a ticker (shares + average cost). */
export interface Holding {
  id: string;
  user_id: string;
  ticker: string;
  shares: number;
  avg_cost: number;
  created_at: string;
  updated_at: string;
}

/** Envelope returned by `GET /v1/holdings`. */
export interface HoldingsResponse {
  count: number;
  holdings: Holding[];
}

/** Lists the caller's holdings. Requires authentication. */
export function getHoldings(token: string | null, signal?: AbortSignal): Promise<HoldingsResponse> {
  return getJson<HoldingsResponse>('/v1/holdings', signal, token);
}

/** Creates or updates (upsert by ticker) a holding. Requires authentication. */
export function createHolding(
  token: string | null,
  input: {ticker: string; shares: number; avg_cost: number},
  signal?: AbortSignal,
): Promise<Holding> {
  return postJson<Holding>('/v1/holdings', input, signal, token);
}

/** Deletes a holding. Requires authentication. */
export function deleteHolding(
  token: string | null,
  id: string,
  signal?: AbortSignal,
): Promise<{deleted: boolean}> {
  return deleteJson<{deleted: boolean}>(`/v1/holdings/${encodeURIComponent(id)}`, signal, token);
}

/** A public user comment on a stock or the global community board. */
export interface Comment {
  id: string;
  user_id: string;
  author: string;
  ticker?: string;
  body: string;
  created_at: string;
  /** Set when the author has edited the comment. */
  edited_at?: string;
  /** Total like count (per-user deduped). */
  likes: number;
  /** Whether the requesting (authenticated) viewer has liked it. */
  liked?: boolean;
  /** $TICKER cashtags extracted from the body — the comment also appears in
   *  each mentioned stock's comment list. */
  mentions?: string[];
}

/** Envelope returned by `GET /v1/comments`. */
export interface CommentsResponse {
  ticker: string;
  count: number;
  comments: Comment[];
}

/** Lists public comments for a ticker, or the global board when ticker is empty. Public (no auth). */
export function getComments(
  ticker: string,
  limit = 100,
  signal?: AbortSignal,
): Promise<CommentsResponse> {
  const q = new URLSearchParams();
  if (ticker) q.set('ticker', normalizeTicker(ticker));
  q.set('limit', String(limit));
  return getJson<CommentsResponse>(`/v1/comments?${q.toString()}`, signal);
}

/** Posts a public comment (stock-scoped when ticker is set). Requires authentication. */
export function postComment(
  token: string | null,
  input: {ticker?: string; body: string},
  signal?: AbortSignal,
): Promise<Comment> {
  return postJson<Comment>('/v1/comments', input, signal, token);
}

/** Deletes a comment (author or admin). Requires authentication. */
export function deleteComment(
  token: string | null,
  id: string,
  signal?: AbortSignal,
): Promise<{deleted: boolean}> {
  return deleteJson<{deleted: boolean}>(
    `/v1/comments/${encodeURIComponent(id)}`,
    signal,
    token,
  );
}

/** Edits the caller's own comment (body only). Requires authentication. */
export function updateComment(
  token: string | null,
  id: string,
  body: string,
  signal?: AbortSignal,
): Promise<Comment> {
  return patchJson<Comment>(`/v1/comments/${encodeURIComponent(id)}`, {body}, signal, token);
}

/** Toggles the caller's like on a comment, returning the new state + count. Requires auth. */
export function likeComment(
  token: string | null,
  id: string,
  signal?: AbortSignal,
): Promise<{liked: boolean; likes: number}> {
  return postJson<{liked: boolean; likes: number}>(
    `/v1/comments/${encodeURIComponent(id)}/like`,
    {},
    signal,
    token,
  );
}

/** Flags a comment for moderation. Requires authentication. */
export function reportComment(
  token: string | null,
  id: string,
  signal?: AbortSignal,
): Promise<{reported: boolean}> {
  return postJson<{reported: boolean}>(
    `/v1/comments/${encodeURIComponent(id)}/report`,
    {},
    signal,
    token,
  );
}

/**
 * One offering on the US IPO calendar. Numeric-looking fields are the source's
 * display strings (Nasdaq returns them pre-formatted, may be ranges or empty);
 * render them as-is. `kind` mirrors the calendar section.
 */
export interface IPO {
  ticker: string;
  company: string;
  exchange: string;
  price: string; // proposed/priced share price, e.g. "$18.00"
  shares: string; // shares offered, e.g. "10,000,000"
  amount: string; // dollar value of shares offered
  date: string; // priced date or expected price date (source-formatted)
  status: string; // deal status, when provided
  kind: 'priced' | 'upcoming' | 'filed';
}

/** Envelope returned by `GET /v1/ipo` — the US IPO calendar, by section. */
export interface IPOCalendarResponse {
  priced: IPO[];
  upcoming: IPO[];
  filed: IPO[];
  updated_at: string; // RFC 3339
}

/**
 * Fetches the US IPO calendar (recently priced, upcoming, and newly-filed
 * offerings) from Nasdaq. Public endpoint; each section is `[]` until the data
 * source is ready. Delayed / display-only — not investment advice.
 */
export function getIPO(signal?: AbortSignal): Promise<IPOCalendarResponse> {
  return getJson<IPOCalendarResponse>('/v1/ipo', signal);
}

/** One watchlist row in the personalized overnight digest (`GET /v1/me/digest`). */
export interface DigestStock {
  ticker: string;
  /** Company / instrument name; may be empty until the security is tracked. */
  name: string;
  /** Overnight change %, null when no prev-close reference is available. */
  change_pct: number | null;
  /** Freshest news headline (Chinese-preferred for zh UI); absent when none. */
  headline?: string;
  /** Link to the original article for {@link headline}. */
  headline_url?: string;
  /** Next earnings/event label, e.g. "财报 11-02 盘后" / "Earnings 11-02 AMC". */
  next_event?: string;
}

/** The signed-in user's personalized overnight report from `GET /v1/me/digest`. */
/** How to treat the AI overview ("Tonight's overview"), composed off the request path. */
export type DigestSummaryStatus =
  | 'ready' // summary is final (present, or empty when nothing to summarize)
  | 'generating' // Pro: the overview is composing in the background — poll until ready
  | 'pro_required' // non-Pro: the overview is a Pro feature (show an upgrade card)
  | 'unavailable'; // the LLM is disabled (hide the overview)

export interface MyDigest {
  /** ET day, YYYY-MM-DD. */
  date: string;
  /** AI overview (2-3 sentences) in the requested language; empty when the LLM
   *  is off or there's no material. */
  summary: string;
  stocks: DigestStock[];
  /** State of the AI overview — the data rows always serve instantly regardless. */
  summary_status?: DigestSummaryStatus;
}

/**
 * Fetches the caller's "我的隔夜报告" — a personalized morning report over their
 * watchlist (overnight change %, freshest headline, next event) plus an optional
 * AI overview, in the given UI language ("zh"|"en"). Cached server-side per
 * (user, ET day, language). Requires authentication; an empty watchlist resolves
 * to `{stocks: []}`.
 */
export function getMyDigest(
  token: string | null,
  lang: string,
  signal?: AbortSignal,
): Promise<MyDigest> {
  const q = lang === 'en' ? '?lang=en' : '';
  return getJson<MyDigest>(`/v1/me/digest${q}`, signal, token);
}

/** Fetches backend health. */
export function getHealth(signal?: AbortSignal): Promise<Health> {
  return getJson<Health>('/healthz', signal);
}

/** Envelope returned by the watchlist endpoints. */
export interface WatchlistResponse {
  tickers: string[];
}

/** Fetches the caller's watchlist. Requires authentication. */
export function getWatchlist(
  token: string | null,
  signal?: AbortSignal,
): Promise<WatchlistResponse> {
  return getJson<WatchlistResponse>('/v1/watchlist', signal, token);
}

/** Adds a ticker to the caller's watchlist; returns the updated list. */
export function addToWatchlist(
  token: string | null,
  ticker: string,
  signal?: AbortSignal,
): Promise<WatchlistResponse> {
  return postJson<WatchlistResponse>('/v1/watchlist', {ticker}, signal, token);
}

/** Removes a ticker from the caller's watchlist; returns the updated list. */
export function removeFromWatchlist(
  token: string | null,
  ticker: string,
  signal?: AbortSignal,
): Promise<WatchlistResponse> {
  return deleteJson<WatchlistResponse>(
    `/v1/watchlist/${encodeURIComponent(normalizeTicker(ticker))}`,
    signal,
    token,
  );
}
