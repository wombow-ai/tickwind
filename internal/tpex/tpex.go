// Package tpex is a client for the Taipei Exchange (TPEx) OpenAPI — the free,
// no-key, redistribution-permitted (Taiwan Open Government Data License) source
// of end-of-day prices and the company directory for Taiwan's OTC / over-the-
// counter market (".TWO"). One call prices the whole OTC main board.
package tpex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/symbols"
)

const defaultBase = "https://www.tpex.org.tw/openapi/v1"

// Client fetches TPEx OpenAPI data.
type Client struct {
	http *http.Client
	base string
}

// New returns a Client pointed at the live TPEx OpenAPI.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}, base: defaultBase}
}

// otcRow is one record of /tpex_mainboard_daily_close_quotes. Field names differ
// from TWSE and some values carry trailing spaces.
type otcRow struct {
	Date  string `json:"Date"`                  // ROC calendar
	Code  string `json:"SecuritiesCompanyCode"` // e.g. "006201"
	Name  string `json:"CompanyName"`           //
	Close string `json:"Close"`                 //
	Chg   string `json:"Change"`                // may have a trailing space
}

// EODQuotes fetches the OTC main board's daily close, one quote per ".TWO" ticker.
func (c *Client) EODQuotes(ctx context.Context) (map[string]store.Quote, error) {
	rows, err := c.rows(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]store.Quote, len(rows))
	for _, r := range rows {
		close, ok := parseNum(r.Close)
		if !ok || close <= 0 {
			continue
		}
		chg, _ := parseNum(r.Chg)
		ticker := strings.TrimSpace(r.Code) + ".TWO"
		out[ticker] = store.Quote{
			Ticker:    ticker,
			Price:     close,
			PrevClose: close - chg,
			Session:   "closed",
			Source:    "tpex",
			At:        rocDate(r.Date),
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("tpex: daily quotes returned no usable rows")
	}
	return out, nil
}

// Companies returns the OTC symbol directory derived from the same daily feed.
func (c *Client) Companies(ctx context.Context) ([]symbols.Symbol, error) {
	rows, err := c.rows(ctx)
	if err != nil {
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
			Ticker:   code + ".TWO",
			Name:     name,
			Exchange: "TPEx",
			Country:  "TW",
		})
	}
	return out, nil
}

func (c *Client) rows(ctx context.Context) ([]otcRow, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/tpex_mainboard_daily_close_quotes", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Tickwind/1.0 (+https://tickwind.com)")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "identity") // avoid gzip-stream truncation
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tpex: get quotes: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tpex: get quotes: %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tpex: read quotes: %w", err)
	}
	var rows []otcRow
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("tpex: decode quotes: %w", err)
	}
	return rows, nil
}

// rocDate converts a Republic-of-China date ("1150605") to UTC midnight
// (year = 1911 + ROC year). Zero time on malformed input.
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

// parseNum parses a TPEx numeric string, tolerating commas, surrounding spaces
// and "--"/empty placeholders.
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
