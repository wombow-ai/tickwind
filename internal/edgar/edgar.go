// Package edgar is a minimal client for the free SEC EDGAR APIs.
// No API key required — only a descriptive User-Agent and <=10 req/s.
// Docs: https://www.sec.gov/search-filings/edgar-application-programming-interfaces
package edgar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

const (
	tickersURL     = "https://www.sec.gov/files/company_tickers.json"
	submissionsURL = "https://data.sec.gov/submissions/CIK%s.json"
)

type Client struct {
	http      *http.Client
	userAgent string

	mu        sync.RWMutex
	tickerMap map[string]tickerInfo // UPPER(ticker) -> info
}

type tickerInfo struct {
	CIK   string // zero-padded to 10 digits
	Title string
}

func New(userAgent string) *Client {
	return &Client{
		http:      &http.Client{Timeout: 20 * time.Second},
		userAgent: userAgent,
	}
}

func (c *Client) get(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	// SEC requires a descriptive User-Agent identifying the app + contact.
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("edgar: GET %s -> %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// loadTickers fetches and caches the full ticker→CIK directory.
func (c *Client) loadTickers(ctx context.Context) error {
	var raw map[string]struct {
		CIK    int    `json:"cik_str"`
		Ticker string `json:"ticker"`
		Title  string `json:"title"`
	}
	if err := c.get(ctx, tickersURL, &raw); err != nil {
		return err
	}
	m := make(map[string]tickerInfo, len(raw))
	for _, v := range raw {
		m[strings.ToUpper(v.Ticker)] = tickerInfo{
			CIK:   fmt.Sprintf("%010d", v.CIK),
			Title: v.Title,
		}
	}
	c.mu.Lock()
	c.tickerMap = m
	c.mu.Unlock()
	return nil
}

func (c *Client) lookup(ctx context.Context, ticker string) (tickerInfo, error) {
	c.mu.RLock()
	m := c.tickerMap
	c.mu.RUnlock()
	if m == nil {
		if err := c.loadTickers(ctx); err != nil {
			return tickerInfo{}, err
		}
		c.mu.RLock()
		m = c.tickerMap
		c.mu.RUnlock()
	}
	info, ok := m[strings.ToUpper(ticker)]
	if !ok {
		return tickerInfo{}, fmt.Errorf("edgar: ticker %q not found (US-listed only)", ticker)
	}
	return info, nil
}

type submissionsResp struct {
	Name    string `json:"name"`
	Filings struct {
		Recent struct {
			AccessionNumber       []string `json:"accessionNumber"`
			FilingDate            []string `json:"filingDate"`
			Form                  []string `json:"form"`
			PrimaryDocument       []string `json:"primaryDocument"`
			PrimaryDocDescription []string `json:"primaryDocDescription"`
		} `json:"recent"`
	} `json:"filings"`
}

// RecentFilings returns the security and its most recent filings (newest first).
func (c *Client) RecentFilings(ctx context.Context, ticker string, limit int) (store.Security, []store.Filing, error) {
	info, err := c.lookup(ctx, ticker)
	if err != nil {
		return store.Security{}, nil, err
	}

	var sub submissionsResp
	if err := c.get(ctx, fmt.Sprintf(submissionsURL, info.CIK), &sub); err != nil {
		return store.Security{}, nil, err
	}

	sec := store.Security{
		Ticker: strings.ToUpper(ticker),
		CIK:    info.CIK,
		Name:   sub.Name,
		Market: "US",
	}

	r := sub.Filings.Recent
	cikTrimmed := strings.TrimLeft(info.CIK, "0")
	out := make([]store.Filing, 0, limit)
	for i := 0; i < len(r.AccessionNumber) && len(out) < limit; i++ {
		filedAt, _ := time.Parse("2006-01-02", at(r.FilingDate, i))
		form := at(r.Form, i)
		// Use a human-readable name for the form type, unless the document has
		// its own *meaningful* description (the feed often just repeats the form,
		// e.g. "FORM 4" or "10-Q", which is no better than our mapping).
		title := formTitle(form)
		if d := at(r.PrimaryDocDescription, i); usefulDesc(d, form) {
			title = d
		}
		accNoDashes := strings.ReplaceAll(r.AccessionNumber[i], "-", "")
		out = append(out, store.Filing{
			Ticker:      sec.Ticker,
			Form:        form,
			Title:       title,
			FiledAt:     filedAt,
			AccessionNo: r.AccessionNumber[i],
			URL: fmt.Sprintf("https://www.sec.gov/Archives/edgar/data/%s/%s/%s",
				cikTrimmed, accNoDashes, at(r.PrimaryDocument, i)),
		})
	}
	return sec, out, nil
}

func at(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return ""
}

// formDescriptions maps common SEC form types to a human-readable name.
var formDescriptions = map[string]string{
	"10-K":    "Annual report (10-K)",
	"10-Q":    "Quarterly report (10-Q)",
	"8-K":     "Current report (8-K)",
	"4":       "Insider transaction (Form 4)",
	"3":       "Initial insider holdings (Form 3)",
	"5":       "Annual insider holdings (Form 5)",
	"144":     "Notice of proposed sale (Form 144)",
	"SD":      "Specialized disclosure (Form SD)",
	"6-K":     "Foreign issuer report (6-K)",
	"20-F":    "Foreign annual report (20-F)",
	"S-1":     "Registration statement (S-1)",
	"S-3":     "Shelf registration (S-3)",
	"S-8":     "Employee plan registration (S-8)",
	"11-K":    "Employee benefit plan report (11-K)",
	"FWP":     "Free writing prospectus",
	"25-NSE":  "Notification of delisting",
	"CERT":    "Exchange certification",
	"EFFECT":  "Notice of effectiveness",
	"DEF 14A": "Proxy statement (DEF 14A)",
}

// usefulDesc reports whether a primary-document description adds anything over
// the form type (i.e. it isn't empty, the bare form, or "FORM <x>").
func usefulDesc(desc, form string) bool {
	u := strings.ToUpper(strings.TrimSpace(desc))
	f := strings.ToUpper(strings.TrimSpace(form))
	return u != "" && u != f && u != "FORM "+f && u != "FORM"+f
}

// formTitle returns a friendly name for an SEC form type, falling back to the
// raw form. Amendment suffixes ("/A") are noted.
func formTitle(form string) string {
	f := strings.TrimSpace(form)
	if f == "" {
		return "Filing"
	}
	amended := strings.HasSuffix(f, "/A")
	base := strings.TrimSuffix(f, "/A")
	desc, ok := formDescriptions[base]
	if !ok {
		switch {
		case strings.HasPrefix(base, "SC 13D"):
			desc = "Beneficial ownership (13D)"
		case strings.HasPrefix(base, "SC 13G"):
			desc = "Beneficial ownership (13G)"
		case strings.HasPrefix(base, "424B"):
			desc = "Prospectus"
		case strings.HasPrefix(base, "13F"):
			desc = "Institutional holdings (13F)"
		case strings.HasPrefix(base, "DEF 14"), strings.HasPrefix(base, "DEFA"):
			desc = "Proxy statement"
		default:
			desc = base + " filing"
		}
	}
	if amended {
		desc += " (amended)"
	}
	return desc
}
