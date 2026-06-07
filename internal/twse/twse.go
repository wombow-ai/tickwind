// Package twse is a client for the Taiwan Stock Exchange (TWSE) OpenAPI — the
// free, no-key, redistribution-permitted (Taiwan Open Government Data License)
// source of end-of-day prices and the listed-company directory for main-board
// (".TW") stocks. One call to STOCK_DAY_ALL prices the entire main board.
package twse

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/symbols"
)

// defaultBase is the TWSE OpenAPI root.
const defaultBase = "https://openapi.twse.com.tw/v1"

// Client fetches TWSE OpenAPI data.
type Client struct {
	http *http.Client
	base string
}

// New returns a Client pointed at the live TWSE OpenAPI.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}, base: defaultBase}
}

// stockDayRow is one record of /exchangeReport/STOCK_DAY_ALL (all values are
// strings; "--"/empty mark a halted stock).
type stockDayRow struct {
	Date  string `json:"Date"`         // ROC calendar, e.g. "1150605" = 2026-06-05
	Code  string `json:"Code"`         // e.g. "2330"
	Name  string `json:"Name"`         // Chinese name, e.g. "台積電"
	Close string `json:"ClosingPrice"` //
	Chg   string `json:"Change"`       // signed change vs previous close
}

// EODQuotes fetches the whole main board's daily close and returns a quote per
// ".TW" ticker. Prices are EOD, so Session is "closed".
func (c *Client) EODQuotes(ctx context.Context) (map[string]store.Quote, error) {
	var rows []stockDayRow
	if err := c.getJSON(ctx, "/exchangeReport/STOCK_DAY_ALL", &rows); err != nil {
		return nil, err
	}
	out := make(map[string]store.Quote, len(rows))
	for _, r := range rows {
		close, ok := parseNum(r.Close)
		if !ok || close <= 0 {
			continue
		}
		chg, _ := parseNum(r.Chg)
		ticker := strings.TrimSpace(r.Code) + ".TW"
		out[ticker] = store.Quote{
			Ticker:    ticker,
			Price:     close,
			PrevClose: close - chg, // STOCK_DAY_ALL carries the signed daily change
			Session:   "closed",
			Source:    "twse",
			At:        rocDate(r.Date),
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("twse: STOCK_DAY_ALL returned no usable rows")
	}
	return out, nil
}

// Companies returns the main-board symbol directory (ticker + name), derived
// from the same daily feed so no extra request is needed.
func (c *Client) Companies(ctx context.Context) ([]symbols.Symbol, error) {
	var rows []stockDayRow
	if err := c.getJSON(ctx, "/exchangeReport/STOCK_DAY_ALL", &rows); err != nil {
		return nil, err
	}
	out := make([]symbols.Symbol, 0, len(rows))
	for _, r := range rows {
		code := strings.TrimSpace(r.Code)
		name := strings.TrimSpace(r.Name)
		if code == "" || name == "" {
			continue
		}
		out = append(out, symbols.Symbol{
			Ticker:   code + ".TW",
			Name:     name,
			Exchange: "TWSE",
			Country:  "TW",
		})
	}
	return out, nil
}

func (c *Client) getJSON(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Tickwind/1.0 (+https://tickwind.com)")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("twse: get %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("twse: get %s: %s", path, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("twse: decode %s: %w", path, err)
	}
	return nil
}

// rocDate converts a Republic-of-China calendar date ("1150605") to UTC
// midnight (year = 1911 + ROC year). Zero time on malformed input.
func rocDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if len(s) < 7 {
		return time.Time{}
	}
	roc, e1 := strconv.Atoi(s[:len(s)-4])
	mm, e2 := strconv.Atoi(s[len(s)-4 : len(s)-2])
	dd, e3 := strconv.Atoi(s[len(s)-2:])
	if e1 != nil || e2 != nil || e3 != nil || mm < 1 || mm > 12 || dd < 1 || dd > 31 {
		return time.Time{}
	}
	return time.Date(1911+roc, time.Month(mm), dd, 0, 0, 0, 0, time.UTC)
}

// parseNum parses a TWSE numeric string, tolerating commas, surrounding spaces
// and the "--"/empty placeholders used for halted stocks.
func parseNum(s string) (float64, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if s == "" || s == "--" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}
