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
  ticker: string;
  /** Company / instrument name. */
  name: string;
  /** Heat rank, 1 = hottest. */
  rank: number;
  /** Discussion volume — mentions in the last 24h. */
  mentions: number;
  /** Mentions in the same window 24h earlier. */
  mentions_prev: number;
  /** Mention growth vs 24h ago as a fraction (0.2 = +20%). */
  change: number;
  /** Total upvotes on those mentions. */
  upvotes: number;
  /** Composite heat score (volume × momentum). */
  heat: number;
  updated_at: string;
}

/** Envelope returned by `GET /v1/hot`. */
export interface HotResponse {
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

/** Envelope returned by `GET /v1/stocks/{ticker}/bars`. */
export interface BarsResponse {
  ticker: string;
  /** Recent daily closing prices, oldest first. Empty when unavailable. */
  closes: number[];
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
): Promise<NewsFeedResponse> {
  const q = tickers.map(normalizeTicker).join(',');
  return getJson<NewsFeedResponse>(
    `/v1/news?tickers=${encodeURIComponent(q)}&limit=${encodeURIComponent(String(perTicker))}`,
    signal,
  );
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
 * Fetches the market-wide trending leaderboard (most-discussed stocks),
 * hottest first.
 *
 * @param limit Maximum number of stocks to return (defaults to 40).
 */
export function getHot(limit = 40, signal?: AbortSignal): Promise<HotResponse> {
  return getJson<HotResponse>(
    `/v1/hot?limit=${encodeURIComponent(String(limit))}`,
    signal,
  );
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
