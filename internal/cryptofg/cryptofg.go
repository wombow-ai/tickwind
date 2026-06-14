// Package cryptofg is a keyless client + in-memory cache for the crypto market
// mood: the widely watched crypto Fear & Greed index (alternative.me) plus a
// best-effort spot price + 24h change for Bitcoin and Ethereum (CoinGecko). It
// is relevant context for the crypto-linked U.S. equities the audience follows
// (COIN / MSTR / RIOT / MARA), which trade with the tape of crypto sentiment.
//
// Data sources, both keyless:
//
//   - alternative.me Fear & Greed (the CORE signal):
//     https://api.alternative.me/fng/?limit=1 →
//     {"data":[{"value":"63","value_classification":"Greed","timestamp":"…"}]}.
//     The value is 0–100 (0 = extreme fear, 100 = extreme greed); timestamp is a
//     Unix-seconds string for the index's "as of" day.
//
//   - CoinGecko simple price (BEST-EFFORT, degrades gracefully):
//     https://api.coingecko.com/api/v3/simple/price?ids=bitcoin,ethereum&vs_currencies=usd&include_24hr_change=true
//     → {"bitcoin":{"usd":64413,"usd_24h_change":1.01},"ethereum":{…}}. Free but
//     rate-limited; if it fails or rate-limits, prices are simply omitted — the
//     F&G score alone is the feature, and the fetch NEVER blocks on prices.
//
// IMPORTANT, anti-fabrication: a price is emitted only when the source actually
// returned a positive number — a missing / zero / non-finite price is ABSENT
// (Present=false), never a fabricated 0. A missing or non-numeric F&G value is an
// error (ErrNoData), never a stand-in score.
package cryptofg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Default endpoints (overridable in tests).
const (
	fngURL   = "https://api.alternative.me/fng/?limit=1"
	priceURL = "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin,ethereum&vs_currencies=usd&include_24hr_change=true"
)

// ErrNoData is returned when the F&G endpoint yields no usable score (empty data
// array, or a value that does not parse as a number). The core signal is then
// unavailable, so the caller keeps the last-good snapshot.
var ErrNoData = errors.New("cryptofg: no fear & greed data available")

// Price is a coin's best-effort spot price and 24h change. Present is true only
// when the source returned a positive USD price; when false the other fields are
// zero and must be treated as absent (never rendered as a real 0).
type Price struct {
	USD       float64 `json:"price"`      // spot price in USD (only when Present)
	Change24h float64 `json:"change_24h"` // 24h change, percent (only when Present)
	Present   bool    `json:"-"`          // true only when the source gave a real price
}

// Index is the latest crypto market-mood snapshot: the Fear & Greed score and
// its classification, plus best-effort BTC/ETH prices. BTC/ETH may be absent
// (Present=false) without invalidating the snapshot — the F&G score is the core.
type Index struct {
	// Score is the F&G index value, 0–100 (0 = extreme fear, 100 = extreme greed).
	Score int `json:"score"`
	// Label is the source's classification, e.g. "Greed", "Extreme Fear".
	Label string `json:"label"`
	// AsOf is the index's day, formatted "2006-01-02" (from the source timestamp);
	// empty when the source omitted a parseable timestamp.
	AsOf string `json:"as_of"`
	// BTC and ETH are best-effort spot prices; check Present before using.
	BTC Price `json:"btc"`
	ETH Price `json:"eth"`
}

// Client fetches the crypto F&G index plus best-effort BTC/ETH prices. Keyless;
// a bare Go User-Agent works for both hosts (no browser-UA trick needed).
type Client struct {
	http     *http.Client
	fngURL   string // F&G endpoint (overridable in tests)
	priceURL string // price endpoint (overridable in tests)
}

// New builds a crypto F&G client with sane timeouts. Keyless.
func New() *Client {
	return &Client{
		http:     &http.Client{Timeout: 15 * time.Second},
		fngURL:   fngURL,
		priceURL: priceURL,
	}
}

// Latest fetches the current F&G index (required) and then best-effort BTC/ETH
// prices. A price failure or rate-limit is swallowed — prices are left absent and
// the F&G snapshot is still returned. An error is returned only when the core F&G
// signal itself is unavailable.
func (c *Client) Latest(ctx context.Context) (Index, error) {
	idx, err := c.fetchFNG(ctx)
	if err != nil {
		return Index{}, err
	}
	// Best-effort prices: never let a failure block the F&G feature.
	if btc, eth, perr := c.fetchPrices(ctx); perr == nil {
		idx.BTC = btc
		idx.ETH = eth
	}
	return idx, nil
}

// fngResponse mirrors the alternative.me payload (only the fields we use).
type fngResponse struct {
	Data []struct {
		Value          string `json:"value"`
		Classification string `json:"value_classification"`
		Timestamp      string `json:"timestamp"`
	} `json:"data"`
}

// fetchFNG fetches and parses the core Fear & Greed index. Returns ErrNoData when
// the payload has no usable score.
func (c *Client) fetchFNG(ctx context.Context) (Index, error) {
	body, err := c.get(ctx, c.fngURL)
	if err != nil {
		return Index{}, fmt.Errorf("cryptofg fng fetch: %w", err)
	}
	return ParseFNG(body)
}

// ParseFNG parses an alternative.me F&G body into an Index (without prices).
// Anti-fabrication: a missing/non-numeric value yields ErrNoData rather than a
// stand-in score. Exported for table-driven tests.
func ParseFNG(body []byte) (Index, error) {
	var r fngResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return Index{}, fmt.Errorf("cryptofg fng parse: %w", err)
	}
	if len(r.Data) == 0 {
		return Index{}, ErrNoData
	}
	d := r.Data[0]
	score, err := strconv.Atoi(strings.TrimSpace(d.Value))
	if err != nil {
		return Index{}, ErrNoData // non-numeric value → no data, never fabricated
	}
	if score < 0 {
		score = 0
	} else if score > 100 {
		score = 100
	}
	idx := Index{Score: score, Label: strings.TrimSpace(d.Classification)}
	if ts := strings.TrimSpace(d.Timestamp); ts != "" {
		if sec, err := strconv.ParseInt(ts, 10, 64); err == nil {
			idx.AsOf = time.Unix(sec, 0).UTC().Format("2006-01-02")
		}
	}
	return idx, nil
}

// priceResponse mirrors the CoinGecko simple/price payload.
type priceResponse struct {
	Bitcoin  coinPrice `json:"bitcoin"`
	Ethereum coinPrice `json:"ethereum"`
}

type coinPrice struct {
	USD       float64 `json:"usd"`
	Change24h float64 `json:"usd_24h_change"`
}

// fetchPrices fetches best-effort BTC/ETH prices. Any error (network, status,
// parse) returns the error so Latest can swallow it and leave prices absent.
func (c *Client) fetchPrices(ctx context.Context) (btc, eth Price, err error) {
	body, err := c.get(ctx, c.priceURL)
	if err != nil {
		return Price{}, Price{}, fmt.Errorf("cryptofg price fetch: %w", err)
	}
	btc, eth, err = ParsePrices(body)
	return btc, eth, err
}

// ParsePrices parses a CoinGecko simple/price body into BTC and ETH prices. A
// coin with a missing / non-positive / non-finite USD price is returned absent
// (Present=false), never zero-filled. Exported for table-driven tests.
func ParsePrices(body []byte) (btc, eth Price, err error) {
	var r priceResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return Price{}, Price{}, fmt.Errorf("cryptofg price parse: %w", err)
	}
	return toPrice(r.Bitcoin), toPrice(r.Ethereum), nil
}

// toPrice converts a raw coin quote to a Price, present only when the USD price
// is a finite positive number (a missing field unmarshals to 0 → absent).
func toPrice(cp coinPrice) Price {
	if cp.USD <= 0 || math.IsNaN(cp.USD) || math.IsInf(cp.USD, 0) {
		return Price{} // absent — never fabricate a 0 price
	}
	chg := cp.Change24h
	if math.IsNaN(chg) || math.IsInf(chg, 0) {
		chg = 0 // a bad change figure on a real price → 0% (price still shown)
	}
	return Price{USD: cp.USD, Change24h: chg, Present: true}
}

// get performs a GET and returns the (size-capped) body, erroring on non-200.
func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // payloads are tiny; cap defensively
}
