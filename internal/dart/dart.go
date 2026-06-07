// Package dart is a client for Korea's OpenDART (Financial Supervisory Service)
// disclosure API — the official filings/announcements source for KR-listed
// companies. It needs a free crtfc_key and self-disables when the key is empty.
// Tickers map to DART's internal corp_code via the (zipped) corpCode directory.
package dart

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

const defaultBase = "https://opendart.fss.or.kr/api"

// Client fetches OpenDART data. A blank key disables it.
type Client struct {
	http *http.Client
	base string
	key  string
}

// New returns a Client. key is the OpenDART crtfc_key (empty → disabled).
func New(key string) *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}, base: defaultBase, key: key}
}

// Enabled reports whether a key is configured.
func (c *Client) Enabled() bool { return strings.TrimSpace(c.key) != "" }

// CorpCodeMap downloads the DART corp-code directory (a ZIP wrapping CORPCODE.xml)
// and returns a 6-digit stock_code → 8-digit corp_code map (unlisted entries,
// which have a blank stock_code, are skipped). Returns nil when disabled.
func (c *Client) CorpCodeMap(ctx context.Context) (map[string]string, error) {
	if !c.Enabled() {
		return nil, nil
	}
	u := fmt.Sprintf("%s/corpCode.xml?crtfc_key=%s", c.base, url.QueryEscape(c.key))
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		// On a key/IP error DART returns a small XML/JSON error instead of a zip.
		return nil, fmt.Errorf("dart: corpCode not a zip (key/quota?): %w", err)
	}
	var xmlBytes []byte
	for _, f := range zr.File {
		if strings.HasSuffix(strings.ToUpper(f.Name), ".XML") {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("dart: open corpCode entry: %w", err)
			}
			xmlBytes, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("dart: read corpCode entry: %w", err)
			}
			break
		}
	}
	var doc struct {
		List []struct {
			CorpCode  string `xml:"corp_code"`
			StockCode string `xml:"stock_code"`
		} `xml:"list"`
	}
	if err := xml.Unmarshal(xmlBytes, &doc); err != nil {
		return nil, fmt.Errorf("dart: parse corpCode xml: %w", err)
	}
	out := make(map[string]string, len(doc.List))
	for _, l := range doc.List {
		sc := strings.TrimSpace(l.StockCode)
		if sc == "" {
			continue // unlisted entity
		}
		out[sc] = strings.TrimSpace(l.CorpCode)
	}
	return out, nil
}

// RecentFilings fetches recent disclosures for corpCode, tagged to ticker (the
// suffixed symbol, e.g. "005930.KS"). status "013" (no data) returns an empty
// list, not an error. Returns nil when disabled or corpCode is blank.
func (c *Client) RecentFilings(ctx context.Context, ticker, corpCode string, limit int) ([]store.Filing, error) {
	if !c.Enabled() || corpCode == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	q := url.Values{}
	q.Set("crtfc_key", c.key)
	q.Set("corp_code", corpCode)
	q.Set("page_count", strconv.Itoa(limit))
	q.Set("sort_mth", "desc")
	body, err := c.get(ctx, c.base+"/list.json?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var doc struct {
		Status string `json:"status"`
		List   []struct {
			ReportNm string `json:"report_nm"`
			RceptNo  string `json:"rcept_no"`
			RceptDt  string `json:"rcept_dt"`
		} `json:"list"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("dart: decode list: %w", err)
	}
	switch doc.Status {
	case "000":
		// ok
	case "013":
		return nil, nil // no disclosures
	default:
		return nil, fmt.Errorf("dart: list status %s", doc.Status)
	}
	out := make([]store.Filing, 0, len(doc.List))
	for _, f := range doc.List {
		rcpt := strings.TrimSpace(f.RceptNo)
		out = append(out, store.Filing{
			Ticker:      ticker,
			Form:        "DART",
			Title:       strings.TrimSpace(f.ReportNm),
			FiledAt:     parseDt(f.RceptDt),
			AccessionNo: rcpt,
			URL:         "https://dart.fss.or.kr/dsaf001/main.do?rcpNo=" + rcpt,
		})
	}
	return out, nil
}

func (c *Client) get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Tickwind/1.0 (+https://tickwind.com)")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dart: get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dart: get: %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func parseDt(s string) time.Time {
	t, err := time.Parse("20060102", strings.TrimSpace(s))
	if err != nil {
		return time.Time{}
	}
	return t
}
