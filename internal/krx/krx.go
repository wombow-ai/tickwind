// Package krx is a client for the Korea Exchange (KRX) Open API — the official
// source of end-of-day prices and the listed-issue directory for KOSPI (.KS) and
// KOSDAQ (.KQ) stocks. It needs a free AUTH_KEY (sent as a header) and self-
// disables when the key is empty. Data is EOD; one call per board prices the
// whole market.
package krx

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

const defaultBase = "https://data-dbg.krx.co.kr/svc/apis/sto"

// board maps a KRX daily-trade endpoint to the ticker suffix it produces.
type board struct {
	path, suffix, exch string
}

var boards = []board{
	{"stk_bydd_trd", ".KS", "KOSPI"},
	{"ksq_bydd_trd", ".KQ", "KOSDAQ"},
}

// Client fetches KRX Open API data. A blank key disables it.
type Client struct {
	http *http.Client
	base string
	key  string
	loc  *time.Location
}

// New returns a Client. key is the KRX AUTH_KEY (empty → disabled via Enabled).
func New(key string) *Client {
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		loc = time.UTC
	}
	return &Client{http: &http.Client{Timeout: 30 * time.Second}, base: defaultBase, key: key, loc: loc}
}

// Enabled reports whether a key is configured.
func (c *Client) Enabled() bool { return strings.TrimSpace(c.key) != "" }

// resp is the KRX daily-trade response wrapper.
type resp struct {
	OutBlock []row `json:"OutBlock_1"`
}

type row struct {
	ISUCd string `json:"ISU_CD"`        // 6-digit short code, e.g. "005930"
	ISUNm string `json:"ISU_NM"`        // Korean name
	Close string `json:"TDD_CLSPRC"`    // close
	Chg   string `json:"CMPPREVDD_PRC"` // change vs previous close
}

// EODQuotes fetches the latest available KOSPI + KOSDAQ daily close and returns
// a quote per .KS/.KQ ticker. Returns nil (no error) when disabled.
func (c *Client) EODQuotes(ctx context.Context) (map[string]store.Quote, error) {
	if !c.Enabled() {
		return nil, nil
	}
	out := make(map[string]store.Quote)
	for _, b := range boards {
		rows, day := c.recent(ctx, b.path)
		for _, r := range rows {
			close, ok := parseNum(r.Close)
			if !ok || close <= 0 {
				continue
			}
			chg, _ := parseNum(r.Chg)
			tk := strings.TrimSpace(r.ISUCd) + b.suffix
			out[tk] = store.Quote{
				Ticker:    tk,
				Price:     close,
				PrevClose: close - chg,
				Session:   "closed",
				Source:    "krx",
				At:        day,
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("krx: no usable rows (check key / market day)")
	}
	return out, nil
}

// Companies returns the KOSPI + KOSDAQ symbol directory (ticker + Korean name),
// derived from the same daily feed. Returns nil when disabled.
func (c *Client) Companies(ctx context.Context) ([]symbols.Symbol, error) {
	if !c.Enabled() {
		return nil, nil
	}
	var out []symbols.Symbol
	for _, b := range boards {
		rows, _ := c.recent(ctx, b.path)
		for _, r := range rows {
			code := strings.TrimSpace(r.ISUCd)
			name := strings.TrimSpace(r.ISUNm)
			if code == "" || name == "" {
				continue
			}
			out = append(out, symbols.Symbol{
				Ticker:   code + b.suffix,
				Name:     name,
				Exchange: b.exch,
				Country:  "KR",
			})
		}
	}
	return out, nil
}

// recent tries the last several KST trading days (newest first) and returns the
// first non-empty result + its date — so weekends/holidays/pre-close fall back
// to the latest available day.
func (c *Client) recent(ctx context.Context, path string) ([]row, time.Time) {
	now := time.Now().In(c.loc)
	for d := 0; d <= 6; d++ {
		day := now.AddDate(0, 0, -d)
		rows, err := c.fetch(ctx, path, day.Format("20060102"))
		if err == nil && len(rows) > 0 {
			return rows, time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
		}
	}
	return nil, time.Time{}
}

func (c *Client) fetch(ctx context.Context, path, basDd string) ([]row, error) {
	url := fmt.Sprintf("%s/%s?basDd=%s", c.base, path, basDd)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("AUTH_KEY", c.key)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("krx: get %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("krx: get %s: %s", path, resp.Status)
	}
	var body krxBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("krx: decode %s: %w", path, err)
	}
	return body.OutBlock, nil
}

// krxBody aliases resp for decoding (kept separate so resp can document fields).
type krxBody = resp

// parseNum parses a KRX numeric string (commas / spaces / empty tolerated).
func parseNum(s string) (float64, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if s == "" || s == "-" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// Compile-time check that Client satisfies what the ingest KR adapter needs.
var _ interface {
	EODQuotes(context.Context) (map[string]store.Quote, error)
	Companies(context.Context) ([]symbols.Symbol, error)
} = (*Client)(nil)
