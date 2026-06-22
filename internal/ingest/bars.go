package ingest

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/alpaca"
	"github.com/wombow-ai/tickwind/internal/store"
)

// candleDays is how many daily OHLC bars the K-line chart fetches — ~5 trading
// years, so the Yearly timeframe has real bars and panning/scrolling left reveals
// plenty of history without a round trip (well past the ≥250 RSI/EMA window).
const candleDays = 1300

// BarCache serves recent daily closing prices for sparklines, caching each
// ticker's series for a TTL so repeated page views don't re-hit Alpaca (daily
// bars only change once per day). It satisfies api.BarSource.
type BarCache struct {
	client *alpaca.Client
	limit  int
	ttl    time.Duration
	// candleFetch is the underlying daily-OHLC fetch (defaults to client.DailyOHLC; overridable in
	// tests). DailyCandles coalesces concurrent calls for one ticker down to a single invocation.
	candleFetch func(ctx context.Context, ticker string, days int) ([]store.Candle, error)

	mu        sync.Mutex
	entries   map[string]barEntry
	candles   map[string]candleEntry
	candleInf map[string]*candleCall // in-flight daily-candle fetches, for request coalescing
	intraday  map[string]candleEntry // key: ticker|resolution
	quotes    map[string]quoteEntry
}

// candleCall is one in-flight DailyCandles fetch that concurrent callers share (singleflight): the
// first caller fetches, the rest wait on `done` and read the result — so a burst of background scans
// requesting the SAME ticker collapses to ONE Alpaca call instead of storming the rate-limited API.
type candleCall struct {
	done    chan struct{}
	candles []store.Candle
	err     error
}

type barEntry struct {
	closes []float64
	at     time.Time
}

type candleEntry struct {
	candles []store.Candle
	at      time.Time
	failed  bool // a recent fetch ERRORED — negative-cache it briefly (candleNegTTL)
}

// candleNegTTL briefly negative-caches a FAILED daily-candle fetch (a malformed / throttled /
// timed-out ticker — NOT a 200-with-empty-bars, which is already cached as success) so the public
// stats endpoints (/relative-strength, /seasonality, /earnings-reaction, /scorecard, …) can't be
// looped to re-hit Alpaca every request for the same bad symbol. Short, so a transient upstream
// error self-heals quickly.
const candleNegTTL = 3 * time.Minute

// errCandleMissCached is returned for a ticker whose recent fetch failed and is still within
// candleNegTTL — the caller treats it like any DailyCandles error (no data), without an Alpaca hit.
var errCandleMissCached = errors.New("daily candles unavailable (cached miss)")

type quoteEntry struct {
	q  store.Quote
	at time.Time
}

// quoteTTL caps how often an on-demand (non-polled) quote re-hits Alpaca.
const quoteTTL = 20 * time.Second

// NewBarCache builds a cache fetching `limit` daily closes per ticker, holding
// each series for ttl.
func NewBarCache(client *alpaca.Client, limit int, ttl time.Duration) *BarCache {
	bc := &BarCache{
		client:    client,
		limit:     limit,
		ttl:       ttl,
		entries:   make(map[string]barEntry),
		candles:   make(map[string]candleEntry),
		candleInf: make(map[string]*candleCall),
		intraday:  make(map[string]candleEntry),
		quotes:    make(map[string]quoteEntry),
	}
	bc.candleFetch = client.DailyOHLC
	return bc
}

// DailyBars returns the cached series for ticker, fetching and caching it when
// missing or stale.
func (b *BarCache) DailyBars(ctx context.Context, ticker string) ([]float64, error) {
	b.mu.Lock()
	e, ok := b.entries[ticker]
	b.mu.Unlock()
	if ok && time.Since(e.at) < b.ttl {
		return e.closes, nil
	}

	closes, err := b.client.DailyBars(ctx, ticker, b.limit)
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	b.entries[ticker] = barEntry{closes: closes, at: time.Now()}
	b.mu.Unlock()
	return closes, nil
}

// DailyCandles returns the cached OHLC series for ticker (for the candlestick
// chart), fetching ~candleDays of history when missing or stale.
func (b *BarCache) DailyCandles(ctx context.Context, ticker string) ([]store.Candle, error) {
	// One critical section decides cache-hit vs join-in-flight vs become-leader, so two callers can
	// never both miss and both fetch the same ticker (the storm a broadened multi-scan set caused).
	b.mu.Lock()
	if e, ok := b.candles[ticker]; ok {
		if e.failed && time.Since(e.at) < candleNegTTL {
			b.mu.Unlock()
			return nil, errCandleMissCached // recent failure — don't re-hit Alpaca
		}
		if !e.failed && time.Since(e.at) < b.ttl {
			b.mu.Unlock()
			return e.candles, nil
		}
	}
	if call, ok := b.candleInf[ticker]; ok {
		// A fetch for this ticker is already running — wait for it and share its result.
		b.mu.Unlock()
		select {
		case <-call.done:
			return call.candles, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	call := &candleCall{done: make(chan struct{})}
	b.candleInf[ticker] = call
	b.mu.Unlock()

	cs, err := b.candleFetch(ctx, ticker, candleDays)

	b.mu.Lock()
	if err != nil {
		b.candles[ticker] = candleEntry{at: time.Now(), failed: true} // negative-cache the error
	} else {
		b.candles[ticker] = candleEntry{candles: cs, at: time.Now()}
	}
	delete(b.candleInf, ticker)
	call.candles, call.err = cs, err
	b.mu.Unlock()
	close(call.done) // wake any waiters (they read call.candles/err, set before this close)
	return cs, err
}

// intradayCfg maps a chart resolution to the Alpaca timeframe + lookback window
// (the chart's 1D view uses 5Min, 5D uses 15Min). Unknown → no data.
var intradayCfg = map[string]struct {
	tf   string
	days int
}{
	"5Min":  {"5Min", 4},
	"15Min": {"15Min", 8},
	"1Hour": {"1Hour", 16},
}

// intradayTTL caps how often intraday bars re-hit Alpaca (they move constantly).
const intradayTTL = 60 * time.Second

// IntradayCandles returns intraday OHLC for a resolution (5Min/15Min/1Hour),
// cached briefly. Unknown resolutions return a nil slice (no data).
func (b *BarCache) IntradayCandles(ctx context.Context, ticker, resolution string) ([]store.Candle, error) {
	cfg, ok := intradayCfg[resolution]
	if !ok {
		return nil, nil
	}
	key := ticker + "|" + resolution
	b.mu.Lock()
	e, ok := b.intraday[key]
	b.mu.Unlock()
	if ok && time.Since(e.at) < intradayTTL {
		return e.candles, nil
	}

	cs, err := b.client.IntradayOHLC(ctx, ticker, cfg.tf, time.Now().AddDate(0, 0, -cfg.days))
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	b.intraday[key] = candleEntry{candles: cs, at: time.Now()}
	b.mu.Unlock()
	return cs, nil
}

// LatestQuote returns an on-demand quote for a ticker the price poller doesn't
// cover (e.g. a stock the user just navigated to). Cached briefly so repeated
// views don't hammer Alpaca. ok=false when there's no price.
func (b *BarCache) LatestQuote(ctx context.Context, ticker string) (store.Quote, bool, error) {
	b.mu.Lock()
	e, ok := b.quotes[ticker]
	b.mu.Unlock()
	if ok && time.Since(e.at) < quoteTTL {
		return e.q, true, nil
	}

	q, err := b.client.LatestQuote(ctx, ticker)
	if err != nil {
		return store.Quote{}, false, err
	}

	// Last-resort fallback for brand-new / very thin listings (e.g. a just-IPO'd
	// ticker) and any US name with no live IEX print: the snapshot has no IEX
	// trade, so q.Price is still 0 — yet the daily-candle path (DailyOHLC)
	// usually DOES have bars, which is why the K-line chart shows a price while
	// the detail-card PriceTag / market-cap stay empty. Carry the
	// latest REAL daily close so the cards populate, labeled as a closed
	// (non-live) as-of-the-candle-date price so it's never mislabeled as a live
	// trade. NEVER fabricates: only a real candle close is used, and if there are
	// no candles either we stay empty (—), exactly as before.
	if q.Price == 0 {
		if c, ok := b.latestDailyClose(ctx, ticker); ok {
			q = store.Quote{
				Ticker:       ticker,
				Price:        c.Close,
				PrevClose:    c.Close, // day-change 0 — this IS the close, not a live move
				RegularClose: c.Close,
				Session:      "closed",
				Source:       "daily",
				At:           c.Time,
			}
		}
	}

	if q.Price == 0 {
		return store.Quote{}, false, nil
	}

	b.mu.Lock()
	b.quotes[ticker] = quoteEntry{q: q, at: time.Now()}
	b.mu.Unlock()
	return q, true, nil
}

// latestDailyClose returns the most recent daily candle for ticker (newest of
// the cached DailyCandles series), used as the last-resort quote price when no
// live or consolidated trade is available. ok=false when there are no candles
// or the newest close is non-positive (so the caller stays empty rather than
// fabricating a price).
func (b *BarCache) latestDailyClose(ctx context.Context, ticker string) (store.Candle, bool) {
	cs, err := b.DailyCandles(ctx, ticker)
	if err != nil || len(cs) == 0 {
		return store.Candle{}, false
	}
	last := cs[len(cs)-1] // DailyCandles is oldest-first
	if last.Close <= 0 {
		return store.Candle{}, false
	}
	return last, true
}
