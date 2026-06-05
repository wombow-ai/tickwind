/**
 * Typed client for the Tickwind backend JSON API.
 *
 * All requests target {@link API_BASE}, which is read from
 * `NEXT_PUBLIC_API_BASE` at build time and falls back to the local dev server.
 * The app is statically exported, so every call here runs in the browser.
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

/**
 * Performs a typed GET against the API and parses the JSON body.
 *
 * @param path Absolute API path beginning with `/`.
 * @param signal Optional abort signal to cancel the request.
 * @throws {ApiError} If the network call fails or the status is not 2xx.
 */
async function getJson<T>(path: string, signal?: AbortSignal): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`${API_BASE}${path}`, {
      headers: {Accept: 'application/json'},
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

/** Fetches backend health. */
export function getHealth(signal?: AbortSignal): Promise<Health> {
  return getJson<Health>('/healthz', signal);
}
