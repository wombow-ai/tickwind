// Package nasdaq is a minimal client for Nasdaq's public IPO-calendar API,
// used to power the US "IPO calendar" feature (recently-priced, upcoming, and
// newly-filed offerings).
//
// NOTE on access: api.nasdaq.com blocks datacenter IPs (it returns an empty
// body, not an error, from a cloud host), so in production this client is given
// an http.Client routed through a RESIDENTIAL proxy (see config.ProxyHTTPClient)
// AND must send a full browser-like header set — without those headers the API
// also returns empty data. The header set is applied here; the proxied client is
// injected by the caller via New. Tests inject a plain client + a test base URL,
// so they exercise the parsing without touching the network.
//
// Data is a delayed, display-only convenience over Nasdaq's public calendar; the
// UI labels the source and adds a not-investment-advice disclaimer.
package nasdaq

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// calendarBaseURL is the Nasdaq IPO-calendar endpoint. The full request is
// baseURL + "?date=YYYY-MM". Overridable in tests.
const calendarBaseURL = "https://api.nasdaq.com/api/ipo/calendar"

// Kind classifies an IPO row by which section of the calendar it came from.
type Kind string

const (
	// KindPriced is a completed offering (shares priced / began trading).
	KindPriced Kind = "priced"
	// KindUpcoming is an expected/scheduled offering not yet priced.
	KindUpcoming Kind = "upcoming"
	// KindFiled is a newly-registered offering (S-1 filed), no date yet.
	KindFiled Kind = "filed"
)

// IPO is one normalized offering on the calendar. Numeric-looking fields are
// kept as the source's display strings (e.g. "$18.00", "10,000,000") because
// Nasdaq returns them pre-formatted and they may be ranges or "-" when unknown;
// the UI renders them as-is. Empty strings mean the source omitted the field.
type IPO struct {
	Ticker   string `json:"ticker"`
	Company  string `json:"company"`
	Exchange string `json:"exchange"`
	Price    string `json:"price"`  // proposed/priced share price, e.g. "$18.00"
	Shares   string `json:"shares"` // shares offered, e.g. "10,000,000"
	Amount   string `json:"amount"` // dollar value of shares offered
	Date     string `json:"date"`   // priced date or expected price date (source-formatted)
	Status   string `json:"status"` // deal status, when provided
	Kind     Kind   `json:"kind"`   // priced | upcoming | filed
}

// Calendar is one month's IPO calendar, split by section. Each slice is non-nil
// (possibly empty) so JSON marshals as [] not null.
type Calendar struct {
	Priced   []IPO `json:"priced"`
	Upcoming []IPO `json:"upcoming"`
	Filed    []IPO `json:"filed"`
}

// Client fetches and parses the Nasdaq IPO calendar. The http.Client is injected
// (a residential-proxy client in production, a test client in tests).
type Client struct {
	hc   *http.Client
	base string // overridable in tests
}

// New builds a client over the given http.Client (which should be a
// residential-proxy client in production — Nasdaq blocks datacenter IPs). A nil
// client falls back to a default with a sane timeout.
func New(hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	return &Client{hc: hc, base: calendarBaseURL}
}

// browserHeaders is the full header set Nasdaq's API requires — it returns an
// empty body when any of these are missing, even over a residential IP. Mirrors
// what a real Safari session sends.
func browserHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Origin", "https://www.nasdaq.com")
	req.Header.Set("Referer", "https://www.nasdaq.com/")
}

// calendarResp models the relevant slices of the Nasdaq IPO-calendar JSON. All
// fields are optional — Nasdaq omits whole sections on quiet months — so the
// decode is tolerant and missing sections simply yield empty slices.
type calendarResp struct {
	Data struct {
		Priced struct {
			Rows []rawRow `json:"rows"`
		} `json:"priced"`
		Upcoming struct {
			UpcomingTable struct {
				Rows []rawRow `json:"rows"`
			} `json:"upcomingTable"`
		} `json:"upcoming"`
		Filed struct {
			Rows []rawRow `json:"rows"`
		} `json:"filed"`
	} `json:"data"`
}

// rawRow is one IPO row as Nasdaq returns it. Field names differ slightly across
// sections (priced/upcoming/filed), so we capture the superset and pick per kind.
type rawRow struct {
	ProposedTickerSymbol       string `json:"proposedTickerSymbol"`
	Symbol                     string `json:"symbol"`
	CompanyName                string `json:"companyName"`
	ProposedExchange           string `json:"proposedExchange"`
	ProposedSharePrice         string `json:"proposedSharePrice"`
	SharesOffered              string `json:"sharesOffered"`
	PricedDate                 string `json:"pricedDate"`
	ExpectedPriceDate          string `json:"expectedPriceDate"`
	DollarValueOfSharesOffered string `json:"dollarValueOfSharesOffered"`
	DealStatus                 string `json:"dealStatus"`
}

// Calendar fetches and parses one month's IPO calendar. month must be a time in
// the target month; only its year+month are used (the API is keyed by YYYY-MM).
// It is fault-tolerant: a non-2xx status, an empty body (the datacenter-IP block
// symptom), or unparseable JSON returns an error, and any individual missing
// section just yields an empty slice rather than failing the whole call.
func (c *Client) Calendar(ctx context.Context, month time.Time) (Calendar, error) {
	url := c.base + "?date=" + month.Format("2006-01")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Calendar{}, fmt.Errorf("nasdaq: build request: %w", err)
	}
	browserHeaders(req)

	resp, err := c.hc.Do(req)
	if err != nil {
		return Calendar{}, fmt.Errorf("nasdaq: fetch ipo calendar: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return Calendar{}, fmt.Errorf("nasdaq: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Calendar{}, fmt.Errorf("nasdaq: ipo calendar status %d", resp.StatusCode)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		// Empty body = the datacenter-IP block (or a quiet endpoint); treat as an
		// error so the ingestor keeps the previous good snapshot.
		return Calendar{}, fmt.Errorf("nasdaq: empty ipo calendar body (proxy/headers?)")
	}

	var cr calendarResp
	if err := json.Unmarshal(body, &cr); err != nil {
		return Calendar{}, fmt.Errorf("nasdaq: decode ipo calendar: %w", err)
	}

	cal := Calendar{
		Priced:   make([]IPO, 0, len(cr.Data.Priced.Rows)),
		Upcoming: make([]IPO, 0, len(cr.Data.Upcoming.UpcomingTable.Rows)),
		Filed:    make([]IPO, 0, len(cr.Data.Filed.Rows)),
	}
	for _, r := range cr.Data.Priced.Rows {
		cal.Priced = append(cal.Priced, r.toIPO(KindPriced))
	}
	for _, r := range cr.Data.Upcoming.UpcomingTable.Rows {
		cal.Upcoming = append(cal.Upcoming, r.toIPO(KindUpcoming))
	}
	for _, r := range cr.Data.Filed.Rows {
		cal.Filed = append(cal.Filed, r.toIPO(KindFiled))
	}
	return cal, nil
}

// toIPO normalizes a raw row to an IPO, picking the right date column for its
// kind and falling back to the alternate ticker field when the proposed one is
// absent.
func (r rawRow) toIPO(kind Kind) IPO {
	ticker := strings.TrimSpace(r.ProposedTickerSymbol)
	if ticker == "" {
		ticker = strings.TrimSpace(r.Symbol)
	}
	date := strings.TrimSpace(r.PricedDate)
	if date == "" {
		date = strings.TrimSpace(r.ExpectedPriceDate)
	}
	return IPO{
		Ticker:   strings.ToUpper(ticker),
		Company:  strings.TrimSpace(r.CompanyName),
		Exchange: strings.TrimSpace(r.ProposedExchange),
		Price:    strings.TrimSpace(r.ProposedSharePrice),
		Shares:   strings.TrimSpace(r.SharesOffered),
		Amount:   strings.TrimSpace(r.DollarValueOfSharesOffered),
		Date:     date,
		Status:   strings.TrimSpace(r.DealStatus),
		Kind:     kind,
	}
}
