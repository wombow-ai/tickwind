// Package finrashvol is a keyless client + in-memory cache for FINRA's public
// daily short-volume files (the "RegSHO daily" consolidated NMS feed). Each
// trading day FINRA publishes one whole-universe pipe-delimited file at
//
//	https://cdn.finra.org/equity/regsho/daily/CNMSshvol{YYYYMMDD}.txt
//
// with a header line and rows of
//
//	Date|Symbol|ShortVolume|ShortExemptVolume|TotalVolume|Market
//
// and a trailing "Total" summary row (skipped). The signal we derive is the
// percentage of the day's reported volume that was short = ShortVolume /
// TotalVolume * 100 — a free, ticker-keyed "short pressure" gauge that
// complements the slower twice-monthly consolidated short interest in
// internal/finra.
//
// The file is public domain (a regulatory disclosure). FINRA's terms are
// display-only: site display + derived rankings are fine, but the raw data must
// NOT be redistributed in bulk, so callers should not expose a raw-row API.
//
// Files are published only on trading days and lag the close, so callers should
// try the target date and fall back to prior business days until they get data;
// a missing/unpublished day yields ErrNoData (a 404, an empty body, or a file
// with no usable rows).
package finrashvol

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// baseURL is the CDN prefix for the daily consolidated-NMS short-volume files.
// The full URL is baseURL + "CNMSshvol{YYYYMMDD}.txt". Overridable in tests.
const baseURL = "https://cdn.finra.org/equity/regsho/daily/"

// ErrNoData is returned by FetchDaily when no short-volume file is available for
// the requested date: a 404, an empty body, or a file with no usable data rows
// (e.g. a weekend, holiday, or a day not yet published). Callers detect it with
// errors.Is and fall back to the previous trading day.
var ErrNoData = errors.New("finrashvol: no short-volume data for date")

// ShortVol is one symbol's reported short volume for one trading day. ShortPct
// is the share of the day's total reported volume that was short, in percent
// (0 when TotalVolume is 0). Date is the FINRA report date, formatted as
// YYYY-MM-DD.
type ShortVol struct {
	Symbol      string  `json:"symbol"`
	ShortVolume int64   `json:"short_volume"`
	TotalVolume int64   `json:"total_volume"`
	ShortPct    float64 `json:"short_pct"`
	Date        string  `json:"date"`
}

// Client fetches and parses FINRA's daily short-volume files anonymously.
type Client struct {
	hc   *http.Client
	base string // overridable in tests
}

// New returns a ready Client with a sensible HTTP timeout. The daily file is a
// few MB, so the timeout is generous.
func New() *Client {
	return &Client{hc: &http.Client{Timeout: 30 * time.Second}, base: baseURL}
}

// FileName returns the FINRA file name for a date, e.g. "CNMSshvol20260605.txt".
func FileName(date time.Time) string {
	return "CNMSshvol" + date.Format("20060102") + ".txt"
}

// FetchDaily downloads and parses the whole short-volume file for date. It
// returns ErrNoData (via errors.Is) when the file is missing, empty, or has no
// usable rows so the caller can fall back to the previous trading day. The
// trailing "Total" summary row and any malformed rows are skipped.
func (c *Client) FetchDaily(ctx context.Context, date time.Time) ([]ShortVol, error) {
	url := c.base + FileName(date)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/plain")
	req.Header.Set("User-Agent", "Tickwind/0.1 (contact@tickwind.com)")
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		// proceed
	case http.StatusNotFound, http.StatusForbidden:
		// Unpublished day (weekend/holiday/not-yet-posted). 403 is observed
		// for some missing paths on the CDN too — treat both as "no data".
		return nil, fmt.Errorf("%w: %s: %s", ErrNoData, FileName(date), resp.Status)
	default:
		return nil, fmt.Errorf("finrashvol: %s: %s", FileName(date), resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	rows, err := parseFile(raw)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: %s: empty", ErrNoData, FileName(date))
	}
	return rows, nil
}

// parseFile parses the pipe-delimited short-volume file body. It tolerates the
// header line, the trailing "Total" summary row, blank lines, and malformed
// rows (all skipped). Column order is taken from the header when present, so
// the parser is resilient to FINRA reordering columns; absent a header it falls
// back to the documented order.
func parseFile(raw []byte) ([]ShortVol, error) {
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// Default column indexes per the documented layout:
	// Date|Symbol|ShortVolume|ShortExemptVolume|TotalVolume|Market
	idxDate, idxSym, idxShort, idxTotal := 0, 1, 2, 4
	haveHeader := false

	out := make([]ShortVol, 0, 8192)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fields := strings.Split(line, "|")
		// Header row: a "Symbol" column name signals the layout. Map columns by
		// name so reordering doesn't break parsing.
		if !haveHeader && hasHeader(fields) {
			haveHeader = true
			for i, f := range fields {
				switch strings.ToLower(strings.TrimSpace(f)) {
				case "date":
					idxDate = i
				case "symbol":
					idxSym = i
				case "shortvolume":
					idxShort = i
				case "totalvolume":
					idxTotal = i
				}
			}
			continue
		}
		// The file ends with a "Total" summary row (its second field is the
		// running short total, not a symbol). Skip any row whose symbol cell is
		// the literal "Total".
		if maxIdx(idxDate, idxSym, idxShort, idxTotal) >= len(fields) {
			continue
		}
		sym := strings.TrimSpace(fields[idxSym])
		if sym == "" || strings.EqualFold(sym, "Total") {
			continue
		}
		short, ok1 := parseVol(fields[idxShort])
		total, ok2 := parseVol(fields[idxTotal])
		if !ok1 || !ok2 {
			continue // malformed numeric cell — skip the row
		}
		out = append(out, ShortVol{
			Symbol:      sym,
			ShortVolume: short,
			TotalVolume: total,
			ShortPct:    shortPct(short, total),
			Date:        formatDate(strings.TrimSpace(fields[idxDate])),
		})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("finrashvol: read file: %w", err)
	}
	return out, nil
}

// hasHeader reports whether fields look like the column-name header row (any
// cell equals "Symbol", case-insensitively).
func hasHeader(fields []string) bool {
	for _, f := range fields {
		if strings.EqualFold(strings.TrimSpace(f), "Symbol") {
			return true
		}
	}
	return false
}

// parseVol parses a volume cell as a whole-share count. FINRA's consolidated
// file carries fractional volumes (e.g. "380098.039916", from odd-lot/fractional
// aggregation), so parse as float and round — the prior int-only parse silently
// dropped almost every row.
func parseVol(s string) (int64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || f < 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return int64(math.Round(f)), true
}

// shortPct is ShortVolume/TotalVolume*100, or 0 when TotalVolume is 0.
func shortPct(short, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(short) / float64(total) * 100
}

// formatDate converts FINRA's YYYYMMDD report-date cell to YYYY-MM-DD. If the
// cell isn't 8 digits it is returned unchanged.
func formatDate(s string) string {
	if len(s) != 8 {
		return s
	}
	for i := 0; i < 8; i++ {
		if s[i] < '0' || s[i] > '9' {
			return s
		}
	}
	return s[0:4] + "-" + s[4:6] + "-" + s[6:8]
}

// maxIdx returns the largest of its arguments.
func maxIdx(xs ...int) int {
	m := xs[0]
	for _, x := range xs[1:] {
		if x > m {
			m = x
		}
	}
	return m
}
