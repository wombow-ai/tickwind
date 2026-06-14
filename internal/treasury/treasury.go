// Package treasury is a keyless client + in-memory cache for the U.S. Treasury's
// published daily par yield curve — the official benchmark used for the widely
// watched 2s10s recession signal (the 10-Year yield minus the 2-Year yield; a
// negative spread = an "inverted" curve, a classic recession-watch indicator).
//
// The Treasury publishes the daily par yield curve rates with NO API key as a
// per-year CSV at
//
//	https://home.treasury.gov/resource-center/data-chart-center/interest-rates/daily-treasury-rates.csv/{YEAR}/all?type=daily_treasury_yield_curve&field_tdr_date_value={YEAR}&page&_format=csv
//
// whose header is a Date column plus one column per maturity tenor, e.g.
//
//	Date,"1 Mo","1.5 Month","2 Mo","3 Mo","4 Mo","6 Mo","1 Yr","2 Yr","3 Yr","5 Yr","7 Yr","10 Yr","20 Yr","30 Yr"
//	06/12/2026,3.69,3.70,3.70,3.78,3.79,3.82,3.86,4.09,4.12,4.21,4.34,4.48,4.98,4.97
//
// Rows are newest-first, so the first data row is the latest business day's
// curve. Rates are par yields in percent.
//
// IMPORTANT, anti-fabrication: the column set varies by year (e.g. "1.5 Month"
// only appears in later years, "4 Mo" was added mid-2022) and an individual cell
// can be blank when a tenor was not auctioned that day. The parser therefore maps
// tenors by HEADER NAME (never by fixed position) and only emits a tenor whose
// cell is present and parses as a number — a missing/blank tenor is simply absent
// from the result, never invented or zero-filled.
//
// This is public-domain U.S. government data, redistribution-safe. A bare Go
// User-Agent gets an empty reply from the Treasury host, so the client sends a
// browser-like User-Agent + Accept headers (mirroring internal/yahoo and the
// nasdaq/substack clients).
package treasury

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// baseURL is the Treasury daily-rates CSV endpoint prefix; the full URL appends
// the year and the daily_treasury_yield_curve query. Overridable in tests.
const baseURL = "https://home.treasury.gov/resource-center/data-chart-center/interest-rates/daily-treasury-rates.csv"

// ErrNoData is returned when the year's CSV has a header but no usable data row
// (e.g. the very first days of a new year before the first publication, or an
// empty body). Callers fall back to the prior year.
var ErrNoData = errors.New("treasury: no yield-curve data available")

// Yield is one maturity tenor's par yield for the curve's date. Tenor is the
// canonical short label ("2Y", "10Y", "3M", "30Y"); Rate is the par yield in
// percent (e.g. 4.48). A tenor only appears when its source cell was present and
// numeric — there is never a fabricated or zero-filled tenor.
type Yield struct {
	Tenor string  `json:"tenor"`
	Rate  float64 `json:"rate"`
}

// Curve is the latest daily par yield curve: the business day it is for, the
// per-tenor yields (canonical order), and the derived 2s10s spread.
type Curve struct {
	// Date is the curve's business day, formatted "2006-01-02".
	Date string `json:"date"`
	// Yields are the present tenors in canonical (short→long) order. Only tenors
	// the source actually published for Date appear — never invented.
	Yields []Yield `json:"yields"`
	// Spread2s10s is 10Y − 2Y in percentage points (e.g. 0.39), present only when
	// BOTH the 2Y and 10Y tenors are available. HasSpread guards it.
	Spread2s10s float64 `json:"spread_2s10s"`
	// HasSpread is true only when both 2Y and 10Y were present (so the spread is
	// real, not a 0 standing in for a missing leg).
	HasSpread bool `json:"-"`
	// Inverted is true when the spread is present and negative (10Y < 2Y), the
	// classic recession-watch signal. Meaningless unless HasSpread.
	Inverted bool `json:"inverted"`
}

// Rate returns the par yield for a canonical tenor label (e.g. "2Y") and whether
// it was present in the curve.
func (c Curve) Rate(tenor string) (float64, bool) {
	for _, y := range c.Yields {
		if y.Tenor == tenor {
			return y.Rate, true
		}
	}
	return 0, false
}

// headerTenor maps a Treasury CSV header label to a canonical short tenor label.
// The Treasury's labels are inconsistent ("1 Mo" vs "1.5 Month"), and the column
// set changes between years, so every accepted label is enumerated here and
// matched by name. Labels not in this map (only the leading "Date" today) are
// ignored. Keys are lower-cased + space-collapsed before lookup.
var headerTenor = map[string]string{
	"1 mo":      "1M",
	"1.5 month": "1.5M",
	"2 mo":      "2M",
	"3 mo":      "3M",
	"4 mo":      "4M",
	"6 mo":      "6M",
	"1 yr":      "1Y",
	"2 yr":      "2Y",
	"3 yr":      "3Y",
	"5 yr":      "5Y",
	"7 yr":      "7Y",
	"10 yr":     "10Y",
	"20 yr":     "20Y",
	"30 yr":     "30Y",
	// Tolerate plausible alternate spellings of the sub-year tenors so a future
	// "1.5 Mo"/"1 Month" header rename doesn't silently drop a column.
	"1 month": "1M",
	"1.5 mo":  "1.5M",
	"2 month": "2M",
	"3 month": "3M",
	"4 month": "4M",
	"6 month": "6M",
	"1 year":  "1Y",
	"2 year":  "2Y",
	"3 year":  "3Y",
	"5 year":  "5Y",
	"7 year":  "7Y",
	"10 year": "10Y",
	"20 year": "20Y",
	"30 year": "30Y",
}

// tenorOrder is the canonical short→long display order. A tenor not listed here
// sorts last (alphabetically), but every label headerTenor can produce is here.
var tenorOrder = map[string]int{
	"1M": 0, "1.5M": 1, "2M": 2, "3M": 3, "4M": 4, "6M": 5,
	"1Y": 6, "2Y": 7, "3Y": 8, "5Y": 9, "7Y": 10, "10Y": 11, "20Y": 12, "30Y": 13,
}

// Client fetches and parses the Treasury daily par yield curve. Keyless.
type Client struct {
	http *http.Client
	ua   string
	base string // CSV endpoint base (overridable in tests)
	now  func() time.Time
}

// New builds a Treasury client. A browser-like User-Agent is required — the
// Treasury host returns an empty reply to a bare Go User-Agent.
func New() *Client {
	return &Client{
		http: &http.Client{Timeout: 20 * time.Second},
		ua:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
		base: baseURL,
		now:  func() time.Time { return time.Now().UTC() },
	}
}

// Latest fetches the most recent daily par yield curve. It requests the current
// year's CSV and parses the newest row; if that year has no usable rows yet
// (e.g. the first days of January), it falls back to the prior year. The 2s10s
// spread is computed only when both the 2Y and 10Y tenors are present.
func (c *Client) Latest(ctx context.Context) (Curve, error) {
	year := c.now().Year()
	curve, err := c.fetchYear(ctx, year)
	if errors.Is(err, ErrNoData) {
		// Early in a new year (or a brief publication gap) the current year's file
		// can have only a header — fall back to the prior year's last business day.
		return c.fetchYear(ctx, year-1)
	}
	return curve, err
}

// fetchYear fetches and parses one calendar year's daily-rates CSV, returning the
// newest business day's curve. Returns ErrNoData when the file has no usable row.
func (c *Client) fetchYear(ctx context.Context, year int) (Curve, error) {
	url := fmt.Sprintf("%s/%d/all?type=daily_treasury_yield_curve&field_tdr_date_value=%d&page&_format=csv", c.base, year, year)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Curve{}, err
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", "text/csv,text/html,application/xhtml+xml,*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := c.http.Do(req)
	if err != nil {
		return Curve{}, fmt.Errorf("treasury fetch %d: %w", year, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Curve{}, fmt.Errorf("treasury fetch %d: status %d", year, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // par-yield CSVs are ~10 KB; cap defensively
	if err != nil {
		return Curve{}, fmt.Errorf("treasury read %d: %w", year, err)
	}
	return ParseLatest(body)
}

// ParseLatest parses a Treasury daily-rates CSV body and returns the newest
// business day's curve. Tenors are matched by header name and only emitted when
// the cell is present and numeric (a blank/non-numeric cell drops that tenor —
// never invented). The 2s10s spread is set only when both 2Y and 10Y are present.
// Returns ErrNoData when there is no usable data row.
func ParseLatest(body []byte) (Curve, error) {
	r := csv.NewReader(strings.NewReader(string(body)))
	r.FieldsPerRecord = -1 // tolerate ragged rows defensively
	r.TrimLeadingSpace = true

	header, err := r.Read()
	if err != nil {
		return Curve{}, fmt.Errorf("treasury parse header: %w", err)
	}
	// Build column-index → canonical tenor from the header by NAME. Position is
	// never assumed, so a year with a different column set still parses correctly.
	col2tenor := make(map[int]string, len(header))
	for i, h := range header {
		key := normalizeHeader(h)
		if t, ok := headerTenor[key]; ok {
			col2tenor[i] = t
		}
	}
	dateCol := dateColumn(header)
	if dateCol < 0 {
		return Curve{}, fmt.Errorf("treasury parse: no Date column in header %v", header)
	}

	// Rows are newest-first; take the first row that has a parseable date and at
	// least one numeric tenor.
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Curve{}, fmt.Errorf("treasury parse row: %w", err)
		}
		curve, ok := rowToCurve(rec, dateCol, col2tenor)
		if ok {
			return curve, nil
		}
	}
	return Curve{}, ErrNoData
}

// rowToCurve converts one CSV data row to a Curve, returning ok=false when the
// row has no parseable date or no numeric tenor at all. Each tenor cell is
// emitted only when present and numeric — blank/garbage cells are skipped, so a
// tenor the Treasury did not publish that day is simply absent.
func rowToCurve(rec []string, dateCol int, col2tenor map[int]string) (Curve, bool) {
	if dateCol >= len(rec) {
		return Curve{}, false
	}
	date, ok := parseDate(rec[dateCol])
	if !ok {
		return Curve{}, false
	}
	yields := make([]Yield, 0, len(col2tenor))
	for i, tenor := range col2tenor {
		if i >= len(rec) {
			continue
		}
		cell := strings.TrimSpace(rec[i])
		if cell == "" {
			continue // tenor not published this day → absent, never zero-filled
		}
		v, err := strconv.ParseFloat(cell, 64)
		if err != nil {
			continue // non-numeric → skip rather than fabricate
		}
		yields = append(yields, Yield{Tenor: tenor, Rate: v})
	}
	if len(yields) == 0 {
		return Curve{}, false
	}
	sort.SliceStable(yields, func(a, b int) bool {
		oa, ok := tenorOrder[yields[a].Tenor]
		if !ok {
			oa = len(tenorOrder)
		}
		ob, ok := tenorOrder[yields[b].Tenor]
		if !ok {
			ob = len(tenorOrder)
		}
		if oa != ob {
			return oa < ob
		}
		return yields[a].Tenor < yields[b].Tenor
	})

	curve := Curve{Date: date, Yields: yields}
	two, hasTwo := lookup(yields, "2Y")
	ten, hasTen := lookup(yields, "10Y")
	if hasTwo && hasTen {
		curve.Spread2s10s = round2(ten - two)
		curve.HasSpread = true
		curve.Inverted = curve.Spread2s10s < 0
	}
	return curve, true
}

// lookup returns the rate for a tenor in a yields slice and whether it is present.
func lookup(yields []Yield, tenor string) (float64, bool) {
	for _, y := range yields {
		if y.Tenor == tenor {
			return y.Rate, true
		}
	}
	return 0, false
}

// normalizeHeader lower-cases and collapses internal whitespace in a CSV header
// label so "1 Mo", " 1  Mo " and "1\tMo" all map identically.
func normalizeHeader(h string) string {
	return strings.Join(strings.Fields(strings.ToLower(h)), " ")
}

// dateColumn finds the index of the Date column (it is the first column today,
// but located by name for robustness). Returns -1 if absent.
func dateColumn(header []string) int {
	for i, h := range header {
		if normalizeHeader(h) == "date" {
			return i
		}
	}
	return -1
}

// parseDate parses the Treasury's MM/DD/YYYY date into a "2006-01-02" string.
// ok=false on an unrecognized format.
func parseDate(s string) (string, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"01/02/2006", "1/2/2006", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02"), true
		}
	}
	return "", false
}

// round2 rounds to two decimals (basis-point precision for percent yields).
func round2(v float64) float64 {
	return float64(int64(v*100+sign(v)*0.5)) / 100
}

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}
