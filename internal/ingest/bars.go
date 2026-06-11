package ingest

import (
	"context"
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
	fb     ConsolidatedQuoter // consolidated-tape freshness fallback (nil = off)
	limit  int
	ttl    time.Duration

	mu       sync.Mutex
	entries  map[string]barEntry
	candles  map[string]candleEntry
	intraday map[string]candleEntry // key: ticker|resolution
	quotes   map[string]quoteEntry
}

type barEntry struct {
	closes []float64
	at     time.Time
}

type candleEntry struct {
	candles []store.Candle
	at      time.Time
}

type quoteEntry struct {
	q  store.Quote
	at time.Time
}

// quoteTTL caps how often an on-demand (non-polled) quote re-hits Alpaca.
const quoteTTL = 20 * time.Second

// staleQuoteAfter: when the freshest IEX trade is older than this, the
// consolidated-tape fallback kicks in — thin names can go hours between IEX
// prints (free Alpaca is IEX-only, ~1-2% of US volume) while still trading
// elsewhere.
const staleQuoteAfter = 5 * time.Minute

// ConsolidatedQuoter returns the consolidated-tape last trade (all exchanges)
// for a symbol — price, previous close, and trade time. Satisfied by
// *finnhub.Client; nil disables the freshness fallback.
type ConsolidatedQuoter interface {
	Quote(ctx context.Context, symbol string) (price, prevClose float64, at time.Time, ok bool, err error)
}

// NewBarCache builds a cache fetching `limit` daily closes per ticker, holding
// each series for ttl. fb (optional) is the consolidated-tape quote fallback.
func NewBarCache(client *alpaca.Client, limit int, ttl time.Duration, fb ConsolidatedQuoter) *BarCache {
	return &BarCache{
		client:   client,
		fb:       fb,
		limit:    limit,
		ttl:      ttl,
		entries:  make(map[string]barEntry),
		candles:  make(map[string]candleEntry),
		intraday: make(map[string]candleEntry),
		quotes:   make(map[string]quoteEntry),
	}
}

// overlayConsolidated overlays a fresher consolidated-tape print onto an (older)
// IEX-derived quote: price/time/source/session come from the consolidated trade;
// the IEX-derived prev/regular-close baselines are kept (filled from the
// consolidated prev close only when missing). Pure — unit-tested.
func overlayConsolidated(q store.Quote, price, prevClose float64, at time.Time, session string) store.Quote {
	q.Price = price
	q.At = at
	q.Source = "finnhub"
	q.Session = session
	if q.RegularClose == 0 && prevClose > 0 {
		q.RegularClose = prevClose
	}
	// prev_close drives the day-change and MUST share RegularClose's basis.
	if session == "regular" {
		q.RegularClose = price // live regular price is the regular close
		if q.PrevClose == 0 && prevClose > 0 {
			q.PrevClose = prevClose // same (consolidated) source as price → consistent
		}
	} else if q.RegularClose > 0 {
		// Extended hours: pairing an IEX/daily-bar RegularClose with a stale or
		// sparse prev bar (this is the fallback path, taken because IEX was
		// stale) manufactures a phantom day-change on thin names — e.g. a +90%
		// headline. Anchor prev_close to RegularClose so the day-change is 0;
		// the extended delta (price vs close) carries the real move.
		q.PrevClose = q.RegularClose
	} else if q.PrevClose == 0 && prevClose > 0 {
		q.PrevClose = prevClose
	}
	return q
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
	b.mu.Lock()
	e, ok := b.candles[ticker]
	b.mu.Unlock()
	if ok && time.Since(e.at) < b.ttl {
		return e.candles, nil
	}

	cs, err := b.client.DailyOHLC(ctx, ticker, candleDays)
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	b.candles[ticker] = candleEntry{candles: cs, at: time.Now()}
	b.mu.Unlock()
	return cs, nil
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

	// Freshness fallback: IEX-only quotes go stale for thin names (no IEX print
	// for minutes–hours while the stock trades elsewhere). When the IEX trade is
	// old — or IEX has nothing at all — overlay the consolidated-tape last trade.
	if b.fb != nil && (q.Price == 0 || time.Since(q.At) > staleQuoteAfter) {
		if p, pc, at, ok, ferr := b.fb.Quote(ctx, ticker); ferr == nil && ok && at.After(q.At) {
			q.Ticker = ticker
			q = overlayConsolidated(q, p, pc, at, b.client.SessionAt(at))
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
