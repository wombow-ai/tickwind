/** Static app configuration. */

/**
 * Default watchlist shown on the home board.
 *
 * v1 is US-first (SEC EDGAR). This mirrors the backend's default `WATCHLIST`
 * env (`AAPL,NVDA,TSLA`); HK/KR tickers are added once those sources land.
 */
export const DEFAULT_WATCHLIST: readonly string[] = ['AAPL', 'NVDA', 'TSLA'];

/** Product name, used in the header and document title. */
export const APP_NAME = 'Tickwind';

/** Short tagline shown in the header. */
export const APP_TAGLINE = 'Personal stock command center';
