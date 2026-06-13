// Package congress ingests U.S. Congress financial-disclosure filings from the
// official, public-domain House Clerk dataset (disclosures-clerk.house.gov) —
// no key, freely redistributable government data.
//
// The Clerk publishes one ZIP per calendar year containing an XML index of every
// financial-disclosure filing. FilingType "P" marks a Periodic Transaction
// Report (PTR) — the stock-trade disclosures we surface as the "Congress
// trading" board. The per-transaction detail (ticker, amount, buy/sell) lives in
// a per-filing PDF, which we link to rather than parse (PDF extraction is a
// later, heavier step); the index alone gives member · date · official PDF link.
package congress

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://disclosures-clerk.house.gov"
	userAgent      = "tickwind/0.1 (inverael@gmail.com)"
	maxZipBytes    = 64 << 20 // generous cap; a year's index ZIP is ~100KB
)

// Filing is one House financial-disclosure filing from the Clerk index. For PTRs
// (FilingType "P") PDFURL points at the official transaction-report PDF.
type Filing struct {
	Name       string    `json:"name"`        // "Richard W. Allen"
	Last       string    `json:"last"`        // "Allen"
	First      string    `json:"first"`       // "Richard W."
	State      string    `json:"state"`       // "GA" (from StateDst "GA12")
	District   string    `json:"district"`    // "12"
	FilingType string    `json:"filing_type"` // "P" = periodic transaction report
	Year       int       `json:"year"`        // 2025
	FiledDate  time.Time `json:"filed_date"`  // parsed from "1/16/2025"
	DocID      string    `json:"doc_id"`      // "20026537"
	PDFURL     string    `json:"pdf_url"`     // official filing PDF (PTRs only)
}

// xmlFD mirrors the Clerk FD index XML root (<FinancialDisclosure><Member>…).
type xmlFD struct {
	Members []xmlMember `xml:"Member"`
}

type xmlMember struct {
	Prefix     string `xml:"Prefix"`
	Last       string `xml:"Last"`
	First      string `xml:"First"`
	Suffix     string `xml:"Suffix"`
	FilingType string `xml:"FilingType"`
	StateDst   string `xml:"StateDst"`
	Year       string `xml:"Year"`
	FilingDate string `xml:"FilingDate"`
	DocID      string `xml:"DocID"`
}

// Client fetches House Clerk disclosure data.
type Client struct {
	http    *http.Client
	baseURL string
}

// New returns a Client pointed at the live House Clerk dataset.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 45 * time.Second}, baseURL: defaultBaseURL}
}

// maxPDFBytes caps a single PTR PDF download; digital PTRs are well under 1 MB,
// but the bound keeps a surprise large/hostile response from exhausting memory.
const maxPDFBytes = 16 << 20

// FetchPDF downloads one filing's PDF (e.g. a Filing.PDFURL) and returns its raw
// bytes, for the ptr package to parse. It reuses the Client's HTTP client and the
// same Clerk/EDGAR User-Agent as the index fetch.
func (c *Client) FetchPDF(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch ptr pdf: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ptr pdf %s: status %d", url, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxPDFBytes))
	if err != nil {
		return nil, fmt.Errorf("read ptr pdf %s: %w", url, err)
	}
	return raw, nil
}

// FetchHousePTRs downloads the given year's financial-disclosure ZIP, parses its
// XML index, and returns the Periodic Transaction Reports (stock-trade filings)
// with their official PDF links, newest filing date first.
func (c *Client) FetchHousePTRs(ctx context.Context, year int) ([]Filing, error) {
	url := fmt.Sprintf("%s/public_disc/financial-pdfs/%dFD.ZIP", c.baseURL, year)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch house FD %d: %w", year, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("house FD %d: status %d", year, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxZipBytes))
	if err != nil {
		return nil, fmt.Errorf("read house FD %d: %w", year, err)
	}
	filings, err := parseHouseFDZip(raw, year)
	if err != nil {
		return nil, err
	}
	ptrs := filterPTRs(filings, c.baseURL)
	sort.SliceStable(ptrs, func(i, j int) bool { return ptrs[i].FiledDate.After(ptrs[j].FiledDate) })
	return ptrs, nil
}

// parseHouseFDZip extracts the "{year}FD.xml" entry from the ZIP and parses it.
func parseHouseFDZip(zipBytes []byte, year int) ([]Filing, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, fmt.Errorf("open house FD zip: %w", err)
	}
	want := fmt.Sprintf("%dFD.xml", year)
	for _, f := range zr.File {
		if !strings.EqualFold(f.Name, want) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", want, err)
		}
		data, err := io.ReadAll(io.LimitReader(rc, maxZipBytes))
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", want, err)
		}
		return parseHouseFD(data)
	}
	return nil, fmt.Errorf("%s not found in zip", want)
}

// parseHouseFD parses the Clerk FD index XML into Filings (pure; the unit-test
// entry point). Rows missing a last name or DocID are dropped.
func parseHouseFD(data []byte) ([]Filing, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")) // strip UTF-8 BOM if present
	var doc xmlFD
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse house FD xml: %w", err)
	}
	out := make([]Filing, 0, len(doc.Members))
	for _, m := range doc.Members {
		last := strings.TrimSpace(m.Last)
		docID := strings.TrimSpace(m.DocID)
		if last == "" || docID == "" {
			continue
		}
		state, district := splitStateDst(m.StateDst)
		f := Filing{
			Name:       fullName(m.First, last),
			Last:       last,
			First:      strings.TrimSpace(m.First),
			State:      state,
			District:   district,
			FilingType: strings.TrimSpace(m.FilingType),
			DocID:      docID,
			FiledDate:  parseFilingDate(m.FilingDate),
		}
		if y, err := strconv.Atoi(strings.TrimSpace(m.Year)); err == nil {
			f.Year = y
		}
		out = append(out, f)
	}
	return out, nil
}

// filterPTRs keeps Periodic Transaction Reports (FilingType "P") and attaches the
// official PDF link, derived from the filing year and DocID.
func filterPTRs(all []Filing, baseURL string) []Filing {
	out := make([]Filing, 0)
	for _, f := range all {
		if !strings.EqualFold(f.FilingType, "P") {
			continue
		}
		yr := f.Year
		if yr == 0 {
			yr = f.FiledDate.Year()
		}
		f.PDFURL = fmt.Sprintf("%s/public_disc/ptr-pdfs/%d/%s.pdf", baseURL, yr, f.DocID)
		out = append(out, f)
	}
	return out
}

// splitStateDst splits a "GA12" code into ("GA", "12"). At-large districts are
// "00"; we keep the raw two-letter state and the remaining digits verbatim.
func splitStateDst(s string) (state, district string) {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		state = s[:2]
	}
	if len(s) > 2 {
		district = s[2:]
	}
	return state, district
}

// fullName joins first + last with a neutral fallback when first is blank.
func fullName(first, last string) string {
	first = strings.TrimSpace(first)
	last = strings.TrimSpace(last)
	if first == "" {
		return last
	}
	return first + " " + last
}

// parseFilingDate parses the Clerk's "M/D/YYYY" (un-padded) filing date; an
// unparseable/blank date yields the zero time.
func parseFilingDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{"1/2/2006", "01/02/2006", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
