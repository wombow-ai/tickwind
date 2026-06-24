package edgar

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"
)

// ErrNoNPORT is returned by ETFHoldings when a ticker resolves to a CIK but has no recent N-PORT-P
// portfolio filing — i.e. it is not a fund/ETF that discloses holdings (or none is on file yet).
// Callers map it to a 404 (nothing to show), distinct from a transient fetch error (502).
var ErrNoNPORT = errors.New("edgar: no N-PORT holdings filing")

// ETFHolding is one position from a fund/ETF's most recent SEC Form N-PORT-P portfolio report.
// Every field is parsed VERBATIM from the filing (anti-hallucination-safe — nothing derived or
// guessed). Ticker is set ONLY when the filing carries a ticker identifier (N-PORT positions often
// carry just a CUSIP/ISIN); it is never inferred from the name.
type ETFHolding struct {
	Name     string  `json:"name"`                // issuer / security name as filed
	Ticker   string  `json:"ticker,omitempty"`    // only when the filing provides one
	CUSIP    string  `json:"cusip,omitempty"`     // as filed
	PctVal   float64 `json:"pct_val"`             // percent of the fund's net assets
	ValUSD   float64 `json:"val_usd"`             // market value in USD as filed
	AssetCat string  `json:"asset_cat,omitempty"` // N-PORT asset category code (EC=equity common, DBT=debt, …)
	Country  string  `json:"country,omitempty"`   // investment country code
}

// ETFHoldings returns a fund/ETF's largest positions from its most recent Form N-PORT-P, the
// filing date, or an error. It resolves the ticker→CIK via the shared SEC ticker directory (which
// includes ETFs), finds the latest NPORT-P submission, fetches its RAW primary_doc.xml, and parses
// the <invstOrSec> positions — sorted by percent-of-net-assets descending, capped at max. Returns
// ErrNoNPORT for a ticker with no N-PORT filing (an operating company, or a fund yet to file).
// max <= 0 defaults to 25.
func (c *Client) ETFHoldings(ctx context.Context, ticker string, max int) ([]ETFHolding, time.Time, error) {
	if max <= 0 {
		max = 25
	}
	info, err := c.lookup(ctx, ticker)
	if err != nil {
		return nil, time.Time{}, err
	}
	var sub submissionsResp
	if err := c.get(ctx, fmt.Sprintf(submissionsURL, info.CIK), &sub); err != nil {
		return nil, time.Time{}, err
	}
	r := sub.Filings.Recent
	idx := -1
	for i := 0; i < len(r.Form); i++ {
		if strings.HasPrefix(r.Form[i], "NPORT-P") { // NPORT-P + NPORT-P/A amendments; recent[] is newest-first
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, time.Time{}, ErrNoNPORT
	}
	asOf, _ := time.Parse("2006-01-02", at(r.FilingDate, idx))
	accNoDashes := strings.ReplaceAll(r.AccessionNumber[idx], "-", "")
	cikTrimmed := strings.TrimLeft(info.CIK, "0")
	// submissions' primaryDocument points at the XSL-styled HTML view
	// (xslFormNPORT-P_X01/primary_doc.xml); the RAW XML is that file's basename at the accession root.
	doc := path.Base(at(r.PrimaryDocument, idx))
	if doc == "" || doc == "." {
		doc = "primary_doc.xml"
	}
	url := fmt.Sprintf("https://www.sec.gov/Archives/edgar/data/%s/%s/%s", cikTrimmed, accNoDashes, doc)
	body, err := c.getText(ctx, url)
	if err != nil {
		return nil, time.Time{}, err
	}
	holdings, err := parseNPORTHoldings(body, max)
	if err != nil {
		return nil, time.Time{}, err
	}
	return holdings, asOf, nil
}

// nportInvst mirrors the fields ETFHoldings surfaces from one <invstOrSec> position. Decoded by
// LOCAL element name, so it is robust to the filing's XML namespace.
type nportInvst struct {
	Name        string  `xml:"name"`
	Title       string  `xml:"title"`
	CUSIP       string  `xml:"cusip"`
	ValUSD      float64 `xml:"valUSD"`
	PctVal      float64 `xml:"pctVal"`
	AssetCat    string  `xml:"assetCat"`
	Country     string  `xml:"invCountry"`
	Identifiers struct {
		Ticker struct {
			Value string `xml:"value,attr"`
		} `xml:"ticker"`
	} `xml:"identifiers"`
}

// parseNPORTHoldings extracts the <invstOrSec> positions from a raw N-PORT-P primary_doc.xml, sorts
// them by percent-of-net-assets descending (ties by value, then name — deterministic), and returns
// the top max. A malformed position is skipped (never fabricated); a position with no name or no
// weight is dropped. The token-stream decode matches invstOrSec by LOCAL name (namespace-robust).
func parseNPORTHoldings(body string, max int) ([]ETFHolding, error) {
	dec := xml.NewDecoder(strings.NewReader(body))
	out := make([]ETFHolding, 0, 128)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "invstOrSec" {
			continue
		}
		var v nportInvst
		if err := dec.DecodeElement(&v, &se); err != nil {
			continue // skip a malformed position rather than fail the whole report
		}
		name := strings.TrimSpace(v.Name)
		if name == "" {
			name = strings.TrimSpace(v.Title)
		}
		if name == "" || v.PctVal <= 0 {
			continue // no identity or no weight → not presentable
		}
		out = append(out, ETFHolding{
			Name:     name,
			Ticker:   strings.ToUpper(strings.TrimSpace(v.Identifiers.Ticker.Value)),
			CUSIP:    strings.TrimSpace(v.CUSIP),
			PctVal:   v.PctVal,
			ValUSD:   v.ValUSD,
			AssetCat: strings.TrimSpace(v.AssetCat),
			Country:  strings.TrimSpace(v.Country),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].PctVal != out[j].PctVal {
			return out[i].PctVal > out[j].PctVal
		}
		if out[i].ValUSD != out[j].ValUSD {
			return out[i].ValUSD > out[j].ValUSD
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > max {
		out = out[:max]
	}
	return out, nil
}
