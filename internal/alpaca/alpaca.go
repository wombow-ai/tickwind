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
	"strings"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
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
	url := fmt.Sprintf("%s/v2/stocks/%s/snapshot?feed=%s", c.dataURL, ticker, c.feed)
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
		c.dataURL, ticker, start, limit, c.feed,
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
		c.dataURL, ticker, start, limit, c.feed,
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
		c.dataURL, ticker, timeframe, start.UTC().Format(time.RFC3339), c.feed,
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

// Snapshots returns the latest price per symbol, fetched in bulk (the daily
// bar's close, falling back to the previous daily bar off-hours, then the latest
// trade). Symbols with no usable price are omitted. Batches at 100 symbols per
// request to keep URLs sane; one call can price the whole small-cap candidate
// set. Used to compute market cap (= shares × price) for the Opportunity board.
func (c *Client) Snapshots(ctx context.Context, symbols []string) (map[string]float64, error) {
	const batch = 100
	out := make(map[string]float64, len(symbols))
	var lastErr error
	for i := 0; i < len(symbols); i += batch {
		end := i + batch
		if end > len(symbols) {
			end = len(symbols)
		}
		url := fmt.Sprintf("%s/v2/stocks/snapshots?symbols=%s&feed=%s",
			c.dataURL, strings.Join(symbols[i:end], ","), c.feed)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("APCA-API-KEY-ID", c.keyID)
		req.Header.Set("APCA-API-SECRET-KEY", c.secret)

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("alpaca: get snapshots: %w", err)
			continue
		}
		// Skip a bad batch (e.g. an unknown symbol → 400) rather than failing the
		// whole call; surface the reason if every batch fails.
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
			resp.Body.Close()
			lastErr = fmt.Errorf("alpaca: snapshots %s: %s", resp.Status, strings.TrimSpace(string(b)))
			continue
		}
		var body map[string]snapshotResp
		err = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("alpaca: decode snapshots: %w", err)
			continue
		}
		for sym, snap := range body {
			price := snap.DailyBar.Close
			if price <= 0 {
				price = snap.PrevDailyBar.Close
			}
			if price <= 0 {
				price = snap.LatestTrade.Price
			}
			if price > 0 {
				out[sym] = price
			}
		}
	}
	if len(out) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return out, nil
}

// SnapshotQuotes is like Snapshots but returns full quotes (price + prev-close +
// session) per symbol — the change reference the universe price cache needs.
// Symbols with no usable price are omitted; batches at 100/req.
func (c *Client) SnapshotQuotes(ctx context.Context, symbols []string) (map[string]store.Quote, error) {
	const batch = 100
	out := make(map[string]store.Quote, len(symbols))
	var lastErr error
	for i := 0; i < len(symbols); i += batch {
		end := i + batch
		if end > len(symbols) {
			end = len(symbols)
		}
		url := fmt.Sprintf("%s/v2/stocks/snapshots?symbols=%s&feed=%s",
			c.dataURL, strings.Join(symbols[i:end], ","), c.feed)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("APCA-API-KEY-ID", c.keyID)
		req.Header.Set("APCA-API-SECRET-KEY", c.secret)

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("alpaca: get snapshot quotes: %w", err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
			resp.Body.Close()
			lastErr = fmt.Errorf("alpaca: snapshot quotes %s: %s", resp.Status, strings.TrimSpace(string(b)))
			continue
		}
		var body map[string]snapshotResp
		err = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("alpaca: decode snapshot quotes: %w", err)
			continue
		}
		for sym, snap := range body {
			price := snap.DailyBar.Close
			if price <= 0 {
				price = snap.PrevDailyBar.Close
			}
			if price <= 0 {
				price = snap.LatestTrade.Price
			}
			if price <= 0 {
				continue
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
		}
	}
	if len(out) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return out, nil
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
