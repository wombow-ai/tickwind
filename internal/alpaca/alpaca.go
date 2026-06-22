// Package alpaca is a minimal client for the Alpaca Market Data API. It fetches
// the per-symbol snapshot, whose latest trade includes pre-market, after-hours
// and overnight prints (so Tickwind can show an all-session price) and whose
// previous daily bar gives the prior close for the day's change. Market data
// works with an unfunded paper account, so no real money is ever at risk.
package alpaca

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	urlpkg "net/url"
	"strings"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/symbols"
)

// DefaultDataURL is the Alpaca market-data base URL.
const DefaultDataURL = "https://data.alpaca.markets"

// Client fetches market data from Alpaca.
type Client struct {
	http    *http.Client
	keyID   string
	secret  string
	dataURL string
	feed    string
	loc     *time.Location
}

// New returns a Client. Empty dataURL falls back to DefaultDataURL; empty feed
// falls back to "iex" (the free feed).
func New(keyID, secret, dataURL, feed string) *Client {
	if dataURL == "" {
		dataURL = DefaultDataURL
	}
	if feed == "" {
		feed = "iex"
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		loc = time.UTC
	}
	return &Client{
		http:    &http.Client{Timeout: 10 * time.Second},
		keyID:   keyID,
		secret:  secret,
		dataURL: dataURL,
		feed:    feed,
		loc:     loc,
	}
}

// NormalizeSymbol converts a ticker to the form Alpaca's market-data API expects.
// The SEC directory (the universe sweep's source) writes class / preferred shares
// with a hyphen — e.g. "BRK-B", "BF-A" — while Alpaca (like most US vendors and
// Tickwind's own canonical form) uses a dot: "BRK.B", "BF.A". Sending the hyphen
// form makes Alpaca 400 the WHOLE batch ("invalid symbol: BRK-B"), which silently
// drops every other symbol in that 100-ticker request — and because the SEC
// directory front-loads the mega-caps (NVDA/AAPL/MSFT… in the first batch, which
// also contains BRK-B), that one bad symbol was wiping out all the most-liquid
// names from the universe/screener. Mapping the single internal "-" class suffix
// to "." fixes the symbol so the batch succeeds; non-class tickers are unchanged.
//
// This delegates to symbols.Canonical so the dot/hyphen rule has exactly ONE
// definition shared across the app (price universe, SEC symbols index, EDGAR
// lookup) — the dotted form (BRK.B) is Tickwind's canonical symbol everywhere.
func NormalizeSymbol(ticker string) string {
	return symbols.Canonical(ticker)
}

// bar is a single OHLC bar; only the close is used here.
type bar struct {
	Close float64 `json:"c"`
}

type snapshotResp struct {
	LatestTrade struct {
		Timestamp time.Time `json:"t"`
		Price     float64   `json:"p"`
	} `json:"latestTrade"`
	PrevDailyBar bar `json:"prevDailyBar"`
	DailyBar     bar `json:"dailyBar"`
}

// LatestQuote returns the most recent trade for ticker as a store.Quote,
// including extended-hours and overnight prints, along with the previous
// trading day's close (for the day's change). Uses the snapshot endpoint so
// price, session and prior close come from a single request.
func (c *Client) LatestQuote(ctx context.Context, ticker string) (store.Quote, error) {
	url := fmt.Sprintf("%s/v2/stocks/%s/snapshot?feed=%s", c.dataURL, urlpkg.PathEscape(ticker), c.feed)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return store.Quote{}, err
	}
	req.Header.Set("APCA-API-KEY-ID", c.keyID)
	req.Header.Set("APCA-API-SECRET-KEY", c.secret)

	resp, err := c.http.Do(req)
	if err != nil {
		return store.Quote{}, fmt.Errorf("alpaca: get snapshot %s: %w", ticker, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return store.Quote{}, fmt.Errorf("alpaca: snapshot %s: %s", ticker, resp.Status)
	}

	var body snapshotResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return store.Quote{}, fmt.Errorf("alpaca: decode snapshot %s: %w", ticker, err)
	}
	return store.Quote{
		Ticker:       ticker,
		Price:        body.LatestTrade.Price,
		PrevClose:    body.PrevDailyBar.Close,
		RegularClose: regularClose(body.DailyBar.Close, body.PrevDailyBar.Close),
		Session:      c.sessionAt(body.LatestTrade.Timestamp),
		Source:       "alpaca",
		At:           body.LatestTrade.Timestamp,
	}, nil
}

// regularClose picks the regular-session close: the day bar's close when present,
// else the previous day's close (e.g. pre-market before today's bar exists), so
// extended-hours change always has a sane reference.
func regularClose(dailyClose, prevClose float64) float64 {
	if dailyClose > 0 {
		return dailyClose
	}
	return prevClose
}

type barsResp struct {
	Bars []bar `json:"bars"`
}

// DailyBars returns up to limit recent daily closing prices, oldest first, for
// drawing a trend sparkline. Returns a nil slice (not an error) when there is
// no data. Split-adjusted for visual continuity.
func (c *Client) DailyBars(ctx context.Context, ticker string, limit int) ([]float64, error) {
	if limit <= 0 {
		limit = 30
	}
	// Look back generously (weekends/holidays) and take the most recent `limit`.
	start := time.Now().In(c.loc).AddDate(0, 0, -(limit*2 + 20)).Format("2006-01-02")
	url := fmt.Sprintf(
		"%s/v2/stocks/%s/bars?timeframe=1Day&start=%s&limit=%d&sort=desc&adjustment=split&feed=%s",
		c.dataURL, urlpkg.PathEscape(ticker), start, limit, c.feed,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("APCA-API-KEY-ID", c.keyID)
	req.Header.Set("APCA-API-SECRET-KEY", c.secret)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("alpaca: get bars %s: %w", ticker, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("alpaca: bars %s: %s", ticker, resp.Status)
	}

	var body barsResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("alpaca: decode bars %s: %w", ticker, err)
	}
	// Response is newest-first (sort=desc); reverse to oldest-first.
	closes := make([]float64, 0, len(body.Bars))
	for i := len(body.Bars) - 1; i >= 0; i-- {
		closes = append(closes, body.Bars[i].Close)
	}
	return closes, nil
}

// ohlcBar is a full daily bar (timestamp + OHLC + volume) for the candlestick chart.
type ohlcBar struct {
	Time   time.Time `json:"t"`
	Open   float64   `json:"o"`
	High   float64   `json:"h"`
	Low    float64   `json:"l"`
	Close  float64   `json:"c"`
	Volume float64   `json:"v"`
}

type ohlcResp struct {
	Bars []ohlcBar `json:"bars"`
}

// DailyOHLC returns up to limit recent daily OHLC bars (+ volume), oldest first,
// for the K-line chart. Split-adjusted. Returns a nil slice (not an error) when
// there's no data.
func (c *Client) DailyOHLC(ctx context.Context, ticker string, limit int) ([]store.Candle, error) {
	if limit <= 0 {
		limit = 250
	}
	start := time.Now().In(c.loc).AddDate(0, 0, -(limit*2 + 30)).Format("2006-01-02")
	url := fmt.Sprintf(
		"%s/v2/stocks/%s/bars?timeframe=1Day&start=%s&limit=%d&sort=desc&adjustment=split&feed=%s",
		c.dataURL, urlpkg.PathEscape(ticker), start, limit, c.feed,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("APCA-API-KEY-ID", c.keyID)
	req.Header.Set("APCA-API-SECRET-KEY", c.secret)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("alpaca: get ohlc %s: %w", ticker, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("alpaca: ohlc %s: %s", ticker, resp.Status)
	}

	var body ohlcResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("alpaca: decode ohlc %s: %w", ticker, err)
	}
	out := make([]store.Candle, 0, len(body.Bars))
	for i := len(body.Bars) - 1; i >= 0; i-- { // newest-first → oldest-first
		b := body.Bars[i]
		out = append(out, store.Candle{
			Time: b.Time, Open: b.Open, High: b.High, Low: b.Low, Close: b.Close, Volume: b.Volume,
		})
	}
	return out, nil
}

// IntradayOHLC returns intraday OHLC bars (oldest first) for the given timeframe
// (e.g. "5Min", "15Min", "1Hour") since start. Split-adjusted, IEX feed — for the
// 1D/5D chart views. Returns a nil slice (not an error) when there's no data.
func (c *Client) IntradayOHLC(ctx context.Context, ticker, timeframe string, start time.Time) ([]store.Candle, error) {
	url := fmt.Sprintf(
		"%s/v2/stocks/%s/bars?timeframe=%s&start=%s&limit=10000&adjustment=split&feed=%s&sort=asc",
		c.dataURL, urlpkg.PathEscape(ticker), timeframe, start.UTC().Format(time.RFC3339), c.feed,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("APCA-API-KEY-ID", c.keyID)
	req.Header.Set("APCA-API-SECRET-KEY", c.secret)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("alpaca: get intraday %s: %w", ticker, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("alpaca: intraday %s: %s", ticker, resp.Status)
	}

	var body ohlcResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("alpaca: decode intraday %s: %w", ticker, err)
	}
	out := make([]store.Candle, 0, len(body.Bars))
	for _, b := range body.Bars { // sort=asc → already oldest-first
		out = append(out, store.Candle{
			Time: b.Time, Open: b.Open, High: b.High, Low: b.Low, Close: b.Close, Volume: b.Volume,
		})
	}
	return out, nil
}

// snapshotBatch is the per-request cap. Alpaca's snapshots endpoint accepts a
// comma-joined symbol list; 100 keeps URLs sane and one call can price a whole
// small-cap candidate set.
const snapshotBatch = 100

// snapPrice extracts the usable price from a snapshot (daily-bar close, falling
// back to the previous daily bar off-hours, then the latest trade). Returns 0
// when none is usable (the caller omits the symbol).
func snapPrice(snap snapshotResp) float64 {
	if snap.DailyBar.Close > 0 {
		return snap.DailyBar.Close
	}
	if snap.PrevDailyBar.Close > 0 {
		return snap.PrevDailyBar.Close
	}
	if snap.LatestTrade.Price > 0 {
		return snap.LatestTrade.Price
	}
	return 0
}

// fetchSnapshots requests a snapshot for each symbol and invokes emit for every
// returned symbol. It batches at snapshotBatch and is RESILIENT to a poisoned
// batch: when Alpaca rejects a whole batch with HTTP 400 (one invalid symbol
// fails the entire request — e.g. a malformed ticker), it recursively bisects
// that batch so only the genuinely-bad symbol(s) are dropped and every valid
// symbol still gets priced. Without this, a single bad ticker would silently
// drop up to 99 good ones (this was why the mega-caps and ~3.7k other names were
// missing from the universe). Returns the last error if NOTHING was emitted.
func (c *Client) fetchSnapshots(ctx context.Context, symbols []string, emit func(sym string, snap snapshotResp)) error {
	var lastErr error
	for i := 0; i < len(symbols); i += snapshotBatch {
		end := i + snapshotBatch
		if end > len(symbols) {
			end = len(symbols)
		}
		if err := c.fetchSnapshotChunk(ctx, symbols[i:end], emit); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// fetchSnapshotChunk fetches one comma-joined request. On HTTP 400 (a single
// invalid symbol poisons the request) it bisects and retries each half, so valid
// symbols survive; a 1-symbol chunk that still 400s is simply the bad one,
// dropped. Other failures (network, non-400 status, decode) are returned as-is.
func (c *Client) fetchSnapshotChunk(ctx context.Context, chunk []string, emit func(sym string, snap snapshotResp)) error {
	if len(chunk) == 0 {
		return nil
	}
	url := fmt.Sprintf("%s/v2/stocks/snapshots?symbols=%s&feed=%s",
		c.dataURL, strings.Join(chunk, ","), c.feed)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("APCA-API-KEY-ID", c.keyID)
	req.Header.Set("APCA-API-SECRET-KEY", c.secret)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("alpaca: get snapshots: %w", err)
	}
	if resp.StatusCode == http.StatusBadRequest && len(chunk) > 1 {
		// One symbol in the chunk is invalid and Alpaca rejects the whole request.
		// Bisect so the valid symbols still get priced. (A 1-symbol chunk that
		// 400s falls through below and is dropped — it's the bad one.)
		resp.Body.Close()
		mid := len(chunk) / 2
		errA := c.fetchSnapshotChunk(ctx, chunk[:mid], emit)
		errB := c.fetchSnapshotChunk(ctx, chunk[mid:], emit)
		if errA != nil {
			return errA
		}
		return errB
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		resp.Body.Close()
		return fmt.Errorf("alpaca: snapshots %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var body map[string]snapshotResp
	err = json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	if err != nil {
		return fmt.Errorf("alpaca: decode snapshots: %w", err)
	}
	for sym, snap := range body {
		emit(sym, snap)
	}
	return nil
}

// Snapshots returns the latest price per symbol, fetched in bulk. Symbols with no
// usable price are omitted. Class/preferred tickers are normalized to Alpaca's
// dot form (see NormalizeSymbol) and a poisoned batch is bisected rather than
// dropped wholesale. Used to compute market cap (= shares × price) for the
// Opportunity board.
func (c *Client) Snapshots(ctx context.Context, symbols []string) (map[string]float64, error) {
	out := make(map[string]float64, len(symbols))
	norm := normalizeSymbols(symbols)
	err := c.fetchSnapshots(ctx, norm, func(sym string, snap snapshotResp) {
		if price := snapPrice(snap); price > 0 {
			out[sym] = price
		}
	})
	if len(out) == 0 && err != nil {
		return nil, err
	}
	return out, nil
}

// SnapshotQuotes is like Snapshots but returns full quotes (price + prev-close +
// session) per symbol — the change reference the universe price cache needs.
// Symbols with no usable price are omitted; class/preferred tickers are
// normalized and a poisoned batch is bisected (so the mega-caps survive).
func (c *Client) SnapshotQuotes(ctx context.Context, symbols []string) (map[string]store.Quote, error) {
	out := make(map[string]store.Quote, len(symbols))
	norm := normalizeSymbols(symbols)
	err := c.fetchSnapshots(ctx, norm, func(sym string, snap snapshotResp) {
		price := snapPrice(snap)
		if price <= 0 {
			return
		}
		out[sym] = store.Quote{
			Ticker:       sym,
			Price:        price,
			PrevClose:    snap.PrevDailyBar.Close,
			RegularClose: regularClose(snap.DailyBar.Close, snap.PrevDailyBar.Close),
			Session:      c.sessionAt(snap.LatestTrade.Timestamp),
			Source:       "alpaca",
			At:           snap.LatestTrade.Timestamp,
		}
	})
	if len(out) == 0 && err != nil {
		return nil, err
	}
	return out, nil
}

// SnapshotQuotesLive is the LIVE-pricing bulk counterpart to SnapshotQuotes: it
// builds each quote from the LATEST TRADE (session-aware, includes pre-/after-hours
// prints), exactly like the singular LatestQuote — NOT SnapshotQuotes' daily-close-
// first price (which is for market-cap stability, wrong for live display, esp. in
// extended hours). The price poller uses this to refresh the whole tracked set in a
// few bulk requests per cycle (≤100 symbols each) instead of one serial REST call
// per ticker — a ~200-ticker cycle drops from tens of seconds to ~1-2s. Symbols with
// no usable latest trade (price ≤ 0) are omitted so an empty print never clobbers a
// good last quote. Resilient to a poisoned batch via fetchSnapshots' bisection.
func (c *Client) SnapshotQuotesLive(ctx context.Context, symbols []string) (map[string]store.Quote, error) {
	out := make(map[string]store.Quote, len(symbols))
	norm := normalizeSymbols(symbols)
	err := c.fetchSnapshots(ctx, norm, func(sym string, snap snapshotResp) {
		if snap.LatestTrade.Price <= 0 {
			return
		}
		out[sym] = store.Quote{
			Ticker:       sym,
			Price:        snap.LatestTrade.Price,
			PrevClose:    snap.PrevDailyBar.Close,
			RegularClose: regularClose(snap.DailyBar.Close, snap.PrevDailyBar.Close),
			Session:      c.sessionAt(snap.LatestTrade.Timestamp),
			Source:       "alpaca",
			At:           snap.LatestTrade.Timestamp,
		}
	})
	if len(out) == 0 && err != nil {
		return nil, err
	}
	return out, nil
}

// normalizeSymbols maps each symbol to Alpaca's expected form (deduping after
// normalization, since e.g. a directory rarely holds both forms). The response
// is keyed by the SENT (normalized) symbol — which is Tickwind's own canonical
// dot form for class shares — so the universe/screener key matches the rest of
// the app (aliases, /stock/{t} URLs, cashtag parsing all use BRK.B, not BRK-B).
func normalizeSymbols(symbols []string) []string {
	out := make([]string, 0, len(symbols))
	seen := make(map[string]struct{}, len(symbols))
	for _, s := range symbols {
		n := NormalizeSymbol(s)
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}

// SessionAt is the exported session classifier (used by the WS streamer to
// label real-time trades the same way snapshots are labeled).
func (c *Client) SessionAt(t time.Time) string { return c.sessionAt(t) }

// sessionAt classifies a US-equity trading session for a timestamp, evaluated
// in America/New_York. Holidays are not accounted for (best-effort, for display
// only): pre 04:00–09:30, regular 09:30–16:00, post 16:00–20:00, otherwise
// overnight; weekends are "closed".
func (c *Client) sessionAt(t time.Time) string {
	if t.IsZero() {
		return "closed"
	}
	et := t.In(c.loc)
	if wd := et.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return "closed"
	}
	mins := et.Hour()*60 + et.Minute()
	switch {
	case mins >= 4*60 && mins < 9*60+30:
		return "pre"
	case mins >= 9*60+30 && mins < 16*60:
		return "regular"
	case mins >= 16*60 && mins < 20*60:
		return "post"
	default:
		return "overnight"
	}
}
